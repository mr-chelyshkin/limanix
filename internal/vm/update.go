package vm

import (
	"context"
	"errors"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
)

// Update retains identity, disk, and managed home. Each attempt stages a new
// generation; only a successful commit permits best-effort removal of old inputs.
func (m *Manager) Update(ctx context.Context, path string) (result domain.Instance, err error) {
	cfg, err := config.Load(path)
	if err != nil {
		return result, err
	}

	if err = m.preflight(ctx, cfg); err != nil {
		return result, err
	}

	lock, err := m.store.InstanceLock(ctx, cfg.Name)
	if err != nil {
		return result, err
	}

	defer func() {
		err = errors.Join(err, lock.Close())
	}()

	result, err = m.store.Load(cfg.Name)
	if err != nil {
		return result, err
	}

	if err = validateIdentityUpdate(cfg, result.Identity); err != nil {
		return result, err
	}

	if _, err = m.checkUpdateBackend(ctx, result.Identity, cfg.Resources.Disk); err != nil {
		return result, err
	}

	generation, err := randomID()
	if err != nil {
		return result, err
	}

	result.BeginUpdate(generation)

	template, err := m.prepareUpdate(ctx, result, cfg)
	if err != nil {
		return result, err
	}

	if err = m.applyUpdate(ctx, result, cfg.Resources.Disk, template); err != nil {
		return result, m.recordFailure(result, err)
	}

	result.MarkReady()
	if err = m.store.Save(result); err != nil {
		return result, err
	}

	for _, warning := range m.generations.prune(result) {
		m.Warn("%v", warning)
	}

	return result, nil
}

// prepareUpdate retains a generation when Save may already have committed it.
func (m *Manager) prepareUpdate(ctx context.Context, instance domain.Instance, cfg config.Config) (template string, failure error) {
	defer func() {
		if failure != nil {
			failure = errors.Join(failure, m.discardUncommitted(instance))
		}
	}()

	template, err := m.generations.prepare(ctx, instance, cfg)
	if err != nil {
		return "", err
	}

	if err := m.store.Save(instance); err != nil {
		return "", err
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	return template, nil
}

func (m *Manager) discardUncommitted(instance domain.Instance) error {
	saved, err := m.store.Load(instance.Identity.Name)
	if err != nil {
		return err
	}

	if saved.Generation == instance.Generation {
		return nil
	}

	return m.generations.discard(instance)
}

// applyUpdate rechecks live disk and power state after preparation.
// An external Lima operation may have changed either while inputs were staged.
func (m *Manager) applyUpdate(ctx context.Context, instance domain.Instance, disk domain.ByteSize, template string) error {
	actual, err := m.checkUpdateBackend(ctx, instance.Identity, disk)
	if err != nil {
		return err
	}

	name := instance.Identity.LimaName()
	if actual.Status == lima.Running {
		if err = m.backend.Stop(ctx, name); err != nil {
			return err
		}
	}

	if err = m.backend.Edit(ctx, name, template); err != nil {
		return err
	}

	return m.applyGuest(ctx, instance)
}

func (m *Manager) checkUpdateBackend(ctx context.Context, identity domain.Identity, disk domain.ByteSize) (lima.Instance, error) {
	actual, err := m.requireLima(ctx, identity)
	if err != nil {
		return lima.Instance{}, err
	}

	return actual, validateDiskUpdate(disk, actual.Disk)
}

func validateIdentityUpdate(cfg config.Config, identity domain.Identity) error {
	switch {
	case cfg.Resources.Arch != identity.Arch:
		return ErrIdentityChange
	case cfg.User.Name != identity.Username:
		return ErrIdentityChange
	case cfg.User.Home != identity.UserHome:
		return ErrIdentityChange
	case cfg.Home.Root != identity.HomeRoot:
		return ErrIdentityChange
	default:
		return nil
	}
}

func validateDiskUpdate(requested domain.ByteSize, reported *int64) error {
	if reported == nil {
		return ErrDiskUnknown
	}

	if int64(requested) < *reported {
		return ErrDiskShrink
	}

	return nil
}
