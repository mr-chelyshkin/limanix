package lima

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/lima-vm/lima/v2/pkg/limatype/dirnames"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func withNetworkLock(ctx context.Context, action func() error) (failure error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	directory, err := dirnames.LimaNetworksDir()
	if err != nil {
		return err
	}
	if err = filesystem.CheckDirectory(directory); err != nil {
		return err
	}
	if err = os.MkdirAll(directory, 0o700); err != nil {
		return err
	}

	file, err := filesystem.OpenRegular(filepath.Join(directory, "limanix-lifecycle.lock"), unix.O_CREAT|unix.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		failure = errors.Join(failure, file.Close())
	}()

	if err = waitForNetworkLock(ctx, file); err != nil {
		return err
	}

	return action()
}

func waitForNetworkLock(ctx context.Context, file *os.File) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	waiting := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) {
			return err
		}

		if !waiting {
			logrus.Info("Waiting for another Limanix network lifecycle operation")
			waiting = true
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
