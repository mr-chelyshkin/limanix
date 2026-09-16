package modules

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Remove atomically detaches an imported module under an exclusive lock, then removes its files.
func (r *Registry) Remove(ctx context.Context, name domain.ModuleName) error {
	if _, err := domain.NewModuleName(string(name)); err != nil {
		return err
	}

	staging, err := r.detach(ctx, name)
	if staging == "" {
		return err
	}

	return errors.Join(err, os.RemoveAll(staging))
}

func (r *Registry) detach(ctx context.Context, name domain.ModuleName) (staging string, failure error) {
	lock, err := r.store.RegistryLock(ctx, false, state.RegistryLockTimeout)
	if err != nil {
		return "", err
	}

	defer func() {
		failure = errors.Join(failure, lock.Close())
	}()

	destination := filepath.Join(r.directory(), string(name))

	info, err := os.Lstat(destination)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", importedError(string(name), ErrNotInstalled)
	case err != nil:
		return "", importedError(string(name), err)
	case !info.IsDir() || info.Mode()&fs.ModeSymlink != 0:
		return "", importedError(string(name), ErrNotInstalled)
	}

	staging, err = os.MkdirTemp(r.directory(), "."+string(name)+".remove-")
	if err != nil {
		return "", err
	}

	err = os.Rename(destination, filepath.Join(staging, "module"))
	return staging, err
}
