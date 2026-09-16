package lima

import (
	"encoding/json"
	"net"
	"sort"
	"strings"

	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/ptr"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
	"github.com/opencontainers/go-digest"
)

// Render uses Lima's schema types, emitting JSON (a YAML subset) without secret ENV values.
func Render(cfg config.Config, managedHome, runtimeDir string, hostArch domain.Architecture, hostUID int) ([]byte, error) {
	if _, err := domain.NewArchitecture(string(hostArch)); err != nil {
		return nil, err
	}

	arch, err := cfg.Resources.Arch.LimaArch()
	if err != nil {
		return nil, err
	}

	memory, err := cfg.Resources.Mem.GiB()
	if err != nil {
		return nil, err
	}

	disk, err := cfg.Resources.Disk.GiB()
	if err != nil {
		return nil, err
	}

	if hostUID <= 0 || managedHome == "" || runtimeDir == "" {
		return nil, ErrManagedMounts
	}

	image, err := nixos.BaseImage(cfg.Resources.Arch)
	if err != nil {
		return nil, err
	}

	document := machineTemplate(cfg.Resources.Arch, hostArch)
	document.Arch = &arch
	document.Images = []limatype.Image{{
		File: limatype.File{
			Location: image.Location,
			Arch:     arch,
			Digest:   digest.NewDigestFromEncoded(digest.SHA256, image.SHA256),
		},
	}}
	document.CPUs = ptr.Of(cfg.Resources.CPU)
	document.Memory = &memory
	document.Disk = &disk
	document.User = managementUser(hostUID)
	document.Mounts = templateMounts(cfg, managedHome, runtimeDir)

	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(encoded, '\n'), nil
}

// usesVZ selects Apple's native hypervisor only for the hardware architecture.
func usesVZ(architecture, host domain.Architecture) bool {
	switch architecture {
	case domain.ARM64, domain.AMD64:
		return architecture == host
	default:
		return false
	}
}

func machineTemplate(architecture, host domain.Architecture) limatype.LimaYAML {
	document := limatype.LimaYAML{
		VMType:    ptr.Of(limatype.QEMU),
		MountType: ptr.Of(limatype.NINEP),
		Networks: []limatype.Network{
			{Lima: "shared"},
		},
		SSH: limatype.SSH{
			LoadDotSSHPubKeys: ptr.Of(false),
			ForwardAgent:      ptr.Of(false),
		},
		PropagateProxyEnv: ptr.Of(false),
		PortForwards: []limatype.PortForward{
			{
				GuestIP:           net.IPv4zero,
				GuestIPMustBeZero: ptr.Of(false),
				GuestPortRange:    [2]int{1, 65535},
				Proto:             limatype.ProtoAny,
				Ignore:            true,
			},
		},
		Containerd: limatype.Containerd{
			System: ptr.Of(false),
			User:   ptr.Of(false),
		},
	}

	if usesVZ(architecture, host) {
		document.VMType = ptr.Of(limatype.VZ)
		document.MountType = ptr.Of(limatype.VIRTIOFS)
		document.Networks = []limatype.Network{
			{VZNAT: ptr.Of(true)},
		}
	}

	return document
}

func managementUser(hostUID int) limatype.User {
	adminUID := uint32(1000)
	if hostUID == 1000 {
		adminUID = 1001
	}

	return limatype.User{
		Name:             ptr.Of("limanix-admin"),
		Home:             ptr.Of("/home/limanix-admin"),
		UID:              &adminUID,
		PasswordlessSudo: ptr.Of(true),
	}
}

func templateMounts(cfg config.Config, managedHome, runtimeDir string) []limatype.Mount {
	mounts := []limatype.Mount{
		{
			Location:   managedHome,
			MountPoint: ptr.Of(string(cfg.User.Home)),
			Writable:   ptr.Of(true),
		},
		{
			Location:   runtimeDir,
			MountPoint: ptr.Of("/mnt/limanix"),
			Writable:   ptr.Of(false),
		},
	}

	for _, mount := range cfg.Mounts {
		mounts = append(mounts, limatype.Mount{
			Location:   mount.Source,
			MountPoint: ptr.Of(string(mount.Target)),
			Writable:   ptr.Of(mount.Mode == "rw"),
		})
	}

	sort.SliceStable(mounts, func(i, j int) bool {
		return strings.Count(*mounts[i].MountPoint, "/") < strings.Count(*mounts[j].MountPoint, "/")
	})

	return mounts
}
