package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestParseNumArg(t *testing.T) {
	tests := []struct {
		input     string
		wantNum   int64
		wantStart bool
		wantErr   bool
	}{
		// Basic numbers
		{"10", 10, false, false},
		{"+5", 5, true, false},
		{"-5", 5, false, false},
		{"", 0, false, false},

		// Suffixes (binary - powers of 1024)
		{"5K", 5 * 1024, false, false},
		{"5k", 5 * 1024, false, false},
		{"2M", 2 * 1024 * 1024, false, false},
		{"1G", 1 * 1024 * 1024 * 1024, false, false},

		// Suffixes (decimal - powers of 1000)
		{"5KB", 5 * 1000, false, false},
		{"5kB", 5 * 1000, false, false},
		{"2MB", 2 * 1000 * 1000, false, false},
		{"1GB", 1 * 1000 * 1000 * 1000, false, false},

		// Block suffix (512 bytes)
		{"10b", 10 * 512, false, false},

		// With + prefix
		{"+5K", 5 * 1024, true, false},

		// Invalid
		{"abc", 0, false, true},
		{"5X", 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			num, fromStart, err := parseNumArg(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNumArg(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if num != tt.wantNum {
					t.Errorf("parseNumArg(%q) num = %d, want %d", tt.input, num, tt.wantNum)
				}
				if fromStart != tt.wantStart {
					t.Errorf("parseNumArg(%q) fromStart = %v, want %v", tt.input, fromStart, tt.wantStart)
				}
			}
		})
	}
}

// newTestCmd creates a fresh command instance for testing (avoids global state issues)
func newTestCmd() *cobra.Command {
	// Reset viper for each test
	viper.Reset()

	cmd := &cobra.Command{
		Use:  "wail [file...]",
		Args: cobra.ArbitraryArgs,
		RunE: runTail,
	}
	cmd.Flags().StringP("lines", "n", "10", "")
	cmd.Flags().StringP("bytes", "c", "", "")
	cmd.Flags().StringP("follow", "f", "", "")
	cmd.Flags().Lookup("follow").NoOptDefVal = "descriptor"
	cmd.Flags().BoolP("follow-name", "F", false, "")
	cmd.Flags().Float64P("sleep-interval", "s", 0.1, "")
	cmd.Flags().Int("pid", 0, "")
	cmd.Flags().BoolP("quiet", "q", false, "")
	cmd.Flags().BoolP("verbose", "v", false, "")
	cmd.Flags().Bool("retry", false, "")
	cmd.Flags().BoolP("zero-terminated", "z", false, "")
	cmd.Flags().Int("max-unchanged-stats", 0, "")

	// Bind viper to flags
	viper.BindPFlag("lines", cmd.Flags().Lookup("lines"))
	viper.BindPFlag("bytes", cmd.Flags().Lookup("bytes"))
	viper.BindPFlag("follow", cmd.Flags().Lookup("follow"))
	viper.BindPFlag("follow-name", cmd.Flags().Lookup("follow-name"))
	viper.BindPFlag("sleep-interval", cmd.Flags().Lookup("sleep-interval"))
	viper.BindPFlag("pid", cmd.Flags().Lookup("pid"))
	viper.BindPFlag("quiet", cmd.Flags().Lookup("quiet"))
	viper.BindPFlag("verbose", cmd.Flags().Lookup("verbose"))
	viper.BindPFlag("retry", cmd.Flags().Lookup("retry"))
	viper.BindPFlag("zero-terminated", cmd.Flags().Lookup("zero-terminated"))
	viper.BindPFlag("max-unchanged-stats", cmd.Flags().Lookup("max-unchanged-stats"))

	return cmd
}

func TestCLI_ReadFile(t *testing.T) {
	// Create test file
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-n", "3", testFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "line3\nline4\nline5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCLI_ReadStdinExplicit(t *testing.T) {
	// Test reading from stdin using explicit "-" argument
	input := "line1\nline2\nline3\nline4\nline5\n"

	// Save and restore original stdin
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	// Create a pipe and write test data
	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.WriteString(input)
		w.Close()
	}()

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-n", "2", "-"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "line4\nline5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCLI_ReadStdinPiped(t *testing.T) {
	// Test reading from stdin without arguments (piped input).
	// This tests auto-detection of piped stdin, which works via:
	// - ModeCharDevice == 0 on Unix
	// - ModeNamedPipe != 0 on Windows
	// os.Pipe() creates a pipe that satisfies both conditions.
	input := "line1\nline2\nline3\n"

	// Save and restore original stdin
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	// Create a pipe (simulates piped input - sets ModeNamedPipe, clears ModeCharDevice)
	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.WriteString(input)
		w.Close()
	}()

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-n", "2"}) // No file argument!

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "line2\nline3\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCLI_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "file1.txt")
	file2 := filepath.Join(dir, "file2.txt")
	os.WriteFile(file1, []byte("a1\na2\na3\n"), 0644)
	os.WriteFile(file2, []byte("b1\nb2\nb3\n"), 0644)

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-n", "2", file1, file2})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// Should have headers for multiple files
	if !strings.Contains(got, "==> "+file1+" <==") {
		t.Errorf("missing header for file1, got: %q", got)
	}
	if !strings.Contains(got, "==> "+file2+" <==") {
		t.Errorf("missing header for file2, got: %q", got)
	}
	if !strings.Contains(got, "a2\na3") {
		t.Errorf("missing content from file1, got: %q", got)
	}
	if !strings.Contains(got, "b2\nb3") {
		t.Errorf("missing content from file2, got: %q", got)
	}
}

func TestCLI_BytesMode(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("0123456789"), 0644)

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-c", "5", testFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if got != "56789" {
		t.Errorf("got %q, want %q", got, "56789")
	}
}

func TestCLI_FromStart(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644)

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-n", "+3", testFile}) // Start from line 3

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	want := "line3\nline4\nline5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCLI_QuietMode(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "file1.txt")
	file2 := filepath.Join(dir, "file2.txt")
	os.WriteFile(file1, []byte("a1\n"), 0644)
	os.WriteFile(file2, []byte("b1\n"), 0644)

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-q", file1, file2})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// Should NOT have headers with -q
	if strings.Contains(got, "==>") {
		t.Errorf("should not have headers with -q, got: %q", got)
	}
}

func TestCLI_VerboseMode(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("line1\n"), 0644)

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-v", testFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// Should have header even for single file with -v
	if !strings.Contains(got, "==> "+testFile+" <==") {
		t.Errorf("should have header with -v, got: %q", got)
	}
}

func TestCLI_FromStartStdin(t *testing.T) {
	// Test +N syntax with stdin (start from line N)
	input := "line1\nline2\nline3\nline4\nline5\n"

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.WriteString(input)
		w.Close()
	}()

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-n", "+3"}) // Start from line 3

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// Should output lines 3, 4, 5 (everything from line 3 onwards)
	want := "line3\nline4\nline5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCLI_NonExistentFile(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"/nonexistent/file.txt"})

	// Should not return error (matches tail behavior - prints error to stderr but continues)
	cmd.Execute()

	// Should have error message in stderr
	if !strings.Contains(errOut.String(), "wail:") {
		t.Errorf("expected error in stderr, got: %q", errOut.String())
	}
}

func TestCLI_BytesFromStart(t *testing.T) {
	// Test -c +N (output from byte N onwards)
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("0123456789"), 0644)

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-c", "+5", testFile}) // Start from byte 5 (1-indexed)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// +5 means start from byte 5 (1-indexed), so bytes 5-10 = "456789"
	if got != "456789" {
		t.Errorf("got %q, want %q", got, "456789")
	}
}

func TestCLI_BytesFromStart_Stdin(t *testing.T) {
	// Test -c +N with stdin
	input := "0123456789ABCDEF"

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.WriteString(input)
		w.Close()
	}()

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-c", "+5"}) // Start from byte 5

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// +5 means start from byte 5 (1-indexed), so bytes 5-16 = "456789ABCDEF"
	want := "456789ABCDEF"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCLI_BytesLastN_Stdin(t *testing.T) {
	// Test -c N with stdin (last N bytes)
	input := "0123456789ABCDEF"

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.WriteString(input)
		w.Close()
	}()

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-c", "5"}) // Last 5 bytes

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// Last 5 bytes = "BCDEF"
	want := "BCDEF"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCLI_SizeSuffixes(t *testing.T) {
	// Test size suffix parsing (K, KB, M, etc.)
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")

	// Create a small file
	content := strings.Repeat("x", 100)
	os.WriteFile(testFile, []byte(content), 0644)

	tests := []struct {
		arg  string
		want int // expected bytes to read (or all if larger than file)
	}{
		{"50", 50},    // plain number
		{"1K", 100},   // 1K = 1024, but file is only 100 bytes
		{"100b", 100}, // 100 * 512 = 51200, file is 100 bytes
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newTestCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"-c", tt.arg, testFile})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			got := len(out.String())
			if got > tt.want {
				t.Errorf("got %d bytes, want at most %d", got, tt.want)
			}
		})
	}
}

func TestCLI_ZeroTerminated(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")

	// Create file with NUL-delimited content
	content := "line1\x00line2\x00line3\x00"
	os.WriteFile(testFile, []byte(content), 0644)

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-z", "-n", "2", testFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// Should output last 2 NUL-delimited lines with NUL terminators
	if !strings.Contains(got, "line2\x00") {
		t.Errorf("expected 'line2' with NUL in output, got %q", got)
	}
	if !strings.Contains(got, "line3\x00") {
		t.Errorf("expected 'line3' with NUL in output, got %q", got)
	}
}

func TestCLI_ParseNumArg_Suffixes(t *testing.T) {
	// Test all supported suffixes
	tests := []struct {
		input   string
		wantNum int64
		wantErr bool
	}{
		// Basic numbers
		{"10", 10, false},
		{"+10", 10, false},

		// Block suffix (512 bytes)
		{"1b", 512, false},
		{"2b", 1024, false},

		// Kilobyte suffixes
		{"1K", 1024, false},
		{"1k", 1024, false},
		{"2K", 2048, false},
		{"1KB", 1000, false},
		{"1kB", 1000, false},

		// Megabyte suffixes
		{"1M", 1024 * 1024, false},
		{"1m", 1024 * 1024, false},
		{"1MB", 1000 * 1000, false},

		// Gigabyte suffixes
		{"1G", 1024 * 1024 * 1024, false},
		{"1GB", 1000 * 1000 * 1000, false},

		// Invalid
		{"abc", 0, true},
		{"1X", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			num, _, err := parseNumArg(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNumArg(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && num != tt.wantNum {
				t.Errorf("parseNumArg(%q) = %d, want %d", tt.input, num, tt.wantNum)
			}
		})
	}
}

func TestCLI_NoFilesNoStdin(t *testing.T) {
	// Test that we get an error when no files specified and stdin is not piped.
	// We simulate a non-pipe stdin by using os.DevNull which is a regular file,
	// not a character device or named pipe.
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	// Open /dev/null (or NUL on Windows) as stdin - it's neither a char device nor a pipe
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()
	os.Stdin = devNull

	var out bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{}) // No file arguments

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no files specified and stdin is not piped")
	}
	if !strings.Contains(err.Error(), "no files specified") {
		t.Errorf("expected 'no files specified' error, got: %v", err)
	}
}
