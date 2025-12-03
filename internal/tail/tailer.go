package tail

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jmurray2011/wail/internal/filesystem"
)

// Tailer reads the last N lines of a file and optionally follows for new content.
type Tailer interface {
	// Tail outputs the last N lines to the writer, then follows if configured.
	// Blocks until context is cancelled (in follow mode) or file is read (non-follow).
	Tail(ctx context.Context, output io.Writer) error

	// TailReader outputs the last N lines from a reader (e.g., stdin).
	// Follow mode is not supported for readers.
	TailReader(ctx context.Context, input io.Reader, output io.Writer) error
}

// TailerConfig holds configuration for the tailer.
type TailerConfig struct {
	Path              string
	Lines             int
	Bytes             int64 // If > 0, output last N bytes instead of lines
	FromStart         bool  // If true, start from line/byte N instead of last N
	Follow            bool
	FollowName        bool          // Follow by name (detect rotation) - like -F
	Retry             bool          // Keep trying to open file if inaccessible
	PID               int           // If > 0, terminate when this process dies
	PollInterval      time.Duration
	ZeroTerminated    bool // If true, use NUL as line delimiter instead of newline
	MaxUnchangedStats int  // With --follow=name, reopen file after N unchanged polls
}

// tailer implements Tailer.
type tailer struct {
	config TailerConfig
	opener filesystem.FileOpener
}

// NewTailer creates a new Tailer with the given configuration.
func NewTailer(config TailerConfig) Tailer {
	if config.PollInterval == 0 {
		config.PollInterval = 100 * time.Millisecond
	}
	return &tailer{
		config: config,
		opener: filesystem.NewFileOpener(),
	}
}

// Tail outputs the last N lines to the writer, then follows if configured.
func (t *tailer) Tail(ctx context.Context, output io.Writer) error {
	// If retry is enabled, wait for file to appear
	if t.config.Retry {
		return t.tailWithRetry(ctx, output)
	}

	f, err := t.opener.Open(t.config.Path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	// Don't defer close - managed by follow functions or closed below

	var pos int64

	// Bytes mode: output last N bytes (or from byte N if FromStart)
	if t.config.Bytes > 0 {
		info, err := os.Stat(t.config.Path)
		if err != nil {
			return fmt.Errorf("stat file: %w", err)
		}

		var startPos int64
		if t.config.FromStart {
			// +N means start from byte N (1-indexed, so byte 1 = offset 0)
			startPos = t.config.Bytes - 1
			if startPos < 0 {
				startPos = 0
			}
		} else {
			// -N means last N bytes
			startPos = info.Size() - t.config.Bytes
			if startPos < 0 {
				startPos = 0
			}
		}

		_, err = f.Seek(startPos, io.SeekStart)
		if err != nil {
			return fmt.Errorf("seeking: %w", err)
		}

		// Stream bytes to output (avoids loading entire file into memory)
		if err := t.streamBytes(f, output); err != nil {
			return fmt.Errorf("reading bytes: %w", err)
		}
		pos, _ = f.Seek(0, io.SeekCurrent)
	} else if t.config.FromStart {
		// FromStart mode: output from line N onwards
		lines, err := t.readFromLineN(f)
		if err != nil {
			return fmt.Errorf("reading lines: %w", err)
		}

		t.writeLines(output, lines)

		// Get current position for following
		pos, err = f.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("getting position: %w", err)
		}
	} else {
		// Lines mode: output last N lines
		lines, err := t.readLastNLines(f)
		if err != nil {
			return fmt.Errorf("reading lines: %w", err)
		}

		t.writeLines(output, lines)

		// Get current position for following
		pos, err = f.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("getting position: %w", err)
		}
	}

	if !t.config.Follow {
		f.Close()
		return nil
	}

	// For follow-by-descriptor (-f), pass the open file handle
	// For follow-by-name (-F), we'll reopen by path in followByName
	if t.config.FollowName {
		f.Close() // Close and reopen by path
		return t.followByName(ctx, output, pos)
	}
	return t.followByDescriptor(ctx, f, output, pos)
}

// TailReader outputs the last N lines from a reader (e.g., stdin).
func (t *tailer) TailReader(ctx context.Context, input io.Reader, output io.Writer) error {
	// Byte mode for stdin
	if t.config.Bytes > 0 {
		return t.tailReaderBytes(input, output)
	}

	// Line mode
	var lines []string
	var err error

	if t.config.FromStart {
		lines, err = t.readFromLineN(input)
	} else {
		lines, err = t.readLastNLines(input)
	}
	if err != nil {
		return fmt.Errorf("reading lines: %w", err)
	}

	t.writeLines(output, lines)

	return nil
}

// tailReaderBytes handles byte mode for non-seekable readers (stdin/pipes).
func (t *tailer) tailReaderBytes(input io.Reader, output io.Writer) error {
	if t.config.FromStart {
		// +N means skip first N-1 bytes and output the rest
		skipBytes := t.config.Bytes - 1
		if skipBytes < 0 {
			skipBytes = 0
		}

		// Skip the first N-1 bytes
		if skipBytes > 0 {
			_, err := io.CopyN(io.Discard, input, skipBytes)
			if err != nil && err != io.EOF {
				return fmt.Errorf("skipping bytes: %w", err)
			}
		}

		// Stream remaining bytes to output
		return t.streamBytes(input, output)
	}

	// -N means last N bytes - need to buffer since we can't seek
	// Use a ring buffer approach to avoid loading entire stream
	n := t.config.Bytes
	buf := make([]byte, n)
	total := int64(0)

	for {
		readN, err := input.Read(buf[total%n : min(n, total%n+chunkSize)])
		if readN > 0 {
			total += int64(readN)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading bytes: %w", err)
		}
	}

	if total == 0 {
		return nil
	}

	// Output the last N bytes (or all if less than N)
	if total <= n {
		output.Write(buf[:total])
	} else {
		// Ring buffer wraparound - output in correct order
		start := total % n
		output.Write(buf[start:])
		output.Write(buf[:start])
	}

	return nil
}

// tailWithRetry keeps trying to open the file until it exists or context is cancelled.
func (t *tailer) tailWithRetry(ctx context.Context, output io.Writer) error {
	ticker := time.NewTicker(t.config.PollInterval)
	defer ticker.Stop()

	for {
		f, err := t.opener.Open(t.config.Path)
		if err == nil {
			// File exists, read it using the same logic as Tail()
			var pos int64

			if t.config.Bytes > 0 {
				// Bytes mode: output last N bytes (or from byte N if FromStart)
				info, err := os.Stat(t.config.Path)
				if err != nil {
					f.Close()
					return fmt.Errorf("stat file: %w", err)
				}

				var startPos int64
				if t.config.FromStart {
					startPos = t.config.Bytes - 1
					if startPos < 0 {
						startPos = 0
					}
				} else {
					startPos = info.Size() - t.config.Bytes
					if startPos < 0 {
						startPos = 0
					}
				}

				_, err = f.Seek(startPos, io.SeekStart)
				if err != nil {
					f.Close()
					return fmt.Errorf("seeking: %w", err)
				}

				if err := t.streamBytes(f, output); err != nil {
					f.Close()
					return fmt.Errorf("reading bytes: %w", err)
				}
				pos, _ = f.Seek(0, io.SeekCurrent)
			} else if t.config.FromStart {
				// FromStart mode: output from line N onwards
				lines, err := t.readFromLineN(f)
				if err != nil {
					f.Close()
					return fmt.Errorf("reading lines: %w", err)
				}
				t.writeLines(output, lines)
				pos, _ = f.Seek(0, io.SeekCurrent)
			} else {
				// Lines mode: output last N lines
				lines, err := t.readLastNLines(f)
				if err != nil {
					f.Close()
					return fmt.Errorf("reading lines: %w", err)
				}
				t.writeLines(output, lines)
				pos, _ = f.Seek(0, io.SeekCurrent)
			}

			f.Close()

			if !t.config.Follow {
				return nil
			}

			if t.config.FollowName {
				return t.followByName(ctx, output, pos)
			}
			// For follow-by-descriptor, reopen and keep the handle
			f2, err := t.opener.Open(t.config.Path)
			if err != nil {
				return fmt.Errorf("reopening file: %w", err)
			}
			return t.followByDescriptor(ctx, f2, output, pos)
		}

		// File doesn't exist, wait and retry
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Continue to next iteration
		}
	}
}

// newLineReader creates the appropriate LineReader based on config.
func (t *tailer) newLineReader(r io.Reader) LineReader {
	if t.config.ZeroTerminated {
		return NewLineReaderWithDelimiter(r, '\x00')
	}
	return NewLineReader(r)
}

// writeLines writes lines to output with the appropriate delimiter.
func (t *tailer) writeLines(output io.Writer, lines []string) {
	for _, line := range lines {
		if t.config.ZeroTerminated {
			fmt.Fprint(output, line)
			output.Write([]byte{'\x00'})
		} else {
			fmt.Fprintln(output, line)
		}
	}
}

// writeLine writes a single line to output with the appropriate delimiter.
func (t *tailer) writeLine(output io.Writer, line string) {
	if t.config.ZeroTerminated {
		fmt.Fprint(output, line)
		output.Write([]byte{'\x00'})
	} else {
		fmt.Fprintln(output, line)
	}
}

// chunkSize is the size of chunks for reading
const chunkSize = 64 * 1024 // 64KB

// streamBytes copies bytes from reader to writer in chunks.
// This avoids loading the entire file into memory.
func (t *tailer) streamBytes(r io.Reader, w io.Writer) error {
	buf := make([]byte, chunkSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// readLastNLines reads all lines and returns the last N.
// For seekable readers, uses efficient backward reading.
func (t *tailer) readLastNLines(r io.Reader) ([]string, error) {
	// Try to use optimized backward reading for seekable files
	// Note: *os.File implements io.ReadSeeker but stdin/pipes fail on actual seek
	if seeker, ok := r.(io.ReadSeeker); ok {
		// Test if seeking actually works (stdin implements Seeker but errors)
		if _, err := seeker.Seek(0, io.SeekCurrent); err == nil {
			return t.readLastNLinesBackward(seeker)
		}
	}
	// Fallback to forward reading with ring buffer for non-seekable
	return t.readLastNLinesForward(r)
}

// readLastNLinesBackward reads last N lines by reading backwards from EOF.
func (t *tailer) readLastNLinesBackward(r io.ReadSeeker) ([]string, error) {
	// Get file size
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}

	if size == 0 {
		return nil, nil
	}

	// For small files, just read forward
	if size <= chunkSize {
		r.Seek(0, io.SeekStart)
		return t.readLastNLinesForward(r)
	}

	// Read backwards to find start position
	delimiter := byte('\n')
	if t.config.ZeroTerminated {
		delimiter = '\x00'
	}

	linesNeeded := t.config.Lines + 1 // +1 because last char might be delimiter
	linesFound := 0
	pos := size
	buf := make([]byte, chunkSize)

	for pos > 0 && linesFound < linesNeeded {
		// Calculate read position and size
		readSize := int64(chunkSize)
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize

		// Read chunk
		_, err := r.Seek(pos, io.SeekStart)
		if err != nil {
			return nil, err
		}

		n, err := r.Read(buf[:readSize])
		if err != nil && err != io.EOF {
			return nil, err
		}

		// Count delimiters backwards in this chunk
		for i := n - 1; i >= 0; i-- {
			if buf[i] == delimiter {
				linesFound++
				if linesFound >= linesNeeded {
					// Found enough lines, calculate exact position
					pos += int64(i) + 1
					break
				}
			}
		}
	}

	// Read from found position to end
	_, err = r.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	return t.readLastNLinesForward(r)
}

// readLastNLinesForward reads lines forward, keeping only last N in ring buffer.
func (t *tailer) readLastNLinesForward(r io.Reader) ([]string, error) {
	lr := t.newLineReader(r)

	// Use ring buffer for efficiency
	n := t.config.Lines
	if n <= 0 {
		n = 10
	}
	ring := make([]string, n)
	count := 0

	for {
		line, err := lr.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		ring[count%n] = line
		count++
	}

	// Extract lines in order
	if count <= n {
		return ring[:count], nil
	}

	// Reorder from ring buffer
	result := make([]string, n)
	start := count % n
	for i := 0; i < n; i++ {
		result[i] = ring[(start+i)%n]
	}
	return result, nil
}

// readFromLineN reads all lines starting from line N (1-indexed).
func (t *tailer) readFromLineN(r io.Reader) ([]string, error) {
	lr := t.newLineReader(r)
	var lines []string
	lineNum := 0

	for {
		line, err := lr.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		lineNum++
		// Include lines starting from line N
		if lineNum >= t.config.Lines {
			lines = append(lines, line)
		}
	}

	return lines, nil
}


// followByDescriptor follows the open file handle (-f mode).
// This continues reading from the same file descriptor even if the file is renamed.
func (t *tailer) followByDescriptor(ctx context.Context, f filesystem.ReadSeekCloser, output io.Writer, startPos int64) error {
	defer f.Close()

	ticker := time.NewTicker(t.config.PollInterval)
	defer ticker.Stop()

	lastPos := startPos

	for {
		// Check if monitored process is still alive
		if t.config.PID > 0 && !processExists(t.config.PID) {
			return nil
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Seek to current position and try to read more
			_, err := f.Seek(lastPos, io.SeekStart)
			if err != nil {
				continue
			}

			lr := t.newLineReader(f)
			for {
				line, err := lr.ReadLine()
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
				t.writeLine(output, line)
			}

			// Update position
			newPos, _ := f.Seek(0, io.SeekCurrent)
			lastPos = newPos
		}
	}
}

// followByName watches for file changes by path and outputs new lines (-F mode).
// This reopens the file by path, detecting rotation/replacement.
func (t *tailer) followByName(ctx context.Context, output io.Writer, startPos int64) error {
	ticker := time.NewTicker(t.config.PollInterval)
	defer ticker.Stop()

	lastPos := startPos
	var lastSize int64
	var lastFileInfo os.FileInfo
	unchangedCount := 0

	// Get initial file info
	info, err := os.Stat(t.config.Path)
	if err == nil {
		lastSize = info.Size()
		lastFileInfo = info
	}

	for {
		// Check if monitored process is still alive
		if t.config.PID > 0 && !processExists(t.config.PID) {
			return nil
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			info, err := os.Stat(t.config.Path)
			if err != nil {
				if t.config.FollowName && t.config.Retry {
					// File disappeared, wait for it to reappear
					continue
				}
				continue
			}

			currentSize := info.Size()

			// Check for file replacement (rotation) when following by name
			if t.config.FollowName && lastFileInfo != nil && !os.SameFile(lastFileInfo, info) {
				// File was replaced, read from beginning
				lastPos = 0
				lastSize = 0
				lastFileInfo = info
				unchangedCount = 0
			}

			// Check for truncation
			if currentSize < lastSize {
				lastPos = 0
				lastSize = currentSize
			}

			if currentSize == lastSize && currentSize == lastPos {
				// No change detected
				unchangedCount++

				// If MaxUnchangedStats is set and reached, re-check for file replacement
				if t.config.FollowName && t.config.MaxUnchangedStats > 0 &&
					unchangedCount >= t.config.MaxUnchangedStats {
					// Re-stat to check if file was replaced (some rotations may not change inode immediately)
					newInfo, err := os.Stat(t.config.Path)
					if err == nil && lastFileInfo != nil && !os.SameFile(lastFileInfo, newInfo) {
						lastPos = 0
						lastSize = 0
						lastFileInfo = newInfo
					}
					unchangedCount = 0
				}
				continue
			}

			// Reset unchanged counter when we see changes
			unchangedCount = 0

			// Read new content
			f, err := t.opener.Open(t.config.Path)
			if err != nil {
				continue
			}

			_, err = f.Seek(lastPos, io.SeekStart)
			if err != nil {
				f.Close()
				continue
			}

			lr := t.newLineReader(f)
			for {
				line, err := lr.ReadLine()
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
				t.writeLine(output, line)
			}

			// Update position and file info
			newPos, _ := f.Seek(0, io.SeekCurrent)
			lastPos = newPos
			lastSize = currentSize
			lastFileInfo = info
			f.Close()
		}
	}
}
