package modules

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Info is a catalog row, including an error for a damaged imported module.
type Info struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	Description string  `json:"description"`
	Error       *string `json:"error"`
}

// Available returns bundled modules and independently reports invalid catalog entries.
func (r *Registry) Available(ctx context.Context) (entries []Info, failure error) {
	for name, description := range r.system {
		entries = append(entries, Info{
			Name:        "lmx:" + name,
			Source:      "lmx",
			Description: description,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	lock, err := r.store.RegistryLock(ctx, true, state.RegistryLockTimeout)
	if err != nil {
		return nil, err
	}

	defer func() {
		failure = errors.Join(failure, lock.Close())
	}()

	paths, err := os.ReadDir(r.directory())
	if err != nil {
		return nil, fmt.Errorf("cannot list imported modules: %w", err)
	}

	for _, path := range paths {
		if strings.HasPrefix(path.Name(), ".") {
			continue
		}

		entries = append(entries, r.importedInfo(path.Name()))
	}

	return entries, nil
}

func (r *Registry) importedInfo(name string) Info {
	entry := Info{
		Name:        "third-party:" + name,
		Source:      "third-party",
		Description: "Locally imported NixOS module",
	}

	if err := r.validateImport(name); err != nil {
		entry.Error = new(err.Error())
	}

	return entry
}

func (r *Registry) validateImport(name string) error {
	if _, err := domain.NewModuleName(name); err != nil {
		return err
	}

	return ValidateDirectory(filepath.Join(r.directory(), name))
}
