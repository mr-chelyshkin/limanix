package vmnet

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/lima-vm/lima/v2/pkg/networks"
)

func validateRootPath(path string) error {
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
