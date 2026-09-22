package modulegen

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/nixos/catalog"
)

// Generate publishes a validated catalog for the requested Git release tag.
func Generate(ctx context.Context, root, version string, diagnostics io.Writer) error {
	if err := catalog.ValidateVersion(version); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	directory, err := filesystem.RequireDirectory(filepath.Join(root, "internal", "nixos", "resources"))
	if err != nil {
		return err
	}

	var (
		filename = filepath.Join(directory, "modules.zip")
		logger   = log.New(diagnostics, "bundle-modules: ", 0)
	)

	if err = checkExisting(filename, version); err == nil {
		logger.Printf("%s tag %s is already prepared.", catalog.Repository, version)
		return nil
	}

	logger.Printf("Preparing %s tag %s: %v", catalog.Repository, version, err)
	data, err := download(ctx, version)
	if err != nil {
		return err
	}

	if _, err = catalog.Open(data); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}

	if err = filesystem.WriteFileAtomic(filename, data, 0o644); err != nil {
		return fmt.Errorf("publish module catalog: %w", err)
	}

	logger.Printf("Prepared %s tag %s.", catalog.Repository, version)
	return nil
}

func checkExisting(filename, version string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	stored, err := catalog.Open(data)
	if err != nil {
		return err
	}
	if stored.Version != version {
		return fmt.Errorf("catalog tag is %s, expected %s", stored.Version, version)
	}

	return nil
}
