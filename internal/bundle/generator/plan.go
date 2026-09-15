package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

type buildPlan struct {
	target target
	flags  []string
	build  buildMetadata
}

type moduleInfo struct {
	Path    string
	Version string
	Sum     string
	Replace *moduleInfo
}

func (m moduleInfo) identity() moduleIdentity {
	result := moduleIdentity{Path: m.Path, Version: m.Version, Sum: m.Sum}
	if result.Version == "" {
		result.Version = "(devel)"
	}

	if m.Replace != nil {
		result.Replace = new(m.Replace.identity())
		result.Sum = ""
	}
	return result
}

func (tool *goToolchain) plan(ctx context.Context, t target, version string) (buildPlan, error) {
	var (
		plan     = buildPlan{target: t, flags: buildFlags(version)}
		modules  = map[string]moduleIdentity{}
		settings = map[string]string{
			"-buildmode": "exe", "-compiler": "gc", "-trimpath": "true",
			"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": t.goArch,
		}
	)

	args := append([]string{"list"}, plan.flags...)
	args = append(args, "-deps", "-json=ImportPath,Module,DefaultGODEBUG", agentPackage)
	data, err := tool.run(ctx, tool.targetEnvironment(t), args...)
	if err != nil {
		return plan, fmt.Errorf("resolve linux/%s dependencies: %w", t.goArch, err)
	}

	key, value, _ := strings.Cut(t.variant, "=")
	settings[key] = value

	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var pkg struct {
			ImportPath     string
			DefaultGODEBUG string
			Module         *moduleInfo
		}
		if err = decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return plan, fmt.Errorf("decode linux/%s dependency graph: %w", t.goArch, err)
		}

		if pkg.Module != nil {
			modules[pkg.Module.Path] = pkg.Module.identity()
		}
		if pkg.ImportPath == agentPackage && pkg.DefaultGODEBUG != "" {
			settings["DefaultGODEBUG"] = pkg.DefaultGODEBUG
		}
	}

	main, found := modules[limaModulePath]
	if !found || main.Version != version || main.Replace != nil {
		return plan, errors.New("guest-agent package graph does not match the resolved Lima module")
	}
	delete(modules, limaModulePath)

	dependencies := make([]moduleIdentity, 0, len(modules))
	for _, module := range modules {
		dependencies = append(dependencies, module)
	}

	slices.SortFunc(dependencies, func(a, b moduleIdentity) int {
		return strings.Compare(a.Path, b.Path)
	})
	plan.build = buildMetadata{
		Package: agentPackage, GoVersion: tool.version,
		Main: main, Dependencies: dependencies, Settings: settings,
	}
	return plan, nil
}
