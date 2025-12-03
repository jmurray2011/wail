package tail

import (
	"bufio"
	"bytes"
	"io"
)

// LineReader reads lines from a source, handling both LF and CRLF endings.
type LineReader interface {
	// ReadLine reads the next line, stripping the line ending.
	// Returns io.EOF when no more lines are available.
	ReadLine() (string, error)
}

// lineReader implements LineReader using bufio.Scanner.
type lineReader struct {
	scanner *bufio.Scanner
	err     error
}

// maxLineSize is the maximum line length we support (1MB)
const maxLineSize = 1024 * 1024

// NewLineReader creates a LineReader from an io.Reader.
// It handles both LF and CRLF line endings transparently.
func NewLineReader(r io.Reader) LineReader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxLineSize)
	scanner.Split(scanLinesWithCRLF)
	return &lineReader{scanner: scanner}
}

// NewLineReaderWithDelimiter creates a LineReader with a custom delimiter byte.
// Use '\x00' for NUL-terminated lines (-z flag).
func NewLineReaderWithDelimiter(r io.Reader, delim byte) LineReader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxLineSize)
	scanner.Split(makeScanDelimited(delim))
	return &lineReader{scanner: scanner}
}

// makeScanDelimited creates a split function that uses the given delimiter.
func makeScanDelimited(delim byte) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		if i := bytes.IndexByte(data, delim); i >= 0 {
			return i + 1, data[0:i], nil
		}

		// At EOF with remaining data - return it as final token
		if atEOF {
			return len(data), data, nil
		}

		// Request more data
		return 0, nil, nil
	}
}

// ReadLine reads the next line, stripping the line ending.
func (lr *lineReader) ReadLine() (string, error) {
	if lr.err != nil {
		return "", lr.err
	}

	if lr.scanner.Scan() {
		return lr.scanner.Text(), nil
	}

	if err := lr.scanner.Err(); err != nil {
		lr.err = err
		return "", err
	}

	lr.err = io.EOF
	return "", io.EOF
}

// scanLinesWithCRLF is a split function for bufio.Scanner that handles
// both LF and CRLF line endings. Based on bufio.ScanLines but strips \r.
func scanLinesWithCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// Found a newline
		line := data[0:i]
		// Strip trailing \r if present (CRLF)
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		return i + 1, line, nil
	}

	// At EOF with remaining data - return it as final line
	if atEOF {
		line := data
		// Strip trailing \r if present
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		return len(data), line, nil
	}

	// Request more data
	return 0, nil, nil
}
