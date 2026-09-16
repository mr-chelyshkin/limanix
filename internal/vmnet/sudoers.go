package vmnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lima-vm/lima/v2/pkg/networks"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

func checkSudoers(ctx context.Context, cfg networks.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if cfg.Paths.Sudoers == "" {
		// Custom setups may authorize sudo without a dedicated Lima rules file.
		return cfg.VerifySudoAccess(ctx, "")
	}

	installed, err := os.ReadFile(cfg.Paths.Sudoers)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %w", ErrSetupRequired, err)
	}
	if err != nil {
		return err
	}

	expected, err := networks.Sudoers()
	if err != nil {
		return err
	}
	if string(installed) != expected {
		return fmt.Errorf("%w: sudoers file %q does not match Lima's network configuration (group %q)",
			ErrSetupRequired, cfg.Paths.Sudoers, cfg.Group)
	}

	return ctx.Err()
}

func installSudoers(ctx context.Context, cfg networks.Config, diagnostics io.Writer) (failure error) {
	directory, err := os.MkdirTemp("/private/tmp", "limanix-vmnet-")
	if err != nil {
		return err
	}
	defer func() {
		failure = errors.Join(failure, os.RemoveAll(directory))
	}()

	if err = loadSnapshot(directory, cfg); err != nil {
		return err
	}

	rules, err := networks.Sudoers()
	if err != nil {
		return err
	}

	preview := filepath.Join(directory, "sudoers")
	if err = os.WriteFile(preview, []byte(rules), 0o600); err != nil {
		return err
	}

	command := exec.CommandContext(ctx, "/usr/sbin/visudo", "-c", "-s", "-f", preview)
	command.Stdout = diagnostics
	command.Stderr = diagnostics
	if err = command.Run(); err != nil {
		return fmt.Errorf("validate generated sudoers: %w", err)
	}

	if err = secureDirectory(filepath.Dir(sudoersPath)); err != nil {
		return err
	}

	if err = backupSudoers(diagnostics); err != nil {
		return err
	}

	if err = ctx.Err(); err != nil {
		return err
	}

	return filesystem.WriteFileAtomic(sudoersPath, []byte(rules), 0o644)
}

func loadSnapshot(directory string, cfg networks.Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	configDirectory := filepath.Join(directory, "_config")
	if err = os.Mkdir(configDirectory, 0o700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(configDirectory, "networks.yaml"), data, 0o600); err != nil {
		return err
	}
	if err = os.Setenv("LIMA_HOME", directory); err != nil {
		return err
	}

	loaded, err := networks.LoadConfig()
	if err != nil {
		return err
	}
	return loaded.Validate()
}

func backupSudoers(diagnostics io.Writer) (failure error) {
	_, err := os.Lstat(sudoersPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	if err = secureFile(sudoersPath); err != nil {
		return err
	}

	data, err := os.ReadFile(sudoersPath)
	if err != nil {
		return err
	}

	backup, err := os.CreateTemp(filepath.Dir(sudoersPath), ".lima-limanix-backup-")
	if err != nil {
		return err
	}
	defer func() {
		failure = errors.Join(failure, backup.Close())
	}()

	if _, err = backup.Write(data); err != nil {
		return err
	}
	if err = backup.Sync(); err != nil {
		return err
	}

	_, err = fmt.Fprintf(diagnostics, "Previous sudoers saved to %s\n", backup.Name())
	return err
}
