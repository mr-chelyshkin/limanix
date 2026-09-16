package app

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/mr-chelyshkin/limanix/internal/bundle"
	"github.com/mr-chelyshkin/limanix/internal/guest"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/managedhome"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
	"github.com/mr-chelyshkin/limanix/internal/state"
	"github.com/mr-chelyshkin/limanix/internal/vm"
)

// Services is the lazy composition root for one CLI invocation.
// Its zero value is not usable; construct Services with New.
type Services struct {
	input       io.Reader
	output      io.Writer
	diagnostics io.Writer
	store       func() (*state.Store, error)
}

// New retains streams without initializing host services.
func New(input io.Reader, output, diagnostics io.Writer) *Services {
	return &Services{
		input:       input,
		output:      output,
		diagnostics: diagnostics,
		store: sync.OnceValues(func() (*state.Store, error) {
			return state.NewStore("")
		}),
	}
}

// Manager assembles VM lifecycle services after checking the native architecture.
// Returned errors describe host compatibility or state-path resolution failures,
// not programming errors in the service wiring passed to vm.New.
func (s *Services) Manager() (*vm.Manager, error) {
	host, err := lima.HostArchitecture()
	if err != nil {
		return nil, err
	}

	if err = lima.RequireNativeArchitecture(host); err != nil {
		return nil, err
	}

	store, err := s.store()
	if err != nil {
		return nil, err
	}

	var (
		registry = modules.NewRegistry(store, nixos.BuiltinModules())
		agents   = bundle.New(filepath.Join(store.Root(), "runtime", "guestagents"))
		backend  = lima.NewClient(agents.Path)
	)

	backend.Stdin = s.input
	backend.Stdout = s.output
	backend.Stderr = s.diagnostics

	manager := vm.New(vm.Dependencies{
		Store:   store,
		Backend: backend,
		Modules: registry,
		Homes:   &managedhome.Manager{},
		Guest:   guest.New(backend),
		HostUID: os.Getuid(),
	})
	manager.Warn = log.New(s.diagnostics, "limanix: warning: ", 0).Printf

	return manager, nil
}

// Registry opens module services without initializing Lima or guest access.
func (s *Services) Registry() (*modules.Registry, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}

	return modules.NewRegistry(store, nixos.BuiltinModules()), nil
}
