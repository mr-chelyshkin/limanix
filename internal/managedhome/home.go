package managedhome

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Manager creates and removes host-owned allocations. Its zero value is ready to use.
type Manager struct{}

// Create allocates a private empty home; an existing path is never adopted.
func (*Manager) Create(identity domain.Identity) (string, error) {
	root, home, err := checkedPaths(identity)
	if err != nil {
		return "", err
	}

	if err = os.MkdirAll(root, 0o700); err != nil {
		return "", createError(home, err)
	}

	if _, _, err = checkedPaths(identity); err != nil {
		return "", err
	}

	if err = os.Mkdir(home, 0o700); err != nil {
		return "", createError(home, err)
	}

	return home, nil
}

// Remove deletes the recorded allocation without trusting guest-writable markers.
func (*Manager) Remove(identity domain.Identity) error {
	_, home, err := checkedPaths(identity)
	if err != nil {
		return err
	}

	info, err := os.Lstat(home)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("cannot remove managed home %q: %w", home, err)
	case !info.IsDir():
		return fmt.Errorf("%w: %s", ErrNotDirectory, home)
	}

	if err = os.RemoveAll(home); err != nil {
		return fmt.Errorf("cannot remove managed home %q: %w", home, err)
	}

	return nil
}

func checkedPaths(identity domain.Identity) (root, home string, err error) {
	root, home, err = identity.HomePaths()
	if err != nil {
		return "", "", fmt.Errorf("cannot use managed home %q: %w", identity.Home, err)
	}

	for _, path := range []string{root, home} {
		if err = checkPath(path); err != nil {
			return "", "", err
		}
	}

	return root, home, nil
}

func checkPath(path string) error {
	resolved, err := filesystem.Resolve(path)
	if err != nil {
		return fmt.Errorf("cannot use managed home %q: %w", path, err)
	}

	if resolved != path {
		return fmt.Errorf("%w: %s", ErrRedirectedPath, path)
	}

	info, err := os.Lstat(path)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%w: %s", ErrSymlink, path)
	default:
		return nil
	}
}

func createError(home string, err error) error {
	return fmt.Errorf("cannot create managed home %q: %w; set home.root to a writable directory", home, err)
}
