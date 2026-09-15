package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

// RegistryLockTimeout bounds waits between module registry operations.
const RegistryLockTimeout = 30 * time.Second

// ErrLockBusy identifies valid contention, distinct from filesystem or lock failures.
var ErrLockBusy = errors.New("state lock is held by another operation")

// Lock releases its descriptor exactly once; Close may be called more than once.
type Lock struct {
	file *os.File
	once sync.Once
	err  error
}

// Close releases the flock by closing its descriptor.
func (l *Lock) Close() error {
	l.once.Do(func() { l.err = l.file.Close() })
	return l.err
}

// InstanceLock immediately rejects another operation on the same VM.
func (s *Store) InstanceLock(ctx context.Context, name domain.VMName) (*Lock, error) {
	return s.vmLock(ctx, name, false)
}

func (s *Store) vmLock(ctx context.Context, name domain.VMName, shared bool) (*Lock, error) {
	if _, err := domain.NewVMName(string(name)); err != nil {
		return nil, err
	}
	return s.lock(ctx, filepath.Join(s.root, "locks", "instances", string(name)+".lock"), shared, 0,
		fmt.Sprintf("another operation is running for VM %q", name))
}

// RegistryLock supports concurrent readers and waits a bounded time for writers.
func (s *Store) RegistryLock(ctx context.Context, shared bool, timeout time.Duration) (*Lock, error) {
	if timeout < 0 {
		return nil, errors.New("registry lock timeout must be nonnegative")
	}
	return s.lock(ctx, filepath.Join(s.root, "locks", "registry.lock"), shared, timeout,
		fmt.Sprintf("timed out after %s waiting for the module registry lock", timeout))
}

func (s *Store) lock(ctx context.Context, path string, shared bool, timeout time.Duration, busy string) (*Lock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.Initialize(); err != nil {
		return nil, err
	}
	file, err := filesystem.OpenRegular(path, unix.O_CREAT|unix.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot open state lock %s: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	operation := unix.LOCK_EX
	if shared {
		operation = unix.LOCK_SH
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		err := unix.Flock(int(file.Fd()), operation|unix.LOCK_NB)
		if err == nil {
			return &Lock{file: file}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return nil, errors.Join(fmt.Errorf("cannot acquire state lock %s: %w", path, err), file.Close())
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, errors.Join(fmt.Errorf("%s: %w", busy, ErrLockBusy), file.Close())
		}
		timer := time.NewTimer(min(50*time.Millisecond, remaining))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, errors.Join(ctx.Err(), file.Close())
		case <-timer.C:
		}
	}
}
