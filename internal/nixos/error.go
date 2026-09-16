package nixos

import "errors"

var (
	// ErrInvalidUID rejects a root or invalid host UID for the development user.
	ErrInvalidUID = errors.New("guest UID must be positive")

	// ErrBundleExists prevents overwriting an existing generation's flake inputs.
	ErrBundleExists = errors.New("NixOS bundle already exists")
)
