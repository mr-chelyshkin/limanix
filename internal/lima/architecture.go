package lima

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"golang.org/x/sys/unix"
)

// RequireNativeArchitecture refuses translated VM operations because Lima's native drivers select host capabilities
// using the process architecture.
func RequireNativeArchitecture(host domain.Architecture) error {
	if _, err := domain.NewArchitecture(string(host)); err != nil {
		return fmt.Errorf("invalid host architecture: %w", err)
	}

	if runtime.GOOS != "darwin" {
		return nil
	}

	process, err := domain.NewArchitecture(runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("determine process architecture: %w", err)
	}

	return requireNativeArchitecture(process, host)
}

func requireNativeArchitecture(process, host domain.Architecture) error {
	if process == host {
		return nil
	}

	if process == domain.AMD64 && host == domain.ARM64 {
		return ErrRosetta
	}

	return fmt.Errorf("this Limanix binary is built for %s, but the host is %s; use the matching native binary for VM operations", process, host)
}

func darwinHostArchitecture(process string, translated func() (uint32, error)) (domain.Architecture, error) {
	architecture, err := domain.NewArchitecture(process)
	if err != nil {
		return "", fmt.Errorf("determine process architecture: %w", err)
	}

	if architecture == domain.ARM64 {
		return architecture, nil
	}

	value, err := translated()
	if errors.Is(err, unix.ENOENT) {
		// Intel systems without Rosetta do not expose this sysctl.
		return architecture, nil
	}

	if err != nil {
		return "", fmt.Errorf("determine host architecture using sysctl.proc_translated: %w", err)
	}

	switch value {
	case 0:
		return architecture, nil
	case 1:
		return domain.ARM64, nil
	default:
		return "", fmt.Errorf("unexpected sysctl.proc_translated value: %d", value)
	}
}
