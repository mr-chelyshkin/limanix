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
	"slices"
	"strings"
)

// goToolchain pins the selected compiler executable and its sanitized environment.
type goToolchain struct {
	executable  string
	root        string
	version     string
	env         []string
	diagnostics io.Writer
}

func resolveToolchain(ctx context.Context, root string, diagnostics io.Writer) (*goToolchain, error) {
	path, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("find Go compiler: %w", err)
	}

	var (
		selected struct {
			GOROOT    string
			GOVERSION string
		}
		tool = &goToolchain{
			executable:  path,
			root:        root,
			diagnostics: diagnostics,
			env:         buildEnvironment(os.Environ()),
		}
	)

	discovery := append(slices.Clone(tool.env), "GOTOOLCHAIN=auto")
	data, err := tool.run(ctx, discovery, "env", "-json", "GOROOT", "GOVERSION")
	if err != nil {
		return nil, fmt.Errorf("resolve Go compiler: %w", err)
	}

	if err = json.Unmarshal(data, &selected); err != nil {
		return nil, fmt.Errorf("decode Go compiler settings: %w", err)
	}

	if !filepath.IsAbs(selected.GOROOT) || selected.GOVERSION == "" {
		return nil, ErrInvalidToolchain
	}

	tool.executable = filepath.Join(selected.GOROOT, "bin", "go")
	tool.env = append(tool.env, "GOROOT="+selected.GOROOT, "GOTOOLCHAIN=local")
	tool.version = selected.GOVERSION
	return tool, nil
}

func (tool *goToolchain) run(ctx context.Context, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, tool.executable, args...)
	cmd.Dir = tool.root
	cmd.Env = tool.env
	if env != nil {
		cmd.Env = env
	}

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		failure := fmt.Errorf("go %s: %w", strings.Join(args, " "), errors.Join(err, ctx.Err()))
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			failure = fmt.Errorf("%w\n%s", failure, detail)
		}

		return nil, failure
	}

	if stderr.Len() != 0 {
		if _, err := io.Copy(tool.diagnostics, &stderr); err != nil {
			return nil, fmt.Errorf("write compiler diagnostics: %w", err)
		}
	}

	return stdout.Bytes(), nil
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
