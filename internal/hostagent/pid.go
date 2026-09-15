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

func acquirePID(filename string) (*pidLease, error) {
	file, err := filesystem.OpenRegular(filename, unix.O_CREAT|unix.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*pidLease, error) { return nil, errors.Join(err, file.Close()) }
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fail(fmt.Errorf("another host agent owns the PID file: %w", err))
	}
	data, err := io.ReadAll(io.LimitReader(file, 64))
	if err != nil {
		return fail(err)
	}
	if text := strings.TrimSpace(string(data)); text != "" {
		pid, err := strconv.Atoi(text)
		if err != nil || pid < 1 {
			return fail(errors.New("host-agent PID file has an invalid process identifier"))
		}
		err = syscall.Kill(pid, 0)
		if err == nil || errors.Is(err, syscall.EPERM) {
			return fail(fmt.Errorf("another host agent may be running with PID %d", pid))
		}
		if !errors.Is(err, syscall.ESRCH) {
			return fail(err)
		}
	}
	if err := file.Chmod(0o600); err != nil {
		return fail(err)
	}
	data = []byte(strconv.Itoa(os.Getpid()) + "\n")
	if _, err := file.WriteAt(data, 0); err != nil {
		return fail(err)
	}
	if err := file.Truncate(int64(len(data))); err != nil {
		return fail(err)
	}
	if err := file.Sync(); err != nil {
		return fail(err)
	}
	return &pidLease{file: file, path: filename}, nil
}

func (lease *pidLease) Close() error {
	info, err := os.Lstat(lease.path)
	if err == nil {
		original, statErr := lease.file.Stat()
		if statErr != nil {
			err = statErr
		} else if os.SameFile(info, original) {
			err = os.Remove(lease.path)
		} else {
			err = errors.New("host-agent PID path was replaced during operation")
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return errors.Join(err, lease.file.Close())
}
