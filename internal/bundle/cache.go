package bundle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

// prepareDirectory checks and creates the cache root and archive digest directory.
func (cache *Cache) prepareDirectory(digest string) (string, error) {
	if cache.root == "" {
		return "", ErrEmptyRoot
	}

	root, err := filesystem.Resolve(cache.root)
	if err != nil {
		return "", err
	}

	if err = filesystem.CheckDirectory(root); err != nil {
		return "", err
	}

	if err = os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}

	directory := filepath.Join(root, digest)

	if err = filesystem.CheckDirectory(directory); err != nil {
		return "", err
	}

	if err = os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}

	return directory, nil
}

func ensureCachedArchive(ctx context.Context, destination string, archive []byte) error {
	file, err := filesystem.OpenRegular(destination, unix.O_RDONLY, 0)
	if errors.Is(err, fs.ErrNotExist) {
		if err = ctx.Err(); err != nil {
			return err
		}

		return filesystem.WriteFileAtomic(destination, archive, 0o600)
	}

	if err != nil {
		return err
	}

	var (
		info, statErr = file.Stat()
		actual        []byte
		readErr       error
	)

	if statErr == nil && info.Size() == int64(len(archive)) {
		actual, readErr = io.ReadAll(file)
	}

	closeErr := file.Close()

	if err = errors.Join(statErr, readErr, closeErr); err != nil {
		return err
	}

	if !bytes.Equal(actual, archive) {
		return ErrArchiveMismatch
	}

	if info.Mode().Perm() != 0o600 {
		return ErrArchivePermissions
	}

	return nil
}
