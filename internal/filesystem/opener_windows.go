//go:build windows

package filesystem

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// windowsOpener implements FileOpener with Windows-specific share modes.
type windowsOpener struct{}

// NewFileOpener returns a FileOpener that uses Windows share modes
// to allow reading files that other processes have open.
func NewFileOpener() FileOpener {
	return &windowsOpener{}
}

// Open opens the named file for reading with FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE.
// Supports extended-length paths (>260 chars) by automatically adding \\?\ prefix.
func (o *windowsOpener) Open(name string) (ReadSeekCloser, error) {
	// Convert to extended-length path if needed (paths >260 chars hit MAX_PATH limit)
	// See: https://docs.microsoft.com/en-us/windows/win32/fileio/maximum-file-path-limitation
	if len(name) > 259 && !strings.HasPrefix(name, `\\?\`) {
		if strings.HasPrefix(name, `\\`) {
			// UNC path: \\server\share -> \\?\UNC\server\share
			name = `\\?\UNC\` + name[2:]
		} else {
			// Local path: C:\... -> \\?\C:\...
			name = `\\?\` + name
		}
	}

	pathPtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", name, err)
	}

	return os.NewFile(uintptr(handle), name), nil
}
