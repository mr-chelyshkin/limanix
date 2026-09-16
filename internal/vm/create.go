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

// Create prepares a generation and home before invoking Lima.
func (m *Manager) Create(ctx context.Context, path string) (result state.Instance, err error) {
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
	result, err = m.newInstance(cfg)
	if err != nil {
		return result, err
	}
	template, err := m.prepareCreation(ctx, result, cfg, directory)
	if err != nil {
		return result, err
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

// newInstance builds the initial identity and generation record.
func (m *Manager) newInstance(cfg config.Config) (state.Instance, error) {
	id, err := m.newID()
	if err != nil {
		return state.Instance{}, err
	}
	home, err := state.HomePath(cfg.Home.Root, cfg.Name, id)
	if err != nil {
		return state.Instance{}, err
	}
	generation, err := m.newID()
	if err != nil {
		return state.Instance{}, err
	}
	return state.Instance{
		Identity: state.Identity{
			ID: id, Name: cfg.Name, Arch: cfg.Resources.Arch,
			Username: cfg.User.Name, UserHome: cfg.User.Home,
			HomeRoot: cfg.Home.Root, Home: home,
			CreatedAt: m.now().UTC().Format(time.RFC3339Nano),
		},
		Status: state.Creating, Generation: generation,
	}, nil
}

func (m *Manager) prepareCreation(ctx context.Context, instance state.Instance, cfg config.Config, directory string) (string, error) {
	template, err := m.prepareGeneration(ctx, instance, cfg)
	if err != nil {
		return "", errors.Join(err, m.removeAll(directory))
	}

	if _, err = m.createHome(instance.Identity); err != nil {
		return "", errors.Join(err, m.removeAll(directory))
	}

	err = m.store.Save(instance)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		return template, nil
	}
	if cleanupErr := m.removeHome(instance.Identity); cleanupErr != nil {
		return "", m.recordFailure(instance, errors.Join(err, cleanupErr))
	}
	return "", errors.Join(err, m.removeAll(directory))
}
