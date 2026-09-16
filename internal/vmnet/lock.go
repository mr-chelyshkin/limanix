package vmnet

import (
	"errors"
	"os"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

func installationLock() (*os.File, error) {
	const (
		directory = "/opt/socket_vmnet"
		path      = directory + "/.limanix-setup.lock"
	)
	if err := secureDirectory(directory); err != nil {
		return nil, err
	}

	file, err := filesystem.OpenRegular(path, unix.O_CREAT|unix.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err = secureFile(path); err != nil {
		return nil, errors.Join(err, file.Close())
	}

	if err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			err = ErrSetupBusy
		}
		return nil, errors.Join(err, file.Close())
	}

	return file, nil
}
