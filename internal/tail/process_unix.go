//go:build !windows

package tail

import (
	"os"
	"syscall"
)

// processExists checks if a process with the given PID exists on Unix.
// Uses signal 0 which doesn't actually send a signal but checks if the
// process exists and we have permission to signal it.
func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks if process exists without sending an actual signal
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
