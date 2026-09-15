package lima

import (
	"encoding/json"
	"errors"
	"net"
	"sort"
	"strings"

	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/ptr"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/opencontainers/go-digest"
)

const imageRelease = "https://github.com/nixos-lima/nixos-lima/releases/download/v0.2.1"

// usesVZ selects Apple's native hypervisor only for the hardware architecture.
func usesVZ(architecture, host domain.Architecture) bool {
	switch architecture {
	case domain.ARM64, domain.AMD64:
		return architecture == host
	default:
		return false
	}
}

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
		return nil, errors.New("a positive host UID and managed mount paths are required")
	}
	imageDigest := "ebdf8363bcb51542892963790c08ddfbffe45443ab19c5e70275c4f0c0aa6f11"
	if arch == limatype.X8664 {
		imageDigest = "967da3baf4ea410e728c751ca9e0a617299b6809297e4e50e773cae4ce79197d"
	}
	native := usesVZ(cfg.Resources.Arch, hostArch)
	vmType, mountType := limatype.QEMU, limatype.NINEP
	networks := []limatype.Network{{Lima: "shared"}}
	if native {
		vmType, mountType = limatype.VZ, limatype.VIRTIOFS
		networks = []limatype.Network{{VZNAT: ptr.Of(true)}}
	}
	adminUID := uint32(1000)
	if hostUID == 1000 {
		adminUID = 1001
	}
	mounts := []limatype.Mount{
		{Location: managedHome, MountPoint: ptr.Of(string(cfg.User.Home)), Writable: ptr.Of(true)},
		{Location: runtimeDir, MountPoint: ptr.Of("/mnt/limanix"), Writable: ptr.Of(false)},
	}
	for _, mount := range cfg.Mounts {
		mounts = append(mounts, limatype.Mount{Location: mount.Source, MountPoint: ptr.Of(string(mount.Target)), Writable: ptr.Of(mount.Mode == "rw")})
	}
	sort.SliceStable(mounts, func(i, j int) bool {
		return strings.Count(*mounts[i].MountPoint, "/") < strings.Count(*mounts[j].MountPoint, "/")
	})
	document := limatype.LimaYAML{
		VMType: &vmType, Arch: &arch,
		Images: []limatype.Image{{File: limatype.File{Location: imageRelease + "/nixos-lima-v0.2.1-" + arch + ".qcow2", Arch: arch, Digest: digest.Digest("sha256:" + imageDigest)}}},
		CPUs:   ptr.Of(cfg.Resources.CPU), Memory: &memory, Disk: &disk,
		User:      limatype.User{Name: ptr.Of("limanix-admin"), Home: ptr.Of("/home/limanix-admin"), UID: &adminUID, PasswordlessSudo: ptr.Of(true)},
		MountType: &mountType, Mounts: mounts, Networks: networks,
		SSH:               limatype.SSH{LoadDotSSHPubKeys: ptr.Of(false), ForwardAgent: ptr.Of(false)},
		PropagateProxyEnv: ptr.Of(false),
		PortForwards:      []limatype.PortForward{{GuestIP: net.IPv4zero, GuestIPMustBeZero: ptr.Of(false), GuestPortRange: [2]int{1, 65535}, Proto: limatype.ProtoAny, Ignore: true}},
		Containerd:        limatype.Containerd{System: ptr.Of(false), User: ptr.Of(false)},
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
