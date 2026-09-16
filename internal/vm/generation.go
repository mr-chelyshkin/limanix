package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
)

type generationBuilder struct {
	modules *modules.Registry
	backend Backend
	store   Store
	hostUID int

	removeAll func(string) error
	readDir   func(string) ([]os.DirEntry, error)
}

func newGenerationBuilder(deps Dependencies) *generationBuilder {
	return &generationBuilder{
		store:     deps.Store,
		backend:   deps.Backend,
		modules:   deps.Modules,
		hostUID:   deps.HostUID,
		removeAll: os.RemoveAll,
		readDir:   os.ReadDir,
	}
}

func (g *generationBuilder) prepare(ctx context.Context, instance domain.Instance, cfg config.Config) (string, error) {
	hostArch, err := lima.HostArchitecture()
	if err != nil {
		return "", err
	}

	if err = lima.RequireNativeArchitecture(hostArch); err != nil {
		return "", err
	}

	directory, err := g.store.GenerationDir(instance)
	if err != nil {
		return "", err
	}

	if err = g.prepareBundle(ctx, cfg, directory); err != nil {
		return "", err
	}

	document, err := lima.Render(cfg, instance.Identity.Home, directory, hostArch, g.hostUID)
	if err != nil {
		return "", err
	}

	template := filepath.Join(directory, "lima.yaml")
	if err = filesystem.WriteFileAtomic(template, document, 0o600); err != nil {
		return "", err
	}

	if err = g.backend.Validate(ctx, template); err != nil {
		return "", err
	}

	return template, ctx.Err()
}

func (g *generationBuilder) prepareBundle(ctx context.Context, cfg config.Config, directory string) (err error) {
	sources, err := g.modules.Sources(ctx, cfg.NixOS.Modules)
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, sources.Close())
	}()

	if _, err = nixos.Prepare(cfg, directory, sources.Sources, g.hostUID); err != nil {
		return err
	}

	return ctx.Err()
}

func (g *generationBuilder) discard(instance domain.Instance) error {
	directory, err := g.store.GenerationDir(instance)
	if err != nil {
		return err
	}

	return g.removeAll(directory)
}

func (g *generationBuilder) prune(instance domain.Instance) []error {
	current, err := g.store.GenerationDir(instance)
	if err != nil {
		return []error{fmt.Errorf("VM '%s' was updated, but generations could not be found: %w", instance.Identity.Name, err)}
	}

	parent := filepath.Dir(current)
	entries, err := g.readDir(parent)
	if err != nil {
		return []error{fmt.Errorf("VM '%s' was updated, but old generations could not be listed: %w", instance.Identity.Name, err)}
	}

	var failures []error

	for _, entry := range entries {
		candidate := filepath.Join(parent, entry.Name())
		if candidate == current {
			continue
		}

		if err = g.removeAll(candidate); err != nil {
			failures = append(failures, fmt.Errorf(
				"VM '%s' was updated, but old generation '%s' could not be removed: %w",
				instance.Identity.Name, candidate, err,
			))
		}
	}

	return failures
}
