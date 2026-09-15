package vm

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

func (m *Manager) generationDir(instance state.Instance) (string, error) {
	directory, err := m.store.InstanceDir(instance.Identity.Name)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "generations", instance.Generation), nil
}

func (m *Manager) prepareGeneration(ctx context.Context, instance state.Instance, cfg config.Config) (string, error) {
	hostArch, err := lima.HostArchitecture()
	if err != nil {
		return "", err
	}
	if err := lima.RequireNativeArchitecture(hostArch); err != nil {
		return "", err
	}
	directory, err := m.generationDir(instance)
	if err != nil {
		return "", err
	}
	sources, err := m.registry.Sources(ctx, cfg.NixOS.Modules)
	if err != nil {
		return "", err
	}
	_, bundleErr := nixos.Prepare(cfg, directory, sources.Sources, m.getUID())
	if err := errors.Join(bundleErr, sources.Close()); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	document, err := lima.Render(cfg, instance.Identity.Home, directory, hostArch, m.getUID())
	if err != nil {
		return "", err
	}
	template := filepath.Join(directory, "lima.yaml")
	if err := filesystem.WriteFileAtomic(template, document, 0o600); err != nil {
		return "", err
	}
	if err := m.backend.Validate(ctx, template); err != nil {
		return "", err
	}
	return template, ctx.Err()
}

func (m *Manager) pruneGenerations(instance state.Instance) {
	current, err := m.generationDir(instance)
	if err != nil {
		m.Warn("VM '%s' was updated, but generations could not be found: %v", instance.Identity.Name, err)
		return
	}
	parent := filepath.Dir(current)
	entries, err := m.readDir(parent)
	if err != nil {
		m.Warn("VM '%s' was updated, but old generations could not be listed: %v", instance.Identity.Name, err)
		return
	}
	for _, entry := range entries {
		candidate := filepath.Join(parent, entry.Name())
		if candidate == current {
			continue
		}
		if err := m.removeAll(candidate); err != nil {
			m.Warn("VM '%s' was updated, but old generation '%s' could not be removed: %v", instance.Identity.Name, candidate, err)
		}
	}
}
