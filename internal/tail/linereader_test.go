package tail

import (
	"io"
	"strings"
	"testing"
)

func TestLineReader_LFEndings(t *testing.T) {
	input := "line1\nline2\nline3\n"
	reader := NewLineReader(strings.NewReader(input))

	expected := []string{"line1", "line2", "line3"}
	for i, want := range expected {
		got, err := reader.ReadLine()
		if err != nil {
			t.Fatalf("line %d: ReadLine() error = %v", i+1, err)
		}
		if got != want {
			t.Errorf("line %d: got %q, want %q", i+1, got, want)
		}
	}

	// Next read should return EOF
	_, err := reader.ReadLine()
	if err != io.EOF {
		t.Errorf("expected io.EOF after last line, got %v", err)
	}
}

func TestLineReader_CRLFEndings(t *testing.T) {
	input := "line1\r\nline2\r\nline3\r\n"
	reader := NewLineReader(strings.NewReader(input))

	expected := []string{"line1", "line2", "line3"}
	for i, want := range expected {
		got, err := reader.ReadLine()
		if err != nil {
			t.Fatalf("line %d: ReadLine() error = %v", i+1, err)
		}
		if got != want {
			t.Errorf("line %d: got %q, want %q", i+1, got, want)
		}
	}
}

func TestLineReader_MixedEndings(t *testing.T) {
	input := "line1\nline2\r\nline3\n"
	reader := NewLineReader(strings.NewReader(input))

	expected := []string{"line1", "line2", "line3"}
	for i, want := range expected {
		got, err := reader.ReadLine()
		if err != nil {
			t.Fatalf("line %d: ReadLine() error = %v", i+1, err)
		}
		if got != want {
			t.Errorf("line %d: got %q, want %q", i+1, got, want)
		}
	}
}

func TestLineReader_NoTrailingNewline(t *testing.T) {
	input := "line1\nline2"
	reader := NewLineReader(strings.NewReader(input))

	got1, err := reader.ReadLine()
	if err != nil {
		t.Fatalf("line 1: ReadLine() error = %v", err)
	}
	if got1 != "line1" {
		t.Errorf("line 1: got %q, want %q", got1, "line1")
	}

	got2, err := reader.ReadLine()
	if err != nil {
		t.Fatalf("line 2: ReadLine() error = %v", err)
	}
	if got2 != "line2" {
		t.Errorf("line 2: got %q, want %q", got2, "line2")
	}

	_, err = reader.ReadLine()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestLineReader_EmptyInput(t *testing.T) {
	reader := NewLineReader(strings.NewReader(""))

	_, err := reader.ReadLine()
	if err != io.EOF {
		t.Errorf("expected io.EOF for empty input, got %v", err)
	}
}

func TestLineReader_EmptyLines(t *testing.T) {
	input := "line1\n\nline3\n"
	reader := NewLineReader(strings.NewReader(input))

	expected := []string{"line1", "", "line3"}
	for i, want := range expected {
		got, err := reader.ReadLine()
		if err != nil {
			t.Fatalf("line %d: ReadLine() error = %v", i+1, err)
		}
		if got != want {
			t.Errorf("line %d: got %q, want %q", i+1, got, want)
		}
	}
}
