package generator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

type builtArtifact struct {
	record  artifact
	archive []byte
}

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
	version, err := tool.limaVersion(ctx)
	if err != nil {
		return err
	}

	plans := make([]buildPlan, 0, len(targets()))
	for _, t := range targets() {
		plan, err := tool.plan(ctx, t, version)
		if err != nil {
			return err
		}
		plans = append(plans, plan)
	}

	var (
		directory = filepath.Join(root, "internal", "bundle", "resources")
		logger    = log.New(diagnostics, "bundle-guestagent: ", 0)
	)
	if err = checkAssets(directory, plans); err == nil {
		logger.Println("Embedded guest agents match the source, compiler, and build settings.")
		return ctx.Err()
	} else {
		logger.Printf("Rebuilding guest agents: %v", err)
	}

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

func buildAgents(ctx context.Context, tool *goToolchain, plans []buildPlan, logger *log.Logger) (artifacts []builtArtifact, result error) {
	directory, err := os.MkdirTemp("", "limanix-guestagent-")
	if err != nil {
		return nil, fmt.Errorf("create guest-agent build directory: %w", err)
	}

	defer func() {
		if err = os.RemoveAll(directory); err != nil {
			result = errors.Join(result, fmt.Errorf("remove guest-agent build directory: %w", err))
		}
	}()

	for _, plan := range plans {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		logger.Printf("Building Lima %s for linux/%s with %s.", plan.build.Main.Version, plan.target.goArch, tool.version)

		path := filepath.Join(directory, "lima-guestagent-"+plan.target.goArch)
		if err = tool.build(ctx, plan, path); err != nil {
			return nil, fmt.Errorf("build linux/%s: %w", plan.target.goArch, err)
		}

		built, err := prepareArtifact(path, plan)
		if err != nil {
			return nil, fmt.Errorf("validate linux/%s: %w", plan.target.goArch, err)
		}
		artifacts = append(artifacts, built)
	}
	return artifacts, nil
}

func prepareArtifact(path string, plan buildPlan) (builtArtifact, error) {
	binary, err := os.ReadFile(path)
	if err != nil {
		return builtArtifact{}, fmt.Errorf("read executable: %w", err)
	}
	actual, err := inspectBinary(binary, plan.target)
	if err != nil {
		return builtArtifact{}, err
	}

	if err = actual.compare(plan.build); err != nil {
		return builtArtifact{}, err
	}

	archive, err := gzipBinary(binary)
	if err != nil {
		return builtArtifact{}, fmt.Errorf("compress executable: %w", err)
	}

	return builtArtifact{
		record: artifact{
			Name: plan.target.archiveName(), Arch: plan.target.goArch,
			SHA256: archiveDigest(archive), BuildFlags: plan.flags, Build: actual,
		},
		archive: archive,
	}, nil
}
