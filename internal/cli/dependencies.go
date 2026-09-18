package cli

import (
	"context"
	"io"

	"github.com/mr-chelyshkin/limanix/internal/app"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/vm"
)

// IO allows commands to inherit the terminal or use explicit streams in tests.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Manager is the VM surface consumed by the CLI.
type Manager interface {
	Create(context.Context, string) (domain.Instance, error)
	Update(context.Context, string) (domain.Instance, error)
	FetchAll(context.Context) ([]vm.Info, error)
	Start(context.Context, domain.VMName) error
	Stop(context.Context, domain.VMName) error
	Delete(context.Context, domain.VMName, bool, bool) (string, error)
	Shell(context.Context, domain.VMName, []string) (int, error)
}

// Registry is the module surface consumed by the CLI.
type Registry interface {
	Available(context.Context) ([]modules.Info, error)
	Add(context.Context, domain.ModuleName, string) error
	Remove(context.Context, domain.ModuleName) error
}

// Dependencies creates host services lazily; help and documentation need none.
type Dependencies struct {
	Manager  func() (Manager, error)
	Registry func() (Registry, error)
}

func withDefaultDependencies(streams IO, dependencies Dependencies) Dependencies {
	services := app.New(streams.In, streams.Out, streams.Err)

	if dependencies.Manager == nil {
		dependencies.Manager = func() (Manager, error) {
			return services.Manager()
		}
	}

	if dependencies.Registry == nil {
		dependencies.Registry = func() (Registry, error) {
			return services.Registry()
		}
	}

	return dependencies
}
