// Package bundle supplies the embedded Linux guest agent used by Lima at VM startup.
//
// Lima's startup API needs a file path, but the agents are stored inside the Limanix executable.
// Cache selects the archive for the guest architecture, validates its contents, and makes it available
// as a private file on the host. No external agent installation or download is used during startup.
//
//	Build time:
//	  pinned Lima dependency
//	          ↓
//	  cmd/bundle-guestagent → generator → resources/*.gz + manifest.json
//	                                            ↓
//	                                         go:embed
//	                                            ↓
//	                                      Limanix binary
//
//	VM startup:
//	  embedded gzip → Cache.Path(ctx, guestArch)
//	                          ↓
//	              <root>/<sha256>/<archive>.gz → Lima StartWithPaths
//
// New performs no I/O. Cache.Path validates gzip contents and ELF architecture,
// then reuses an identical cached archive or writes a missing one atomically
// with mode 0600. The SHA-256 directory identifies the compressed payload.
// Decompression is only used for validation; the file returned to Lima is gzip.
// A mismatched cached file or incorrect file permissions produce an error.
//
// The generator subpackage owns compilation and manifest checks.
// This runtime package neither rebuilds agents nor reads the build manifest.
package bundle

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

// Embedding the directory excludes dot-prefixed build and atomic-write leftovers.
//
//go:embed resources
var resources embed.FS

// Cache materializes compressed guest agents in a host-only, content-addressed directory.
type Cache struct {
	root     string
	readFile func(string) ([]byte, error)
}

// New selects a cache beneath Limanix's runtime storage.
func New(root string) *Cache { return &Cache{root: root, readFile: resources.ReadFile} }

func resourceName(arch domain.Architecture) (string, error) {
	limaArch, err := arch.LimaArch()
	if err != nil {
		return "", err
	}
	return "lima-guestagent.Linux-" + limaArch + ".gz", nil
}

// Path returns the private gzip archive accepted by Lima's native StartWithPaths.
func (cache *Cache) Path(ctx context.Context, arch domain.Architecture) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	name, archive, err := cache.embeddedArchive(arch)
	if err != nil {
		return "", err
	}
	directory, err := cache.prepareDirectory(archive)
	if err != nil {
		return "", err
	}

	destination := filepath.Join(directory, name)
	if err = ensureCachedArchive(ctx, destination, archive); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return destination, nil
}

// embeddedArchive resolves the guest archive name and validates its embedded payload.
func (cache *Cache) embeddedArchive(arch domain.Architecture) (string, []byte, error) {
	name, err := resourceName(arch)
	if err != nil {
		return "", nil, err
	}

	archive, err := cache.readFile("resources/" + name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, errors.New("embedded Lima guest agents are missing; run task assets/generate before building Limanix")
	}
	if err != nil {
		return "", nil, fmt.Errorf("read embedded guest agent: %w", err)
	}

	if err = validateArchive(archive, arch); err != nil {
		return "", nil, fmt.Errorf("invalid embedded guest agent: %w", err)
	}
	return name, archive, nil
}

// prepareDirectory checks and creates the cache root and archive digest directory.
func (cache *Cache) prepareDirectory(archive []byte) (string, error) {
	if cache.root == "" {
		return "", errors.New("guest-agent cache root is empty")
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

	var (
		digest    = sha256.Sum256(archive)
		directory = filepath.Join(root, hex.EncodeToString(digest[:]))
	)
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
		return errors.New("cached guest-agent archive does not match the embedded payload")
	}
	if info.Mode().Perm() != 0o600 {
		return errors.New("cached guest-agent archive must have private 0600 permissions")
	}
	return nil
}

func validateArchive(archive []byte, arch domain.Architecture) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	gzipReader.Multistream(false)

	binary, readErr := io.ReadAll(gzipReader)
	if err = errors.Join(readErr, gzipReader.Close()); err != nil {
		return err
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
	if executable.Class != elf.ELFCLASS64 || executable.Data != elf.ELFDATA2LSB ||
		executable.Machine != expectedMachine || !validType {
		return errors.New("guest-agent archive has the wrong executable architecture")
	}
	return nil
}
