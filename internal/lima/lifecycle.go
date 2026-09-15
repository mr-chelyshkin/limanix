package lima

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/lima-vm/lima/v2/pkg/instance"
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

func (client *Client) Create(ctx context.Context, name, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateOwnedInstanceName(name); err != nil {
		return &Error{Operation: "create", Err: err}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return &Error{Operation: "create", Err: err}
	}
	_, err = client.create(ctx, name, data, false)
	if err != nil {
		return operationError(ctx, "create", err)
	}
	return nil
}

func inspectionErrors(inst *limatype.Instance) error {
	if len(inst.Errors) != 0 {
		return fmt.Errorf("errors inspecting instance: %w", errors.Join(inst.Errors...))
	}
	if inst.Config == nil {
		return errors.New("instance configuration is unavailable")
	}
	return nil
}

func (client *Client) Start(ctx context.Context, name string) error {
	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return &Error{Operation: "start", Err: err}
	}
	if err := inspectionErrors(inst); err != nil {
		return &Error{Operation: "start", Err: err}
	}
	if inst.Status == limatype.StatusRunning {
		return nil
	}
	arch, err := guestArchitecture(inst.Arch)
	if err != nil {
		return &Error{Operation: "start", Err: err}
	}
	if client.agentPath == nil {
		return &Error{Operation: "start", Err: errors.New("packaged guest-agent provider is unavailable")}
	}
	guestAgentPath, err := client.agentPath(ctx, arch)
	if err != nil {
		return &Error{Operation: "start", Err: err}
	}
	if guestAgentPath == "" {
		return &Error{Operation: "start", Err: errors.New("packaged guest-agent provider returned an empty path")}
	}
	executablePath, err := client.executable()
	if err != nil {
		return &Error{Operation: "start", Err: err}
	}
	if executablePath == "" {
		return &Error{Operation: "start", Err: errors.New("limanix executable path is unavailable")}
	}
	if err := client.reconcile(ctx, name); err != nil {
		return operationError(ctx, "start", err)
	}
	// The hostagent is a persistent child. Forward cancellation while startup is
	// underway, then detach it from the command context after successful boot.
	// Otherwise the CLI's deferred signal cleanup would kill a healthy VM.
	// Limanix reports operation success after applying NixOS; suppress Lima's
	// intermediate instruction to run the separately installed limactl shell.
	startup, cancel := context.WithCancel(instance.WithLaunchingShell(context.WithoutCancel(ctx)))
	stopCancellation := context.AfterFunc(ctx, cancel)
	if err := client.start(startup, inst, false, false, executablePath, guestAgentPath); err != nil {
		stopCancellation()
		cancel()
		return operationError(ctx, "start", err)
	}
	if !stopCancellation() {
		cancel()
		return &Error{Operation: "start", Err: ctx.Err()}
	}
	if err := ctx.Err(); err != nil {
		cancel()
		return &Error{Operation: "start", Err: err}
	}
	return nil
}

func guestArchitecture(arch string) (domain.Architecture, error) {
	switch arch {
	case limatype.AARCH64:
		return domain.ARM64, nil
	case limatype.X8664:
		return domain.AMD64, nil
	default:
		return "", errors.New("expected an arm64 or amd64 guest")
	}
}

func (client *Client) Stop(ctx context.Context, name string) error {
	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return &Error{Operation: "stop", Err: err}
	}
	if err := client.stop(ctx, inst, false); err != nil {
		return operationError(ctx, "stop", err)
	}
	if err := client.reconcile(ctx, ""); err != nil {
		return operationError(ctx, "stop", err)
	}
	return nil
}

func (client *Client) Delete(ctx context.Context, name string, force bool) error {
	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return &Error{Operation: "delete", Err: err}
	}
	if err := client.delete(ctx, inst, force); err != nil {
		return operationError(ctx, "delete", err)
	}
	if err := client.reconcile(ctx, ""); err != nil {
		return operationError(ctx, "delete", err)
	}
	return nil
}
