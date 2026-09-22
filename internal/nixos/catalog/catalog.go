package catalog

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Repository identifies the standard catalog source in download URLs and ZIP comments.
const Repository = "github.com/mr-chelyshkin/limanix-modules"

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Catalog retains validated archive contents and an independent metadata index.
type Catalog struct {
	Version string

	files   fs.FS
	modules map[string]selection
}

// Module exposes a complete source tree and the selected NixOS entry point within it.
type Module struct {
	fs.FS
	EntryPoint string
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
	result := make(map[string]string, len(catalog.modules))
	for name, module := range catalog.modules {
		result[name] = module.description
	}

	return result
}

// Module resolves a local selector to its source tree and entry point.
func (catalog *Catalog) Module(name string) (Module, error) {
	selected, exists := catalog.modules[name]
	if !exists {
		return Module{}, fmt.Errorf("%w: %q", ErrModule, name)
	}

	files, err := fs.Sub(catalog.files, path.Join("modules", selected.directory))
	if err != nil {
		return Module{}, err
	}

	return Module{FS: files, EntryPoint: selected.entryPoint}, nil
}

func readModules(files fs.FS) (map[string]selection, error) {
	entries, err := fs.ReadDir(files, "modules")
	if err != nil {
		return nil, fmt.Errorf("%w: read modules: %w", ErrMetadata, err)
	}

	result := make(map[string]selection, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if _, err = domain.NewModuleName(name); err != nil || !entry.IsDir() {
			return nil, fmt.Errorf("%w: expected a module directory, got %q", ErrMetadata, name)
		}

		metadata, err := readMetadata(files, path.Join("modules", name))
		if err != nil {
			return nil, fmt.Errorf("module %q: %w", name, err)
		}

		for selector, selected := range metadata.selections(name) {
			if _, exists := result[selector]; exists {
				return nil, fmt.Errorf("%w: duplicate selector %q", ErrMetadata, selector)
			}

			result[selector] = selected
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("%w: no modules found", ErrMetadata)
	}

	return result, nil
}
