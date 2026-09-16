package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Save writes mutable state and creates an immutable identity if not yet present.
func (s *Store) Save(instance domain.Instance) error {
	identity, err := instance.Identity.Normalized()
	if err != nil {
		return fmt.Errorf("save VM identity: %w", err)
	}

	record := runtimeRecord{
		SchemaVersion: schemaVersion,
		Status:        instance.Status,
		Generation:    instance.Generation,
		Error:         instance.Error,
	}
	if err = validateRuntime(record); err != nil {
		return fmt.Errorf("save VM record: %w", err)
	}

	if err = s.Initialize(); err != nil {
		return err
	}

	directory, err := s.InstanceDir(identity.Name)
	if err != nil {
		return err
	}

	if err = os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}

	if err := s.ensureIdentity(directory, identity); err != nil {
		return err
	}

	return writeRecord(filepath.Join(directory, "instance.json"), record)
}

// ensureIdentity creates ownership once and rejects attempts to replace it.
func (s *Store) ensureIdentity(directory string, identity domain.Identity) error {
	identityPath := filepath.Join(directory, "identity.json")
	_, err := os.Lstat(identityPath)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		return writeRecord(identityPath, identityRecord{
			SchemaVersion: schemaVersion,
			Identity:      identity,
		})
	case err != nil:
		return err
	}

	saved, err := s.LoadIdentity(identity.Name)
	if err != nil {
		return err
	}

	if saved != identity {
		return &OwnershipError{
			Cause:   ErrIdentityConflict,
			Message: fmt.Sprintf("VM %q identity cannot be changed", identity.Name),
		}
	}

	return nil
}

// LoadIdentity reads ownership independently of mutable state.
func (s *Store) LoadIdentity(name domain.VMName) (domain.Identity, error) {
	directory, err := s.InstanceDir(name)
	if err != nil {
		return domain.Identity{}, err
	}

	var record identityRecord

	if err = readRecord(filepath.Join(directory, "identity.json"), &record); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return domain.Identity{}, fmt.Errorf("VM %q has no managed identity: %w", name, err)
		}

		return domain.Identity{}, fmt.Errorf("cannot read VM identity %q: %w", name, err)
	}

	identity, err := decodeIdentityRecord(record, name)
	if err != nil {
		return domain.Identity{}, fmt.Errorf("cannot read VM identity %q: %w", name, err)
	}

	return identity, nil
}

// Load reads both records without applying current user configuration validation.
func (s *Store) Load(name domain.VMName) (domain.Instance, error) {
	identity, err := s.LoadIdentity(name)
	if err != nil {
		return domain.Instance{}, err
	}

	return s.loadInstance(identity)
}

func (s *Store) loadInstance(identity domain.Identity) (domain.Instance, error) {
	directory, err := s.InstanceDir(identity.Name)
	if err != nil {
		return domain.Instance{}, err
	}

	var record runtimeRecord

	if err = readRecord(filepath.Join(directory, "instance.json"), &record); err != nil {
		return domain.Instance{}, fmt.Errorf("cannot read VM record %q: %w", identity.Name, err)
	}

	if err = validateRuntime(record); err != nil {
		return domain.Instance{}, fmt.Errorf("cannot read VM record %q: %w", identity.Name, err)
	}

	return domain.Instance{
		Identity:   identity,
		Status:     record.Status,
		Generation: record.Generation,
		Error:      record.Error,
	}, nil
}
