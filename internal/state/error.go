package state

import "errors"

// Persistence errors distinguish invalid records, ownership conflicts and contention.
var (
	ErrLockBusy           = errors.New("state lock is held by another operation")
	ErrInvalidLockTimeout = errors.New("registry lock timeout must be nonnegative")
	ErrIdentityConflict   = errors.New("saved identity cannot be changed")
	ErrUnsupportedSchema  = errors.New("unsupported state schema")
	ErrIdentityName       = errors.New("identity name differs from its directory")
	ErrInvalidGeneration  = errors.New("invalid generation")
	ErrComputedStatus     = errors.New("interrupted is a computed listing status")
	ErrInvalidStatus      = errors.New("invalid lifecycle state")
	ErrRecordEncoding     = errors.New("record is not valid UTF-8")
	ErrRecordFields       = errors.New("unexpected record fields")
)

// OwnershipError retains a stable cause and identifies the rejected ownership change.
type OwnershipError struct {
	Cause   error
	Message string
}

// Error returns the ownership diagnostic without exposing guest configuration.
func (e *OwnershipError) Error() string {
	return e.Message
}

// Unwrap exposes the ownership conflict to errors.Is.
func (e *OwnershipError) Unwrap() error {
	return e.Cause
}
