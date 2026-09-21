package vm

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/lima"
)

// Manager coordinates application operations using explicitly supplied services.
type Manager struct {
	backend Backend
	store   Store
	homes   Homes
	guest   Guest

	generations *generationBuilder
	hostUID     int

	// Warn receives non-fatal cleanup diagnostics after a successful update.
	Warn func(string, ...any)
}

// New creates a lifecycle manager without inspecting the host or creating state.
func New(deps Dependencies) *Manager {
	switch {
	case missingDependency(deps.Store):
		panic("vm: missing state store")
	case missingDependency(deps.Backend):
		panic("vm: missing backend")
	case deps.Modules == nil:
		panic("vm: missing module registry")
	case missingDependency(deps.Homes):
		panic("vm: missing managed-home service")
	case missingDependency(deps.Guest):
		panic("vm: missing guest service")
	}

	return &Manager{
		store:       deps.Store,
		backend:     deps.Backend,
		homes:       deps.Homes,
		guest:       deps.Guest,
		hostUID:     deps.HostUID,
		generations: newGenerationBuilder(deps),
		Warn:        log.New(os.Stderr, "limanix: warning: ", 0).Printf,
	}
}

func (m *Manager) preflight(ctx context.Context, cfg config.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if m.hostUID == 0 {
		return ErrRootUser
	}

	if err := m.generations.modules.Check(ctx, cfg.NixOS.Modules); err != nil {
		return err
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

func (m *Manager) recordFailure(instance domain.Instance, cause error) error {
	path, err := m.store.RecordPath(instance.Identity.Name)
	if err != nil {
		return errors.Join(cause, err)
	}

	message := fmt.Sprintf("Operation failed. VM state file: '%s'. Fix the reported issue before retrying update or delete.", path)
	instance.MarkFailed(message)

	return errors.Join(cause, m.store.Save(instance))
}

func (m *Manager) requireLima(ctx context.Context, identity domain.Identity) (lima.Instance, error) {
	instances, err := m.backend.FetchAll(ctx)
	if err != nil {
		return lima.Instance{}, err
	}

	for _, instance := range instances {
		if instance.Name == identity.LimaName() {
			return instance, nil
		}
	}

	return lima.Instance{}, &InstanceError{
		Name:  identity.Name,
		Cause: ErrBackendMissing,
	}
}

func (m *Manager) applyGuest(ctx context.Context, instance domain.Instance) error {
	name := instance.Identity.LimaName()
	if err := m.backend.Start(ctx, name); err != nil {
		return err
	}

	return m.guest.Apply(ctx, name, instance.Identity.Username)
}
