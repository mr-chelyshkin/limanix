package lima

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
)

func TestTemplateBoundaries(t *testing.T) {
	cfg := config.Default()
	cfg.User = config.User{Name: "developer", Home: "/home/developer", Sudo: false}
	cfg.Mounts = []config.Mount{{Source: "/nested", Target: "/workspace/data", Mode: "rw"}, {Source: "/project", Target: "/workspace", Mode: "rw"}, {Source: "/keys", Target: "/mnt/keys", Mode: "ro"}}
	cfg.Env = map[domain.EnvName]domain.EnvValue{"API_TOKEN": "test-secret-absent-from-template"}
	for _, hostUID := range []int{501, 1000} {
		data, err := Render(cfg, "/managed/home", "/managed/runtime", "arm64", hostUID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "API_TOKEN") || strings.Contains(string(data), "test-secret") {
			t.Fatal("secret environment entered Lima template")
		}
		var rendered limatype.LimaYAML
		if err := json.Unmarshal(data, &rendered); err != nil {
			t.Fatal(err)
		}
		if *rendered.VMType != limatype.VZ || *rendered.MountType != limatype.VIRTIOFS || len(rendered.Networks) != 1 || !*rendered.Networks[0].VZNAT {
			t.Fatalf("unexpected native settings: %s", data)
		}
		if int(*rendered.User.UID) == hostUID || !*rendered.User.PasswordlessSudo || *rendered.User.Name != "limanix-admin" {
			t.Fatal("management identity overlaps development identity")
		}
		if *rendered.PropagateProxyEnv || *rendered.SSH.ForwardAgent || !rendered.PortForwards[0].Ignore || *rendered.PortForwards[0].GuestIPMustBeZero {
			t.Fatal("unexpected automatic exposure")
		}
		if len(rendered.Mounts) != 5 {
			t.Fatalf("unexpected implicit mounts: %v", rendered.Mounts)
		}
		parent, nested := -1, -1
		for index, mount := range rendered.Mounts {
			if *mount.MountPoint == "/workspace" {
				parent = index
			}
			if *mount.MountPoint == "/workspace/data" {
				nested = index
			}
			if *mount.MountPoint == "/mnt/keys" && *mount.Writable {
				t.Fatal("readonly mount became writable")
			}
		}
		if parent < 0 || nested < parent {
			t.Fatal("parent mount must precede its child")
		}
	}
}

func TestNativeValidation(t *testing.T) {
	t.Setenv("LIMA_HOME", t.TempDir())
	directory := t.TempDir()
	home := filepath.Join(directory, "home")
	runtime := filepath.Join(directory, "runtime")
	for _, dir := range []string{home, runtime} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Default()
	cfg.Mounts = nil
	data, err := Render(cfg, home, runtime, "arm64", 501)
	if err != nil {
		t.Fatal(err)
	}
	template := filepath.Join(directory, "lima.yaml")
	if err := os.WriteFile(template, data, 0o600); err != nil {
		t.Fatal(err)
	}
	client := NewClient(nil)
	if err := client.Validate(context.Background(), template); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(template, []byte(`{"cpus":-1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := client.Validate(context.Background(), template); err == nil {
		t.Fatal("native Lima schema accepted invalid CPU")
	}
}

func TestArchitectureTemplatesUseHardwareArchitecture(t *testing.T) {
	for _, test := range []struct {
		name   string
		host   domain.Architecture
		guest  domain.Architecture
		vmType limatype.VMType
	}{
		{"Apple Silicon native", domain.ARM64, domain.ARM64, limatype.VZ},
		{"Intel native", domain.AMD64, domain.AMD64, limatype.VZ},
		{"amd64 guest on Apple Silicon", domain.ARM64, domain.AMD64, limatype.QEMU},
		{"arm64 guest on Intel", domain.AMD64, domain.ARM64, limatype.QEMU},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Resources.Arch = test.guest
			data, err := Render(cfg, "/home", "/runtime", test.host, 501)
			if err != nil {
				t.Fatal(err)
			}
			var rendered limatype.LimaYAML
			if err := json.Unmarshal(data, &rendered); err != nil {
				t.Fatal(err)
			}
			if *rendered.VMType != test.vmType || usesVZ(test.guest, test.host) != (test.vmType == limatype.VZ) {
				t.Fatalf("selected incorrect driver: %s", data)
			}
			guestArch, err := test.guest.LimaArch()
			if err != nil || *rendered.Arch != guestArch || rendered.Images[0].Arch != guestArch {
				t.Fatalf("changed guest architecture: %s", data)
			}
			if test.vmType == limatype.QEMU && (*rendered.MountType != limatype.NINEP || rendered.Networks[0].Lima != "shared") {
				t.Fatal("foreign guest requires QEMU/shared")
			}
			image, err := nixos.BaseImage(test.guest)
			if err != nil {
				t.Fatal(err)
			}
			if rendered.Images[0].Location != image.Location || rendered.Images[0].Digest.String() != "sha256:"+image.SHA256 {
				t.Fatal("template image differs from the pinned NixOS base")
			}
		})
	}
	for _, host := range []domain.Architecture{"", "aarch64", "unknown"} {
		if _, err := Render(config.Default(), "/home", "/runtime", host, 501); err == nil {
			t.Fatalf("invalid host architecture accepted: %q", host)
		}
	}
	if usesVZ("unknown", "unknown") {
		t.Fatal("invalid architectures selected VZ")
	}
}
