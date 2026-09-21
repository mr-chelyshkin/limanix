package modules

import (
	"errors"
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

var (
	ErrAlreadyExists     = errors.New("module already exists")
	ErrNotInstalled      = errors.New("module is not installed")
	ErrUnknownSystem     = errors.New("unknown standard module")
	ErrUnknownCatalog    = errors.New("unknown module catalog; use limanix modules list")
	ErrInvalidDirectory  = errors.New("module must be a directory, not a symbolic link")
	ErrInvalidEntry      = errors.New("module entry point must be a regular default.nix file")
	ErrRecursiveCopy     = errors.New("a module cannot be copied into its own source directory")
	ErrUnsupportedFile   = errors.New("unsupported file or symlink in module")
	ErrDestinationExists = errors.New("module destination already exists")
)

// Error identifies a selected module and preserves the failure for errors.Is/As.
type Error struct {
	ID  domain.ModuleID
	Err error
}

// Error returns a module-specific diagnostic with recovery advice where applicable.
func (err *Error) Error() string {
	switch {
	case errors.Is(err.Err, ErrAlreadyExists):
		return fmt.Sprintf("module '%s' already exists; remove it before importing a replacement", err.ID)
	case errors.Is(err.Err, ErrNotInstalled):
		return fmt.Sprintf("module '%s' is not installed", err.ID)
	case errors.Is(err.Err, ErrUnknownSystem):
		return fmt.Sprintf("unknown standard module %q; use limanix modules list", err.ID)
	default:
		return fmt.Sprintf("module %q: %v", err.ID, err.Err)
	}
}

// Unwrap exposes the selection, validation or IO error.
func (err *Error) Unwrap() error {
	return err.Err
}

func importedError(name string, cause error) error {
	return &Error{
		ID:  domain.ModuleID("third-party:" + name),
		Err: cause,
	}
}
