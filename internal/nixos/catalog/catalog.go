package catalog

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/pelletier/go-toml/v2"
)

// Repository identifies the standard catalog source in download URLs and ZIP comments.
// Changing it invalidates archives packaged from another repository, even at the same tag.
const Repository = "github.com/mr-chelyshkin/limanix-modules"

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Catalog retains validated archive contents and an independent metadata index.
type Catalog struct {
	Version string

	files   fs.FS
	modules map[string]string
}

// ValidateVersion checks a release tag before it is used in an archive URL.
func ValidateVersion(version string) error {
	if !tagPattern.MatchString(version) {
		return fmt.Errorf("%w: %q", ErrVersion, version)
	}

	return nil
}

// Open validates a complete catalog without extracting it to the host filesystem.
func Open(data []byte) (*Catalog, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrArchive, err)
	}

	if archive.Comment != Repository {
		return nil, fmt.Errorf("%w: got %q, expected %q", ErrRepository, archive.Comment, Repository)
	}

	if err = validateArchive(archive); err != nil {
		return nil, err
	}

	version, err := fs.ReadFile(archive, "version")
	if err != nil {
		return nil, fmt.Errorf("%w: read version: %w", ErrArchive, err)
	}

	tag := strings.TrimSpace(string(version))
	if err = ValidateVersion(tag); err != nil {
		return nil, err
	}

	if _, err = fs.ReadFile(archive, "LICENSE"); err != nil {
		return nil, fmt.Errorf("%w: read LICENSE: %w", ErrArchive, err)
	}

	metadata, err := readModules(archive)
	if err != nil {
		return nil, err
	}

	return &Catalog{Version: tag, files: archive, modules: metadata}, nil
}

// Modules returns a copy of the local module names and their descriptions.
func (catalog *Catalog) Modules() map[string]string {
	return maps.Clone(catalog.modules)
}

// Module returns the read-only tree belonging to one local module name.
func (catalog *Catalog) Module(name string) (fs.FS, error) {
	if _, exists := catalog.modules[name]; !exists {
		return nil, fmt.Errorf("%w: %q", ErrModule, name)
	}

	return fs.Sub(catalog.files, path.Join("modules", name))
}

func readModules(files fs.FS) (map[string]string, error) {
	entries, err := fs.ReadDir(files, "modules")
	if err != nil {
		return nil, fmt.Errorf("%w: read modules: %w", ErrMetadata, err)
	}

	result := make(map[string]string, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if _, err = domain.NewModuleName(name); err != nil || !entry.IsDir() {
			return nil, fmt.Errorf("%w: expected a module directory, got %q", ErrMetadata, name)
		}

		description, err := readDescription(files, path.Join("modules", name))
		if err != nil {
			return nil, fmt.Errorf("module %q: %w", name, err)
		}

		result[name] = description
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("%w: no modules found", ErrMetadata)
	}

	return result, nil
}

func readDescription(files fs.FS, directory string) (string, error) {
	entry, err := fs.Stat(files, path.Join(directory, "default.nix"))
	if err != nil {
		return "", fmt.Errorf("%w: default.nix: %w", ErrMetadata, err)
	}
	if !entry.Mode().IsRegular() {
		return "", fmt.Errorf("%w: default.nix must be a regular file", ErrMetadata)
	}

	data, err := fs.ReadFile(files, path.Join(directory, "module.toml"))
	if err != nil {
		return "", fmt.Errorf("%w: module.toml: %w", ErrMetadata, err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%w: module.toml must be UTF-8", ErrMetadata)
	}

	var metadata struct {
		Description string `toml:"description"`
	}

	decoder := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err = decoder.Decode(&metadata); err != nil {
		return "", fmt.Errorf("%w: module.toml: %w", ErrMetadata, err)
	}
	if strings.TrimSpace(metadata.Description) == "" {
		return "", fmt.Errorf("%w: description must not be empty", ErrMetadata)
	}

	return metadata.Description, nil
}
