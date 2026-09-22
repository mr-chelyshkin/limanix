package vm

import (
	"context"
	"errors"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
)

// Start starts the backend independently of the mutable runtime record.
func (m *Manager) Start(ctx context.Context, name domain.VMName) error {
	return m.lifecycle(ctx, name, m.backend.Start)
}

// Stop preserves the guest disk and host home.
func (m *Manager) Stop(ctx context.Context, name domain.VMName) error {
	return m.lifecycle(ctx, name, m.backend.Stop)
}

func (m *Manager) lifecycle(ctx context.Context, name domain.VMName, operation func(context.Context, string) error) (err error) {
	lock, err := m.store.InstanceLock(ctx, name)
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, lock.Close())
	}()

	identity, err := m.store.LoadIdentity(name)
	if err != nil {
		return err
	}

	if _, err = m.requireLima(ctx, identity); err != nil {
		return err
	}

	return operation(ctx, identity.LimaName())
}

// Shell passes command arguments unchanged to the configured development user.
func (m *Manager) Shell(ctx context.Context, name domain.VMName, command []string) (int, error) {
	identity, err := m.store.LoadIdentity(name)
	if err != nil {
		return 1, err
	}

	actual, err := m.requireLima(ctx, identity)
	if err != nil {
		return 1, err
	}

	if actual.Status != lima.Running {
		return 1, &InstanceError{
			Name:  name,
			Cause: ErrNotRunning,
		}
	}

	return m.guest.Shell(ctx, identity.LimaName(), identity.Username, command)
}
