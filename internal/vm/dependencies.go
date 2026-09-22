package vm

import (
	"context"
	"io"
	"reflect"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Backend is the VM lifecycle contract implemented by the Lima adapter.
type Backend interface {
	Preflight(context.Context, config.Config) error
	FetchAll(context.Context) ([]lima.Instance, error)
	Validate(context.Context, string) error
	Create(context.Context, string, string) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Delete(context.Context, string, bool) error
	Edit(context.Context, string, string) error
}

// Store owns persisted VM records, operation locks and input-generation paths.
type Store interface {
	Save(domain.Instance) error
	Load(domain.VMName) (domain.Instance, error)
	LoadIdentity(domain.VMName) (domain.Identity, error)
	FetchAll() ([]state.Entry, error)

	// InstanceLock returns a non-nil lock on success and a nil interface on failure.
	InstanceLock(context.Context, domain.VMName) (io.Closer, error)

	RequireAbsent(domain.VMName) error
	Remove(domain.VMName) error
	RecordPath(domain.VMName) (string, error)
	GenerationDir(domain.Instance) (string, error)
	PreserveHome(domain.Identity) (string, error)
	ForgetHome(domain.Identity) error
}

// Homes manages the exact home allocation described by a persisted identity.
type Homes interface {
	Create(domain.Identity) (string, error)
	Remove(domain.Identity) error
}

// Guest applies a prepared generation and opens the development user's session.
type Guest interface {
	Apply(context.Context, string, domain.Username) error
	Address(context.Context, lima.Instance) string
	Shell(context.Context, string, domain.Username, []string) (int, error)
}

// Dependencies contains the host services assembled before constructing a Manager.
type Dependencies struct {
	Modules *modules.Registry
	Backend Backend
	Store   Store
	Homes   Homes
	Guest   Guest
	HostUID int
}

func missingDependency(dependency any) bool {
	if dependency == nil {
		return true
	}

	value := reflect.ValueOf(dependency)

	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
