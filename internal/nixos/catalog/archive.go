package catalog

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

func validateArchive(archive *zip.Reader) error {
	entries := make(map[string]fs.FileMode, len(archive.File))

	for _, file := range archive.File {
		name := strings.TrimSuffix(file.Name, "/")
		mode := file.Mode()

		switch {
		case !fs.ValidPath(name) || name == "." || strings.Contains(name, `\`):
			return fmt.Errorf("%w: unsafe path %q", ErrArchive, file.Name)
		case !mode.IsRegular() && !mode.IsDir():
			return fmt.Errorf("%w: unsupported file %q", ErrArchive, file.Name)
		}

		if _, exists := entries[name]; exists {
			return fmt.Errorf("%w: duplicate path %q", ErrArchive, name)
		}
		entries[name] = mode

		if mode.IsRegular() {
			if err := checkFile(file); err != nil {
				return fmt.Errorf("%w: %s: %w", ErrArchive, name, err)
			}
		}
	}

	for name := range entries {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if mode, exists := entries[parent]; exists && !mode.IsDir() {
				return fmt.Errorf("%w: file %q is also used as a directory", ErrArchive, parent)
			}
		}
	}

	return nil
}

func checkFile(file *zip.File) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}

	_, err = io.Copy(io.Discard, reader)
	return errors.Join(err, reader.Close())
}
