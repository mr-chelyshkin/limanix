package nixos

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/modules"
)

// BuiltinModules returns independent metadata for modules embedded in every binary.
func BuiltinModules() map[string]string {
	return map[string]string{
		"git":    "Git version control.",
		"neovim": "Neovim editor.",
		"rust":   "Rust compiler, Cargo, rustfmt, Clippy, and rust-analyzer.",
	}
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
