package managedhome

import "errors"

var (
	ErrRedirectedPath = errors.New("managed-home path redirects through a symbolic link")
	ErrSymlink        = errors.New("managed-home path is a symbolic link")
	ErrNotDirectory   = errors.New("managed home is not a directory")
)
