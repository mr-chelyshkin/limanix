package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

func TestUsageAndHelpNeverGenerateFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"help", []string{"--help"}, 0},
		{"unknown flag", []string{"--unknown"}, 2},
		{"missing root", []string{"--root"}, 2},
		{"invalid boolean", []string{"--check=invalid"}, 2},
		{"positional before check", []string{"typo", "--check"}, 2},
		{"positional after check", []string{"--check", "typo"}, 2},
		{"separator positional", []string{"--", "typo"}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			var diagnostics bytes.Buffer
			code := run(test.args, &diagnostics, func(string, bool) error {
				t.Fatal("invalid invocation reached document generation")
				return nil
			})
			if code != test.code || diagnostics.Len() == 0 {
				t.Fatalf("exit = %d, diagnostics = %q", code, diagnostics.String())
			}
		})
	}
}

func TestRunPreservesOptionsAndExitStatus(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		want    options
		failure error
	}{
		{name: "defaults", want: options{root: "."}},
		{name: "check", args: []string{"--check", "--root", "/project with spaces"}, want: options{root: "/project with spaces", check: true}},
		{name: "single dash", args: []string{"-root=project", "-check=true"}, want: options{root: "project", check: true}},
		{name: "explicit false", args: []string{"--check=false"}, want: options{root: "."}},
		{name: "generation failure", args: []string{"--root", "project", "--check"}, want: options{root: "project", check: true}, failure: errors.New("cannot write reference")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var diagnostics bytes.Buffer
			calls := 0
			code := run(test.args, &diagnostics, func(root string, check bool) error {
				calls++
				if root != test.want.root || check != test.want.check {
					t.Fatalf("generation arguments changed: root=%q, check=%v", root, check)
				}
				return test.failure
			})
			wantCode, wantDiagnostics := 0, ""
			if test.failure != nil {
				wantCode, wantDiagnostics = 1, "docsgen: cannot write reference\n"
			}
			if calls != 1 || code != wantCode || diagnostics.String() != wantDiagnostics {
				t.Fatalf("calls=%d, exit=%d, diagnostics=%q", calls, code, diagnostics.String())
			}
		})
	}
}

func TestGeneratedContractAndCLI(t *testing.T) {
	root := t.TempDir()
	if err := generate(root, false); err != nil {
		t.Fatal(err)
	}
	if err := generate(root, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "_generated", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"limanix create", "limanix modules add", "--remove-home", "--config"} {
		if !strings.Contains(string(data), word) {
			t.Fatalf("reference missing %q", word)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "limanix.example.toml"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(root, true); err == nil {
		t.Fatal("stale example accepted")
	}
}

func TestTrackedSandboxExamples(t *testing.T) {
	examples := filepath.Join("..", "..", "examples")
	store, err := state.NewStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	registry := modules.NewRegistry(store, nixos.BuiltinModules())
	directories, err := os.ReadDir(filepath.Join(examples, "modules"))
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		name, err := domain.NewModuleName(directory.Name())
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Add(context.Background(), name, filepath.Join(examples, "modules", directory.Name())); err != nil {
			t.Fatal(err)
		}
	}
	files, err := filepath.Glob(filepath.Join(examples, "*.toml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("sandbox examples are missing: %v", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			cfg, err := config.Load(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, mount := range cfg.Mounts {
				if _, err := filesystem.RequireDirectory(mount.Source); err != nil {
					t.Fatalf("example mount source is unavailable: %v", err)
				}
			}
			sources, err := registry.Sources(context.Background(), cfg.NixOS.Modules)
			if err != nil {
				t.Fatal(err)
			}
			if err := sources.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
