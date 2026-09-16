package vm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Create prepares a generation and home before invoking Lima.
func (m *Manager) Create(ctx context.Context, path string) (result domain.Instance, err error) {
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

	if err = m.store.RequireAbsent(cfg.Name); err != nil {
		if errors.Is(err, fs.ErrExist) {
			err = &InstanceError{
				Name:  cfg.Name,
				Cause: ErrAlreadyExists,
			}
		}

		return result, err
	}

	result, err = newInstance(cfg)
	if err != nil {
		return result, err
	}

	template, err := m.prepareCreation(ctx, result, cfg)
	if err != nil {
		return result, err
	}

	if err = m.backend.Create(ctx, result.Identity.LimaName(), template); err != nil {
		return result, m.recordFailure(result, err)
	}

	if err = m.applyGuest(ctx, result); err != nil {
		return result, m.recordFailure(result, err)
	}

	result.MarkReady()
	return result, m.store.Save(result)
}

func newInstance(cfg config.Config) (domain.Instance, error) {
	id, err := randomID()
	if err != nil {
		return domain.Instance{}, err
	}

	home, err := domain.HomePath(cfg.Home.Root, cfg.Name, id)
	if err != nil {
		return domain.Instance{}, err
	}

	generation, err := randomID()
	if err != nil {
		return domain.Instance{}, err
	}

	return domain.Instance{
		Identity: domain.Identity{
			ID:        id,
			Name:      cfg.Name,
			Arch:      cfg.Resources.Arch,
			Username:  cfg.User.Name,
			UserHome:  cfg.User.Home,
			HomeRoot:  cfg.Home.Root,
			Home:      home,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		Status:     domain.Creating,
		Generation: generation,
	}, nil
}

// prepareCreation owns local rollback until ownership has been saved successfully.
func (m *Manager) prepareCreation(ctx context.Context, instance domain.Instance, cfg config.Config) (string, error) {
	template, err := m.generations.prepare(ctx, instance, cfg)
	if err != nil {
		return "", errors.Join(err, m.store.Remove(instance.Identity.Name))
	}

	if _, err = m.homes.Create(instance.Identity); err != nil {
		return "", errors.Join(err, m.store.Remove(instance.Identity.Name))
	}

	if err = m.store.Save(instance); err != nil {
		return "", m.rollbackCreation(instance, err)
	}

	if err = ctx.Err(); err != nil {
		return "", m.rollbackCreation(instance, err)
	}

	return template, nil
}

// rollbackCreation never drops ownership when the home could not be removed.
func (m *Manager) rollbackCreation(instance domain.Instance, cause error) error {
	if err := m.homes.Remove(instance.Identity); err != nil {
		return m.recordFailure(instance, errors.Join(cause, err))
	}

	return errors.Join(cause, m.store.Remove(instance.Identity.Name))
}

func randomID() (string, error) {
	var token [domain.IDLength / 2]byte

	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate VM identifier: %w", err)
	}

	return hex.EncodeToString(token[:]), nil
}
