package vm

import (
	"context"
	"errors"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Delete controls Lima force and owned-home removal independently. An intact
// identity suffices even when instance.json is damaged. Home preservation is
// recorded before state removal, so an archival failure can be retried safely.
func (m *Manager) Delete(ctx context.Context, name domain.VMName, force, removeHome bool) (home string, err error) {
	lock, err := m.store.InstanceLock(ctx, name)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	identity, err := m.store.LoadIdentity(name)
	if err != nil {
		return "", err
	}
	record, recordErr := m.store.Load(name)
	if recordErr == nil {
		record.Status = state.Deleting
		if err := m.store.Save(record); err != nil {
			return "", err
		}
	}
	if operationErr := m.deleteResources(ctx, identity, force, removeHome); operationErr != nil {
		if recordErr == nil {
			operationErr = m.recordFailure(record, operationErr)
		}
		return identity.Home, operationErr
	}
	return identity.Home, nil
}

func (m *Manager) deleteResources(ctx context.Context, identity state.Identity, force, removeHome bool) error {
	instances, err := m.backend.FetchAll(ctx)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		if instance.Name != identity.LimaName() {
			continue
		}
		if instance.Status == lima.Running && !force {
			if err := m.backend.Stop(ctx, instance.Name); err != nil {
				return err
			}
		}
		if err := m.backend.Delete(ctx, instance.Name, force); err != nil {
			return err
		}
		break
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if removeHome {
		if err := m.removeHome(identity); err != nil {
			return err
		}
		if err := m.store.ForgetHome(identity); err != nil {
			return err
		}
	} else if _, err := m.store.PreserveHome(identity); err != nil {
		return err
	}
	directory, err := m.store.InstanceDir(identity.Name)
	if err != nil {
		return err
	}
	return m.removeAll(directory)
}
