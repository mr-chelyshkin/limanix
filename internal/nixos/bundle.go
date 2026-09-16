package nixos

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/modules"
)

// Prepare copies module trees once into a generation and writes runtime ENV
// files beside its flake, keeping their values out of flake sources and Nix store.
func Prepare(cfg config.Config, runtimeDir string, sources []modules.Source, uid int) (string, error) {
	if uid <= 0 {
		return "", ErrInvalidUID
	}

	serviceEnvironment, shellEnvironment, err := renderEnvironmentFiles(cfg.Env)
	if err != nil {
		return "", err
	}

	flakeDir := filepath.Join(runtimeDir, "flake")
	if err := requireNewBundle(flakeDir); err != nil {
		return "", err
	}

	if err := validateSources(sources); err != nil {
		return "", err
	}

	if err := filesystem.CheckDirectory(runtimeDir); err != nil {
		return "", err
	}

	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return "", err
	}

	if err := copyResource("resources/base", flakeDir); err != nil {
		return "", err
	}

	imports, err := copyModules(flakeDir, sources)
	if err != nil {
		return "", err
	}

	if err := writeRuntime(filepath.Join(flakeDir, "runtime.json"), cfg, imports, uid); err != nil {
		return "", err
	}

	if err := writeEnvironmentFiles(runtimeDir, serviceEnvironment, shellEnvironment); err != nil {
		return "", err
	}

	return flakeDir, nil
}

func requireNewBundle(directory string) error {
	_, err := os.Lstat(directory)

	switch {
	case err == nil:
		return fmt.Errorf("%w: %s", ErrBundleExists, directory)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return err
	}
}
