package managedhome

import "errors"

// Managed-home errors distinguish redirected allocations, final symlinks,
// and non-directory entries. Ownership validation errors come from domain.
var (
	ErrRedirectedPath = errors.New("managed-home path redirects through a symbolic link")
	ErrSymlink        = errors.New("managed-home path is a symbolic link")
	ErrNotDirectory   = errors.New("managed home is not a directory")
)
