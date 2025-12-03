package tail

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkTailer_LastNLines_SmallFile(b *testing.B) {
	dir := b.TempDir()
	testFile := filepath.Join(dir, "small.log")

	// 1000 lines (~11KB)
	var content bytes.Buffer
	for i := 1; i <= 1000; i++ {
		fmt.Fprintf(&content, "line%05d\n", i)
	}
	if err := os.WriteFile(testFile, content.Bytes(), 0644); err != nil {
		b.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		tailer := NewTailer(TailerConfig{
			Path:  testFile,
			Lines: 10,
		})
		tailer.Tail(ctx, &buf)
	}
}

func BenchmarkTailer_LastNLines_LargeFile(b *testing.B) {
	dir := b.TempDir()
	testFile := filepath.Join(dir, "large.log")

	// 100,000 lines (~1.1MB)
	var content bytes.Buffer
	for i := 1; i <= 100000; i++ {
		fmt.Fprintf(&content, "line%06d\n", i)
	}
	if err := os.WriteFile(testFile, content.Bytes(), 0644); err != nil {
		b.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		tailer := NewTailer(TailerConfig{
			Path:  testFile,
			Lines: 10,
		})
		tailer.Tail(ctx, &buf)
	}
}

func BenchmarkTailer_LastNLines_VeryLargeFile(b *testing.B) {
	dir := b.TempDir()
	testFile := filepath.Join(dir, "verylarge.log")

	// 1,000,000 lines (~13MB)
	var content bytes.Buffer
	for i := 1; i <= 1000000; i++ {
		fmt.Fprintf(&content, "line%07d\n", i)
	}
	if err := os.WriteFile(testFile, content.Bytes(), 0644); err != nil {
		b.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		tailer := NewTailer(TailerConfig{
			Path:  testFile,
			Lines: 10,
		})
		tailer.Tail(ctx, &buf)
	}
}
