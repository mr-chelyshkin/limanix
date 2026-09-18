package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/vm"
)

type managerCall struct {
	operation         string
	name              domain.VMName
	path              string
	force, removeHome bool
	args              []string
}
type fakeManager struct {
	calls       []managerCall
	entries     []vm.Info
	failure     error
	shellStatus int
	create      func(context.Context) (domain.Instance, error)
	shell       func(context.Context) (int, error)
}

func (manager *fakeManager) Create(ctx context.Context, path string) (domain.Instance, error) {
	manager.calls = append(manager.calls, managerCall{operation: "create", path: path})
	if manager.create != nil {
		return manager.create(ctx)
	}
	return domain.Instance{Identity: domain.Identity{Name: "sandbox", Home: "/managed/home"}}, manager.failure
}

func (manager *fakeManager) Update(_ context.Context, path string) (domain.Instance, error) {
	manager.calls = append(manager.calls, managerCall{operation: "update", path: path})
	return domain.Instance{Identity: domain.Identity{Name: "sandbox"}}, manager.failure
}

func (manager *fakeManager) FetchAll(context.Context) ([]vm.Info, error) {
	manager.calls = append(manager.calls, managerCall{operation: "list"})
	return manager.entries, manager.failure
}

func (manager *fakeManager) Start(_ context.Context, name domain.VMName) error {
	manager.calls = append(manager.calls, managerCall{operation: "start", name: name})
	return manager.failure
}

func (manager *fakeManager) Stop(_ context.Context, name domain.VMName) error {
	manager.calls = append(manager.calls, managerCall{operation: "stop", name: name})
	return manager.failure
}

func (manager *fakeManager) Delete(_ context.Context, name domain.VMName, force, removeHome bool) (string, error) {
	manager.calls = append(manager.calls, managerCall{operation: "delete", name: name, force: force, removeHome: removeHome})
	return "/managed/home", manager.failure
}

func (manager *fakeManager) Shell(ctx context.Context, name domain.VMName, args []string) (int, error) {
	manager.calls = append(manager.calls, managerCall{operation: "shell", name: name, args: args})
	if manager.shell != nil {
		return manager.shell(ctx)
	}
	return manager.shellStatus, manager.failure
}

type registryCall struct {
	operation string
	name      domain.ModuleName
	path      string
}
type fakeRegistry struct {
	calls   []registryCall
	entries []modules.Info
	failure error
}

func (registry *fakeRegistry) Available(context.Context) ([]modules.Info, error) {
	registry.calls = append(registry.calls, registryCall{operation: "list"})
	return registry.entries, registry.failure
}

func (registry *fakeRegistry) Add(_ context.Context, name domain.ModuleName, path string) error {
	registry.calls = append(registry.calls, registryCall{operation: "add", name: name, path: path})
	return registry.failure
}

func (registry *fakeRegistry) Remove(_ context.Context, name domain.ModuleName) error {
	registry.calls = append(registry.calls, registryCall{operation: "remove", name: name})
	return registry.failure
}

func runCLI(args []string, dependencies Dependencies) (int, string, string) {
	var output, diagnostics bytes.Buffer
	status := Execute(context.Background(), args, IO{In: strings.NewReader(""), Out: &output, Err: &diagnostics}, dependencies)
	return status, output.String(), diagnostics.String()
}

func fakeDependencies(manager *fakeManager, registry *fakeRegistry) Dependencies {
	return Dependencies{Manager: func() (Manager, error) { return manager, nil }, Registry: func() (Registry, error) { return registry, nil }}
}

func TestHelpVersionAndReferenceDoNotInitializeHostServices(t *testing.T) {
	dependencies := Dependencies{Manager: func() (Manager, error) { t.Fatal("help initialized VM services"); return nil, nil }, Registry: func() (Registry, error) { t.Fatal("help initialized module services"); return nil, nil }}
	for _, args := range [][]string{nil, {"--help"}, {"help", "create"}, {"create", "--help"}, {"modules", "--help"}, {"shell", "--help"}, {"shell", "-h"}} {
		status, output, diagnostics := runCLI(args, dependencies)
		if status != 0 || !strings.Contains(output, "Usage:") || diagnostics != "" {
			t.Fatalf("help %v failed: %d %q %q", args, status, output, diagnostics)
		}
	}
	status, output, diagnostics := runCLI([]string{"--version"}, dependencies)
	if status != 0 || output != buildinfo.Version+"\n" || diagnostics != "" {
		t.Fatalf("version failed: %d %q %q", status, output, diagnostics)
	}
	reference := Reference()
	if strings.Contains(reference, "hostagent") || strings.Contains(reference, "--debug") {
		t.Fatal("internal Lima subprocess interface entered the public reference")
	}
	for _, fragment := range []string{"`limanix create`", "--config", "`limanix update`", "`limanix modules add`", "--remove-home", "shell NAME [-- COMMAND ...]"} {
		if !strings.Contains(reference, fragment) {
			t.Fatalf("actual CLI missing from reference: %s", fragment)
		}
	}
}

func TestUsageFailuresReturnTwoBeforeInitializingDependencies(t *testing.T) {
	dependencies := Dependencies{Manager: func() (Manager, error) { t.Fatal("invalid usage initialized VM services"); return nil, nil }, Registry: func() (Registry, error) { t.Fatal("invalid usage initialized module services"); return nil, nil }}
	for _, args := range [][]string{{"unknown"}, {"--unknown"}, {"list", "--unknown"}, {"list", "extra"}, {"create"}, {"create", "--config"}, {"create", "--config=", "extra"}, {"create", "name", "--config=cfg.toml"}, {"update"}, {"update", "name", "--config=cfg.toml"}, {"start"}, {"stop", "one", "two"}, {"delete"}, {"delete", "name", "--force=invalid"}, {"shell"}, {"first-config", "one", "two"}, {"modules"}, {"modules", "unknown"}, {"modules", "add", "one"}, {"modules", "remove", "one", "two"}, {"modules", "list", "--unknown"}} {
		status, output, diagnostics := runCLI(args, dependencies)
		if status != 2 || output != "" || !strings.HasPrefix(diagnostics, "limanix: ") || strings.Contains(diagnostics, "warning:") {
			t.Fatalf("usage %v returned %d %q %q", args, status, output, diagnostics)
		}
	}
}

func TestOperationAndDependencyFailuresReturnOne(t *testing.T) {
	manager := &fakeManager{failure: errors.New("VM unavailable")}
	registry := &fakeRegistry{failure: errors.New("registry unavailable")}
	for _, args := range [][]string{{"create", "--config=cfg.toml"}, {"update", "--config=cfg.toml"}, {"list"}, {"start", "sandbox"}, {"stop", "sandbox"}, {"delete", "sandbox"}, {"shell", "sandbox"}, {"modules", "list"}, {"modules", "add", "custom", "/source"}, {"modules", "remove", "custom"}} {
		status, output, diagnostics := runCLI(args, fakeDependencies(manager, registry))
		if status != 1 || output != "" || !strings.HasPrefix(diagnostics, "limanix: ") || strings.Contains(diagnostics, "warning:") {
			t.Fatalf("operation %v returned %d %q %q", args, status, output, diagnostics)
		}
	}
	status, _, diagnostics := runCLI([]string{"list"}, Dependencies{Manager: func() (Manager, error) { return nil, errors.New("cannot open state") }})
	if status != 1 || diagnostics != "limanix: cannot open state\n" {
		t.Fatalf("dependency failure lost: %d %q", status, diagnostics)
	}
	manager.failure = &config.Error{Field: "resources.cpu", Message: "expected positive integer"}
	status, _, _ = runCLI([]string{"create", "--config=cfg.toml"}, fakeDependencies(manager, registry))
	if status != 1 {
		t.Fatal("configuration operation exit status changed")
	}
}

func TestVMCommandsPassConfigurationAndDeleteFlags(t *testing.T) {
	manager := &fakeManager{}
	dependencies := fakeDependencies(manager, &fakeRegistry{})
	for _, args := range [][]string{{"create", "--config", "/project/VM config.toml"}, {"update", "--config=/project/updated.toml"}, {"start", "sandbox"}, {"stop", "sandbox"}} {
		status, _, diagnostics := runCLI(args, dependencies)
		if status != 0 || diagnostics != "" {
			t.Fatalf("operation failed: %d %q", status, diagnostics)
		}
	}
	if !reflect.DeepEqual(manager.calls, []managerCall{{operation: "create", path: "/project/VM config.toml"}, {operation: "update", path: "/project/updated.toml"}, {operation: "start", name: "sandbox"}, {operation: "stop", name: "sandbox"}}) {
		t.Fatalf("VM arguments changed: %#v", manager.calls)
	}
	for _, flags := range [][]string{nil, {"--force"}, {"--remove-home"}, {"--force", "--remove-home"}} {
		args := append([]string{"delete", "sandbox"}, flags...)
		status, output, diagnostics := runCLI(args, dependencies)
		if status != 0 || diagnostics != "" {
			t.Fatalf("delete failed: %d %q", status, diagnostics)
		}
		last := manager.calls[len(manager.calls)-1]
		force, removeHome := false, false
		for _, flag := range flags {
			if flag == "--force" {
				force = true
			}
			if flag == "--remove-home" {
				removeHome = true
			}
		}
		if last.force != force || last.removeHome != removeHome || strings.Contains(output, "Preserved managed home:") == removeHome {
			t.Fatalf("force/remove-home conflated: %#v %q", last, output)
		}
	}
}

func TestShellPreservesLiteralArgumentsFlagsAndChildExit(t *testing.T) {
	values := []string{"printf", "--help", "--unknown", "-n", `spaces; $(touch unwanted) "quotes"`, "", "line one\nline two", "--"}
	for _, separator := range [][]string{nil, {"--"}} {
		manager := &fakeManager{shellStatus: 17}
		args := append([]string{"shell", "sandbox"}, separator...)
		args = append(args, values...)
		status, output, diagnostics := runCLI(args, fakeDependencies(manager, &fakeRegistry{}))
		if status != 17 || output != "" || diagnostics != "" || !reflect.DeepEqual(manager.calls[0].args, values) {
			t.Fatalf("guest command changed: %d %q %q %#v", status, output, diagnostics, manager.calls)
		}
	}
	manager := &fakeManager{}
	status, _, _ := runCLI([]string{"shell", "sandbox", "--"}, fakeDependencies(manager, &fakeRegistry{}))
	if status != 0 || len(manager.calls[0].args) != 0 {
		t.Fatal("empty command must attach interactive shell")
	}
}

func TestListJSONEmptyArraysAndDamagedRows(t *testing.T) {
	manager := &fakeManager{}
	registry := &fakeRegistry{}
	for _, args := range [][]string{{"list", "--json"}, {"modules", "list", "--json"}} {
		status, output, diagnostics := runCLI(args, fakeDependencies(manager, registry))
		if status != 0 || output != "[]\n" || diagnostics != "" {
			t.Fatalf("empty list must be JSON array: %d %q %q", status, output, diagnostics)
		}
	}
	ready := domain.Ready
	running := lima.Running
	failure := "identity.json is damaged"
	home := "/managed/home"
	manager.entries = []vm.Info{{Name: "healthy", OperationStatus: &ready, BackendStatus: &running, Home: &home, Address: "192.0.2.10"}, {Name: "damaged", Error: &failure}}
	status, output, diagnostics := runCLI([]string{"list", "--json"}, fakeDependencies(manager, registry))
	if status != 0 || diagnostics != "" {
		t.Fatalf("damaged row blocked list: %d %q", status, diagnostics)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["status"] != "Running" || rows[1]["error"] != failure || rows[1]["state"] != nil {
		t.Fatalf("damaged row JSON lost: %s", output)
	}
	status, output, diagnostics = runCLI([]string{"list"}, fakeDependencies(manager, registry))
	if status != 0 || diagnostics != "" || !strings.Contains(output, "healthy") || !strings.Contains(output, "damaged") || !strings.Contains(output, "corrupt") || !strings.Contains(output, failure) {
		t.Fatalf("damaged table row lost: %d %q %q", status, output, diagnostics)
	}
	registry.entries = []modules.Info{{Name: "git", Source: "bundled", Description: "Git version control."}, {Name: "third-party:broken", Source: "third-party", Error: &failure}}
	status, output, diagnostics = runCLI([]string{"modules", "list", "--json"}, fakeDependencies(manager, registry))
	if status != 0 || diagnostics != "" {
		t.Fatal("module list failed")
	}
	rows = nil
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["name"] != "git" || rows[1]["error"] != failure {
		t.Fatalf("module JSON contract changed: %s", output)
	}
}

func TestModuleCommandsPassNamesAndSourceDirectory(t *testing.T) {
	registry := &fakeRegistry{}
	dependencies := fakeDependencies(&fakeManager{}, registry)
	for _, args := range [][]string{{"modules", "add", "custom", "/path with spaces/module"}, {"modules", "remove", "custom"}} {
		status, _, diagnostics := runCLI(args, dependencies)
		if status != 0 || diagnostics != "" {
			t.Fatal("module command failed")
		}
	}
	if !reflect.DeepEqual(registry.calls, []registryCall{{operation: "add", name: "custom", path: "/path with spaces/module"}, {operation: "remove", name: "custom"}}) {
		t.Fatalf("module command arguments changed: %#v", registry.calls)
	}
}

func TestFirstConfigCreatesRewritesAndRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	dependencies := fakeDependencies(&fakeManager{}, &fakeRegistry{})
	status, output, diagnostics := runCLI([]string{"first-config", directory}, dependencies)
	if status != 0 || diagnostics != "" || !strings.HasPrefix(output, "Wrote ") {
		t.Fatalf("config write failed: %d %q %q", status, output, diagnostics)
	}
	path := filepath.Join(directory, "limanix.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Parse(data); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("new configuration must be private")
	}
	if err := os.WriteFile(path, []byte("old contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	status, _, _ = runCLI([]string{"first-config", directory}, dependencies)
	if status != 0 {
		t.Fatal("rewrite failed")
	}
	rewritten, err := os.ReadFile(path)
	if err != nil || string(rewritten) != string(data) {
		t.Fatal("existing configuration was not replaced")
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatal("rewrite must preserve permissions")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "target.toml")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	status, _, diagnostics = runCLI([]string{"first-config", directory}, dependencies)
	if status != 1 || !strings.HasPrefix(diagnostics, "limanix: ") {
		t.Fatalf("configuration symlink accepted: %d %q", status, diagnostics)
	}
	targetData, err := os.ReadFile(target)
	if err != nil || string(targetData) != "keep" {
		t.Fatal("symlink target was overwritten")
	}
}

func TestDefaultManagerWarnUsesConfiguredDiagnosticStream(t *testing.T) {
	t.Setenv("LIMANIX_HOME", t.TempDir())
	var diagnostics bytes.Buffer
	services := withDefaultDependencies(IO{In: strings.NewReader(""), Out: io.Discard, Err: &diagnostics}, Dependencies{})
	manager, err := services.Manager()
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Len() != 0 {
		t.Fatal("constructing manager emitted an unsolicited warning")
	}
	manager.(*vm.Manager).Warn("VM %s was updated, but old generations could not be removed", "sandbox")
	if diagnostics.String() != "limanix: warning: VM sandbox was updated, but old generations could not be removed\n" {
		t.Fatalf("warning prefix missing: %q", diagnostics.String())
	}
}

func TestCLIHelper(t *testing.T) {
	mode := os.Getenv("LIMANIX_CLI_HELPER")
	if mode == "" {
		return
	}
	manager := &fakeManager{}
	args := []string{"shell", "sandbox"}
	if mode == "shell-sigint" {
		manager.shell = func(ctx context.Context) (int, error) {
			if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
				return 0, err
			}
			time.Sleep(50 * time.Millisecond)
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			return 17, nil
		}
	} else {
		args = []string{"create", "--config=cfg.toml"}
		manager.create = func(ctx context.Context) (domain.Instance, error) {
			if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
				return domain.Instance{}, err
			}
			select {
			case <-ctx.Done():
				return domain.Instance{}, ctx.Err()
			case <-time.After(time.Second):
				return domain.Instance{}, errors.New("SIGINT was not propagated")
			}
		}
	}
	os.Exit(Execute(context.Background(), args, IO{In: strings.NewReader(""), Out: os.Stdout, Err: os.Stderr}, fakeDependencies(manager, &fakeRegistry{})))
}

func TestSignalsCancelOperationsButKeepInteractiveSSHContext(t *testing.T) {
	for mode, expected := range map[string]int{"shell-sigint": 17, "operation-sigint": 130} {
		command := exec.Command(os.Args[0], "-test.run=TestCLIHelper")
		command.Env = append(os.Environ(), "LIMANIX_CLI_HELPER="+mode, "GORACE="+os.Getenv("GORACE")+" atexit_sleep_ms=0")
		var diagnostics bytes.Buffer
		command.Stderr = &diagnostics
		err := command.Run()
		if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != expected {
			t.Fatalf("signal handling %s changed: %v %q", mode, err, diagnostics.String())
		}
		if mode == "shell-sigint" && diagnostics.Len() != 0 {
			t.Fatalf("shell child exit emitted CLI error: %q", diagnostics.String())
		}
		if mode == "operation-sigint" && diagnostics.String() != "limanix: interrupted.\n" {
			t.Fatalf("interruption diagnostic changed: %q", diagnostics.String())
		}
	}
}
