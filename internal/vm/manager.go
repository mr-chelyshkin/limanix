// Package vm coordinates host ownership, Lima, and guest configuration changes.
package vm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/guest"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/managedhome"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Backend provides the Lima operations needed by VM management and guest access.
type Backend interface {
	Preflight(context.Context, config.Config) error
	FetchAll(context.Context) ([]lima.Instance, error)
	Validate(context.Context, string) error
	Create(context.Context, string, string) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Delete(context.Context, string, bool) error
	Edit(context.Context, string, string) error
	Run(context.Context, string, []string, bool) (string, error)
	Shell(context.Context, string, []string) (int, error)
}

type store interface {
	InstanceDir(domain.VMName) (string, error)
	Save(state.Instance) error
	Load(domain.VMName) (state.Instance, error)
	LoadIdentity(domain.VMName) (state.Identity, error)
	FetchAll() ([]state.Entry, error)
	InstanceLock(context.Context, domain.VMName) (*state.Lock, error)
	PreserveHome(state.Identity) (string, error)
	ForgetHome(state.Identity) error
}

type guestOperations interface {
	Apply(context.Context, string, domain.Username) error
	Address(context.Context, lima.Instance) string
	Shell(context.Context, string, domain.Username, []string) (int, error)
}

// Info combines an independent host record with the current backend status.
// Damaged records remain visible with their own error instead of hiding other VMs.
type Info struct {
	Name            string               `json:"name"`
	Address         string               `json:"address"`
	Home            *string              `json:"home"`
	OperationStatus *state.Status        `json:"state"`
	Arch            *domain.Architecture `json:"arch"`
	BackendStatus   *lima.Status         `json:"status"`
	LimaName        *string              `json:"lima_name"`
	Error           *string              `json:"error"`
}

// Manager sequences operations while the state store locks each VM separately.
// The module registry lock is held only while assembling the input generation.
type Manager struct {
	store      store
	backend    Backend
	registry   *modules.Registry
	guest      guestOperations
	createHome func(state.Identity) (string, error)
	removeHome func(state.Identity) error
	getUID     func() int
	newID      func() (string, error)
	now        func() time.Time
	removeAll  func(string) error
	readDir    func(string) ([]os.DirEntry, error)
	Warn       func(string, ...any)
}

// New creates a manager with host services and an injectable Lima backend.
func New(s *state.Store, backend Backend) *Manager {
	return &Manager{
		store:      s,
		backend:    backend,
		registry:   modules.NewRegistry(s, nixos.BuiltinModules()),
		guest:      guest.New(backend),
		createHome: managedhome.Create,
		removeHome: managedhome.Remove,
		getUID:     os.Getuid,
		newID:      randomID,
		now:        time.Now,
		removeAll:  os.RemoveAll,
		readDir:    os.ReadDir,
		Warn:       log.New(os.Stderr, "limanix: warning: ", 0).Printf,
	}
}

func randomID() (string, error) {
	var token [state.IDLength / 2]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate VM identifier: %w", err)
	}
	return hex.EncodeToString(token[:]), nil
}

func (m *Manager) preflight(ctx context.Context, cfg config.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.getUID() == 0 {
		return errors.New("run Limanix as your regular host user, not root")
	}
	if err := m.backend.Preflight(ctx, cfg); err != nil {
		return err
	}
	for _, mount := range cfg.Mounts {
		if _, err := filesystem.RequireDirectory(mount.Source); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) recordFailure(instance state.Instance, cause error) error {
	instance.Status = state.Error
	directory, err := m.store.InstanceDir(instance.Identity.Name)
	if err != nil {
		return errors.Join(cause, err)
	}
	message := fmt.Sprintf("Operation failed. VM state file: '%s'. Fix the reported issue before retrying update or delete.", filepath.Join(directory, "instance.json"))
	instance.Error = &message
	return errors.Join(cause, m.store.Save(instance))
}

func (m *Manager) requireLima(ctx context.Context, identity state.Identity) (lima.Instance, error) {
	instances, err := m.backend.FetchAll(ctx)
	if err != nil {
		return lima.Instance{}, err
	}
	for _, instance := range instances {
		if instance.Name == identity.LimaName() {
			return instance, nil
		}
	}
	return lima.Instance{}, fmt.Errorf("lima instance for '%s' is missing; delete its saved record", identity.Name)
}
