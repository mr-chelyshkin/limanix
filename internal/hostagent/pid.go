package hostagent

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

type pidLease struct {
	file *os.File
	path string
}

func acquirePID(filename string) (_ *pidLease, failure error) {
	file, err := filesystem.OpenRegular(filename, unix.O_CREAT|unix.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	defer func() {
		if failure != nil {
			failure = errors.Join(failure, file.Close())
		}
	}()

	if err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, fmt.Errorf("another host agent owns the PID file: %w", err)
	}

	if err = checkPreviousPID(file); err != nil {
		return nil, err
	}

	if err = writePID(file); err != nil {
		return nil, err
	}

	return &pidLease{
		file: file,
		path: filename,
	}, nil
}

func checkPreviousPID(file *os.File) error {
	data, err := io.ReadAll(io.LimitReader(file, 64))
	if err != nil {
		return err
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}

	pid, err := strconv.Atoi(text)
	if err != nil || pid < 1 {
		return ErrInvalidPID
	}

	err = syscall.Kill(pid, 0)

	switch {
	case err == nil || errors.Is(err, syscall.EPERM):
		return fmt.Errorf("another host agent may be running with PID %d", pid)
	case errors.Is(err, syscall.ESRCH):
		return nil
	default:
		return err
	}
}

func writePID(file *os.File) error {
	if err := file.Chmod(0o600); err != nil {
		return err
	}

	data := []byte(strconv.Itoa(os.Getpid()) + "\n")
	if _, err := file.WriteAt(data, 0); err != nil {
		return err
	}

	if err := file.Truncate(int64(len(data))); err != nil {
		return err
	}

	return file.Sync()
}

// Close removes the owned PID path and releases its descriptor, preserving both errors.
func (lease *pidLease) Close() error {
	return errors.Join(lease.remove(), lease.file.Close())
}

func (lease *pidLease) remove() error {
	info, err := os.Lstat(lease.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	original, err := lease.file.Stat()
	if err != nil {
		return err
	}

	if !os.SameFile(info, original) {
		return ErrReplacedPID
	}

	return os.Remove(lease.path)
}
