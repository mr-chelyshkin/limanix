package generator

import (
	"fmt"
	"maps"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
)

// moduleIdentity records the version and content checksum of a linked module.
// Local replacements have no content checksum and therefore cannot be reused safely.
type moduleIdentity struct {
	Path    string          `json:"path"`
	Version string          `json:"version"`
	Sum     string          `json:"sum,omitempty"`
	Replace *moduleIdentity `json:"replace,omitempty"`
}

// buildMetadata is read from the executable, then compared with the build plan.
// It deliberately excludes unrelated modules from the host application's go.mod.
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
		replacement := moduleFromBuildInfo(*m.Replace)
		result.Replace = &replacement
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
	slices.SortFunc(result.Dependencies, func(a, b moduleIdentity) int { return strings.Compare(a.Path, b.Path) })
	for _, setting := range info.Settings {
		result.Settings[setting.Key] = setting.Value
	}
	return result
}

// compare reports the first concrete mismatch, for both cache checks and fresh builds.
func (actual buildMetadata) compare(expected buildMetadata) error {
	if actual.Package != expected.Package {
		return fmt.Errorf("package: have %q, want %q", actual.Package, expected.Package)
	}
	if actual.GoVersion != expected.GoVersion {
		return fmt.Errorf("compiler: have %q, want %q", actual.GoVersion, expected.GoVersion)
	}
	if !reflect.DeepEqual(actual.Main, expected.Main) {
		return fmt.Errorf("lima source changed: have %s@%s (%s), want %s@%s (%s)",
			actual.Main.Path, actual.Main.Version, actual.Main.Sum,
			expected.Main.Path, expected.Main.Version, expected.Main.Sum)
	}
	if len(actual.Dependencies) != len(expected.Dependencies) {
		return fmt.Errorf("agent dependency count: have %d, want %d", len(actual.Dependencies), len(expected.Dependencies))
	}
	for i, dep := range expected.Dependencies {
		if !reflect.DeepEqual(actual.Dependencies[i], dep) {
			return fmt.Errorf("agent dependency changed: have %+v, want %+v", actual.Dependencies[i], dep)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(expected.Settings)) {
		have, present := actual.Settings[key]
		if !present {
			return fmt.Errorf("missing build setting %s", key)
		}
		if want := expected.Settings[key]; have != want {
			return fmt.Errorf("build setting %s: have %q, want %q", key, have, want)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(actual.Settings)) {
		if _, expected := expected.Settings[key]; !expected {
			return fmt.Errorf("unexpected build setting %s=%q", key, actual.Settings[key])
		}
	}
	return nil
}

func (build buildMetadata) reusable() error {
	for _, dep := range append([]moduleIdentity{build.Main}, build.Dependencies...) {
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
