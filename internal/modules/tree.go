package modules

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

// CopyTree copies a complete module, preserving relative imports and rejecting symlinks/special files.
func CopyTree(source, destination string) (entry string, failure error) {
	source, err := filesystem.RequireDirectory(source)
	if err != nil {
		return "", err
	}

	destination, err = copyDestination(source, destination)
	if err != nil {
		return "", err
	}

	if err = ValidateDirectory(source); err != nil {
		return "", err
	}

	if err = os.Mkdir(destination, 0o700); err != nil {
		return "", err
	}

	defer func() {
		if failure != nil {
			failure = errors.Join(failure, os.RemoveAll(destination))
		}
	}()

	tree := sourceTree{
		source:      source,
		destination: destination,
	}

	if err = filepath.WalkDir(source, tree.copyEntry); err != nil {
		return "", err
	}

	return filepath.Join(destination, "default.nix"), nil
}

func copyDestination(source, destination string) (string, error) {
	expanded, err := filesystem.ExpandHome(destination)
	if err != nil {
		return "", err
	}

	_, err = os.Lstat(expanded)

	switch {
	case err == nil:
		return "", fmt.Errorf("%w: %s", ErrDestinationExists, expanded)
	case !errors.Is(err, fs.ErrNotExist):
		return "", err
	}

	resolved, err := filesystem.Resolve(expanded)
	if err != nil {
		return "", err
	}

	relative, err := filepath.Rel(source, resolved)
	if err != nil {
		return "", err
	}

	outside := relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
	if !outside {
		return "", ErrRecursiveCopy
	}

	return resolved, nil
}

// ValidateDirectory requires a real module directory and a regular default.nix.
func ValidateDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("cannot read module directory: %w", err)
	}

	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return ErrInvalidDirectory
	}

	entry, err := os.Lstat(filepath.Join(directory, "default.nix"))
	if err != nil {
		return fmt.Errorf("cannot read module entry point: %w", err)
	}

	if !entry.Mode().IsRegular() {
		return ErrInvalidEntry
	}

	return nil
}

// sourceTree owns the source/destination mapping for one validated copy.
type sourceTree struct {
	source      string
	destination string
}

func (tree sourceTree) copyEntry(path string, entry fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	relative, err := filepath.Rel(tree.source, path)
	if err != nil {
		return err
	}

	if relative == "." {
		return nil
	}

	info, err := entry.Info()
	if err != nil {
		return err
	}

	target := filepath.Join(tree.destination, relative)
	if info.IsDir() {
		return os.Mkdir(target, info.Mode().Perm()|0o700)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrUnsupportedFile, relative)
	}

	return copyFile(path, target, info.Mode().Perm())
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := filesystem.OpenRegular(source, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}

	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return errors.Join(err, input.Close())
	}

	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, input.Close(), output.Close())
}
