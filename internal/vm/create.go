package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Create prepares a generation and home before invoking Lima. Once backend
// creation starts, failures retain ownership for recovery through update/delete.
func (m *Manager) Create(ctx context.Context, path string) (result state.Instance, err error) {
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
	directory, err := m.store.InstanceDir(cfg.Name)
	if err != nil {
		return result, err
	}
	if _, err := os.Lstat(directory); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return result, err
		}
		return result, fmt.Errorf("VM '%s' already has state; use update or delete", cfg.Name)
	}
	id, err := m.newID()
	if err != nil {
		return result, err
	}
	home, err := state.HomePath(cfg.Home.Root, cfg.Name, id)
	if err != nil {
		return result, err
	}
	generation, err := m.newID()
	if err != nil {
		return result, err
	}
	result = state.Instance{
		Identity: state.Identity{
			ID: id, Name: cfg.Name, Arch: cfg.Resources.Arch,
			Username: cfg.User.Name, UserHome: cfg.User.Home,
			HomeRoot: cfg.Home.Root, Home: home,
			CreatedAt: m.now().UTC().Format(time.RFC3339Nano),
		},
		Status: state.Creating, Generation: generation,
	}
	homeCreated := false
	template, prepErr := m.prepareGeneration(ctx, result, cfg)
	if prepErr == nil {
		_, prepErr = m.createHome(result.Identity)
		homeCreated = prepErr == nil
	}
	if prepErr == nil {
		prepErr = m.store.Save(result)
	}
	if prepErr == nil {
		prepErr = ctx.Err()
	}
	if prepErr != nil {
		if homeCreated {
			if cleanupErr := m.removeHome(result.Identity); cleanupErr != nil {
				// Keep ownership when cleanup cannot finish. A failed Save may
				// already have written the immutable identity; retrying also
				// makes the failure recoverable through delete.
				return result, m.recordFailure(result, errors.Join(prepErr, cleanupErr))
			}
		}
		return result, errors.Join(prepErr, m.removeAll(directory))
	}
	name := result.Identity.LimaName()
	if err := m.backend.Create(ctx, name, template); err != nil {
		return result, m.recordFailure(result, err)
	}
	if err := m.backend.Start(ctx, name); err != nil {
		return result, m.recordFailure(result, err)
	}
	if err := m.guest.Apply(ctx, name, result.Identity.Username); err != nil {
		return result, m.recordFailure(result, err)
	}
	result.Status = state.Ready
	return result, m.store.Save(result)
}
