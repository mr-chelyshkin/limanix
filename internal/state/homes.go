package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// PreserveHome retains ownership after deletion of a VM state directory.
func (s *Store) PreserveHome(identity Identity) (string, error) {
	identity, err := validatedIdentity(identity)
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
	var saved identityRecord
	if err := readRecord(destination, &saved); err == nil {
		ownership, err := decodeIdentityRecord(saved, identity.Name)
		if err != nil {
			return "", err
		}
		if ownership != identity {
			return "", fmt.Errorf("preserved home %q ownership cannot be changed", filepath.Base(destination))
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if err := writeRecord(destination, identityRecord{SchemaVersion: schemaVersion, Identity: identity}); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	return destination, nil
}

// ForgetHome removes only the matching archived identity; it does not remove home files.
func (s *Store) ForgetHome(identity Identity) error {
	identity, err := validatedIdentity(identity)
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
	var saved identityRecord
	if err := readRecord(destination, &saved); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	ownership, err := decodeIdentityRecord(saved, identity.Name)
	if err != nil {
		return err
	}
	if ownership != identity {
		return fmt.Errorf("preserved home %q ownership differs from the VM", filepath.Base(destination))
	}
	return os.Remove(destination)
}

func (s *Store) preservedHomePath(identity Identity) (string, error) {
	_, home, err := identity.HomePaths()
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, "homes", filepath.Base(home)+".json"), nil
}
