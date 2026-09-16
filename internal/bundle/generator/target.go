package generator

import "debug/elf"

const (
	limaModulePath = "github.com/lima-vm/lima/v2"
	agentPackage   = limaModulePath + "/cmd/lima-guestagent"
)

// target maps compiler architecture settings to Lima's archive and ELF formats.
type target struct {
	goArch   string
	variant  string
	limaArch string
	machine  elf.Machine
}

func targets() []target {
	return []target{
		{
			goArch:   "arm64",
			limaArch: "aarch64",
			machine:  elf.EM_AARCH64,
			variant:  "GOARM64=v8.0",
		},
		{
			goArch:   "amd64",
			limaArch: "x86_64",
			machine:  elf.EM_X86_64,
			variant:  "GOAMD64=v1",
		},
	}
}

func (t target) archiveName() string {
	return "lima-guestagent.Linux-" + t.limaArch + ".gz"
}

func buildFlags(version string) []string {
	return []string{
		"-buildvcs=false",
		"-buildmode=exe",
		"-mod=readonly",
		"-compiler=gc",
		"-trimpath",
		"-pgo=off",
		"-ldflags=-s -w -X " + limaModulePath + "/pkg/version.Version=" + version,
	}
}
