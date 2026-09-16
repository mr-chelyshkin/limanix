package generator

import (
	"context"
	"fmt"
	"strings"
)

// buildPlan records the expected source, compiler and settings for one target.
type buildPlan struct {
	target target
	flags  []string
	build  buildMetadata
}

func (tool *goToolchain) plans(ctx context.Context) ([]buildPlan, error) {
	version, err := tool.limaVersion(ctx)
	if err != nil {
		return nil, err
	}

	plans := make([]buildPlan, 0, len(targets()))

	for _, target := range targets() {
		plan, err := tool.plan(ctx, target, version)
		if err != nil {
			return nil, err
		}

		plans = append(plans, plan)
	}

	return plans, nil
}

func (tool *goToolchain) plan(ctx context.Context, target target, version string) (buildPlan, error) {
	flags := buildFlags(version)

	args := append([]string{"list"}, flags...)
	args = append(args, "-deps", "-json=ImportPath,Module,DefaultGODEBUG", agentPackage)

	data, err := tool.run(ctx, tool.targetEnvironment(target), args...)
	if err != nil {
		return buildPlan{}, fmt.Errorf("resolve linux/%s dependencies: %w", target.goArch, err)
	}

	graph, err := readPackageGraph(data)
	if err != nil {
		return buildPlan{}, fmt.Errorf("decode linux/%s dependency graph: %w", target.goArch, err)
	}

	main, found := graph.modules[limaModulePath]
	if !found || main.Version != version || main.Replace != nil {
		return buildPlan{}, ErrSourceMismatch
	}

	settings := buildSettings(target)
	if graph.defaultGODEBUG != "" {
		settings["DefaultGODEBUG"] = graph.defaultGODEBUG
	}

	return buildPlan{
		target: target,
		flags:  flags,
		build: buildMetadata{
			Package:      agentPackage,
			GoVersion:    tool.version,
			Main:         main,
			Dependencies: graph.dependencies(),
			Settings:     settings,
		},
	}, nil
}

func buildSettings(target target) map[string]string {
	settings := map[string]string{
		"-buildmode":  "exe",
		"-compiler":   "gc",
		"-trimpath":   "true",
		"CGO_ENABLED": "0",
		"GOOS":        "linux",
		"GOARCH":      target.goArch,
	}

	key, value, _ := strings.Cut(target.variant, "=")
	settings[key] = value

	return settings
}
