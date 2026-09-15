//go:build !darwin

package lima

import (
	"runtime"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// HostArchitecture returns the supported native process architecture.
func HostArchitecture() (domain.Architecture, error) {
	return domain.NewArchitecture(runtime.GOARCH)
}
