package generator

import (
	"fmt"
	"maps"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
)

type moduleIdentity struct {
	Path    string          `json:"path"`
	Version string          `json:"version"`
	Sum     string          `json:"sum,omitempty"`
	Replace *moduleIdentity `json:"replace,omitempty"`
}

type buildMetadata struct {
	Package      string            `json:"package"`
	GoVersion    string            `json:"go_version"`
	Main         moduleIdentity    `json:"main"`
	Dependencies []moduleIdentity  `json:"dependencies"`
	Settings     map[string]string `json:"settings"`
}

func moduleFromBuildInfo(m debug.Module) moduleIdentity {
	result := moduleIdentity{Path: m.Path, Version: m.Version, Sum: m.Sum}
	if m.Replace != nil {
		result.Replace = new(moduleFromBuildInfo(*m.Replace))
	}
	return result
}

func metadataFromBuildInfo(info *debug.BuildInfo) buildMetadata {
	result := buildMetadata{
		Package: info.Path, GoVersion: info.GoVersion, Main: moduleFromBuildInfo(info.Main),
		Dependencies: make([]moduleIdentity, 0, len(info.Deps)), Settings: map[string]string{},
	}
	for _, dep := range info.Deps {
		result.Dependencies = append(result.Dependencies, moduleFromBuildInfo(*dep))
	}

	slices.SortFunc(result.Dependencies, func(a, b moduleIdentity) int {
		return strings.Compare(a.Path, b.Path)
	})
	for _, setting := range info.Settings {
		result.Settings[setting.Key] = setting.Value
	}
	return result
}

func (bm buildMetadata) compare(expected buildMetadata) error {
	if bm.Package != expected.Package {
		return fmt.Errorf("package: have %q, want %q", bm.Package, expected.Package)
	}
	if bm.GoVersion != expected.GoVersion {
		return fmt.Errorf("compiler: have %q, want %q", bm.GoVersion, expected.GoVersion)
	}
	if !reflect.DeepEqual(bm.Main, expected.Main) {
		return fmt.Errorf("lima source changed: have %s@%s (%s), want %s@%s (%s)",
			bm.Main.Path, bm.Main.Version, bm.Main.Sum,
			expected.Main.Path, expected.Main.Version, expected.Main.Sum)
	}
	if len(bm.Dependencies) != len(expected.Dependencies) {
		return fmt.Errorf("agent dependency count: have %d, want %d", len(bm.Dependencies), len(expected.Dependencies))
	}

	for i, dep := range expected.Dependencies {
		if !reflect.DeepEqual(bm.Dependencies[i], dep) {
			return fmt.Errorf("agent dependency changed: have %+v, want %+v", bm.Dependencies[i], dep)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(expected.Settings)) {
		have, present := bm.Settings[key]
		if !present {
			return fmt.Errorf("missing build setting %s", key)
		}
		if want := expected.Settings[key]; have != want {
			return fmt.Errorf("build setting %s: have %q, want %q", key, have, want)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(bm.Settings)) {
		if _, expected := expected.Settings[key]; !expected {
			return fmt.Errorf("unexpected build setting %s=%q", key, bm.Settings[key])
		}
	}
	return nil
}

func (bm buildMetadata) reusable() error {
	for _, dep := range append([]moduleIdentity{bm.Main}, bm.Dependencies...) {
		source := dep

		if dep.Replace != nil {
			source = *dep.Replace
		}
		if source.Sum == "" {
			return fmt.Errorf("module %s has no source checksum; rebuild required", dep.Path)
		}
	}
	return nil
}
