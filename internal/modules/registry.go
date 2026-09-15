// Package modules imports trusted NixOS modules and provides stable sources for VM generations.
package modules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/state"
	"golang.org/x/sys/unix"
)

// Info is a catalog row, including an error for a damaged imported module.
type Info struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	Description string  `json:"description"`
	Error       *string `json:"error"`
}

// Source references an embedded module by ID or a stable imported directory by Path.
type Source struct {
	ID   domain.ModuleID
	Path string
}

// SourceSet holds the registry's shared lock until all sources have been copied into a generation.
type SourceSet struct {
	Sources []Source
	lock    *state.Lock
}

// Close releases source stability; imported paths must not be used after it returns.
func (s *SourceSet) Close() error { return s.lock.Close() }

// Registry keeps copied third-party source trees separate from their original checkout.
type Registry struct {
	store    *state.Store
	builtins map[string]string
}

// NewRegistry uses embedded-module metadata supplied by the NixOS package.
func NewRegistry(store *state.Store, builtins map[string]string) *Registry {
	metadata := make(map[string]string, len(builtins))
	maps.Copy(metadata, builtins)
	return &Registry{store: store, builtins: metadata}
}

// Available returns bundled modules and independently reports invalid catalog entries.
func (r *Registry) Available(ctx context.Context) (entries []Info, failure error) {
	for name, description := range r.builtins {
		entries = append(entries, Info{Name: name, Source: "bundled", Description: description})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	lock, err := r.store.RegistryLock(ctx, true, state.RegistryLockTimeout)
	if err != nil {
		return nil, err
	}
	defer func() { failure = errors.Join(failure, lock.Close()) }()
	directory := filepath.Join(r.store.Root(), "modules")
	paths, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("cannot list imported modules: %w", err)
	}
	for _, path := range paths {
		if strings.HasPrefix(path.Name(), ".") {
			continue
		}
		entry := Info{Name: "third-party:" + path.Name(), Source: "third-party", Description: "Locally imported NixOS module"}
		_, err := domain.NewModuleName(path.Name())
		if err == nil {
			err = validateEntry(filepath.Join(directory, path.Name()))
		}
		if err != nil {
			message := err.Error()
			entry.Error = &message
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Add copies outside the registry lock, then commits a complete import under an exclusive lock.
func (r *Registry) Add(ctx context.Context, name domain.ModuleName, source string) (failure error) {
	if _, err := domain.NewModuleName(string(name)); err != nil {
		return err
	}
	directory := filepath.Join(r.store.Root(), "modules")
	destination := filepath.Join(directory, string(name))
	if err := r.checkImportDestination(ctx, destination); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(directory, "."+string(name)+".import-")
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, os.RemoveAll(staging)) }()
	imported := filepath.Join(staging, "module")
	if _, err := CopyTree(source, imported); err != nil {
		return fmt.Errorf("cannot import module %q: %w", name, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lock, err := r.store.RegistryLock(ctx, false, state.RegistryLockTimeout)
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, lock.Close()) }()
	if err := requireAbsent(destination); err != nil {
		return err
	}
	if err := os.Rename(imported, destination); err != nil {
		return fmt.Errorf("cannot commit module %q: %w", name, err)
	}
	return nil
}

// checkImportDestination checks absence under a shared lock before preparing an import.
func (r *Registry) checkImportDestination(ctx context.Context, destination string) (failure error) {
	lock, err := r.store.RegistryLock(ctx, true, state.RegistryLockTimeout)
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, lock.Close()) }()
	return requireAbsent(destination)
}

func requireAbsent(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return fmt.Errorf("module 'third-party:%s' already exists; remove it before importing a replacement", filepath.Base(path))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Remove atomically detaches an imported module under an exclusive lock, then removes its files.
func (r *Registry) Remove(ctx context.Context, name domain.ModuleName) (failure error) {
	if _, err := domain.NewModuleName(string(name)); err != nil {
		return err
	}
	lock, err := r.store.RegistryLock(ctx, false, state.RegistryLockTimeout)
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, lock.Close()) }()
	directory := filepath.Join(r.store.Root(), "modules")
	destination := filepath.Join(directory, string(name))
	info, err := os.Lstat(destination)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("module 'third-party:%s' is not installed", name)
	}
	staging, err := os.MkdirTemp(directory, "."+string(name)+".remove-")
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, os.RemoveAll(staging)) }()
	if err := os.Rename(destination, filepath.Join(staging, "module")); err != nil {
		return err
	}
	// Readers need only wait for detachment, not a potentially large recursive removal.
	if err := lock.Close(); err != nil {
		return err
	}
	return nil
}

// Sources holds a shared registry lock while resolving selected source trees.
func (r *Registry) Sources(ctx context.Context, ids []domain.ModuleID) (*SourceSet, error) {
	lock, err := r.store.RegistryLock(ctx, true, state.RegistryLockTimeout)
	if err != nil {
		return nil, err
	}
	result := &SourceSet{Sources: make([]Source, 0, len(ids)), lock: lock}
	for _, selectedID := range ids {
		id, err := domain.NewModuleID(string(selectedID))
		if err != nil {
			return nil, errors.Join(err, lock.Close())
		}
		source := Source{ID: id}
		if id.IsThirdParty() {
			source.Path = filepath.Join(r.store.Root(), "modules", string(id.Name()))
			if err := validateEntry(source.Path); err != nil {
				return nil, errors.Join(fmt.Errorf("module %q: %w", id, err), lock.Close())
			}
		} else if _, exists := r.builtins[string(id)]; !exists {
			return nil, errors.Join(fmt.Errorf("unknown bundled module %q; use limanix modules list", id), lock.Close())
		}
		result.Sources = append(result.Sources, source)
	}
	return result, nil
}

func validateEntry(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("cannot read module directory: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("module must be a directory, not a symbolic link")
	}
	entry, err := os.Lstat(filepath.Join(directory, "default.nix"))
	if err != nil {
		return fmt.Errorf("cannot read module entry point: %w", err)
	}
	if !entry.Mode().IsRegular() {
		return errors.New("module entry point must be a regular default.nix file")
	}
	return nil
}

// CopyTree copies a complete module, preserving relative imports and rejecting symlinks/special files.
// The destination must not exist and cannot be inside the source directory.
func CopyTree(source, destination string) (entry string, failure error) {
	resolved, err := filesystem.RequireDirectory(source)
	if err != nil {
		return "", err
	}
	destination, err = filesystem.ExpandHome(destination)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(destination); err == nil {
		return "", fmt.Errorf("module destination already exists: %s", destination)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	destination, err = filesystem.Resolve(destination)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(resolved, destination)
	if err != nil {
		return "", err
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return "", errors.New("a module cannot be copied into its own source directory")
	}
	if err := validateEntry(resolved); err != nil {
		return "", err
	}
	if err := filepath.WalkDir(resolved, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			return fmt.Errorf("unsupported file or symlink in module: %s", path)
		}
		return nil
	}); err != nil {
		return "", err
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return "", err
	}
	defer func() {
		if failure != nil {
			failure = errors.Join(failure, os.RemoveAll(destination))
		}
	}()
	err = filepath.WalkDir(resolved, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(resolved, path)
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
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.Mkdir(target, info.Mode().Perm()|0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file or symlink in module: %s", relative)
		}
		input, err := filesystem.OpenRegular(path, unix.O_RDONLY, 0)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return errors.Join(err, input.Close())
		}
		_, copyErr := io.Copy(output, input)
		return errors.Join(copyErr, input.Close(), output.Close())
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(destination, "default.nix"), nil
}
