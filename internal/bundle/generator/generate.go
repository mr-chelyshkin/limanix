package generator

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
)

// Generate resolves inputs, checks reuse, builds all targets, then publishes them.
func Generate(ctx context.Context, root string, diagnostics io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}

	tool, err := resolveToolchain(ctx, root, diagnostics)
	if err != nil {
		return err
	}

	plans, err := tool.plans(ctx)
	if err != nil {
		return err
	}

	var (
		directory = filepath.Join(root, "internal", "bundle", "resources")
		logger    = log.New(diagnostics, "bundle-guestagent: ", 0)
	)

	if err = checkAssets(directory, plans); err == nil {
		logger.Println("Embedded guest agents match the source, compiler, and build settings.")
		return ctx.Err()
	}

	logger.Printf("Rebuilding guest agents: %v", err)

	artifacts, err := buildAgents(ctx, tool, plans, logger)
	if err != nil {
		return err
	}

	if err = ctx.Err(); err != nil {
		return err
	}

	if err = publish(directory, artifacts); err != nil {
		return err
	}

	logger.Println("Generated embedded Linux guest agents and integrity manifest.")
	return nil
}
