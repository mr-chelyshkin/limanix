package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// goToolchain uses one resolved compiler and one environment for queries and builds.
type goToolchain struct {
	executable  string
	root        string
	version     string
	env         []string
	diagnostics io.Writer
}

// resolveToolchain lets Go select the repository's toolchain once, then pins its
// executable and disables further selection for every command in this generation.
func resolveToolchain(ctx context.Context, root string, diagnostics io.Writer) (*goToolchain, error) {
	path, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("find Go compiler: %w", err)
	}
	tool := &goToolchain{
		executable: path, root: root, diagnostics: diagnostics,
		env: append(buildEnvironment(os.Environ()), "GOTOOLCHAIN=auto"),
	}
	data, err := tool.run(ctx, nil, "env", "-json", "GOROOT", "GOVERSION")
	if err != nil {
		return nil, fmt.Errorf("resolve Go compiler: %w", err)
	}
	var selected struct {
		GOROOT    string
		GOVERSION string
	}
	if err := json.Unmarshal(data, &selected); err != nil {
		return nil, fmt.Errorf("decode Go compiler settings: %w", err)
	}
	if !filepath.IsAbs(selected.GOROOT) || selected.GOVERSION == "" {
		return nil, errors.New("go compiler returned an invalid GOROOT or GOVERSION")
	}
	tool.executable = filepath.Join(selected.GOROOT, "bin", "go")
	tool.version = selected.GOVERSION
	tool.env[len(tool.env)-1] = "GOTOOLCHAIN=local"
	tool.env = append(tool.env, "GOROOT="+selected.GOROOT)
	return tool, nil
}

// buildEnvironment removes inherited Go/CGO build controls. Cache, download, and
// authentication settings remain available; persistent go env -w settings do not.
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
	return append(env, "GOENV=off", "GOWORK=off", "GO111MODULE=on", "GOFLAGS=", "GOEXPERIMENT=", "GOFIPS140=off", "CGO_ENABLED=0")
}

func (tool *goToolchain) targetEnvironment(t target) []string {
	return append(append([]string{}, tool.env...), "GOOS=linux", "GOARCH="+t.goArch, t.variant)
}

// run captures query output and retains Go's stderr in failures, including cancellation.
func (tool *goToolchain) run(ctx context.Context, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, tool.executable, args...)
	cmd.Dir = tool.root
	cmd.Env = tool.env
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		failure := fmt.Errorf("go %s: %w", strings.Join(args, " "), errors.Join(err, ctx.Err()))
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			failure = fmt.Errorf("%w\n%s", failure, detail)
		}
		return nil, failure
	}
	if stderr.Len() != 0 {
		_, _ = io.Copy(tool.diagnostics, &stderr)
	}
	return stdout.Bytes(), nil
}

func (tool *goToolchain) limaVersion(ctx context.Context) (string, error) {
	data, err := tool.run(ctx, nil, "list", "-mod=readonly", "-m", "-json", limaModulePath)
	if err != nil {
		return "", fmt.Errorf("resolve Lima dependency: %w", err)
	}
	var module moduleInfo
	if err := json.Unmarshal(data, &module); err != nil {
		return "", fmt.Errorf("decode Lima dependency: %w", err)
	}
	if module.Version == "" || module.Replace != nil {
		return "", errors.New("generation requires an unreplaced, versioned Lima module")
	}
	return module.Version, nil
}

func (tool *goToolchain) build(ctx context.Context, plan buildPlan, destination string) error {
	args := append([]string{"build"}, plan.flags...)
	args = append(args, "-o", destination, agentPackage)
	output, err := tool.run(ctx, tool.targetEnvironment(plan.target), args...)
	if err != nil {
		return err
	}
	_, err = tool.diagnostics.Write(output)
	return err
}
