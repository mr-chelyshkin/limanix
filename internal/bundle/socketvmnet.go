package bundle

import (
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// SocketVMNetTarget identifies an immutable upstream release archive.
type SocketVMNetTarget struct {
	Architecture domain.Architecture
	Filename     string
	SHA256       string
	Size         int64
}

// SocketVMNetPayload contains the executable and its redistribution license.
type SocketVMNetPayload struct {
	Executable []byte
	License    []byte
}

// SocketVMNet verifies the embedded release and selects its native host executable.
func SocketVMNet(architecture domain.Architecture) (SocketVMNetPayload, error) {
	targets, err := SocketVMNetTargets()
	if err != nil {
		return SocketVMNetPayload{}, err
	}

	for _, target := range targets {
		if target.Architecture != architecture {
			continue
		}

		archive, err := resources.ReadFile("resources/" + target.Filename)
		if err != nil {
			return SocketVMNetPayload{}, fmt.Errorf("%w: %w", ErrMissingVMNetAssets, err)
		}

		return DecodeSocketVMNet(target, archive)
	}

	return SocketVMNetPayload{}, fmt.Errorf("%w: %s", ErrVMNetArchitecture, architecture)
}
