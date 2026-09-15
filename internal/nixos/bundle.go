// Package nixos materializes embedded guest configuration and selected module snapshots.
package nixos

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/modules"
)

//go:embed resources
var resources embed.FS

// BuiltinModules returns independent metadata for modules embedded in every binary.
func BuiltinModules() map[string]string {
	return map[string]string{"git": "Git version control.", "neovim": "Neovim editor.", "rust": "Rust compiler, Cargo, rustfmt, Clippy, and rust-analyzer."}
}

// Prepare copies module trees once into a generation and writes runtime ENV
// files beside its flake, keeping their values out of flake sources and Nix store.
func Prepare(cfg config.Config, runtimeDir string, sources []modules.Source, uid int) (string, error) {
	if uid <= 0 {
		return "", errors.New("guest UID must be positive")
	}
	serviceEnvironment, shellEnvironment, err := renderEnvironmentFiles(cfg.Env)
	if err != nil {
		return "", err
	}
	flakeDir := filepath.Join(runtimeDir, "flake")
	_, err = os.Lstat(flakeDir)
	if err == nil {
		return "", fmt.Errorf("NixOS bundle already exists: %s", flakeDir)
	}
	if !errors.Is(err, fs.ErrNotExist) {
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
	for _, file := range []struct{ name, content string }{{"environment", serviceEnvironment}, {"environment.sh", shellEnvironment}} {
		if err := filesystem.WriteFileAtomic(filepath.Join(runtimeDir, file.name), []byte(file.content), 0o600); err != nil {
			return "", err
		}
	}
	return flakeDir, nil
}

func validateSources(sources []modules.Source) error {
	builtins := BuiltinModules()
	for _, source := range sources {
		if _, err := domain.NewModuleID(string(source.ID)); err != nil {
			return fmt.Errorf("invalid module identifier: %w", err)
		}
		if source.Path == "" {
			if _, ok := builtins[string(source.ID)]; !ok {
				return fmt.Errorf("unknown bundled module %s", source.ID)
			}
			continue
		}
		info, err := os.Stat(filepath.Join(source.Path, "default.nix"))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("module entry point is not a regular Nix file")
		}
	}
	return nil
}

func copyModules(flakeDir string, sources []modules.Source) ([]string, error) {
	imports := make([]string, 0, len(sources))
	if len(sources) > 0 {
		if err := os.MkdirAll(filepath.Join(flakeDir, "modules"), 0o700); err != nil {
			return nil, err
		}
	}
	for index, source := range sources {
		name := fmt.Sprintf("%04d", index)
		target := filepath.Join(flakeDir, "modules", name)
		if source.Path == "" {
			if err := copyResource(path.Join("resources/modules", string(source.ID)), target); err != nil {
				return nil, err
			}
		} else {
			if _, err := modules.CopyTree(source.Path, target); err != nil {
				return nil, err
			}
		}
		imports = append(imports, path.Join("modules", name, "default.nix"))
	}
	return imports, nil
}

func copyResource(source, destination string) error {
	return fs.WalkDir(resources, source, func(resourcePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(resourcePath, source), "/")
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := resources.ReadFile(resourcePath)
		if err != nil {
			return err
		}
		return filesystem.WriteFileAtomic(target, data, 0o600)
	})
}

func renderEnvironmentFiles(environment map[domain.EnvName]domain.EnvValue) (string, string, error) {
	names := make([]string, 0, len(environment))
	for name, value := range environment {
		if _, err := domain.NewEnvName(string(name)); err != nil {
			return "", "", err
		}
		if _, err := domain.NewEnvValue(string(value)); err != nil {
			return "", "", err
		}
		names = append(names, string(name))
	}
	sort.Strings(names)
	var service, shell strings.Builder
	escape := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "$", "\\$", "`", "\\`")
	for _, name := range names {
		line := name + "=\"" + escape.Replace(string(environment[domain.EnvName(name)])) + "\"\n"
		service.WriteString(line)
		shell.WriteString("export " + line)
	}
	return service.String(), shell.String(), nil
}
