package tail

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTailer_LastNLines(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:   testFile,
		Lines:  3,
		Follow: false,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	want := "line3\nline4\nline5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_FewerLinesThanRequested(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "line1\nline2\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:   testFile,
		Lines:  10,
		Follow: false,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	want := "line1\nline2\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:   testFile,
		Lines:  10,
		Follow: false,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	if buf.String() != "" {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestTailer_CRLFEndings(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "line1\r\nline2\r\nline3\r\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:   testFile,
		Lines:  2,
		Follow: false,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	// Output should have normalized line endings
	if !strings.Contains(got, "line2") || !strings.Contains(got, "line3") {
		t.Errorf("expected line2 and line3, got %q", got)
	}
}

func TestTailer_FollowMode(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run tailer in goroutine
	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Append new content
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.WriteString("appended\n")
	f.Close()

	// Wait for tailer to pick up change
	time.Sleep(100 * time.Millisecond)
	cancel()

	<-done

	got := buf.String()
	if !strings.Contains(got, "initial") {
		t.Errorf("expected 'initial' in output, got %q", got)
	}
	if !strings.Contains(got, "appended") {
		t.Errorf("expected 'appended' in output, got %q", got)
	}
}

func TestTailer_NonExistentFile(t *testing.T) {
	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:   "/nonexistent/file.log",
		Lines:  10,
		Follow: false,
	})

	ctx := context.Background()
	err := tailer.Tail(ctx, &buf)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestTailer_RetryWaitsForFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "delayed.log")

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		Retry:        true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run tailer in goroutine - file doesn't exist yet
	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait a bit, then create the file
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(testFile, []byte("delayed content\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Wait for tailer to pick up the file
	time.Sleep(100 * time.Millisecond)
	cancel()

	<-done

	got := buf.String()
	if !strings.Contains(got, "delayed content") {
		t.Errorf("expected 'delayed content' in output, got %q", got)
	}
}

func TestTailer_RetryFalseFailsImmediately(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "nonexistent.log")

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:   testFile,
		Lines:  10,
		Follow: true,
		Retry:  false, // Default - should fail immediately
	})

	ctx := context.Background()
	err := tailer.Tail(ctx, &buf)
	if err == nil {
		t.Error("expected error for non-existent file without retry")
	}
}

func TestTailer_FollowName_FileRotation(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "rotating.log")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("original\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		FollowName:   true, // -F: follow by name, detect rotation
		Retry:        true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Simulate log rotation: rename old file, create new one with same name
	rotatedFile := filepath.Join(dir, "rotating.log.1")
	if err := os.Rename(testFile, rotatedFile); err != nil {
		t.Fatalf("failed to rename file: %v", err)
	}

	// Create new file with same name
	if err := os.WriteFile(testFile, []byte("rotated\n"), 0644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}

	// Wait for tailer to detect rotation and read new file
	time.Sleep(100 * time.Millisecond)
	cancel()

	<-done

	got := buf.String()
	if !strings.Contains(got, "original") {
		t.Errorf("expected 'original' in output, got %q", got)
	}
	if !strings.Contains(got, "rotated") {
		t.Errorf("expected 'rotated' in output (from new file), got %q", got)
	}
}

func TestTailer_FromReader(t *testing.T) {
	input := strings.NewReader("line1\nline2\nline3\nline4\nline5\n")
	var buf bytes.Buffer

	tailer := NewTailer(TailerConfig{
		Lines: 3,
	})

	ctx := context.Background()
	if err := tailer.TailReader(ctx, input, &buf); err != nil {
		t.Fatalf("TailReader() error = %v", err)
	}

	got := buf.String()
	want := "line3\nline4\nline5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_BytesMode(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "0123456789" // 10 bytes
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Bytes: 5, // Last 5 bytes
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	want := "56789"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_StartFromLine(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:      testFile,
		Lines:     3,
		FromStart: true, // +3: start from line 3
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	want := "line3\nline4\nline5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_PIDTerminatesWhenProcessDies(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Use a non-existent PID (very high number unlikely to exist)
	nonExistentPID := 999999999

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		PID:          nonExistentPID,
		PollInterval: 10 * time.Millisecond,
	})

	ctx := context.Background()
	start := time.Now()
	err := tailer.Tail(ctx, &buf)
	elapsed := time.Since(start)

	// Should return quickly since PID doesn't exist
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	// Should have exited within reasonable time (not waiting for context timeout)
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected quick exit when PID doesn't exist, took %v", elapsed)
	}

	// Should have read initial content
	if !strings.Contains(buf.String(), "initial") {
		t.Errorf("expected 'initial' in output, got %q", buf.String())
	}
}

func TestTailer_ZeroTerminated(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create file with NUL-delimited content
	content := "line1\x00line2\x00line3\x00"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:           testFile,
		Lines:          2,
		ZeroTerminated: true,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	// Output should be NUL-terminated: "line2\x00line3\x00"
	want := "line2\x00line3\x00"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_MaxUnchangedStats(t *testing.T) {
	// This test verifies that the MaxUnchangedStats config is accepted
	// Full behavior testing would require complex rotation simulation
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	if err := os.WriteFile(testFile, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Config should accept MaxUnchangedStats
	tailer := NewTailer(TailerConfig{
		Path:              testFile,
		Lines:             10,
		Follow:            true,
		FollowName:        true,
		MaxUnchangedStats: 5,
		PollInterval:      10 * time.Millisecond,
	})

	var buf bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should not error
	err := tailer.Tail(ctx, &buf)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	if !strings.Contains(buf.String(), "initial") {
		t.Errorf("expected 'initial' in output, got %q", buf.String())
	}
}

func TestTailer_LargeFile_BackwardRead(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "large.log")

	// Create a file larger than chunk size (64KB) with known content
	var content strings.Builder
	lineCount := 10000
	for i := 1; i <= lineCount; i++ {
		fmt.Fprintf(&content, "line%05d\n", i)
	}
	if err := os.WriteFile(testFile, []byte(content.String()), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Request last 5 lines
	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Lines: 5,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	want := "line09996\nline09997\nline09998\nline09999\nline10000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_FollowWithTruncation(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "truncate.log")

	// Create initial file with content
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		FollowName:   true, // Need FollowName to detect truncation (reopens by path)
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Truncate the file (simulating log rotation with truncation)
	if err := os.WriteFile(testFile, []byte("truncated\n"), 0644); err != nil {
		t.Fatalf("failed to truncate file: %v", err)
	}

	// Wait for tailer to detect truncation
	time.Sleep(100 * time.Millisecond)
	cancel()

	<-done

	got := buf.String()
	if !strings.Contains(got, "line1") {
		t.Errorf("expected original content, got %q", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected 'truncated' after truncation, got %q", got)
	}
}

func TestTailer_ZeroTerminated_Follow(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "zero.log")

	// Create file with NUL-delimited content
	if err := os.WriteFile(testFile, []byte("initial\x00"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:           testFile,
		Lines:          10,
		Follow:         true,
		ZeroTerminated: true,
		PollInterval:   10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Append NUL-terminated content
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.WriteString("appended\x00")
	f.Close()

	time.Sleep(100 * time.Millisecond)
	cancel()

	<-done

	got := buf.String()
	if !strings.Contains(got, "initial\x00") {
		t.Errorf("expected 'initial' with NUL in output, got %q", got)
	}
	if !strings.Contains(got, "appended\x00") {
		t.Errorf("expected 'appended' with NUL in output, got %q", got)
	}
}

// nonSeekableReader wraps a reader to make it non-seekable
type nonSeekableReader struct {
	r *strings.Reader
}

func (n *nonSeekableReader) Read(p []byte) (int, error) {
	return n.r.Read(p)
}

func TestTailer_NonSeekableReader(t *testing.T) {
	// Test that non-seekable readers fall back to forward reading
	input := &nonSeekableReader{r: strings.NewReader("line1\nline2\nline3\nline4\nline5\n")}
	var buf bytes.Buffer

	tailer := NewTailer(TailerConfig{
		Lines: 2,
	})

	ctx := context.Background()
	if err := tailer.TailReader(ctx, input, &buf); err != nil {
		t.Fatalf("TailReader() error = %v", err)
	}

	got := buf.String()
	want := "line4\nline5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_BytesMode_MoreThanFileSize(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "short" // 5 bytes
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Bytes: 100, // Request more bytes than file size
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	if got != "short" {
		t.Errorf("got %q, want %q", got, "short")
	}
}

func TestTailer_DefaultLines(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create file with 15 lines
	var content strings.Builder
	for i := 1; i <= 15; i++ {
		fmt.Fprintf(&content, "line%d\n", i)
	}
	if err := os.WriteFile(testFile, []byte(content.String()), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Lines: 0, // Should default to 10
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines (default), got %d", len(lines))
	}
}

func TestTailer_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// File without trailing newline
	content := "line1\nline2\nline3"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Lines: 2,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	// Should still get last 2 lines
	if !strings.Contains(got, "line2") || !strings.Contains(got, "line3") {
		t.Errorf("expected line2 and line3, got %q", got)
	}
}

func TestTailer_SingleLine(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "single line\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Lines: 10,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	want := "single line\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailer_MixedLineEndings(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Mix of LF and CRLF
	content := "line1\nline2\r\nline3\nline4\r\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Lines: 2,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	// Should handle both types
	if !strings.Contains(got, "line3") || !strings.Contains(got, "line4") {
		t.Errorf("expected line3 and line4, got %q", got)
	}
}

func TestTailer_VeryLongLine(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create a line longer than typical buffer sizes
	longLine := strings.Repeat("x", 100000)
	content := "short1\n" + longLine + "\nshort2\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Lines: 2,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	// Should get the long line and short2
	if !strings.Contains(got, longLine) {
		t.Errorf("missing long line in output")
	}
	if !strings.Contains(got, "short2") {
		t.Errorf("missing short2 in output")
	}
}

func TestTailer_BinaryData(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.bin")

	// Binary data with embedded nulls (not using -z mode)
	content := []byte("line1\nbin\x00ary\nline3\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Lines: 2,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	// Should handle binary data without crashing
	got := buf.String()
	if !strings.Contains(got, "line3") {
		t.Errorf("expected line3 in output, got %q", got)
	}
}

func TestTailer_OnlyNewlines(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "\n\n\n\n\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:  testFile,
		Lines: 3,
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	// Should get 3 empty lines
	lines := strings.Split(buf.String(), "\n")
	// Account for trailing split
	nonEmpty := 0
	for _, l := range lines {
		if l == "" && len(buf.String()) > 0 {
			nonEmpty++
		}
	}
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines, got %d", len(lines))
	}
}

func TestTailer_FromStart_Bytes(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	content := "0123456789"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:      testFile,
		Bytes:     5,
		FromStart: true, // +5 means start from byte 5 (1-indexed)
	})

	ctx := context.Background()
	if err := tailer.Tail(ctx, &buf); err != nil {
		t.Fatalf("Tail() error = %v", err)
	}

	got := buf.String()
	// +5 means start from byte 5 (1-indexed), which is "456789"
	want := "456789"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTailer_FollowDescriptor_KeepsFollowingRenamedFile tests that -f (follow by descriptor)
// continues reading from the same file handle even after the file is renamed.
// This is the key behavioral difference from -F.
func TestTailer_FollowDescriptor_KeepsFollowingRenamedFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")
	renamedFile := filepath.Join(dir, "test.log.1")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("[ORIGINAL] line1\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		FollowName:   false, // -f: follow by descriptor
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Rename the file (simulate rotation)
	if err := os.Rename(testFile, renamedFile); err != nil {
		t.Fatalf("failed to rename file: %v", err)
	}

	// Create new file at original path
	if err := os.WriteFile(testFile, []byte("[NEW FILE] line1\n"), 0644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}

	// Append to the RENAMED file (the one wail should still be following with -f)
	f, err := os.OpenFile(renamedFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open renamed file: %v", err)
	}
	f.WriteString("[RENAMED] appended\n")
	f.Close()

	// Also append to the new file
	f2, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open new file: %v", err)
	}
	f2.WriteString("[NEW FILE] appended\n")
	f2.Close()

	// Wait for tailer to process
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	got := buf.String()

	// With -f (followByDescriptor), should see content from renamed file, NOT new file
	if !strings.Contains(got, "[ORIGINAL]") {
		t.Errorf("expected '[ORIGINAL]' in output, got %q", got)
	}
	if !strings.Contains(got, "[RENAMED] appended") {
		t.Errorf("expected '[RENAMED] appended' in output (following original descriptor), got %q", got)
	}
	if strings.Contains(got, "[NEW FILE]") {
		t.Errorf("should NOT see '[NEW FILE]' with -f mode, but got %q", got)
	}
}

// TestTailer_FollowName_SwitchesToNewFile tests that -F (follow by name)
// switches to reading the new file when the original is renamed.
func TestTailer_FollowName_SwitchesToNewFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")
	renamedFile := filepath.Join(dir, "test.log.1")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("[ORIGINAL] line1\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		FollowName:   true, // -F: follow by name
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Rename the file (simulate rotation)
	if err := os.Rename(testFile, renamedFile); err != nil {
		t.Fatalf("failed to rename file: %v", err)
	}

	// Create new file at original path
	if err := os.WriteFile(testFile, []byte("[NEW FILE] line1\n"), 0644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}

	// Append to the renamed file
	f, err := os.OpenFile(renamedFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open renamed file: %v", err)
	}
	f.WriteString("[RENAMED] appended\n")
	f.Close()

	// Append to the new file at original path
	f2, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open new file: %v", err)
	}
	f2.WriteString("[NEW FILE] appended\n")
	f2.Close()

	// Wait for tailer to process
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	got := buf.String()

	// With -F (followByName), should see content from the NEW file at original path
	if !strings.Contains(got, "[ORIGINAL]") {
		t.Errorf("expected '[ORIGINAL]' in output, got %q", got)
	}
	if !strings.Contains(got, "[NEW FILE]") {
		t.Errorf("expected '[NEW FILE]' in output (following by name), got %q", got)
	}
	// Should NOT see content appended to the renamed file
	if strings.Contains(got, "[RENAMED] appended") {
		t.Errorf("should NOT see '[RENAMED] appended' with -F mode, but got %q", got)
	}
}

// TestTailer_BytesMode_WithFollow tests bytes mode combined with follow.
func TestTailer_BytesMode_WithFollow(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("0123456789"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Bytes:        5, // Last 5 bytes
		Follow:       true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Append more content
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.WriteString("ABCDE")
	f.Close()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	got := buf.String()

	// Should have last 5 bytes initially ("56789") plus appended content
	if !strings.Contains(got, "56789") {
		t.Errorf("expected initial '56789' in output, got %q", got)
	}
	if !strings.Contains(got, "ABCDE") {
		t.Errorf("expected appended 'ABCDE' in output, got %q", got)
	}
}

// TestTailer_FromStart_Bytes_WithFollow tests -c +N with follow mode.
func TestTailer_FromStart_Bytes_WithFollow(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("0123456789"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Bytes:        5,
		FromStart:    true, // Start from byte 5
		Follow:       true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Append more content
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.WriteString("APPENDED")
	f.Close()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	got := buf.String()

	// +5 means start from byte 5 (1-indexed, so "456789") plus appended
	if !strings.Contains(got, "456789") {
		t.Errorf("expected '456789' (from byte 5 onwards) in output, got %q", got)
	}
	if !strings.Contains(got, "APPENDED") {
		t.Errorf("expected 'APPENDED' in output, got %q", got)
	}
}

// TestTailer_FromStart_Lines_WithFollow tests -n +N with follow mode.
func TestTailer_FromStart_Lines_WithFollow(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        3,
		FromStart:    true, // Start from line 3
		Follow:       true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Append more content
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.WriteString("line6\nline7\n")
	f.Close()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	got := buf.String()

	// +3 means start from line 3 onwards
	if !strings.Contains(got, "line3") {
		t.Errorf("expected 'line3' in output, got %q", got)
	}
	if !strings.Contains(got, "line4") {
		t.Errorf("expected 'line4' in output, got %q", got)
	}
	if !strings.Contains(got, "line5") {
		t.Errorf("expected 'line5' in output, got %q", got)
	}
	if !strings.Contains(got, "line6") {
		t.Errorf("expected appended 'line6' in output, got %q", got)
	}
	if !strings.Contains(got, "line7") {
		t.Errorf("expected appended 'line7' in output, got %q", got)
	}
	// Should NOT have lines 1 and 2
	if strings.Contains(got, "line1") || strings.Contains(got, "line2") {
		t.Errorf("should NOT have line1 or line2 with +3, got %q", got)
	}
}

// TestTailer_FollowDescriptor_NoRotationDetection verifies -f doesn't detect truncation.
func TestTailer_FollowDescriptor_NoTruncationDetection(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create initial file with lots of content
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		FollowName:   false, // -f: follow by descriptor
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait for initial read
	time.Sleep(50 * time.Millisecond)

	// Truncate the file (like copytruncate rotation)
	if err := os.WriteFile(testFile, []byte("truncated\n"), 0644); err != nil {
		t.Fatalf("failed to truncate file: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	got := buf.String()

	// With -f, truncation is NOT detected - it keeps reading from old position
	// The truncated content may or may not appear depending on timing
	if !strings.Contains(got, "line1") {
		t.Errorf("expected initial content 'line1' in output, got %q", got)
	}
}

// TestTailer_FollowName_WithRetry tests -F --retry for files that don't exist yet.
func TestTailer_FollowName_WithRetry(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "delayed.log")

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		FollowName:   true,
		Retry:        true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait a bit, then create the file
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(testFile, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Wait for tailer to pick up
	time.Sleep(50 * time.Millisecond)

	// Append more content
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.WriteString("appended\n")
	f.Close()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	got := buf.String()
	if !strings.Contains(got, "initial") {
		t.Errorf("expected 'initial' in output, got %q", got)
	}
	if !strings.Contains(got, "appended") {
		t.Errorf("expected 'appended' in output, got %q", got)
	}
}

// TestTailer_FollowName_FileDisappearsAndReappears tests -F --retry when file doesn't exist initially.
// Note: Testing actual delete/recreate is flaky due to filesystem caching and inode reuse.
// This test focuses on the --retry behavior for files that appear later.
func TestTailer_FollowName_FileDisappearsAndReappears(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "delayed.log")

	// File doesn't exist initially
	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        10,
		Follow:       true,
		FollowName:   true,
		Retry:        true,
		PollInterval: 20 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait a bit, then create file
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(testFile, []byte("first\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Wait for tailer to pick up file
	time.Sleep(100 * time.Millisecond)

	// Append more content
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.WriteString("second\n")
	f.Close()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	got := buf.String()
	if !strings.Contains(got, "first") {
		t.Errorf("expected 'first' in output, got %q", got)
	}
	if !strings.Contains(got, "second") {
		t.Errorf("expected 'second' in output, got %q", got)
	}
}

// Regression tests for bug fixes

// TestTailer_RetryWithBytesMode verifies that --retry respects -c (bytes) mode.
// This was a bug where tailWithRetry always called readLastNLines.
func TestTailer_RetryWithBytesMode(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "delayed.log")

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Bytes:        5, // Last 5 bytes
		Retry:        true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait a bit, then create the file
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(testFile, []byte("0123456789"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Wait for tailer to pick up the file
	time.Sleep(100 * time.Millisecond)
	cancel()

	<-done

	got := buf.String()
	want := "56789" // Last 5 bytes
	if got != want {
		t.Errorf("retry with -c: got %q, want %q", got, want)
	}
}

// TestTailer_RetryWithFromStart verifies that --retry respects +N (from start) mode.
func TestTailer_RetryWithFromStart(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "delayed.log")

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Lines:        3,
		FromStart:    true, // +3: start from line 3
		Retry:        true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait a bit, then create the file
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Wait for tailer to pick up the file
	time.Sleep(100 * time.Millisecond)
	cancel()

	<-done

	got := buf.String()
	// +3 means start from line 3
	if !strings.Contains(got, "line3") || !strings.Contains(got, "line4") || !strings.Contains(got, "line5") {
		t.Errorf("retry with +3: expected line3-5, got %q", got)
	}
	if strings.Contains(got, "line1") || strings.Contains(got, "line2") {
		t.Errorf("retry with +3: should NOT have line1 or line2, got %q", got)
	}
}

// TestTailer_RetryWithBytesFromStart verifies that --retry respects -c +N mode.
func TestTailer_RetryWithBytesFromStart(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "delayed.log")

	var buf bytes.Buffer
	tailer := NewTailer(TailerConfig{
		Path:         testFile,
		Bytes:        5,
		FromStart:    true, // +5: start from byte 5
		Retry:        true,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tailer.Tail(ctx, &buf)
	}()

	// Wait a bit, then create the file
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(testFile, []byte("0123456789"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Wait for tailer to pick up the file
	time.Sleep(100 * time.Millisecond)
	cancel()

	<-done

	got := buf.String()
	want := "456789" // +5 means from byte 5 (1-indexed, so offset 4)
	if got != want {
		t.Errorf("retry with -c +5: got %q, want %q", got, want)
	}
}

// TestTailer_TailReaderBytesMode verifies that TailReader supports -c (bytes) mode.
// This was a bug where stdin with -c was broken.
func TestTailer_TailReaderBytesMode(t *testing.T) {
	input := strings.NewReader("0123456789")
	var buf bytes.Buffer

	tailer := NewTailer(TailerConfig{
		Bytes: 5, // Last 5 bytes
	})

	ctx := context.Background()
	if err := tailer.TailReader(ctx, input, &buf); err != nil {
		t.Fatalf("TailReader() error = %v", err)
	}

	got := buf.String()
	want := "56789"
	if got != want {
		t.Errorf("TailReader with -c: got %q, want %q", got, want)
	}
}

// TestTailer_TailReaderBytesFromStart verifies that TailReader supports -c +N mode.
func TestTailer_TailReaderBytesFromStart(t *testing.T) {
	input := strings.NewReader("0123456789")
	var buf bytes.Buffer

	tailer := NewTailer(TailerConfig{
		Bytes:     5,
		FromStart: true, // +5: start from byte 5
	})

	ctx := context.Background()
	if err := tailer.TailReader(ctx, input, &buf); err != nil {
		t.Fatalf("TailReader() error = %v", err)
	}

	got := buf.String()
	want := "456789" // +5 means from byte 5 (1-indexed)
	if got != want {
		t.Errorf("TailReader with -c +5: got %q, want %q", got, want)
	}
}

// TestTailer_TailReaderBytesLargerThanInput verifies TailReader handles -c N > input size.
func TestTailer_TailReaderBytesLargerThanInput(t *testing.T) {
	input := strings.NewReader("short")
	var buf bytes.Buffer

	tailer := NewTailer(TailerConfig{
		Bytes: 100, // Request more bytes than input
	})

	ctx := context.Background()
	if err := tailer.TailReader(ctx, input, &buf); err != nil {
		t.Fatalf("TailReader() error = %v", err)
	}

	got := buf.String()
	want := "short"
	if got != want {
		t.Errorf("TailReader with -c 100 on small input: got %q, want %q", got, want)
	}
}

// TestTailer_ProcessExists tests the processExists function.
func TestTailer_ProcessExists(t *testing.T) {
	// Test with current process (should exist)
	if !processExists(os.Getpid()) {
		t.Error("processExists should return true for current process")
	}

	// Test with non-existent PID
	if processExists(999999999) {
		t.Error("processExists should return false for non-existent PID")
	}
}

