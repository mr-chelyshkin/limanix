package modules

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Source references an embedded module by ID or a stable imported directory by Path.
type Source struct {
	ID   domain.ModuleID
	Path string
}

// SourceSet holds the registry's shared lock until all sources have been copied into a generation.
type SourceSet struct {
	Sources []Source
	lock    io.Closer
}

// Close releases source stability; imported paths must not be used after it returns.
func (sources *SourceSet) Close() error {
	return sources.lock.Close()
}

// Check performs preliminary selection checks without creating state or acquiring locks.
func (r *Registry) Check(ctx context.Context, ids []domain.ModuleID) error {
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}

		if _, err := r.source(id); err != nil {
			return err
		}
	}

	return ctx.Err()
}

// Sources holds a shared registry lock while resolving selected source trees.
func (r *Registry) Sources(ctx context.Context, ids []domain.ModuleID) (*SourceSet, error) {
	lock, err := r.store.RegistryLock(ctx, true, state.RegistryLockTimeout)
	if err != nil {
		return nil, err
	}

	result := &SourceSet{
		Sources: make([]Source, 0, len(ids)),
		lock:    lock,
	}

	for _, id := range ids {
		source, err := r.source(id)
		if err != nil {
			return nil, errors.Join(err, result.Close())
		}

		result.Sources = append(result.Sources, source)
	}

	return result, nil
}

func (r *Registry) source(selected domain.ModuleID) (Source, error) {
	id, err := domain.NewModuleID(string(selected))
	if err != nil {
		return Source{}, err
	}

	source := Source{ID: id}
	if id.Namespace() == "lmx" {
		if _, exists := r.system[id.Selector()]; !exists {
			return Source{}, &Error{
				ID:  id,
				Err: ErrUnknownSystem,
			}
		}

		return source, nil
	}

	if !id.IsThirdParty() {
		return Source{}, &Error{ID: id, Err: ErrUnknownCatalog}
	}

	name, err := domain.NewModuleName(id.Selector())
	if err != nil {
		return Source{}, &Error{ID: id, Err: err}
	}

	source.Path = filepath.Join(r.directory(), string(name))
	if err = ValidateDirectory(source.Path); err != nil {
		return Source{}, &Error{
			ID:  id,
			Err: err,
		}
	}

	return source, nil
}
