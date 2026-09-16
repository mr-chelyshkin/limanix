package vmnetgen

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/bundle"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Generate publishes verified upstream archives to the embedded resources directory.
func Generate(ctx context.Context, root string, diagnostics io.Writer) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	var (
		directory = filepath.Join(root, "internal", "bundle", "resources")
		logger    = log.New(diagnostics, "bundle-socketvmnet: ", 0)
	)

	for _, target := range bundle.SocketVMNetTargets() {
		if err = ctx.Err(); err != nil {
			return err
		}

		path := filepath.Join(directory, target.Filename)
		if err = checkArchive(path, target); err == nil {
			logger.Printf("%s matches the pinned release.", target.Filename)
			continue
		}

		logger.Printf("Preparing %s: %v", target.Filename, err)
		archive, err := download(ctx, target)
		if err != nil {
			return err
		}

		if err = filesystem.WriteFileAtomic(path, archive, 0o644); err != nil {
			return fmt.Errorf("publish %s: %w", target.Filename, err)
		}
	}

	return ctx.Err()
}

func checkArchive(path string, target bundle.SocketVMNetTarget) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	_, err = bundle.DecodeSocketVMNet(target, data)
	return err
}
