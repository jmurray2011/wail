//go:build windows

package tail

import (
	"golang.org/x/sys/windows"
)

// processExists checks if a process with the given PID exists on Windows.
// Uses OpenProcess with PROCESS_QUERY_LIMITED_INFORMATION to check if the
// process handle can be obtained, then GetExitCodeProcess to verify it's running.
func processExists(pid int) bool {
	// PROCESS_QUERY_LIMITED_INFORMATION is sufficient and requires fewer privileges
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	err = windows.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}

	// STILL_ACTIVE (259) means the process is still running
	return exitCode == 259
}
