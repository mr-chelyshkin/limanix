package nixos

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/modules"
)

func TestEmbeddedModulesAndPinnedBaseCopied(t *testing.T) {
	cfg := config.Default()
	sources := []modules.Source{{ID: "git"}, {ID: "rust"}, {ID: "neovim"}}
	flake, err := Prepare(cfg, filepath.Join(t.TempDir(), "runtime"), sources, 501)
	if err != nil {
		t.Fatal(err)
	}
	for index, source := range sources {
		original, err := resources.ReadFile("resources/modules/" + string(source.ID) + "/default.nix")
		if err != nil {
			t.Fatal(err)
		}
		copied, err := os.ReadFile(filepath.Join(flake, "modules", []string{"0000", "0001", "0002"}[index], "default.nix"))
		if err != nil {
			t.Fatal(err)
		}
		if string(copied) != string(original) {
			t.Fatal("embedded module changed while copying")
		}
	}
	lock, err := os.ReadFile(filepath.Join(flake, "flake.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lock), `"ref": "v0.2.1"`) || strings.Contains(string(lock), "runtimeSpec") {
		t.Fatal("base flake lock differs from pinned contract")
	}
	metadata := BuiltinModules()
	metadata["git"] = "changed"
	if BuiltinModules()["git"] == "changed" {
		t.Fatal("caller modified module metadata")
	}
}

func TestBundleSnapshotsWholeModuleTreeAndKeepsSecretsOutsideFlake(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "module")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	for file, content := range map[string]string{"default.nix": "{...}: { imports = [ ./nested/feature.nix ]; }\n", "nested/feature.nix": "{...}: {}\n"} {
		if err := os.WriteFile(filepath.Join(source, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Default()
	values := map[domain.EnvName]domain.EnvValue{"TOKEN": "test-secret-absent-from-flake", "LIMANIX_TEST_EMPTY": "", "LIMANIX_TEST_QUOTES": "single ' and double \"", "LIMANIX_TEST_LITERAL": "$(touch injected) `touch injected` $HOME", "LIMANIX_TEST_SPACES": "  a\tb  ", "LIMANIX_TEST_LINES": "line one\nline two\r\nend\n", "LIMANIX_TEST_BACKSLASH": "a\\b\\n\\\nlast\\", "LIMANIX_TEST_UNICODE": "Привет ✓"}
	cfg.Env = values
	runtimeDir := filepath.Join(directory, "runtime")
	flake, err := Prepare(cfg, runtimeDir, []modules.Source{{ID: "third-party:custom", Path: source}}, 501)
	if err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(flake, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "test-secret-absent-from-flake") {
			t.Fatalf("ENV secret copied to flake: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(flake, "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	var runtime struct {
		Modules []string `json:"modules"`
		User    struct {
			UID int `json:"uid"`
		} `json:"user"`
	}
	if err := json.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runtime.Modules, []string{"modules/0000/default.nix"}) || runtime.User.UID != 501 {
		t.Fatalf("wrong runtime metadata: %s", data)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "feature.nix"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(filepath.Join(flake, "modules", "0000", "nested", "feature.nix"))
	if err != nil || string(snapshot) != "{...}: {}\n" {
		t.Fatal("generation module changed with registry source")
	}
	for _, file := range []string{"environment", "environment.sh"} {
		info, err := os.Stat(filepath.Join(runtimeDir, file))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("runtime ENV must be private on host")
		}
	}
	if _, err := Prepare(cfg, runtimeDir, nil, 501); err == nil {
		t.Fatal("existing bundle overwritten")
	}
	process := exec.Command("/bin/sh", "-c", `. "$1"; /usr/bin/env -0`, "sh", filepath.Join(runtimeDir, "environment.sh"))
	process.Dir = runtimeDir
	process.Env = []string{"PATH=/usr/bin:/bin"}
	output, err := process.Output()
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string]string{}
	for _, item := range strings.Split(string(output), "\x00") {
		if item == "" {
			continue
		}
		name, value, _ := strings.Cut(item, "=")
		actual[name] = value
	}
	for name, value := range values {
		if actual[string(name)] != string(value) {
			t.Fatalf("ENV literal changed: %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "injected")); !os.IsNotExist(err) {
		t.Fatal("environment performed command substitution")
	}
}

func TestInvalidEnvironmentRejectedBeforeWriting(t *testing.T) {
	for _, values := range []map[domain.EnvName]domain.EnvValue{{"X;touch pwned": "x"}, {"X": "nul\x00value"}, {"X": "\ufeff"}} {
		cfg := config.Default()
		cfg.Env = values
		directory := filepath.Join(t.TempDir(), "absent")
		if _, err := Prepare(cfg, directory, nil, 501); err == nil {
			t.Fatal("invalid environment accepted")
		}
		if _, err := os.Stat(directory); !os.IsNotExist(err) {
			t.Fatal("invalid environment staged files before validation")
		}
	}
}
