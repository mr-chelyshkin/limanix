package domain

import "errors"

var (
	ErrInvalidUTF8         = errors.New("expected UTF-8 text")
	ErrNULCharacter        = errors.New("must not contain NUL characters")
	ErrEmptyValue          = errors.New("must not be empty")
	ErrInvalidVMName       = errors.New("expected a lowercase hostname label of 1 to 63 characters")
	ErrInvalidUsername     = errors.New("expected a non-reserved Linux username of 1 to 32 characters")
	ErrGuestPathWhitespace = errors.New("guest mount paths cannot contain spaces, tabs, or line breaks because NixOS Lima does not escape them in fstab")
	ErrInvalidGuestPath    = errors.New("expected an absolute guest path without '..'")
	ErrGuestRoot           = errors.New("the guest root directory is not allowed")
	ErrInvalidModuleName   = errors.New("expected a module name of 1 to 63 lowercase letters, digits, and single hyphens, starting with a letter")
	ErrInvalidModuleID     = errors.New("expected a qualified module identifier: CATALOG:NAME")
	ErrInvalidEnvName      = errors.New("expected a POSIX environment variable name")
	ErrInvalidEnvValue     = errors.New("contains a character unsupported by guest environment files")
	ErrInvalidArchitecture = errors.New("expected one of arm64, amd64")
	ErrInvalidByteSize     = errors.New("expected a positive integer byte count")
	ErrInvalidSizeFormat   = errors.New("expected a positive whole GiB size, such as 8GiB")
	ErrSizeOverflow        = errors.New("size exceeds the supported integer limit")
	ErrNotWholeGiB         = errors.New("byte count cannot be represented as a positive whole GiB size")
	ErrSizeNotString       = errors.New("expected a whole GiB size string")

	ErrInvalidID           = errors.New("invalid identity id")
	ErrInvalidHomeRoot     = errors.New("invalid managed-home root")
	ErrInvalidHomeIdentity = errors.New("invalid managed-home identity")
	ErrInvalidCreatedAt    = errors.New("invalid created_at")
)
