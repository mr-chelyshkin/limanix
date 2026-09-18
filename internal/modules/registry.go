package modules

import (
	"maps"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Registry keeps copied third-party source trees separate from their original checkout.
type Registry struct {
	store    *state.Store
	builtins map[string]string
}

// NewRegistry takes its own copy of the embedded-module metadata supplied by NixOS.
func NewRegistry(store *state.Store, builtins map[string]string) *Registry {
	return &Registry{
		store:    store,
		builtins: maps.Clone(builtins),
	}
}

func (r *Registry) directory() string {
	return filepath.Join(r.store.Root(), "modules")
}
