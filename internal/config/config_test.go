package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

func TestDefaultRoundTripAndIndependentCollections(t *testing.T) {
	expected := Default()
	parsed, err := Parse(nil)
	if err != nil || !reflect.DeepEqual(parsed, expected) {
		t.Fatalf("defaults changed: %#v, %v", parsed, err)
	}
	example, err := RenderExample()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = Parse(example)
	if err != nil || !reflect.DeepEqual(parsed, expected) {
		t.Fatalf("example round trip: %#v, %v", parsed, err)
	}
	parsed.Env["CUSTOM"] = "changed"
	parsed.Mounts[0].Source = "changed"
	parsed.NixOS.Modules = append(parsed.NixOS.Modules, "lmx:changed")
	parsed.Network.Ports.TCP[0] = 9090
	if fresh := Default(); !reflect.DeepEqual(fresh, expected) {
		t.Fatal("default collections are shared")
	}
}

func TestPartialTablesAndExplicitEmptyCollections(t *testing.T) {
	parsed, err := Parse([]byte("name = 'rust-box-2'\nmounts = []\n[resources]\narch = 'amd64'\ncpu = 2\n[nixos]\nmodules = []\n[network.ports]\ntcp = []\nudp = [1,65535]\n[env]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Resources.Arch != domain.AMD64 || parsed.Resources.CPU != 2 || parsed.Resources.Mem != Default().Resources.Mem {
		t.Fatalf("partial resources lost defaults: %#v", parsed.Resources)
	}
	if len(parsed.Env) != 0 || len(parsed.Mounts) != 0 || len(parsed.NixOS.Modules) != 0 || len(parsed.Network.Ports.TCP) != 0 {
		t.Fatalf("empty collections replaced with defaults: %#v", parsed)
	}
	rendered, err := Render(parsed)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Parse(rendered)
	if err != nil || !reflect.DeepEqual(restored, parsed) {
		t.Fatalf("empty collection round trip: %#v, %v", restored, err)
	}
}

func TestStrictFieldAndTypeDiagnostics(t *testing.T) {
	tests := []struct{ input, field string }{
		{"unknown = true", "unknown"},
		{"[resources]\ncpus = 4", "resources.cpus"},
		{"[user]\nuid = 1000", "user.uid"},
		{"[home]\npath = '/opt/limanix'", "home.path"},
		{"[nixos]\nmodule = 'git'", "nixos.module"},
		{"[network]\nbridge = 'en0'", "network.bridge"},
		{"[network.ports]\nhttp = [80]", "network.ports.http"},
		{"[[mounts]]\nwrong = 'mount'", "mounts[0].wrong"},
		{"schema_version = true", "schema_version"},
		{"schema_version = '1'", "schema_version"},
		{"name = 1", "name"},
		{"resources = []", "resources"},
		{"[resources]\ncpu = true", "resources.cpu"},
		{"[resources]\ncpu = 1.0", "resources.cpu"},
		{"[user]\nsudo = 1", "user.sudo"},
		{"[user]\nsudo = 'false'", "user.sudo"},
		{"[network.ports]\ntcp = [true]", "network.ports.tcp[0]"},
		{"[network.ports]\ntcp = '8080'", "network.ports.tcp"},
		{"[nixos]\nmodules = 'git'", "nixos.modules"},
		{"[nixos]\nmodules = [1]", "nixos.modules[0]"},
		{"mounts = {}", "mounts"},
		{"mounts = ['/workspace']", "mounts[0]"},
		{"env = []", "env"},
		{"[env]\nPORT = 8080", "env.PORT"},
		{"[[mounts]]\ntarget='/workspace'", "mounts[0].source"},
		{"[[mounts]]\nsource='./project'", "mounts[0].target"},
	}
	for _, tt := range tests {
		t.Run(tt.field+"/"+tt.input, func(t *testing.T) {
			_, err := Parse([]byte(tt.input))
			if err == nil || !strings.HasPrefix(err.Error(), tt.field+":") {
				t.Fatalf("expected field %s, got %v", tt.field, err)
			}
		})
	}
}

func TestSemanticBoundaries(t *testing.T) {
	tests := []struct{ input, field string }{
		{"schema_version=2", "schema_version"},
		{"name='Box'", "name"},
		{"[resources]\narch='x86_64'", "resources.arch"},
		{"[resources]\ncpu=0", "resources.cpu"},
		{"[resources]\ncpu=-1", "resources.cpu"},
		{"[resources]\nmem='8GB'", "resources.mem"},
		{"[resources]\ndisk='1.5GiB'", "resources.disk"},
		{"[user]\nname='root'", "user.name"},
		{"[user]\nhome='home/dev'", "user.home"},
		{"[user]\nhome='/home/../dev'", "user.home"},
		{"[user]\nhome='/home'", "user.home"},
		{"[home]\nroot=''", "home.root"},
		{"[home]\nroot='/'", "home.root"},
		{"[home]\nroot='/opt/..'", "home.root"},
		{"[network]\nmode='bridged'", "network.mode"},
		{"[network.ports]\ntcp=[0]", "network.ports.tcp[0]"},
		{"[network.ports]\nudp=[65536]", "network.ports.udp[0]"},
		{"[nixos]\nmodules=['./rust.nix']", "nixos.modules[0]"},
		{"[nixos]\nmodules=['third-party:../rust']", "nixos.modules[0]"},
		{"[nixos]\nmodules=['git']", "nixos.modules[0]"},
		{"[nixos]\nmodules=['lmx:git','lmx:git']", "nixos.modules[1]"},
		{"[[mounts]]\nsource=''\ntarget='/workspace'", "mounts[0].source"},
		{"[[mounts]]\nsource='./project'\ntarget='./workspace'", "mounts[0].target"},
		{"[[mounts]]\nsource='./project'\ntarget='/workspace/../etc'", "mounts[0].target"},
		{"[[mounts]]\nsource='./project'\ntarget='/workspace'\nmode='write'", "mounts[0].mode"},
	}
	for _, tt := range tests {
		_, err := Parse([]byte(tt.input))
		if err == nil || !strings.HasPrefix(err.Error(), tt.field+":") {
			t.Errorf("%q: expected %s, got %v", tt.input, tt.field, err)
		}
	}
}

func TestMountPolicyNormalizationAndWhitespace(t *testing.T) {
	for _, root := range []string{"/etc", "/boot", "/usr", "/var", "/nix", "/run", "/dev", "/proc", "/sys", "/bin", "/sbin", "/mnt/limanix", "/home/limanix-admin"} {
		for _, target := range []string{root, root + "/child"} {
			for _, input := range []string{"mounts=[]\n[user]\nhome=" + quoteTOMLString(target), "[[mounts]]\nsource='./project'\ntarget=" + quoteTOMLString(target)} {
				if _, err := Parse([]byte(input)); err == nil {
					t.Errorf("accepted reserved path %q", target)
				}
			}
		}
	}
	for _, whitespace := range []string{" ", "\t", "\n", "\r"} {
		input := "[[mounts]]\nsource='./my project'\ntarget=" + quoteTOMLString("/workspace"+whitespace+"project")
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), "fstab") {
			t.Errorf("accepted unescaped whitespace: %v", err)
		}
	}
	for _, targets := range [][]string{{"/workspace", "/workspace"}, {"/workspace", "/workspace/subdir"}, {"/workspace/subdir", "/workspace"}} {
		input := ""
		for _, target := range targets {
			input += "[[mounts]]\nsource='./project'\ntarget=" + quoteTOMLString(target) + "\n"
		}
		if _, err := Parse([]byte(input)); err == nil || !strings.Contains(err.Error(), "overlaps") {
			t.Errorf("accepted overlap: %v", err)
		}
	}
	parsed, err := Parse([]byte("[home]\nroot='./managed homes'\n[user]\nhome='/home/./dev//'\n[[mounts]]\nsource='~/my project'\ntarget='/workspace//project/./'\n[[mounts]]\nsource='./nvim'\ntarget='/home/dev/.config/nvim'\nmode='ro'\n[[mounts]]\nsource='./service'\ntarget='/opt/service'"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.User.Home != "/home/dev" || parsed.Mounts[0].Target != "/workspace/project" || parsed.Mounts[0].Mode != "rw" || parsed.Home.Root != "./managed homes" || parsed.Mounts[0].Source != "~/my project" {
		t.Fatalf("normalization/default/path contract: %#v", parsed)
	}
}

func TestEnvironmentIsLiteralAndNeverIncludedInErrors(t *testing.T) {
	for _, suffix := range []string{"\x00", "\ufeff"} {
		_, err := Parse([]byte("[env]\nTOKEN=" + quoteTOMLString("never-display-this-value"+suffix)))
		if err == nil || strings.Contains(err.Error(), "never-display-this-value") {
			t.Errorf("unsafe error: %v", err)
		}
	}
	for _, input := range []string{"[env]\nTOKEN = never-display-this-value", "[env]\n'BAD-NAME'='never-display-this-value'"} {
		if _, err := Parse([]byte(input)); err == nil || strings.Contains(err.Error(), "never-display-this-value") {
			t.Errorf("unsafe error: %v", err)
		}
	}
	parsed, err := Parse([]byte("[env]\nEMPTY=''\nTOKEN=" + quoteTOMLString("Привет 🦀 literal $HOME\nwith quotes '\"\t\r\x01\x7f")))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Env) != 2 || parsed.Env["EMPTY"] != "" {
		t.Fatalf("environment defaults merged: %#v", parsed.Env)
	}
	rendered, err := Render(parsed)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Parse(rendered)
	if err != nil || !reflect.DeepEqual(restored, parsed) {
		t.Fatalf("literal TOML round trip: %v", err)
	}
	encoded, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var jsonRestored Config
	if err := json.Unmarshal(encoded, &jsonRestored); err != nil || !reflect.DeepEqual(jsonRestored, parsed) {
		t.Fatalf("public JSON round trip: %v", err)
	}
}

func TestLoadRealFileRelativePathsAndMissingSources(t *testing.T) {
	root := t.TempDir()
	configs := filepath.Join(root, "configs")
	if err := os.Mkdir(configs, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(configs, "box.toml")
	writeFile(t, file, []byte("[home]\nroot='./managed homes'\n[env]\nHOME_TOKEN='$HOME'\n[[mounts]]\nsource='./missing project'\ntarget='/workspace'\n[[mounts]]\nsource='~/$PROJECT'\ntarget='/mnt/other'"))
	link := filepath.Join(root, "link.toml")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECT", "should-not-be-expanded")
	parsed, err := Load(link)
	if err != nil {
		t.Fatal(err)
	}
	realConfigs, err := filepath.EvalSymlinks(configs)
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	realHome, err := filesystem.Resolve(home)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Home.Root != filepath.Join(realConfigs, "managed homes") || parsed.Mounts[0].Source != filepath.Join(realConfigs, "missing project") || parsed.Mounts[1].Source != filepath.Join(realHome, "$PROJECT") || parsed.Env["HOME_TOKEN"] != "$HOME" {
		t.Fatalf("path contract: %#v", parsed)
	}
	rootLink := filepath.Join(configs, "root-link")
	if err := os.Symlink("/", rootLink); err != nil {
		t.Fatal(err)
	}
	writeFile(t, file, []byte("[home]\nroot='./root-link'"))
	if _, err := Load(file); err == nil || !strings.HasPrefix(err.Error(), "home.root:") {
		t.Fatalf("accepted resolved filesystem root: %v", err)
	}
}

func TestLoadRejectsNonRegularAndInvalidFilesWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "bad.toml")
	writeFile(t, binary, []byte{0xff, 0xfe})
	fifo := filepath.Join(root, "pipe")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{filepath.Join(root, "missing"), root, binary, fifo, filepath.Join(root, "nul\x00path")} {
		if _, err := Load(filename); err == nil {
			t.Errorf("accepted invalid file %q", filename)
		}
	}
}

func TestHostPathsResolveSymlinkBeforeParentTraversal(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "actual", "nested")
	if err := os.MkdirAll(actual, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-target", filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "box.toml")
	writeFile(t, file, []byte("[home]\nroot='./homes'\n[[mounts]]\nsource='./alias/../project'\ntarget='/workspace'\n[[mounts]]\nsource='./broken/child'\ntarget='/mnt/other'"))
	parsed, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Mounts[0].Source != filepath.Join(realRoot, "actual", "project") || parsed.Mounts[1].Source != filepath.Join(realRoot, "missing-target", "child") {
		t.Fatalf("symlink traversal contract: %#v", parsed.Mounts)
	}
}

func TestLoadValidatesResolvedHostPathText(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("missing-"+string([]byte{0xff}), filepath.Join(root, "invalid")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-данные", filepath.Join(root, "unicode")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "box.toml")
	for _, test := range []struct {
		name, input, field string
	}{
		{"home root", "[home]\nroot='./invalid'", "home.root"},
		{"mount source", "[home]\nroot='./homes'\n[[mounts]]\nsource='./invalid/child'\ntarget='/workspace'", "mounts[0].source"},
		{"unicode target", "[home]\nroot='./unicode'\n[[mounts]]\nsource='./unicode/child'\ntarget='/workspace'", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeFile(t, file, []byte(test.input))
			loaded, err := Load(file)
			if test.field != "" {
				if err == nil || err.Error() != test.field+": host path cannot be resolved" {
					t.Fatalf("invalid derived host path: %v", err)
				}
				return
			}
			if err != nil || !strings.HasSuffix(loaded.Home.Root, "missing-данные") || !strings.HasSuffix(loaded.Mounts[0].Source, "missing-данные/child") {
				t.Fatalf("Unicode symlink target changed: %#v, %v", loaded, err)
			}
		})
	}
}

func TestReferenceDocumentsModelDefaultsAndMountRequirements(t *testing.T) {
	reference, err := RenderReference()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"## `resources`", "## `network.ports`", "## `mounts`", "~/.limanix", "string (GiB)", "Required", "third-party:NAME", "&#34;rw&#34; or &#34;ro&#34;"} {
		if !strings.Contains(reference, expected) {
			t.Errorf("reference missing %q", expected)
		}
	}
}

func writeFile(t *testing.T, filename string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
