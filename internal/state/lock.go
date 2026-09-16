package state

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// Lock releases its descriptor exactly once; Close may be called more than once.
type Lock struct {
	file *os.File
	once sync.Once
	err  error
}

// Close releases the flock by closing its descriptor.
func (lock *Lock) Close() error {
	lock.once.Do(func() {
		lock.err = lock.file.Close()
	})

	return lock.err
}

// InstanceLock immediately rejects another operation on the same VM.
// On failure it returns a nil interface, never an interface holding a nil *Lock.
func (s *Store) InstanceLock(ctx context.Context, name domain.VMName) (io.Closer, error) {
	lock, err := s.vmLock(ctx, name, false)
	if err != nil {
		return nil, err
	}

	return lock, nil
}

func (s *Store) vmLock(ctx context.Context, name domain.VMName, shared bool) (*Lock, error) {
	if _, err := domain.NewVMName(string(name)); err != nil {
		return nil, err
	}

	return s.lock(
		ctx,
		filepath.Join(s.root, "locks", "instances", string(name)+".lock"),
		shared,
		0,
		fmt.Sprintf("another operation is running for VM %q", name),
	)
}

// RegistryLock supports concurrent readers and waits a bounded time for writers.
func (s *Store) RegistryLock(ctx context.Context, shared bool, timeout time.Duration) (*Lock, error) {
	if timeout < 0 {
		return nil, ErrInvalidLockTimeout
	}

	return s.lock(
		ctx,
		filepath.Join(s.root, "locks", "registry.lock"),
		shared,
		timeout,
		fmt.Sprintf("timed out after %s waiting for the module registry lock", timeout),
	)
}

func (s *Store) lock(ctx context.Context, path string, shared bool, timeout time.Duration, busy string) (_ *Lock, failure error) {
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

	defer func() {
		if failure != nil {
			failure = errors.Join(failure, file.Close())
		}
	}()

	if err := file.Chmod(0o600); err != nil {
		return nil, err
	}

	err = acquireLock(ctx, file, shared, timeout)

	switch {
	case err == nil:
		return &Lock{file: file}, nil
	case errors.Is(err, ErrLockBusy):
		return nil, fmt.Errorf("%s: %w", busy, err)
	case ctx.Err() != nil:
		return nil, ctx.Err()
	default:
		return nil, fmt.Errorf("cannot acquire state lock %s: %w", path, err)
	}
}

func acquireLock(ctx context.Context, file *os.File, shared bool, timeout time.Duration) error {
	operation := unix.LOCK_EX
	if shared {
		operation = unix.LOCK_SH
	}

	deadline := time.Now().Add(timeout)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := unix.Flock(int(file.Fd()), operation|unix.LOCK_NB)
		if err == nil {
			return nil
		}

		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return err
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ErrLockBusy
		}

		if err := waitForLock(ctx, min(50*time.Millisecond, remaining)); err != nil {
			return err
		}
	}
}

func waitForLock(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
