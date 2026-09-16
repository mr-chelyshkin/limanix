package lima

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Adapter errors distinguish host capability, ownership, metadata, and launch
// requirements. Error preserves these causes and upstream failures for errors.Is/As.
var (
	ErrForeignInstance          = errors.New("instance is not managed by Limanix")
	ErrMissingMetadata          = errors.New("expected instance metadata")
	ErrInvalidStatus            = errors.New("expected a known instance status")
	ErrInvalidDisk              = errors.New("expected a nonnegative disk size in bytes")
	ErrMissingConfig            = errors.New("instance configuration is unavailable")
	ErrMissingAgentProvider     = errors.New("packaged guest-agent provider is unavailable")
	ErrEmptyAgentPath           = errors.New("packaged guest-agent provider returned an empty path")
	ErrMissingExecutable        = errors.New("limanix executable path is unavailable")
	ErrInvalidGuestArchitecture = errors.New("expected an arm64 or amd64 guest")
	ErrRunningEdit              = errors.New("cannot edit a running instance")
	ErrInvalidSSHMetadata       = errors.New("invalid Lima SSH metadata")
	ErrEmptyCommand             = errors.New("a guest command is required")
	ErrManagedMounts            = errors.New("a positive host UID and managed mount paths are required")
	ErrMacOSRequired            = errors.New("limanix VM operations require macOS")
	ErrMissingSSH               = errors.New("make the system SSH client available in PATH")
	ErrMissingVZ                = errors.New("this Limanix build does not include the native macOS VZ driver")
	ErrOldMacOS                 = errors.New("VZ shared networking requires macOS 13 or newer")
	ErrRosetta                  = errors.New("this Limanix binary is running under Rosetta on Apple Silicon; use the darwin-arm64 binary for VM operations")
	ErrMissingVMNet             = errors.New("QEMU shared networking requires socket_vmnet; complete https://lima-vm.io/docs/config/network/vmnet/#socket_vmnet")
)

// Error identifies an operation without including guest command arguments.
type Error struct {
	Operation string
	Err       error
}

// Error returns the adapter operation and its diagnostic.
func (err *Error) Error() string {
	return "Lima " + err.Operation + ": " + err.Err.Error()
}

// Unwrap exposes the native or transport failure.
func (err *Error) Unwrap() error {
	return err.Err
}

// CommandError retains the process exit status and a bounded stderr tail.
// Arguments are intentionally absent: they can contain private guest values.
type CommandError struct {
	Exit   *exec.ExitError
	Detail string
}

// Error reports the remote command's exit status and available stderr.
func (err *CommandError) Error() string {
	message := fmt.Sprintf("exited with status %d", exitStatus(err.Exit))
	if err.Detail != "" {
		message += ": " + err.Detail
	}

	return message
}

// Unwrap retains the original process error for errors.As.
func (err *CommandError) Unwrap() error {
	return err.Exit
}

// Some upstream shutdown paths report a timeout when their context is canceled.
// Preserve the caller's cancellation cause at our adapter boundary.
func operationError(ctx context.Context, operation string, err error) error {
	if err == nil {
		return nil
	}

	if ctx.Err() != nil {
		err = ctx.Err()
	}

	return wrapOperation(operation, err)
}

func wrapOperation(operation string, err error) error {
	if err == nil {
		return nil
	}

	return &Error{
		Operation: operation,
		Err:       err,
	}
}
