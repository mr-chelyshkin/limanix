package generator

import (
	"slices"
	"strings"
)

func buildEnvironment(inherited []string) []string {
	env := make([]string, 0, len(inherited))

	for _, entry := range inherited {
		name, _, _ := strings.Cut(entry, "=")

		switch name {
		case "GOPATH", "GOMODCACHE", "GOCACHE", "GOPROXY", "GOSUMDB", "GOPRIVATE",
			"GONOPROXY", "GONOSUMDB", "GOINSECURE", "GOVCS", "GOAUTH", "GOTELEMETRY":
			env = append(env, entry)
		default:
			if !strings.HasPrefix(name, "GO") && !strings.HasPrefix(name, "CGO_") {
				env = append(env, entry)
			}
		}
	}

	return append(
		env,
		"GOENV=off",
		"GOWORK=off",
		"GO111MODULE=on",
		"GOFLAGS=",
		"GOEXPERIMENT=",
		"GOFIPS140=off",
		"CGO_ENABLED=0",
	)
}

func (tool *goToolchain) targetEnvironment(t target) []string {
	return append(slices.Clone(tool.env), "GOOS=linux", "GOARCH="+t.goArch, t.variant)
}
