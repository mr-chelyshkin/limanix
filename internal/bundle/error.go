package bundle

import "errors"

// Bundle errors distinguish missing build assets, invalid executable contents,
// and disk-cache integrity failures. Callers can inspect them with errors.Is.
var (
	ErrMissingAssets      = errors.New("embedded Lima guest agents are missing; run go run ./cmd/bundle-guestagent before building Limanix")
	ErrEmptyRoot          = errors.New("guest-agent cache root is empty")
	ErrArchiveMismatch    = errors.New("cached guest-agent archive does not match the embedded payload")
	ErrArchivePermissions = errors.New("cached guest-agent archive must have private 0600 permissions")
	ErrArchitecture       = errors.New("guest-agent archive has the wrong executable architecture")
)
