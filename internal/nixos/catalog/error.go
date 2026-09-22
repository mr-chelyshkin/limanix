package catalog

import "errors"

var (
	// ErrRepository rejects archives without the expected source repository marker.
	ErrRepository = errors.New("module catalog repository mismatch")

	// ErrArchive rejects malformed archives, unsafe paths, and unsupported files.
	ErrArchive = errors.New("invalid module catalog archive")

	// ErrMetadata rejects missing entry points and invalid module descriptions.
	ErrMetadata = errors.New("invalid module metadata")

	// ErrVersion rejects empty tags and characters outside a release tag component.
	ErrVersion = errors.New("invalid module catalog tag")

	// ErrModule reports a name absent from the selected catalog.
	ErrModule = errors.New("module is absent from the standard catalog")
)
