package domain

// VMName identifies a VM within the host's Limanix state.
type VMName string

// Username identifies the unprivileged development user in the guest.
type Username string

// GuestPath is a normalized absolute path inside the guest.
type GuestPath string

// ModuleName identifies a built-in or imported NixOS module.
type ModuleName string

// ModuleID includes the source namespace of a selected module.
type ModuleID string

// EnvName is a POSIX environment variable name.
type EnvName string

// EnvValue is a literal guest environment value.
type EnvValue string
