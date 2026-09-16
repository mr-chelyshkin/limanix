package vmnet

import "errors"

// Setup errors distinguish missing authorization from unsafe host state.
var (
	ErrMacOS             = errors.New("socket_vmnet setup requires macOS")
	ErrRunAsUser         = errors.New("run network setup as your regular user; Limanix requests administrator access only for installation")
	ErrRootRequired      = errors.New("the internal network installer requires administrator privileges")
	ErrSetupRequired     = errors.New("socket_vmnet setup is required; run limanix network setup in an interactive terminal")
	ErrSetupDeclined     = errors.New("socket_vmnet setup declined; no privileged files were changed")
	ErrCustomPaths       = errors.New("automatic setup requires Lima's default socketVMNet, varRun and sudoers paths; preserve custom paths and configure them using Lima's vmnet instructions")
	ErrUnsafeFile        = errors.New("expected a regular root-owned file")
	ErrSetupBusy         = errors.New("another socket_vmnet installation is in progress")
	ErrSetupDocument     = errors.New("expected exactly one network setup configuration")
	ErrHelperUnavailable = errors.New("installed socket_vmnet cannot run on this host; check the administrator-managed helper installation")
)
