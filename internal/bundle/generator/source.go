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

// moduleInfo is the dependency identity returned by go list, including replacements.
type moduleInfo struct {
	Path    string
	Version string
	Sum     string
	Replace *moduleInfo
}

func (m moduleInfo) identity() moduleIdentity {
	result := moduleIdentity{
		Path:    m.Path,
		Version: m.Version,
		Sum:     m.Sum,
	}
	if result.Version == "" {
		result.Version = "(devel)"
	}

	if m.Replace != nil {
		result.Replace = new(m.Replace.identity())
		result.Sum = ""
	}

	return result
}

func (tool *goToolchain) limaVersion(ctx context.Context) (string, error) {
	data, err := tool.run(ctx, nil, "list", "-mod=readonly", "-m", "-json", limaModulePath)
	if err != nil {
		return "", fmt.Errorf("resolve Lima dependency: %w", err)
	}

	var module moduleInfo

	if err = json.Unmarshal(data, &module); err != nil {
		return "", fmt.Errorf("decode Lima dependency: %w", err)
	}

	if module.Version == "" || module.Replace != nil {
		return "", ErrUnversionedLima
	}

	return module.Version, nil
}

// packageGraph is the agent's dependency graph, not the application's full go.mod.
type packageGraph struct {
	modules        map[string]moduleIdentity
	defaultGODEBUG string
}

func readPackageGraph(data []byte) (packageGraph, error) {
	graph := packageGraph{
		modules: map[string]moduleIdentity{},
	}

	decoder := json.NewDecoder(bytes.NewReader(data))

	for {
		var pkg struct {
			ImportPath     string
			DefaultGODEBUG string
			Module         *moduleInfo
		}

		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			return graph, nil
		}

		if err != nil {
			return packageGraph{}, err
		}

		if pkg.Module != nil {
			graph.modules[pkg.Module.Path] = pkg.Module.identity()
		}

		if pkg.ImportPath == agentPackage {
			graph.defaultGODEBUG = pkg.DefaultGODEBUG
		}
	}
}

func (graph packageGraph) dependencies() []moduleIdentity {
	result := make([]moduleIdentity, 0, len(graph.modules))

	for path, module := range graph.modules {
		if path != limaModulePath {
			result = append(result, module)
		}
	}

	slices.SortFunc(result, func(a, b moduleIdentity) int {
		return strings.Compare(a.Path, b.Path)
	})

	return result
}
