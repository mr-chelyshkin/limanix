package buildinfo

import (
	"fmt"
	"strconv"
	"strings"
)

// MinimumMacOSVersion is supplied by Taskfile's macos_version through linker -X.
// The same value configures the native compiler, helper generator and tests.
// There is no source default: VM preflight rejects an unconfigured build.
var MinimumMacOSVersion string

// MacOSVersion uses Mach-O's packed major/minor/patch representation.
// Values returned by ParseMacOSVersion can be compared directly with each other
// and with the minimum OS field in LC_BUILD_VERSION.
type MacOSVersion uint32

// MinimumMacOS validates the deployment target supplied when linking this binary.
func MinimumMacOS() (MacOSVersion, error) {
	if MinimumMacOSVersion == "" {
		return 0, ErrMissingPlatform
	}

	version, err := ParseMacOSVersion(MinimumMacOSVersion)
	if err != nil {
		return 0, fmt.Errorf("build minimum: %w", err)
	}

	return version, nil
}

// ParseMacOSVersion accepts major.minor or major.minor.patch within Mach-O's field widths.
func ParseMacOSVersion(value string) (MacOSVersion, error) {
	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("%w: %q (expected major.minor[.patch])", ErrMacOSVersion, value)
	}

	var components [3]uint32
	for i, part := range parts {
		width := 8
		if i == 0 {
			width = 16
		}

		number, err := strconv.ParseUint(part, 10, width)
		if err != nil || strconv.FormatUint(number, 10) != part {
			return 0, fmt.Errorf("%w: %q", ErrMacOSVersion, value)
		}
		components[i] = uint32(number)
	}

	if components[0] == 0 {
		return 0, fmt.Errorf("%w: %q", ErrMacOSVersion, value)
	}

	return MacOSVersion(components[0]<<16 | components[1]<<8 | components[2]), nil
}

// String returns a deployment target, retaining the patch component when nonzero.
func (version MacOSVersion) String() string {
	if patch := version & 0xff; patch != 0 {
		return fmt.Sprintf("%d.%d.%d", version>>16, version>>8&0xff, patch)
	}

	return fmt.Sprintf("%d.%d", version>>16, version>>8&0xff)
}
