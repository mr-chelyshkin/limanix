// Package managedhome manages exact home allocations using ownership stored outside the guest.
package managedhome

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

func checkedPaths(identity state.Identity) (root, home string, err error) {
	root, home, err = identity.HomePaths()
	if err != nil {
		return "", "", fmt.Errorf("cannot use managed home %q: %w", identity.Home, err)
	}
	for _, path := range []string{root, home} {
		resolved, err := filesystem.Resolve(path)
		if err != nil {
			return "", "", fmt.Errorf("cannot use managed home %q: %w", path, err)
		}
		if resolved != path {
			return "", "", fmt.Errorf("managed-home path redirects through a symbolic link: %s", path)
		}
		if info, err := os.Lstat(path); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			return "", "", fmt.Errorf("managed-home path is a symbolic link: %s", path)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
	}
	return root, home, nil
}

// Create allocates a private empty home; an existing path is never adopted.
func Create(identity state.Identity) (string, error) {
	root, home, err := checkedPaths(identity)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("cannot create managed home %q: %w; set home.root to a writable directory", home, err)
	}
	if _, _, err := checkedPaths(identity); err != nil {
		return "", err
	}
	if err := os.Mkdir(home, 0o700); err != nil {
		return "", fmt.Errorf("cannot create managed home %q: %w; set home.root to a writable directory", home, err)
	}
	return home, nil
}

// Remove deletes the recorded allocation without trusting guest-writable markers.
// Symlinks inside the home are removed as links rather than followed.
func Remove(identity state.Identity) error {
	_, home, err := checkedPaths(identity)
	if err != nil {
		return err
	}
	info, err := os.Lstat(home)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot remove managed home %q: %w", home, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("managed home is not a directory: %s", home)
	}
	if err := os.RemoveAll(home); err != nil {
		return fmt.Errorf("cannot remove managed home %q: %w", home, err)
	}
	return nil
}
