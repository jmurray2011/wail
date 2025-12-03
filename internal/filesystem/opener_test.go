package filesystem

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFileOpener_OpenExistingFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	content := []byte("hello\nworld\n")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	opener := NewFileOpener()

	f, err := opener.Open(testFile)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", testFile, err)
	}
	defer f.Close()

	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll error = %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestFileOpener_OpenNonExistentFile(t *testing.T) {
	opener := NewFileOpener()

	_, err := opener.Open("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestFileOpener_SeekInFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	content := []byte("line1\nline2\nline3\n")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	opener := NewFileOpener()

	f, err := opener.Open(testFile)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", testFile, err)
	}
	defer f.Close()

	// Seek to position 6 (start of "line2")
	pos, err := f.Seek(6, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek error = %v", err)
	}
	if pos != 6 {
		t.Errorf("Seek returned pos = %d, want 6", pos)
	}

	buf := make([]byte, 5)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if string(buf[:n]) != "line2" {
		t.Errorf("got %q, want %q", buf[:n], "line2")
	}
}
