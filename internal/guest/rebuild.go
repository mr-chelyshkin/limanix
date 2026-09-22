package guest

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

const rebuildStopTimeout = 30 * time.Second

func (guest *Guest) buildGeneration(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var (
		unit    = "limanix-rebuild-" + rand.Text() + ".service"
		command = []string{
			"sudo", "systemd-run",
			"--unit=" + unit,
			"--wait", "--pipe", "--collect", "--quiet",
			"--service-type=oneshot",
			"--property=KillMode=control-group",
			"--property=TimeoutStartSec=infinity",
			"--property=TimeoutStopSec=5s",
			"--setenv=PATH",
			"/run/current-system/sw/bin/nixos-rebuild", "boot",
			"--flake", "path:/mnt/limanix/flake#runtime",
			"--no-write-lock-file",
		}
	)

	session, closeSession := context.WithCancel(context.WithoutCancel(ctx))
	defer closeSession()

	finished := make(chan struct{})
	var runErr error
	go func() {
		_, runErr = guest.client.Run(session, name, command, false)
		close(finished)
	}()

	select {
	case <-finished:
		if ctx.Err() == nil || runErr == nil {
			return errors.Join(runErr, ctx.Err())
		}
	case <-ctx.Done():
	}

	err := guest.stopRebuild(ctx, name, unit, finished)
	closeSession()
	<-finished

	if err != nil && runErr != nil {
		return fmt.Errorf("cannot confirm guest rebuild stopped (%s): %w", unit, err)
	}

	return ctx.Err()
}

// stopRebuild waits through the launch race: a unit may not exist yet when the caller cancels.
func (guest *Guest) stopRebuild(ctx context.Context, name, unit string, finished <-chan struct{}) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), rebuildStopTimeout)
	defer cancel()

	retry := time.NewTicker(100 * time.Millisecond)
	defer retry.Stop()

	for {
		_, err := guest.client.Run(cleanup, name, []string{"sudo", "systemctl", "stop", unit}, true)
		if err == nil {
			return nil
		}

		select {
		case <-finished:
			return err
		case <-cleanup.Done():
			return err
		case <-retry.C:
		}
	}
}
