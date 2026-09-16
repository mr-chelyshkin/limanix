package state

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Entry retains an independently readable identity when a runtime record is damaged.
// Name comes from the state directory. Instance is nil when the operation record
// cannot be read; Identity may still be available for backend lookup and recovery.
// Error describes a listing-time read failure, distinct from Instance.Error's
// persisted recovery message.
type Entry struct {
	Instance *domain.Instance
	Identity *domain.Identity
	Name     string
	Error    *string
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
	if !instance.Status.InFlight() {
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

	defer func() {
		err = errors.Join(err, lock.Close())
	}()

	// A completed delete may have removed ownership since the initial read.
	// Retain only identity verified under this lock, never a stale pre-delete value.
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

	if instance.Status.InFlight() {
		instance.Status = domain.Interrupted
	}

	entry.Instance = &instance
	return entry, nil
}
