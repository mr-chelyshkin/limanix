package catalog

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/pelletier/go-toml/v2"
)

// metadata declares optional version selectors; package sources remain entirely in Nix.
type metadata struct {
	Description string   `toml:"description"`
	Default     string   `toml:"default"`
	Versions    []string `toml:"versions"`
}

// selection binds a public local identifier to files under one physical module directory.
type selection struct {
	directory   string
	entryPoint  string
	description string
}

func readMetadata(files fs.FS, directory string) (metadata, error) {
	var result metadata

	data, err := fs.ReadFile(files, path.Join(directory, "module.toml"))
	if err != nil {
		return result, fmt.Errorf("%w: module.toml: %w", ErrMetadata, err)
	}

	if !utf8.Valid(data) {
		return result, fmt.Errorf("%w: module.toml must be UTF-8", ErrMetadata)
	}

	decoder := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err = decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("%w: module.toml: %w", ErrMetadata, err)
	}

	if err = result.validate(); err != nil {
		return result, err
	}

	entries := []string{"default.nix"}
	for _, version := range result.Versions {
		entries = append(entries, versionEntry(version))
	}

	for _, name := range entries {
		entry, err := fs.Stat(files, path.Join(directory, name))
		if err != nil {
			return result, fmt.Errorf("%w: %s: %w", ErrMetadata, name, err)
		}

		if !entry.Mode().IsRegular() {
			return result, fmt.Errorf("%w: %s must be a regular file", ErrMetadata, name)
		}
	}

	return result, nil
}

func (metadata metadata) validate() error {
	if strings.TrimSpace(metadata.Description) == "" {
		return fmt.Errorf("%w: description must not be empty", ErrMetadata)
	}

	if len(metadata.Versions) == 0 {
		if metadata.Default != "" {
			return fmt.Errorf("%w: default requires a nonempty versions list", ErrMetadata)
		}

		return nil
	}

	if !slices.Contains(metadata.Versions, metadata.Default) {
		return fmt.Errorf("%w: default must select a declared version", ErrMetadata)
	}

	seen := make(map[string]bool, len(metadata.Versions))
	for _, version := range metadata.Versions {
		if err := domain.ValidateModuleVersion(version); err != nil {
			return fmt.Errorf("%w: version %q: %w", ErrMetadata, version, err)
		}

		if seen[version] {
			return fmt.Errorf("%w: duplicate version %q", ErrMetadata, version)
		}

		seen[version] = true
	}

	return nil
}

func (metadata metadata) selections(name string) map[string]selection {
	base := selection{directory: name, entryPoint: "default.nix", description: metadata.Description}
	if metadata.Default != "" {
		base.description += " (default: " + metadata.Default + ")"
	}

	result := map[string]selection{name: base}
	for _, version := range metadata.Versions {
		result[name+"-"+version] = selection{
			directory:   name,
			entryPoint:  versionEntry(version),
			description: metadata.Description + " (version: " + version + ")",
		}
	}

	return result
}

func versionEntry(version string) string {
	return path.Join("versions", version+".nix")
}
