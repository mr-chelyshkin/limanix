package generator

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Generate renders the example, references, and version metadata, then writes each file atomically to root/docs/_generated.
func Generate(ctx context.Context, root string, diagnostics io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}

	documents, err := render()
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Join(root, "docs", "_generated"), 0o755); err != nil {
		return fmt.Errorf("prepare generated documentation directory: %w", err)
	}

	for _, document := range documents {
		if err = ctx.Err(); err != nil {
			return err
		}

		if err = filesystem.WriteFileAtomic(filepath.Join(root, document.path), document.content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", document.path, err)
		}
	}

	log.New(diagnostics, "docsgen: ", 0).Println("Generated configuration example, CLI and configuration references, and version metadata.")
	return nil
}
