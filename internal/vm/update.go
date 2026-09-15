package vm

import (
	"context"
	"errors"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Update retains identity, disk, and managed home. Each attempt stages a new
// generation; only a successful commit permits best-effort removal of old inputs.
func (m *Manager) Update(ctx context.Context, path string) (result state.Instance, err error) {
	cfg, err := config.Load(path)
	if err != nil {
		return result, err
	}
	if err := m.preflight(ctx, cfg); err != nil {
		return result, err
	}
	lock, err := m.store.InstanceLock(ctx, cfg.Name)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	result, err = m.store.Load(cfg.Name)
	if err != nil {
		return result, err
	}
	identity := result.Identity
	if cfg.Resources.Arch != identity.Arch || cfg.User.Name != identity.Username || cfg.User.Home != identity.UserHome || cfg.Home.Root != identity.HomeRoot {
		return result, errors.New("an update cannot change architecture, username, or managed-home paths; create a new VM for these changes")
	}
	actual, err := m.requireLima(ctx, identity)
	if err != nil {
		return result, err
	}
	if err := validateDiskUpdate(cfg.Resources.Disk, actual.Disk); err != nil {
		return result, err
	}
	result.Generation, err = m.newID()
	if err != nil {
		return result, err
	}
	result.Status, result.Error = state.Updating, nil
	template, prepErr := m.prepareGeneration(ctx, result, cfg)
	if prepErr == nil {
		prepErr = m.store.Save(result)
	}
	if prepErr == nil {
		prepErr = ctx.Err()
	}
	if prepErr != nil {
		saved, loadErr := m.store.Load(cfg.Name)
		if loadErr == nil && saved.Generation != result.Generation {
			if directory, genErr := m.generationDir(result); genErr == nil {
				prepErr = errors.Join(prepErr, m.removeAll(directory))
			}
		}
		return result, prepErr
	}
	// Query again after preparation: an external Lima operation may have stopped
	// the VM or changed its disk configuration while the generation was staged.
	actual, err = m.requireLima(ctx, identity)
	if err != nil {
		return result, m.recordFailure(result, err)
	}
	if err := validateDiskUpdate(cfg.Resources.Disk, actual.Disk); err != nil {
		return result, m.recordFailure(result, err)
	}
	name := identity.LimaName()
	if actual.Status == lima.Running {
		if err := m.backend.Stop(ctx, name); err != nil {
			return result, m.recordFailure(result, err)
		}
	}
	if err := m.backend.Edit(ctx, name, template); err != nil {
		return result, m.recordFailure(result, err)
	}
	if err := m.backend.Start(ctx, name); err != nil {
		return result, m.recordFailure(result, err)
	}
	if err := m.guest.Apply(ctx, name, identity.Username); err != nil {
		return result, m.recordFailure(result, err)
	}
	result.Status = state.Ready
	if err := m.store.Save(result); err != nil {
		return result, err
	}
	m.pruneGenerations(result)
	return result, nil
}

func validateDiskUpdate(requested domain.ByteSize, reported *int64) error {
	if reported == nil {
		return errors.New("lima did not report the disk size; update was not started")
	}
	if int64(requested) < *reported {
		return errors.New("shrinking the guest disk is not supported")
	}
	return nil
}
