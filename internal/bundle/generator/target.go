package generator

import "debug/elf"

const (
	// limaModulePath is shared by module resolution and linker flags.
	limaModulePath = "github.com/lima-vm/lima/v2"
	agentPackage   = limaModulePath + "/cmd/lima-guestagent"
)

// target maps Go's build architecture to Lima's archive name and ELF machine.
type target struct {
	goArch   string
	limaArch string
	machine  elf.Machine
	variant  string
}

// targets returns the supported builds in manifest order.
func targets() []target {
	return []target{
		{goArch: "arm64", limaArch: "aarch64", machine: elf.EM_AARCH64, variant: "GOARM64=v8.0"},
		{goArch: "amd64", limaArch: "x86_64", machine: elf.EM_X86_64, variant: "GOAMD64=v1"},
	}
}

func (t target) archiveName() string {
	return "lima-guestagent.Linux-" + t.limaArch + ".gz"
}

// buildFlags fixes the compilation policy, including Lima's reported version.
// Linker flags are recorded separately: Go omits them from build info with -trimpath.
func buildFlags(version string) []string {
	return []string{
		"-mod=readonly",
		"-trimpath",
		"-buildvcs=false",
		"-buildmode=exe",
		"-compiler=gc",
		"-pgo=off",
		"-ldflags=-s -w -X " + limaModulePath + "/pkg/version.Version=" + version,
	}
}
