package lima

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/lima-vm/lima/v2/pkg/networks"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Preflight checks prerequisites without starting an instance or installing software.
func (client *Client) Preflight(ctx context.Context, cfg config.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Check the full backend name before allocation; generated IDs have fixed length.
	identity := domain.Identity{
		Name: cfg.Name,
		ID:   strings.Repeat("0", domain.IDLength),
	}

	if err := validateInstanceSocket(identity.LimaName()); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		return ErrMacOSRequired
	}

	hostArch, err := HostArchitecture()
	if err != nil {
		return err
	}

	if err := RequireNativeArchitecture(hostArch); err != nil {
		return err
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("%w: %w", ErrMissingSSH, err)
	}

	query, cancel := queryContext(ctx)
	defer cancel()

	if usesVZ(cfg.Resources.Arch, hostArch) {
		return checkVZ(query)
	}

	return checkQEMU(query, cfg.Resources.Arch)
}

func checkVZ(ctx context.Context) error {
	if !nativeVZAvailable() {
		return ErrMissingVZ
	}

	version, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return fmt.Errorf("determine macOS version: %w", err)
	}

	major, err := strconv.Atoi(strings.SplitN(strings.TrimSpace(string(version)), ".", 2)[0])
	if err != nil || major < 13 {
		return ErrOldMacOS
	}

	return nil
}

func checkQEMU(ctx context.Context, architecture domain.Architecture) error {
	arch, err := architecture.LimaArch()
	if err != nil {
		return err
	}

	executable := "qemu-system-" + arch
	if _, err := exec.LookPath(executable); err != nil {
		return fmt.Errorf("install QEMU and make %s available in PATH: %w", executable, err)
	}

	return checkSharedNetworking(ctx)
}

func checkSharedNetworking(ctx context.Context) error {
	cfg, err := networks.LoadConfig()
	if err != nil {
		return err
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	installed, err := cfg.IsDaemonInstalled(networks.SocketVMNet)
	if err != nil {
		return err
	}

	if !installed {
		return ErrMissingVMNet
	}

	if err := cfg.VerifySudoAccess(ctx, cfg.Paths.Sudoers); err != nil {
		return fmt.Errorf("QEMU shared networking requires Lima's socket_vmnet sudo access: %w", err)
	}

	return ctx.Err()
}
