package vmnet

import (
	"context"
	"fmt"

	"github.com/lima-vm/lima/v2/pkg/networks"
)

const (
	licensePath = "/opt/socket_vmnet/share/doc/socket_vmnet/LICENSE"
	helperPath  = "/opt/socket_vmnet/bin/socket_vmnet"
	sudoersPath = "/private/etc/sudoers.d/lima"
	runPath     = "/private/var/run/lima"
)

func inspect(ctx context.Context) (networks.Config, error) {
	cfg, err := networks.LoadConfig()
	if err != nil {
		return cfg, err
	}

	installed, err := cfg.IsDaemonInstalled(networks.SocketVMNet)
	if err != nil {
		return cfg, err
	}

	if !installed {
		return cfg, fmt.Errorf("%w: helper %q is not installed", ErrSetupRequired, cfg.Paths.SocketVMNet)
	}

	if err = cfg.Validate(); err != nil {
		return cfg, err
	}

	if err = checkHelper(ctx, cfg.Paths.SocketVMNet); err != nil {
		return cfg, err
	}

	if err = checkSudoers(ctx, cfg); err != nil {
		return cfg, err
	}

	return cfg, ctx.Err()
}

func validateInstallation(cfg networks.Config) error {
	if cfg.Paths.SocketVMNet != helperPath || cfg.Paths.Sudoers != sudoersPath || cfg.Paths.VarRun != runPath {
		return ErrCustomPaths
	}

	validation := cfg
	validation.Paths.SocketVMNet = ""
	if err := validation.Validate(); err != nil {
		return err
	}

	return validateExistingParent(helperPath)
}
