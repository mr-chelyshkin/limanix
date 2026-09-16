package vmnet

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/lima-vm/lima/v2/pkg/networks"
)

// validateRootPath reuses Lima's no-symlink, root-owner and write-permission checks.
func validateRootPath(path string) error {
	// Validate also checks varRun's nearest existing ancestor. An empty value
	// resolves to "."; use the filesystem root when checking an unrelated path.
	cfg := networks.Config{Paths: networks.Paths{
		SocketVMNet: path,
		VarRun:      "/",
	}}
	return cfg.Validate()
}

func validateExistingParent(path string) error {
	parent := filepath.Dir(path)
	_, err := os.Lstat(parent)
	if errors.Is(err, fs.ErrNotExist) {
		return validateExistingParent(parent)
	}
	if err != nil {
		return err
	}

	return validateRootPath(parent)
}

func secureDirectory(path string) error {
	if err := validateExistingParent(filepath.Join(path, ".entry")); err != nil {
		return err
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}

	return validateRootPath(path)
}

func secureFile(path string) error {
	if err := validateRootPath(path); err != nil {
		return err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrUnsafeFile, path)
	}

	return nil
}
