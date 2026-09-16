package generator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// builtArtifact is a validated archive and the metadata read from its executable.
type builtArtifact struct {
	record  artifact
	archive []byte
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
			Name:       plan.target.archiveName(),
			Arch:       plan.target.goArch,
			SHA256:     archiveDigest(archive),
			BuildFlags: plan.flags,
			Build:      actual,
		},
		archive: archive,
	}, nil
}
