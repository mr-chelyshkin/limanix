package modules

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Add copies outside the registry lock, then commits a complete import under an exclusive lock.
func (r *Registry) Add(ctx context.Context, name domain.ModuleName, source string) (failure error) {
	if _, err := domain.NewModuleName(string(name)); err != nil {
		return err
	}

	destination := filepath.Join(r.directory(), string(name))
	if err := r.checkImportDestination(ctx, destination); err != nil {
		return err
	}

	staging, err := os.MkdirTemp(r.directory(), "."+string(name)+".import-")
	if err != nil {
		return err
	}

	defer func() {
		failure = errors.Join(failure, os.RemoveAll(staging))
	}()

	imported := filepath.Join(staging, "module")
	if _, err = CopyTree(source, imported); err != nil {
		return fmt.Errorf("cannot import module %q: %w", name, err)
	}

	if err = ctx.Err(); err != nil {
		return err
	}

	return r.commitImport(ctx, imported, destination)
}

func (r *Registry) checkImportDestination(ctx context.Context, destination string) (failure error) {
	lock, err := r.store.RegistryLock(ctx, true, state.RegistryLockTimeout)
	if err != nil {
		return err
	}

	defer func() {
		failure = errors.Join(failure, lock.Close())
	}()

	return requireAbsent(destination)
}

func (r *Registry) commitImport(ctx context.Context, imported, destination string) (failure error) {
	lock, err := r.store.RegistryLock(ctx, false, state.RegistryLockTimeout)
	if err != nil {
		return err
	}

	defer func() {
		failure = errors.Join(failure, lock.Close())
	}()

	if err = requireAbsent(destination); err != nil {
		return err
	}

	if err = os.Rename(imported, destination); err != nil {
		return fmt.Errorf("cannot commit module %q: %w", filepath.Base(destination), err)
	}

	return nil
}

func requireAbsent(path string) error {
	_, err := os.Lstat(path)

	switch {
	case err == nil:
		return importedError(filepath.Base(path), ErrAlreadyExists)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return err
	}
}
