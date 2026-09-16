package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

// Load reads a regular UTF-8 TOML file and resolves host paths relative to its real location.
func Load(filename string) (Config, error) {
	if err := domain.ValidateText(filename, false); err != nil {
		return Config{}, wrapField("config", "invalid configuration file path", err)
	}

	resolved, err := filesystem.Resolve(filename)
	if err != nil {
		return Config{}, wrapField("config", "configuration file cannot be read", err)
	}

	data, err := readConfig(resolved)
	if err != nil {
		return Config{}, err
	}

	config, err := Parse(data)
	if err != nil {
		return Config{}, err
	}

	if err := resolveHostPaths(&config, filepath.Dir(resolved)); err != nil {
		return Config{}, err
	}

	return config, nil
}

func readConfig(filename string) ([]byte, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return nil, wrapField("config", "configuration file cannot be read", err)
	}

	if !info.Mode().IsRegular() {
		return nil, fieldError("config", "expected a regular TOML file")
	}

	file, err := filesystem.OpenRegular(filename, unix.O_RDONLY, 0)
	if err != nil {
		return nil, wrapField("config", "configuration file cannot be read", err)
	}

	data, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, wrapField("config", "configuration file cannot be read", err)
	}

	return data, nil
}

func resolveHostPaths(config *Config, parent string) error {
	home, err := canonicalHostPath(config.Home.Root, parent)
	if err != nil {
		return wrapField("home.root", "host path cannot be resolved", err)
	}

	if home == "/" {
		return fieldError("home.root", "the host root directory is not allowed")
	}

	config.Home.Root = home

	for index := range config.Mounts {
		mount := &config.Mounts[index]

		source, err := canonicalHostPath(mount.Source, parent)
		if err != nil {
			return wrapField(fmt.Sprintf("mounts[%d].source", index), "host path cannot be resolved", err)
		}

		mount.Source = source
	}

	return nil
}

func canonicalHostPath(value, parent string) (string, error) {
	expanded, err := filesystem.ExpandHome(value)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(expanded) {
		expanded = parent + string(filepath.Separator) + expanded
	}

	resolved, err := filesystem.Resolve(expanded)
	if err != nil {
		return "", err
	}

	if err = domain.ValidateText(resolved, false); err != nil {
		return "", err
	}

	return resolved, nil
}
