package bundle

import "errors"

var (
	ErrMissingAssets      = errors.New("embedded Lima guest agents are missing; run go run ./cmd/bundle-guestagent before building Limanix")
	ErrEmptyRoot          = errors.New("guest-agent cache root is empty")
	ErrArchiveMismatch    = errors.New("cached guest-agent archive does not match the embedded payload")
	ErrArchivePermissions = errors.New("cached guest-agent archive must have private 0600 permissions")
	ErrArchitecture       = errors.New("guest-agent archive has the wrong executable architecture")
	ErrMissingVMNetAssets = errors.New("embedded socket_vmnet is missing; build with task ci/build")
	ErrVMNetRelease       = errors.New("invalid socket_vmnet build metadata; build with task ci/build")
	ErrVMNetArchive       = errors.New("socket_vmnet archive does not match the pinned release")
	ErrVMNetArchitecture  = errors.New("socket_vmnet has the wrong host executable architecture")
	ErrVMNetDeployment    = errors.New("socket_vmnet exceeds the supported macOS deployment target")
)
