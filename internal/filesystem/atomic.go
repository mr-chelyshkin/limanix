package filesystem

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// WriteTextAtomic writes valid UTF-8, returning the resolved destination.
func WriteTextAtomic(path, text string, mode fs.FileMode) (string, error) {
	if !utf8.ValidString(text) {
		return "", wrapError(path, "write file", ErrInvalidUTF8)
	}

	destination, err := writeDestination(path)
	if err != nil {
		return "", err
	}

	if err = WriteFileAtomic(destination, []byte(text), mode); err != nil {
		return "", err
	}

	return destination, nil
}

// WriteFileAtomic syncs one private temporary file, renames it, then syncs its directory.
func WriteFileAtomic(path string, data []byte, mode fs.FileMode) (failure error) {
	destination, err := writeDestination(path)
	if err != nil {
		return err
	}

	mode, err = destinationMode(destination, mode)
	if err != nil {
		return wrapError(destination, "write file", err)
	}

	var (
		directory = filepath.Dir(destination)
		pattern   = "." + filepath.Base(destination) + "-*.tmp"
	)

	temporary, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return wrapError(destination, "write file", err)
	}

	defer func() {
		err = os.Remove(temporary.Name())
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			failure = errors.Join(failure, wrapError(temporary.Name(), "remove temporary file", err))
		}
	}()

	if err = writeTemporary(temporary, data, mode); err != nil {
		return wrapError(destination, "write file", err)
	}

	if err = os.Rename(temporary.Name(), destination); err != nil {
		return wrapError(destination, "write file", err)
	}

	return wrapError(destination, "sync directory", syncDirectory(directory))
}

func writeDestination(path string) (string, error) {
	parent, name := filepath.Split(path)

	directory, err := RequireDirectory(parent)
	if err != nil {
		return "", err
	}

	return filepath.Join(directory, name), nil
}

func destinationMode(path string, requested fs.FileMode) (fs.FileMode, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if requested == 0 {
			return 0o600, nil
		}

		return requested, nil
	}

	if err != nil {
		return 0, err
	}

	if !info.Mode().IsRegular() {
		return 0, ErrNotRegular
	}

	if requested != 0 {
		return requested, nil
	}

	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o600
	}

	return mode, nil
}

func writeTemporary(file *os.File, data []byte, mode fs.FileMode) (failure error) {
	defer func() {
		failure = errors.Join(failure, file.Close())
	}()

	if err := file.Chmod(mode); err != nil {
		return err
	}

	if _, err := file.Write(data); err != nil {
		return err
	}

	return file.Sync()
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}

	return errors.Join(directory.Sync(), directory.Close())
}
