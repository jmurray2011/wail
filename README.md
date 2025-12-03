# wail

A Windows-native `tail` implementation that handles file locking, CRLF line endings, and log rotation gracefully.

## Installation

```bash
go install github.com/jmurray2011/wail/cmd/wail@latest
```

Or download a pre-built binary from the [releases](https://github.com/jmurray2011/wail/releases) page.

## Usage

```bash
# Last 10 lines (default)
wail app.log

# Last 50 lines
wail -n 50 app.log

# Last 1KB of file
wail -c 1K app.log

# Follow file for new content
wail -f app.log

# Follow with log rotation detection
wail -F app.log

# Start from line 5
wail -n +5 app.log

# Multiple files
wail app.log error.log

# Read from piped stdin (no argument needed)
type app.log | wail -n 20

# Read from stdin with explicit -
cat app.log | wail -n 20 -
```

## Features

| Flag | Description |
|------|-------------|
| `-n NUM` | Output last NUM lines (default: 10) |
| `-n +NUM` | Output starting from line NUM |
| `-c NUM` | Output last NUM bytes |
| `-c +NUM` | Output starting from byte NUM |
| `-f` | Follow file for new content |
| `-F` | Follow by name (detects rotation), implies `--retry` |
| `--follow=name` | Explicit follow-by-name mode |
| `--follow=descriptor` | Explicit follow-by-descriptor mode |
| `-s SEC` | Sleep interval between polls (default: 0.1s) |
| `--pid PID` | Terminate when process PID dies |
| `--retry` | Keep trying if file is inaccessible |
| `-q` | Never print headers |
| `-v` | Always print headers |
| `-z` | Use NUL as line delimiter |
| `--max-unchanged-stats N` | Reopen file after N unchanged polls |

Size suffixes: `b` (512), `K` (1024), `KB` (1000), `M`, `MB`, `G`, `GB`

## Why wail?

Standard Unix `tail` implementations often fail on Windows due to:

- **File locking**: Windows applications frequently hold exclusive locks on log files. wail opens files with shared read access.
- **CRLF handling**: Windows uses `\r\n` line endings. wail handles both `\n` and `\r\n` transparently.
- **Log rotation**: wail detects when files are replaced or truncated and continues from the new content.

## Building

```bash
go build -o wail.exe ./cmd/wail

# With version info
go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD)" -o wail.exe ./cmd/wail
```

## License

MIT
