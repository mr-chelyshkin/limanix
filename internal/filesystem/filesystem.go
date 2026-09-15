// Package filesystem provides checked host paths and durable atomic file writes.
package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// Error reports a filesystem operation together with its destination.
type Error struct {
	Path      string
	Operation string
	Err       error
}

func (e *Error) Error() string { return fmt.Sprintf("cannot %s %q: %v", e.Operation, e.Path, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// ExpandHome expands a leading tilde without cleaning symlinks followed by '..'.
// Named-user forms use the host user database.
func ExpandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	name, remainder, _ := strings.Cut(path[1:], string(filepath.Separator))
	var home string
	if name == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	} else {
		account, err := user.Lookup(name)
		if err != nil {
			return "", err
		}
		home = account.HomeDir
	}
	if remainder == "" {
		return home, nil
	}
	return home + string(filepath.Separator) + remainder, nil
}

// Resolve returns an absolute path, resolving existing ancestors even if its tail is absent.
func Resolve(path string) (string, error) {
	expanded, err := ExpandHome(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		current, err := os.Getwd()
		if err != nil {
			return "", err
		}
		expanded = current + string(filepath.Separator) + expanded
	}
	return resolvePath(expanded, 0)
}

// Resolve each component before interpreting '..'; lexical cleanup would lose
// the parent selected by a symlink. Missing tails remain usable for later creation.
func resolvePath(value string, symlinkCount int) (string, error) {
	if symlinkCount > 40 {
		return "", errors.New("too many symbolic links")
	}
	current := string(filepath.Separator)
	components := strings.Split(strings.TrimPrefix(value, current), current)
	for index, component := range components {
		if component == "" || component == "." {
			continue
		}
		if component == ".." {
			current = filepath.Dir(current)
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(current)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Dir(current) + string(filepath.Separator) + target
		}
		if index+1 < len(components) {
			target += string(filepath.Separator) + strings.Join(components[index+1:], string(filepath.Separator))
		}
		return resolvePath(target, symlinkCount+1)
	}
	return current, nil
}

// RequireDirectory resolves a directory path, including symlinks and a leading tilde.
func RequireDirectory(path string) (string, error) {
	resolved, err := Resolve(path)
	if err != nil {
		return "", &Error{path, "use directory", err}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", &Error{path, "use directory", err}
	}
	if !info.IsDir() {
		return "", &Error{path, "use directory", errors.New("path is not a directory")}
	}
	return resolved, nil
}

// CheckDirectory rejects existing symlinks and non-directories without creating a path.
func CheckDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("directory is a symbolic link: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	return nil
}

// OpenRegular opens a regular file without following a final symlink or blocking on a FIFO.
func OpenRegular(path string, flags int, mode fs.FileMode) (*os.File, error) {
	fd, err := unix.Open(path, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		closeErr := file.Close()
		if err == nil {
			err = errors.New("path is not a regular file")
		}
		return nil, errors.Join(err, closeErr)
	}
	return file, nil
}

// WriteTextAtomic writes valid UTF-8, returning the resolved destination.
// A zero mode preserves an existing file's mode and creates new files with 0600.
func WriteTextAtomic(path, text string, mode fs.FileMode) (string, error) {
	if !utf8.ValidString(text) {
		return "", &Error{path, "write file", errors.New("text is not valid UTF-8")}
	}
	parent, name := filepath.Split(path)
	directory, err := RequireDirectory(parent)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(directory, name)
	if err := WriteFileAtomic(destination, []byte(text), mode); err != nil {
		return "", err
	}
	return destination, nil
}

// WriteFileAtomic syncs one private temporary file, renames it, then syncs its directory.
// Failures before rename preserve old contents; a directory sync error occurs after replacement.
func WriteFileAtomic(path string, data []byte, mode fs.FileMode) (failure error) {
	parent, name := filepath.Split(path)
	directory, err := RequireDirectory(parent)
	if err != nil {
		return err
	}
	path = filepath.Join(directory, name)
	existing, err := OpenRegular(path, unix.O_WRONLY, 0)
	if err == nil {
		info, statErr := existing.Stat()
		closeErr := existing.Close()
		if statErr != nil || closeErr != nil {
			return &Error{path, "write file", errors.Join(statErr, closeErr)}
		}
		if mode == 0 {
			mode = info.Mode().Perm()
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return &Error{path, "write file", err}
	}
	if mode == 0 {
		mode = 0o600
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return &Error{path, "write file", err}
	}
	tempPath := temporary.Name()
	defer func() {
		if temporary != nil {
			failure = errors.Join(failure, temporary.Close())
		}
		if tempPath != "" {
			if err := os.Remove(tempPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				failure = errors.Join(failure, &Error{tempPath, "remove temporary file", err})
			}
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return &Error{path, "write file", err}
	}
	if _, err := temporary.Write(data); err != nil {
		return &Error{path, "write file", err}
	}
	if err := temporary.Sync(); err != nil {
		return &Error{path, "write file", err}
	}
	if err := temporary.Close(); err != nil {
		temporary = nil
		return &Error{path, "write file", err}
	}
	temporary = nil
	if err := os.Rename(tempPath, path); err != nil {
		return &Error{path, "write file", err}
	}
	tempPath = ""
	dir, err := os.Open(directory)
	if err != nil {
		return &Error{path, "sync directory", err}
	}
	return wrapError(path, "sync directory", errors.Join(dir.Sync(), dir.Close()))
}

func wrapError(path, operation string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{path, operation, err}
}
