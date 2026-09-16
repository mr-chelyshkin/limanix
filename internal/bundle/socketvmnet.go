package bundle

import (
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// SocketVMNetVersion pins the upstream macOS helper shipped with Limanix.
const SocketVMNetVersion = "1.2.2"

// SocketVMNetTarget identifies an immutable upstream release archive.
// Architecture is the Mac's architecture, not the guest's architecture.
type SocketVMNetTarget struct {
	Architecture domain.Architecture
	Filename     string
	SHA256       string
	Size         int64
}

// SocketVMNetPayload contains the executable and its redistribution license.
// Only the privileged installer writes these bytes to their final host paths.
type SocketVMNetPayload struct {
	Executable []byte
	License    []byte
}

// SocketVMNetTargets returns the release inputs shared by packaging and runtime.
// Digests and sizes come from the upstream v1.2.2 release and SHA256SUMS.
func SocketVMNetTargets() []SocketVMNetTarget {
	return []SocketVMNetTarget{
		{
			Architecture: domain.ARM64,
			Filename:     "socket_vmnet-1.2.2-arm64.tar.gz",
			SHA256:       "c7bf62308fbcfdc29bdfb8373c9b1951f7ac2396446e4390919796a94972e6dc",
			Size:         21254,
		},
		{
			Architecture: domain.AMD64,
			Filename:     "socket_vmnet-1.2.2-x86_64.tar.gz",
			SHA256:       "2968a82c97e692c2d36f87230152e8018e00589c1b598e8257775adfe83800a1",
			Size:         20212,
		},
	}
}

// SocketVMNet verifies the embedded release and selects its native host executable.
// It does not extract files, run the helper, or perform network requests.
func SocketVMNet(architecture domain.Architecture) (SocketVMNetPayload, error) {
	for _, target := range SocketVMNetTargets() {
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
