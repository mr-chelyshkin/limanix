package filesystem

import (
	"errors"
	"fmt"
)

var (
	ErrSymlinkLimit     = errors.New("too many symbolic links")
	ErrNotDirectory     = errors.New("path is not a directory")
	ErrDirectorySymlink = errors.New("directory is a symbolic link")
	ErrNotRegular       = errors.New("path is not a regular file")
	ErrInvalidUTF8      = errors.New("text is not valid UTF-8")
)

// Error reports a filesystem operation together with its destination.
type Error struct {
	Path      string
	Operation string
	Err       error
}

// Error includes the operation, path and underlying diagnostic.
func (err *Error) Error() string {
	return fmt.Sprintf("cannot %s %q: %v", err.Operation, err.Path, err.Err)
}

// Unwrap exposes the underlying IO error.
func (err *Error) Unwrap() error {
	return err.Err
}

func wrapError(path, operation string, err error) error {
	if err == nil {
		return nil
	}

	return &Error{
		Path:      path,
		Operation: operation,
		Err:       err,
	}
}
