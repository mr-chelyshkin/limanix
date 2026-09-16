package bundle

import (
	"debug/macho"
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
)

// These are Mach-O format identifiers, not dependency versions. Deployment
// versions come from LC_BUILD_VERSION and Taskfile's macos_version instead.
const (
	machoBuildVersion  = 0x32
	machoPlatformMacOS = 1
)

func validateSocketVMNetDeployment(file *macho.File) error {
	baseline, err := buildinfo.MinimumMacOS()
	if err != nil {
		return err
	}

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

	if buildinfo.MacOSVersion(minimum) > baseline {
		return fmt.Errorf("%w: requires %s, supported baseline is %s",
			ErrVMNetDeployment, buildinfo.MacOSVersion(minimum), baseline)
	}

	return nil
}
