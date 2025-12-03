package filesystem

import "io"

// FileOpener opens files for reading with appropriate share modes.
// On Windows, this means FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE
// to allow reading files that other processes have open.
type FileOpener interface {
	// Open opens the named file for reading.
	// The returned ReadSeekCloser allows reading, seeking, and must be closed.
	Open(name string) (ReadSeekCloser, error)
}

// ReadSeekCloser combines io.Reader, io.Seeker, and io.Closer.
type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}
