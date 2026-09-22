package generator

import (
	"encoding/json"
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
	"github.com/mr-chelyshkin/limanix/internal/cli"
	"github.com/mr-chelyshkin/limanix/internal/config"
)

// document is one generated file, addressed relative to the repository root.
type document struct {
	path    string
	content []byte
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
		{
			path:    "docs/_generated/limanix.example.toml",
			content: example,
		},
		{
			path:    "docs/_generated/configuration.md",
			content: []byte(configuration),
		},
		{
			path:    "docs/_generated/cli.md",
			content: []byte(cli.Reference()),
		},
		{
			path:    "docs/_generated/metadata.json",
			content: append(metadata, '\n'),
		},
	}, nil
}
