package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// PreserveHome retains ownership after deletion of a VM state directory.
func (s *Store) PreserveHome(identity domain.Identity) (string, error) {
	identity, err := identity.Normalized()
	if err != nil {
		return "", err
	}

	if err := s.Initialize(); err != nil {
		return "", err
	}

	destination, err := s.preservedHomePath(identity)
	if err != nil {
		return "", err
	}

	if err := preserveIdentity(destination, identity); err != nil {
		return "", err
	}

	return destination, nil
}

func preserveIdentity(destination string, identity domain.Identity) error {
	saved, err := readHomeIdentity(destination, identity.Name)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		return writeRecord(destination, identityRecord{
			SchemaVersion: schemaVersion,
			Identity:      identity,
		})
	case err != nil:
		return err
	case saved != identity:
		return &OwnershipError{
			Cause:   ErrIdentityConflict,
			Message: fmt.Sprintf("preserved home %q ownership cannot be changed", filepath.Base(destination)),
		}
	default:
		return nil
	}
}

// ForgetHome removes only the matching archived identity; it does not remove home files.
func (s *Store) ForgetHome(identity domain.Identity) error {
	identity, err := identity.Normalized()
	if err != nil {
		return err
	}

	destination, err := s.preservedHomePath(identity)
	if err != nil {
		return err
	}

	if err := filesystem.CheckDirectory(filepath.Dir(destination)); err != nil {
		return err
	}

	saved, err := readHomeIdentity(destination, identity.Name)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case saved != identity:
		return &OwnershipError{
			Cause:   ErrIdentityConflict,
			Message: fmt.Sprintf("preserved home %q ownership differs from the VM", filepath.Base(destination)),
		}
	}

	return os.Remove(destination)
}

func readHomeIdentity(path string, name domain.VMName) (domain.Identity, error) {
	var saved identityRecord

	if err := readRecord(path, &saved); err != nil {
		return domain.Identity{}, err
	}

	return decodeIdentityRecord(saved, name)
}

func (s *Store) preservedHomePath(identity domain.Identity) (string, error) {
	_, home, err := identity.HomePaths()
	if err != nil {
		return "", err
	}

	return filepath.Join(s.root, "homes", filepath.Base(home)+".json"), nil
}
