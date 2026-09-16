package bundle

import (
	"debug/macho"
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
)

// debug/macho leaves LC_BUILD_VERSION as raw bytes. Its layout and platform
// identifiers are defined in Apple's mach-o/loader.h.
const (
	machoBuildVersion  = 0x32
	machoPlatformMacOS = 1
)

func validateSocketVMNetDeployment(file *macho.File) error {
	var minimum uint32

	for _, command := range file.Loads {
		raw := command.Raw()
		if file.ByteOrder.Uint32(raw[:4]) != machoBuildVersion {
			continue
		}

		if len(raw) < 24 || minimum != 0 {
			return fmt.Errorf("%w: invalid LC_BUILD_VERSION", ErrVMNetArchive)
		}

		platform := file.ByteOrder.Uint32(raw[8:12])
		if platform != machoPlatformMacOS {
			return fmt.Errorf("%w: expected macOS platform, found %d", ErrVMNetArchive, platform)
		}

		minimum = file.ByteOrder.Uint32(raw[12:16])
		if minimum == 0 {
			return fmt.Errorf("%w: empty macOS deployment target", ErrVMNetArchive)
		}
	}

	if minimum == 0 {
		return fmt.Errorf("%w: missing macOS deployment target", ErrVMNetArchive)
	}

	if minimum > buildinfo.MinimumMacOSMajor<<16 {
		return fmt.Errorf("%w: requires %d.%d.%d, supported baseline is %d.0",
			ErrVMNetDeployment, minimum>>16, minimum>>8&0xff, minimum&0xff, buildinfo.MinimumMacOSMajor)
	}

	return nil
}
