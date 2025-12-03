//go:build !windows

package filesystem

import "os"

// defaultOpener implements FileOpener using standard os.Open.
// On Unix, file sharing is handled differently than Windows.
type defaultOpener struct{}

// NewFileOpener returns a FileOpener appropriate for the current OS.
func NewFileOpener() FileOpener {
	return &defaultOpener{}
}

// Open opens the named file for reading.
func (o *defaultOpener) Open(name string) (ReadSeekCloser, error) {
	return os.Open(name)
}
