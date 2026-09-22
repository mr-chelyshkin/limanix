package nixos

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/nixos/catalog"
)

// SystemModules reads the embedded catalog and returns a copy of its names and descriptions.
func SystemModules() (map[string]string, error) {
	catalog, err := systemCatalog()
	if err != nil {
		return nil, err
	}

	return catalog.Modules(), nil
}

func validateSources(sources []modules.Source) error {
	for _, source := range sources {
		if _, err := domain.NewModuleID(string(source.ID)); err != nil {
			return fmt.Errorf("invalid module identifier: %w", err)
		}

		if source.Path == "" {
			if _, err := systemModule(source.ID); err != nil {
				return err
			}

			continue
		}

		if !source.ID.IsThirdParty() {
			return fmt.Errorf("module %q: only imported modules may use a local source path", source.ID)
		}

		if err := modules.ValidateDirectory(source.Path); err != nil {
			return err
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
		entry, err := copyModule(source, target)
		if err != nil {
			return nil, fmt.Errorf("copy module %q: %w", source.ID, err)
		}

		imports = append(imports, path.Join("modules", name, entry))
	}

	return imports, nil
}

func copyModule(source modules.Source, target string) (string, error) {
	if source.Path != "" {
		_, err := modules.CopyTree(source.Path, target)
		return "default.nix", err
	}

	files, err := systemModule(source.ID)
	if err != nil {
		return "", err
	}

	return files.EntryPoint, copyFiles(files, target)
}

func systemModule(id domain.ModuleID) (catalog.Module, error) {
	if id.Namespace() != "lmx" {
		return catalog.Module{}, fmt.Errorf("module %q: an embedded source requires the lmx namespace", id)
	}

	stored, err := systemCatalog()
	if err != nil {
		return catalog.Module{}, err
	}

	return stored.Module(id.Selector())
}
