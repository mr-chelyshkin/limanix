package vm

import (
	"errors"
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Lifecycle causes can be checked independently of a VM-specific diagnostic.
var (
	ErrAlreadyExists  = errors.New("VM already has state")
	ErrBackendMissing = errors.New("backend instance is missing")
	ErrNotRunning     = errors.New("VM is not running")
	ErrRootUser       = errors.New("run Limanix as your regular host user, not root")
	ErrIdentityChange = errors.New("an update cannot change architecture, username, or managed-home paths; create a new VM for these changes")
	ErrDiskUnknown    = errors.New("lima did not report the disk size; update was not started")
	ErrDiskShrink     = errors.New("shrinking the guest disk is not supported")
)

// InstanceError associates a lifecycle cause with the user's VM name.
type InstanceError struct {
	Name  domain.VMName
	Cause error
}

// Error renders the lifecycle diagnostic at the application boundary.
func (e *InstanceError) Error() string {
	switch e.Cause {
	case ErrAlreadyExists:
		return fmt.Sprintf("VM '%s' already has state; use update or delete", e.Name)
	case ErrBackendMissing:
		return fmt.Sprintf("lima instance for '%s' is missing; delete its saved record", e.Name)
	case ErrNotRunning:
		return fmt.Sprintf("VM '%s' is not running; use limanix start %s", e.Name, e.Name)
	default:
		return fmt.Sprintf("VM '%s': %v", e.Name, e.Cause)
	}
}

// Unwrap exposes the lifecycle cause to errors.Is.
func (e *InstanceError) Unwrap() error {
	return e.Cause
}
