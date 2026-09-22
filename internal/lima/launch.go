package lima

import (
	"context"

	"github.com/lima-vm/lima/v2/pkg/instance"
	"github.com/lima-vm/lima/v2/pkg/limatype"
)

// launchPaths identifies the two executables needed by Lima's persistent host agent.
type launchPaths struct {
	hostAgent  string
	guestAgent string
}

func (client *Client) launchPaths(ctx context.Context, architecture string) (launchPaths, error) {
	arch, err := guestArchitecture(architecture)
	if err != nil {
		return launchPaths{}, err
	}

	if client.agentPath == nil {
		return launchPaths{}, ErrMissingAgentProvider
	}

	guestAgent, err := client.agentPath(ctx, arch)
	if err != nil {
		return launchPaths{}, err
	}

	if guestAgent == "" {
		return launchPaths{}, ErrEmptyAgentPath
	}

	hostAgent, err := client.executable()
	if err != nil {
		return launchPaths{}, err
	}

	if hostAgent == "" {
		return launchPaths{}, ErrMissingExecutable
	}

	return launchPaths{
		hostAgent:  hostAgent,
		guestAgent: guestAgent,
	}, nil
}

func (client *Client) launch(ctx context.Context, inst *limatype.Instance, paths launchPaths) error {
	startup, cancel := context.WithCancel(instance.WithLaunchingShell(context.WithoutCancel(ctx)))
	stopCancellation := context.AfterFunc(ctx, cancel)

	if err := client.native.start(startup, inst, false, false, paths.hostAgent, paths.guestAgent); err != nil {
		stopCancellation()
		cancel()
		return err
	}

	if !stopCancellation() {
		cancel()
		return ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		cancel()
		return err
	}

	return nil
}
