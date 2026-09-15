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

// buildPlan describes the inputs expected from the selected compiler for one target.
type buildPlan struct {
	target target
	flags  []string
	build  buildMetadata
}

// moduleInfo is the part of go list's module record needed for source identity.
// Replacements are retained because they may affect transitive agent dependencies.
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
		replacement := m.Replace.identity()
		result.Replace = &replacement
		result.Sum = ""
	}
	return result
}

// plan loads only packages compiled into this target, using the build environment.
// A Limanix dependency outside this graph cannot invalidate a guest-agent archive.
func (tool *goToolchain) plan(ctx context.Context, t target, version string) (buildPlan, error) {
	plan := buildPlan{target: t, flags: buildFlags(version)}
	args := append([]string{"list"}, plan.flags...)
	args = append(args, "-deps", "-json=ImportPath,Module,DefaultGODEBUG", agentPackage)
	data, err := tool.run(ctx, tool.targetEnvironment(t), args...)
	if err != nil {
		return plan, fmt.Errorf("resolve linux/%s dependencies: %w", t.goArch, err)
	}
	modules := map[string]moduleIdentity{}
	settings := map[string]string{
		"-buildmode": "exe", "-compiler": "gc", "-trimpath": "true",
		"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": t.goArch,
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
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
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
	slices.SortFunc(dependencies, func(a, b moduleIdentity) int { return strings.Compare(a.Path, b.Path) })
	plan.build = buildMetadata{
		Package: agentPackage, GoVersion: tool.version,
		Main: main, Dependencies: dependencies, Settings: settings,
	}
	return plan, nil
}
