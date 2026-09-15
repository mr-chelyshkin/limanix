// Package generator prepares model-derived content for the Hugo documentation site.
package generator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
	"github.com/mr-chelyshkin/limanix/internal/cli"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// document holds generated content and its destination relative to the repository root.
type document struct {
	path    string
	content []byte
}

// Generate renders the example, references, and version metadata, then writes each
// file atomically beneath root. diagnostics receives generation progress.
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
	if err := os.MkdirAll(filepath.Join(root, "docs", "_generated"), 0o755); err != nil {
		return fmt.Errorf("prepare generated documentation directory: %w", err)
	}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := filesystem.WriteFileAtomic(filepath.Join(root, document.path), document.content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", document.path, err)
		}
	}
	log.New(diagnostics, "docsgen: ", 0).Println("Generated configuration example, CLI and configuration references, and version metadata.")
	return nil
}

// render derives all documents from the runtime models without writing files.
func render() ([]document, error) {
	example, err := config.RenderExample()
	if err != nil {
		return nil, fmt.Errorf("render configuration example: %w", err)
	}
	configuration, err := config.RenderReference()
	if err != nil {
		return nil, fmt.Errorf("render configuration reference: %w", err)
	}
	metadata, err := json.Marshal(struct {
		Version string `json:"version"`
	}{Version: buildinfo.Version})
	if err != nil {
		return nil, fmt.Errorf("encode version metadata: %w", err)
	}
	return []document{
		{path: "limanix.example.toml", content: example},
		{path: "docs/_generated/configuration.md", content: []byte(configuration)},
		{path: "docs/_generated/cli.md", content: []byte(cli.Reference())},
		{path: "docs/_generated/metadata.json", content: append(metadata, '\n')},
	}, nil
}
