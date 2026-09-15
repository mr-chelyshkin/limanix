package lima

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/lima-vm/lima/v2/pkg/networks"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

// Preflight checks prerequisites without starting an instance or installing software.
func (client *Client) Preflight(ctx context.Context, cfg config.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// All generated IDs have the same length; preserve the identity's single
	// source of truth for the complete backend name before allocation.
	name := (state.Identity{Name: cfg.Name, ID: strings.Repeat("0", state.IDLength)}).LimaName()
	if err := validateInstanceSocket(name); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return errors.New("limanix VM operations require macOS")
	}
	hostArch, err := HostArchitecture()
	if err != nil {
		return err
	}
	if err := RequireNativeArchitecture(hostArch); err != nil {
		return err
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		return errors.New("make the system SSH client available in PATH")
	}
	query, cancel := queryContext(ctx)
	defer cancel()
	if usesVZ(cfg.Resources.Arch, hostArch) {
		if !nativeVZAvailable() {
			return errors.New("this Limanix build does not include the native macOS VZ driver")
		}
		version, err := exec.CommandContext(query, "sw_vers", "-productVersion").Output()
		if err != nil {
			if query.Err() != nil {
				return query.Err()
			}
			return fmt.Errorf("determine macOS version: %w", err)
		}
		major, err := strconv.Atoi(strings.SplitN(strings.TrimSpace(string(version)), ".", 2)[0])
		if err != nil || major < 13 {
			return errors.New("VZ shared networking requires macOS 13 or newer")
		}
		return nil
	}
	arch, err := cfg.Resources.Arch.LimaArch()
	if err != nil {
		return err
	}
	qemu := "qemu-system-" + arch
	if _, err := exec.LookPath(qemu); err != nil {
		return fmt.Errorf("install QEMU and make %s available in PATH", qemu)
	}
	return checkSharedNetworking(query)
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
		return errors.New("QEMU shared networking requires socket_vmnet; complete https://lima-vm.io/docs/config/network/vmnet/#socket_vmnet")
	}
	if err := cfg.VerifySudoAccess(ctx, cfg.Paths.Sudoers); err != nil {
		return fmt.Errorf("QEMU shared networking requires Lima's socket_vmnet sudo access: %w", err)
	}
	return ctx.Err()
}
