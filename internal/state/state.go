// Package state stores immutable VM identities, operation records, and host locks.
package state

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Status describes a Limanix operation; Interrupted is computed only for listing.
type Status string

const (
	Creating    Status = "creating"
	Ready       Status = "ready"
	Updating    Status = "updating"
	Error       Status = "error"
	Deleting    Status = "deleting"
	Interrupted Status = "interrupted"
)

func (s Status) inFlight() bool { return s == Creating || s == Updating || s == Deleting }

// Instance pairs immutable ownership with mutable operation state.
type Instance struct {
	Identity   Identity
	Status     Status
	Generation string
	Error      *string
}

// Entry retains an independently readable identity when a runtime record is damaged.
type Entry struct {
	Instance *Instance
	Identity *Identity
	Name     string
	Error    *string
}

// Store owns a resolved state directory; construction does not create it.
type Store struct{ root string }

// DefaultRoot returns the platform state location or LIMANIX_HOME override.
func DefaultRoot() (string, error) {
	if override := os.Getenv("LIMANIX_HOME"); override != "" {
		return filesystem.Resolve(override)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Limanix"), nil
	}
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		root = filepath.Join(home, ".local", "state")
	}
	return filesystem.Resolve(filepath.Join(root, "limanix"))
}

// NewStore resolves a supplied root or selects DefaultRoot when root is empty.
func NewStore(root string) (*Store, error) {
	var err error
	if root == "" {
		root, err = DefaultRoot()
	}
	if err != nil {
		return nil, fmt.Errorf("resolve state root: %w", err)
	}
	root, err = filesystem.Resolve(root)
	if err != nil {
		return nil, fmt.Errorf("resolve state root: %w", err)
	}
	return &Store{root: root}, nil
}

// Root returns the resolved root without modifying the filesystem.
func (s *Store) Root() string { return s.root }

// Initialize creates private state directories while refusing redirected paths.
func (s *Store) Initialize() error {
	if err := filesystem.CheckDirectory(s.root); err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("initialize state at %s: %w", s.root, err)
	}

	for _, relative := range []string{"instances", "modules", "homes", "locks", "locks/instances"} {
		directory := filepath.Join(s.root, relative)
		if err := filesystem.CheckDirectory(directory); err != nil {
			return err
		}
		if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("initialize state directory %s: %w", directory, err)
		}
	}
	return nil
}

// InstanceDir returns a checked VM state path without creating it.
func (s *Store) InstanceDir(name domain.VMName) (string, error) {
	if _, err := domain.NewVMName(string(name)); err != nil {
		return "", err
	}

	parent := filepath.Join(s.root, "instances")
	if err := filesystem.CheckDirectory(parent); err != nil {
		return "", err
	}

	directory := filepath.Join(parent, string(name))
	if err := filesystem.CheckDirectory(directory); err != nil {
		return "", err
	}
	return directory, nil
}

// Save writes mutable state and creates an immutable identity if not yet present.
func (s *Store) Save(instance Instance) error {
	identity, err := validatedIdentity(instance.Identity)
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

	identityPath := filepath.Join(directory, "identity.json")
	if _, err = os.Lstat(identityPath); err == nil {
		saved, err := s.LoadIdentity(identity.Name)
		if err != nil {
			return err
		}
		if saved != identity {
			return fmt.Errorf("VM %q identity cannot be changed", identity.Name)
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if err = writeRecord(identityPath, identityRecord{SchemaVersion: schemaVersion, Identity: identity}); err != nil {
			return err
		}
	} else {
		return err
	}
	return writeRecord(filepath.Join(directory, "instance.json"), record)
}

// LoadIdentity reads ownership independently of mutable state.
func (s *Store) LoadIdentity(name domain.VMName) (Identity, error) {
	directory, err := s.InstanceDir(name)
	if err != nil {
		return Identity{}, err
	}

	var record identityRecord
	if err = readRecord(filepath.Join(directory, "identity.json"), &record); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Identity{}, fmt.Errorf("VM %q has no managed identity: %w", name, err)
		}
		return Identity{}, fmt.Errorf("cannot read VM identity %q: %w", name, err)
	}
	identity, err := decodeIdentityRecord(record, name)
	if err != nil {
		return Identity{}, fmt.Errorf("cannot read VM identity %q: %w", name, err)
	}
	return identity, nil
}

// Load reads both records without applying current user configuration validation.
func (s *Store) Load(name domain.VMName) (Instance, error) {
	identity, err := s.LoadIdentity(name)
	if err != nil {
		return Instance{}, err
	}
	return s.loadInstance(identity)
}

func (s *Store) loadInstance(identity Identity) (Instance, error) {
	directory, err := s.InstanceDir(identity.Name)
	if err != nil {
		return Instance{}, err
	}

	var record runtimeRecord
	if err = readRecord(filepath.Join(directory, "instance.json"), &record); err != nil {
		return Instance{}, fmt.Errorf("cannot read VM record %q: %w", identity.Name, err)
	}
	if err = validateRuntime(record); err != nil {
		return Instance{}, fmt.Errorf("cannot read VM record %q: %w", identity.Name, err)
	}
	return Instance{
		Identity:   identity,
		Status:     record.Status,
		Generation: record.Generation,
		Error:      record.Error,
	}, nil
}

// FetchAll lists healthy and damaged records, detecting abandoned operations under a shared lock.
func (s *Store) FetchAll() ([]Entry, error) {
	directory := filepath.Join(s.root, "instances")

	if err := filesystem.CheckDirectory(directory); err != nil {
		return nil, err
	}

	paths, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot list VM records: %w", err)
	}

	entries := make([]Entry, 0, len(paths))
	for _, path := range paths {
		if !path.IsDir() && path.Type()&fs.ModeSymlink == 0 {
			continue
		}
		entry, err := s.loadEntry(domain.VMName(path.Name()))
		if err != nil {
			entry.Instance = nil
			message := err.Error()
			entry.Error = &message
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// loadEntry preserves independently readable ownership when runtime state is damaged.
func (s *Store) loadEntry(name domain.VMName) (Entry, error) {
	entry := Entry{Name: string(name)}

	identity, err := s.LoadIdentity(name)
	if err != nil {
		return entry, err
	}

	entry.Identity = &identity
	instance, err := s.loadInstance(identity)
	if err != nil {
		return entry, err
	}

	entry.Instance = &instance
	if !instance.Status.inFlight() {
		return entry, nil
	}
	return s.refreshInterruptedEntry(name, entry)
}

func (s *Store) refreshInterruptedEntry(name domain.VMName, entry Entry) (result Entry, err error) {
	lock, err := s.vmLock(context.Background(), name, true)
	if errors.Is(err, ErrLockBusy) {
		return entry, nil
	}
	if err != nil {
		return entry, err
	}

	defer func() { err = errors.Join(err, lock.Close()) }()

	entry.Identity = nil
	identity, err := s.LoadIdentity(name)
	if err != nil {
		return entry, err
	}

	entry.Identity = &identity
	instance, err := s.loadInstance(identity)
	if err != nil {
		return entry, err
	}
	if instance.Status.inFlight() {
		instance.Status = Interrupted
	}
	entry.Instance = &instance
	return entry, nil
}
