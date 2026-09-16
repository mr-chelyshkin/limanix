package vmnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lima-vm/lima/v2/pkg/networks"
	"github.com/mr-chelyshkin/limanix/internal/bundle"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Install is the privileged entry point used only by the hidden vmnet-install command.
// It reads a network configuration snapshot, not file paths or executable bytes
// supplied by the invoking user. All executable bytes come from the signed app.
func Install(ctx context.Context, input io.Reader, diagnostics io.Writer) (failure error) {
	if runtime.GOOS != "darwin" {
		return ErrMacOS
	}
	if os.Geteuid() != 0 {
		return ErrRootRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	cfg, err := readConfiguration(input)
	if err != nil {
		return err
	}
	if err = validateInstallation(cfg); err != nil {
		return err
	}

	payload, err := bundle.SocketVMNet(domain.Architecture(runtime.GOARCH))
	if err != nil {
		return err
	}

	lock, err := installationLock()
	if err != nil {
		return err
	}
	defer func() {
		failure = errors.Join(failure, lock.Close())
	}()

	if err = installHelper(payload); err != nil {
		return err
	}
	if err = cfg.Validate(); err != nil {
		return err
	}

	if err = installSudoers(ctx, cfg, diagnostics); err != nil {
		return err
	}

	_, err = fmt.Fprintln(diagnostics, "Lima network helper and sudoers are ready.")
	return err
}

func readConfiguration(input io.Reader) (networks.Config, error) {
	var cfg networks.Config
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("read network setup configuration: %w", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return cfg, ErrSetupDocument
	}

	return cfg, nil
}

func installHelper(payload bundle.SocketVMNetPayload) error {
	_, err := os.Lstat(helperPath)
	if err == nil {
		// A secure pre-existing installation remains administrator-managed.
		if err = secureFile(helperPath); err != nil {
			return err
		}

		cfg := networks.Config{Paths: networks.Paths{SocketVMNet: helperPath}}
		_, err = cfg.IsDaemonInstalled(networks.SocketVMNet)
		return err
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	for _, path := range []string{helperPath, licensePath} {
		if err := secureDirectory(filepath.Dir(path)); err != nil {
			return err
		}
	}

	if err := filesystem.WriteFileAtomic(licensePath, payload.License, 0o644); err != nil {
		return err
	}

	if err := filesystem.WriteFileAtomic(helperPath, payload.Executable, 0o755); err != nil {
		return err
	}

	return secureFile(helperPath)
}
