package lima

import (
	"context"
	"os"

	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Create writes a checked Limanix-owned instance into Lima's native store.
func (client *Client) Create(ctx context.Context, name, path string) (failure error) {
	defer func() {
		failure = operationError(ctx, "create", failure)
	}()

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateOwnedInstanceName(name); err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	_, err = client.native.create(ctx, name, data, false)
	return err
}

// Start resolves packaged executables and starts the persistent host agent.
// An already running instance is left untouched.
func (client *Client) Start(ctx context.Context, name string) (failure error) {
	defer func() {
		failure = operationError(ctx, "start", failure)
	}()

	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return err
	}

	if err := inspectionErrors(inst); err != nil {
		return err
	}

	if inst.Status == limatype.StatusRunning {
		return nil
	}

	if inst.VMType == limatype.QEMU {
		if err := client.ensureSharedNetworking(ctx); err != nil {
			return err
		}
	}

	paths, err := client.launchPaths(ctx, inst.Arch)
	if err != nil {
		return err
	}

	return withNetworkLock(ctx, func() error {
		if err := client.native.reconcile(ctx, name); err != nil {
			return err
		}

		return client.launch(ctx, inst, paths)
	})
}

// Stop shuts down the guest gracefully and reconciles shared networks.
func (client *Client) Stop(ctx context.Context, name string) (failure error) {
	defer func() {
		failure = operationError(ctx, "stop", failure)
	}()

	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return err
	}

	return withNetworkLock(ctx, func() error {
		if err := client.native.stop(ctx, inst, false); err != nil {
			return err
		}

		return client.native.reconcile(ctx, "")
	})
}

// Delete removes Lima's instance resources, including damaged instance metadata.
func (client *Client) Delete(ctx context.Context, name string, force bool) (failure error) {
	defer func() {
		failure = operationError(ctx, "delete", failure)
	}()

	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return err
	}

	return withNetworkLock(ctx, func() error {
		if err := client.native.delete(ctx, inst, force); err != nil {
			return err
		}

		return client.native.reconcile(ctx, "")
	})
}

func guestArchitecture(arch string) (domain.Architecture, error) {
	switch arch {
	case limatype.AARCH64:
		return domain.ARM64, nil
	case limatype.X8664:
		return domain.AMD64, nil
	default:
		return "", ErrInvalidGuestArchitecture
	}
}
