package filesystem

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// OpenRegular opens a regular file without following a final symlink or blocking on a FIFO.
func OpenRegular(path string, flags int, mode fs.FileMode) (*os.File, error) {
	fd, err := unix.Open(
		path,
		flags|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC,
		uint32(mode.Perm()),
	)
	if err != nil {
		return nil, err
	}

	file := os.NewFile(uintptr(fd), path)

	info, err := file.Stat()
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}

	if !info.Mode().IsRegular() {
		return nil, errors.Join(ErrNotRegular, file.Close())
	}

	return file, nil
}
