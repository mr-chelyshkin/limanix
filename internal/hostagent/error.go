package hostagent

import "errors"

var (
	ErrDirectoryPermissions = errors.New("host-agent directory must be a private directory with 0700 permissions")
	ErrForeignInstance      = errors.New("host agent requires a Limanix instance name")
	ErrInvalidIdentifier    = errors.New("host agent requires a generated Limanix instance identifier")
	ErrForeignPaths         = errors.New("host-agent PID and socket paths must belong to the selected instance")
	ErrRelativeResource     = errors.New("agent resources must be absolute local file paths")
	ErrMissingGuestAgent    = errors.New("the embedded guest-agent archive must be specified")
	ErrOccupiedSocket       = errors.New("host-agent socket path is occupied by a non-socket file")
	ErrInvalidPID           = errors.New("host-agent PID file has an invalid process identifier")
	ErrReplacedPID          = errors.New("host-agent PID path was replaced during operation")
)
