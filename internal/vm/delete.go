package vm

import (
	"context"
	"errors"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
)

// Delete controls Lima force and owned-home removal independently.
func (m *Manager) Delete(ctx context.Context, name domain.VMName, force, removeHome bool) (home string, err error) {
	lock, err := m.store.InstanceLock(ctx, name)
	if err != nil {
		return "", err
	}

	defer func() {
		err = errors.Join(err, lock.Close())
	}()

	identity, err := m.store.LoadIdentity(name)
	if err != nil {
		return "", err
	}

	record, recordErr := m.store.Load(name)
	if recordErr == nil {
		record.MarkDeleting()
		if err = m.store.Save(record); err != nil {
			return "", err
		}
	}

	if err = m.deleteResources(ctx, identity, force, removeHome); err != nil {
		if recordErr == nil {
			err = m.recordFailure(record, err)
		}

		return identity.Home, err
	}

	return identity.Home, nil
}

func (m *Manager) deleteResources(ctx context.Context, identity domain.Identity, force, removeHome bool) error {
	if err := m.deleteBackend(ctx, identity.LimaName(), force); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := m.releaseHome(identity, removeHome); err != nil {
		return err
	}

	return m.store.Remove(identity.Name)
}

func (m *Manager) deleteBackend(ctx context.Context, name string, force bool) error {
	instances, err := m.backend.FetchAll(ctx)
	if err != nil {
		return err
	}

	for _, instance := range instances {
		if instance.Name != name {
			continue
		}

		if instance.Status == lima.Running && !force {
			if err = m.backend.Stop(ctx, name); err != nil {
				return err
			}
		}

		return m.backend.Delete(ctx, name, force)
	}

	return nil
}

func (m *Manager) releaseHome(identity domain.Identity, remove bool) error {
	if !remove {
		_, err := m.store.PreserveHome(identity)
		return err
	}

	if err := m.homes.Remove(identity); err != nil {
		return err
	}

	return m.store.ForgetHome(identity)
}
