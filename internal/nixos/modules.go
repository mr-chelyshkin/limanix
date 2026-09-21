package nixos

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/modules"
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
		if err := copyModule(source, target); err != nil {
			return nil, fmt.Errorf("copy module %q: %w", source.ID, err)
		}

		imports = append(imports, path.Join("modules", name, "default.nix"))
	}

	return imports, nil
}

func copyModule(source modules.Source, target string) error {
	if source.Path != "" {
		_, err := modules.CopyTree(source.Path, target)
		return err
	}

	files, err := systemModule(source.ID)
	if err != nil {
		return err
	}

	return copyFiles(files, target)
}

func systemModule(id domain.ModuleID) (fs.FS, error) {
	if id.Namespace() != "lmx" {
		return nil, fmt.Errorf("module %q: an embedded source requires the lmx namespace", id)
	}

	catalog, err := systemCatalog()
	if err != nil {
		return nil, err
	}

	return catalog.Module(string(id.Name()))
}
