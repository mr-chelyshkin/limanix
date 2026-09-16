package lima

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/vmnet"
)

// Preflight checks host prerequisites before allocating an instance.
// QEMU may require an explicitly confirmed administrator setup of Lima networking.
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

	if err := RequireMacOS(ctx); err != nil {
		return err
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

	if usesVZ(cfg.Resources.Arch, hostArch) {
		if !nativeVZAvailable() {
			return ErrMissingVZ
		}
		return nil
	}

	return client.checkQEMU(ctx, cfg.Resources.Arch)
}

// RequireMacOS checks the common host baseline before VM or privileged setup
// operations. The bounded query also covers source builds with an older target.
func RequireMacOS(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return ErrMacOSRequired
	}

	minimum, err := buildinfo.MinimumMacOS()
	if err != nil {
		return err
	}

	query, cancel := queryContext(ctx)
	defer cancel()

	version, err := exec.CommandContext(query, "/usr/bin/sw_vers", "-productVersion").Output()
	if err != nil {
		if query.Err() != nil {
			return query.Err()
		}

		return fmt.Errorf("determine macOS version: %w", err)
	}

	actual := strings.TrimSpace(string(version))
	number, err := buildinfo.ParseMacOSVersion(actual)
	if err != nil {
		return fmt.Errorf("parse macOS version %q: %w", actual, err)
	}
	if number < minimum {
		return fmt.Errorf("%w: need %s or newer, found %s", ErrOldMacOS, minimum, actual)
	}

	return ctx.Err()
}

func (client *Client) checkQEMU(ctx context.Context, architecture domain.Architecture) error {
	arch, err := architecture.LimaArch()
	if err != nil {
		return err
	}

	executable := "qemu-system-" + arch
	if _, err := exec.LookPath(executable); err != nil {
		return fmt.Errorf("install QEMU and make %s available in PATH: %w", executable, err)
	}

	return client.ensureSharedNetworking(ctx)
}

func (client *Client) ensureSharedNetworking(ctx context.Context) error {
	return vmnet.New(client.Stdin, client.Stderr).Ensure(ctx)
}
