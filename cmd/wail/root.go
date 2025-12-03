package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmurray2011/wail/internal/tail"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Version information (set via ldflags during build)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "wail [file...]",
	Short:   "A Windows-native tail implementation",
	Long:    `wail is a Windows-native tail implementation that handles
file locking, CRLF line endings, and log rotation gracefully.`,
	Version: version,
	Args:    cobra.ArbitraryArgs,
	RunE:    runTail,
}

func init() {
	rootCmd.Flags().StringP("lines", "n", "10", "number of lines to output (use +N to start from line N)")
	rootCmd.Flags().StringP("bytes", "c", "", "output the last NUM bytes (use +N to start from byte N)")
	rootCmd.Flags().StringP("follow", "f", "", "follow the file; optionally =name or =descriptor")
	rootCmd.Flags().Lookup("follow").NoOptDefVal = "descriptor" // -f or --follow without value defaults to descriptor
	rootCmd.Flags().BoolP("follow-name", "F", false, "like -f, but follow by name and retry")
	rootCmd.Flags().Float64P("sleep-interval", "s", 0.1, "with -f, sleep for approximately N seconds between iterations")
	rootCmd.Flags().Int("pid", 0, "with -f, terminate after process ID dies")
	rootCmd.Flags().BoolP("quiet", "q", false, "never output headers giving file names")
	rootCmd.Flags().BoolP("verbose", "v", false, "always output headers giving file names")
	rootCmd.Flags().Bool("retry", false, "keep trying to open a file if it is inaccessible")
	rootCmd.Flags().BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	rootCmd.Flags().Int("max-unchanged-stats", 0, "with --follow=name, reopen after N iterations with no change")

	viper.BindPFlag("lines", rootCmd.Flags().Lookup("lines"))
	viper.BindPFlag("bytes", rootCmd.Flags().Lookup("bytes"))
	viper.BindPFlag("follow", rootCmd.Flags().Lookup("follow"))
	viper.BindPFlag("follow-name", rootCmd.Flags().Lookup("follow-name"))
	viper.BindPFlag("sleep-interval", rootCmd.Flags().Lookup("sleep-interval"))
	viper.BindPFlag("pid", rootCmd.Flags().Lookup("pid"))
	viper.BindPFlag("quiet", rootCmd.Flags().Lookup("quiet"))
	viper.BindPFlag("verbose", rootCmd.Flags().Lookup("verbose"))
	viper.BindPFlag("retry", rootCmd.Flags().Lookup("retry"))
	viper.BindPFlag("zero-terminated", rootCmd.Flags().Lookup("zero-terminated"))
	viper.BindPFlag("max-unchanged-stats", rootCmd.Flags().Lookup("max-unchanged-stats"))
}

func Execute() error {
	return rootCmd.Execute()
}

// parseNumArg parses a number argument that may have a + prefix and/or suffix.
// Supports suffixes: b (512), K (1024), KB (1000), M, MB, G, GB, etc.
// Returns the absolute value and whether it starts from beginning.
func parseNumArg(s string) (int64, bool, error) {
	if s == "" {
		return 0, false, nil
	}

	fromStart := false
	if strings.HasPrefix(s, "+") {
		fromStart = true
		s = s[1:]
	} else if strings.HasPrefix(s, "-") {
		s = s[1:]
	}

	// Parse suffix multiplier
	multiplier := int64(1)
	upper := strings.ToUpper(s)

	// Check for suffixes (longest first)
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"GB", 1000 * 1000 * 1000},
		{"MB", 1000 * 1000},
		{"KB", 1000},
		{"G", 1024 * 1024 * 1024},
		{"M", 1024 * 1024},
		{"K", 1024},
		{"B", 512}, // block
	}

	for _, suf := range suffixes {
		if strings.HasSuffix(upper, suf.suffix) {
			multiplier = suf.mult
			s = s[:len(s)-len(suf.suffix)]
			break
		}
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid number: %s", s)
	}

	return n * multiplier, fromStart, nil
}

func runTail(cmd *cobra.Command, args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// If no files specified, check if stdin is piped
	if len(args) == 0 {
		stat, err := os.Stdin.Stat()
		if err != nil {
			return fmt.Errorf("no files specified")
		}
		// Check both ModeCharDevice (Unix) and ModeNamedPipe (Windows) for cross-platform compatibility
		isPipe := (stat.Mode()&os.ModeCharDevice) == 0 || (stat.Mode()&os.ModeNamedPipe) != 0
		if isPipe {
			// stdin is piped, treat as "-"
			args = []string{"-"}
		} else {
			return fmt.Errorf("no files specified")
		}
	}

	// Parse lines argument (supports +N syntax)
	linesStr := viper.GetString("lines")
	lines, linesFromStart, err := parseNumArg(linesStr)
	if err != nil {
		return fmt.Errorf("invalid lines value: %w", err)
	}

	// Parse bytes argument (supports +N syntax)
	bytesStr := viper.GetString("bytes")
	bytes, bytesFromStart, err := parseNumArg(bytesStr)
	if err != nil {
		return fmt.Errorf("invalid bytes value: %w", err)
	}

	// Determine fromStart based on which mode we're in
	fromStart := linesFromStart
	if bytes > 0 {
		fromStart = bytesFromStart
	}

	// Parse --follow flag: can be empty, "descriptor", or "name"
	followStr := viper.GetString("follow")
	var follow, followName bool
	switch followStr {
	case "":
		follow = false
	case "descriptor":
		follow = true
	case "name":
		follow = true
		followName = true
	default:
		return fmt.Errorf("invalid follow mode: %s (use 'name' or 'descriptor')", followStr)
	}

	// -F flag overrides --follow
	if viper.GetBool("follow-name") {
		followName = true
		follow = true
	}
	sleepInterval := time.Duration(viper.GetFloat64("sleep-interval") * float64(time.Second))
	pid := viper.GetInt("pid")
	quiet := viper.GetBool("quiet")
	verbose := viper.GetBool("verbose")
	retry := viper.GetBool("retry")
	zeroTerminated := viper.GetBool("zero-terminated")
	maxUnchangedStats := viper.GetInt("max-unchanged-stats")
	output := cmd.OutOrStdout()
	multiFile := len(args) > 1

	// -F is equivalent to --follow=name --retry
	if followName {
		follow = true
		retry = true
	}

	// Determine if we should show headers
	// Default: show for multiple files only
	// -v/--verbose: always show
	// -q/--quiet: never show (overrides -v)
	showHeaders := (multiFile || verbose) && !quiet

	// For follow mode with multiple files, run concurrently
	if follow && multiFile {
		return runMultiFileFollow(ctx, args, int(lines), bytes, fromStart, sleepInterval, pid, output, showHeaders, retry, followName, zeroTerminated, maxUnchangedStats)
	}

	// Sequential processing for non-follow or single file
	for i, path := range args {
		// Handle stdin ("-")
		if path == "-" {
			if showHeaders {
				if i > 0 {
					fmt.Fprintln(output)
				}
				fmt.Fprintf(output, "==> standard input <==\n")
			}

			config := tail.TailerConfig{
				Lines:          int(lines),
				Bytes:          bytes,
				FromStart:      fromStart,
				ZeroTerminated: zeroTerminated,
			}
			tailer := tail.NewTailer(config)
			if err := tailer.TailReader(ctx, os.Stdin, output); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "wail: standard input: %v\n", err)
			}
			continue
		}

		if showHeaders {
			if i > 0 {
				fmt.Fprintln(output)
			}
			fmt.Fprintf(output, "==> %s <==\n", path)
		}

		config := tail.TailerConfig{
			Path:              path,
			Lines:             int(lines),
			Bytes:             bytes,
			FromStart:         fromStart,
			Follow:            follow,
			FollowName:        followName,
			Retry:             retry,
			PID:               pid,
			PollInterval:      sleepInterval,
			ZeroTerminated:    zeroTerminated,
			MaxUnchangedStats: maxUnchangedStats,
		}

		tailer := tail.NewTailer(config)
		if err := tailer.Tail(ctx, output); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "wail: %s: %v\n", path, err)
		}
	}

	return nil
}

func runMultiFileFollow(ctx context.Context, paths []string, lines int, bytes int64, fromStart bool, sleepInterval time.Duration, pid int, output io.Writer, showHeaders bool, retry bool, followName bool, zeroTerminated bool, maxUnchangedStats int) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	lastPrinted := "" // shared state to track which file header was last printed

	for _, path := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			var w io.Writer = output
			if showHeaders {
				w = &prefixWriter{
					w:           output,
					prefix:      p,
					mu:          &mu,
					lastPrinted: &lastPrinted,
				}
			}

			config := tail.TailerConfig{
				Path:              p,
				Lines:             lines,
				Bytes:             bytes,
				FromStart:         fromStart,
				Follow:            true,
				FollowName:        followName,
				Retry:             retry,
				PID:               pid,
				PollInterval:      sleepInterval,
				ZeroTerminated:    zeroTerminated,
				MaxUnchangedStats: maxUnchangedStats,
			}

			tailer := tail.NewTailer(config)
			tailer.Tail(ctx, w)
		}(path)
	}

	wg.Wait()
	return nil
}

// prefixWriter wraps a writer and prefixes each write with a filename header.
// Headers are only printed when the source changes (like GNU tail).
type prefixWriter struct {
	w           io.Writer
	prefix      string
	mu          *sync.Mutex
	lastPrinted *string // shared pointer to track which file header was last printed
}

func (pw *prefixWriter) Write(p []byte) (n int, err error) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	// Only print header if source changed or this is the first write
	if *pw.lastPrinted != pw.prefix {
		fmt.Fprintf(pw.w, "\n==> %s <==\n", pw.prefix)
		*pw.lastPrinted = pw.prefix
	}
	return pw.w.Write(p)
}
