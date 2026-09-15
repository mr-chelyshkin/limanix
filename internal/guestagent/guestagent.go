// Package guestagent supplies embedded Lima guest binaries to native VM startup.
package guestagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/elf"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

//go:embed resources/*
var resources embed.FS

const maxBinarySize = 128 << 20

// Cache materializes compressed guest agents in a host-only, content-addressed directory.
type Cache struct {
	root     string
	readFile func(string) ([]byte, error)
}

// New selects a cache beneath Limanix's runtime storage; construction performs no I/O.
func New(root string) *Cache { return &Cache{root: root, readFile: resources.ReadFile} }

func resourceName(arch domain.Architecture) (string, error) {
	limaArch, err := arch.LimaArch()
	if err != nil {
		return "", err
	}
	return "lima-guestagent.Linux-" + limaArch + ".gz", nil
}

// Path returns the private gzip archive accepted by Lima's native StartWithPaths.
// Only real embedded executables are accepted; no external installation is consulted.
func (cache *Cache) Path(ctx context.Context, arch domain.Architecture) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name, err := resourceName(arch)
	if err != nil {
		return "", err
	}
	archive, err := cache.readFile("resources/" + name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("embedded Lima guest agents are missing; run task assets/generate before building Limanix")
	}
	if err != nil {
		return "", fmt.Errorf("read embedded guest agent: %w", err)
	}
	if err := validateArchive(archive, arch); err != nil {
		return "", fmt.Errorf("invalid embedded guest agent: %w", err)
	}
	if cache.root == "" {
		return "", errors.New("guest-agent cache root is empty")
	}
	root, err := filesystem.Resolve(cache.root)
	if err != nil {
		return "", err
	}
	if err := filesystem.CheckDirectory(root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	digest := sha256.Sum256(archive)
	directory := filepath.Join(root, hex.EncodeToString(digest[:]))
	if err := filesystem.CheckDirectory(directory); err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	destination := filepath.Join(directory, name)
	file, err := filesystem.OpenRegular(destination, unix.O_RDONLY, 0)
	if err == nil {
		info, statErr := file.Stat()
		var actual []byte
		var readErr error
		if statErr == nil && info.Size() == int64(len(archive)) {
			actual, readErr = io.ReadAll(file)
		}
		closeErr := file.Close()
		if err := errors.Join(statErr, readErr, closeErr); err != nil {
			return "", err
		}
		if !bytes.Equal(actual, archive) {
			return "", errors.New("cached guest-agent archive does not match the embedded payload")
		}
		if info.Mode().Perm() != 0o600 {
			return "", errors.New("cached guest-agent archive must have private 0600 permissions")
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := filesystem.WriteFileAtomic(destination, archive, 0o600); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return destination, nil
}

func validateArchive(archive []byte, arch domain.Architecture) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	gzipReader.Multistream(false)
	binary, readErr := io.ReadAll(io.LimitReader(gzipReader, maxBinarySize+1))
	if err := errors.Join(readErr, gzipReader.Close()); err != nil {
		return err
	}
	if len(binary) > maxBinarySize {
		return errors.New("guest-agent executable exceeds the supported size")
	}
	executable, err := elf.NewFile(bytes.NewReader(binary))
	if err != nil {
		return err
	}
	defer func() { _ = executable.Close() }()
	expectedMachine := elf.EM_AARCH64
	if arch == domain.AMD64 {
		expectedMachine = elf.EM_X86_64
	}
	validType := executable.Type == elf.ET_EXEC || executable.Type == elf.ET_DYN
	if executable.Class != elf.ELFCLASS64 || executable.Data != elf.ELFDATA2LSB || executable.Machine != expectedMachine || !validType {
		return errors.New("guest-agent archive has the wrong executable architecture")
	}
	return nil
}
