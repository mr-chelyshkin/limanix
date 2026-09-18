package bundle

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// guestArchive is a validated, immutable embedded payload shared by all caches.
type guestArchive struct {
	name   string
	data   []byte
	digest string
}

// Loading is lazy and shared across Cache instances. Only disk copies need revalidation.
var embeddedArchives = map[domain.Architecture]func() (*guestArchive, error){
	domain.ARM64: sync.OnceValues(func() (*guestArchive, error) {
		return readArchive(domain.ARM64)
	}),
	domain.AMD64: sync.OnceValues(func() (*guestArchive, error) {
		return readArchive(domain.AMD64)
	}),
}

func embeddedArchive(arch domain.Architecture) (*guestArchive, error) {
	load, exists := embeddedArchives[arch]
	if !exists {
		return nil, domain.ErrInvalidArchitecture
	}

	return load()
}

func resourceName(arch domain.Architecture) (string, error) {
	limaArch, err := arch.LimaArch()
	if err != nil {
		return "", err
	}

	return "lima-guestagent.Linux-" + limaArch + ".gz", nil
}

func readArchive(arch domain.Architecture) (*guestArchive, error) {
	name, err := resourceName(arch)
	if err != nil {
		return nil, err
	}

	archive, err := resources.ReadFile("resources/" + name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrMissingAssets
	}

	if err != nil {
		return nil, fmt.Errorf("read embedded guest agent: %w", err)
	}

	if err = validateArchive(archive, arch); err != nil {
		return nil, fmt.Errorf("invalid embedded guest agent: %w", err)
	}

	digest := sha256.Sum256(archive)
	return &guestArchive{
		name:   name,
		data:   archive,
		digest: hex.EncodeToString(digest[:]),
	}, nil
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

	defer func() {
		_ = executable.Close()
	}()

	expectedMachine := elf.EM_AARCH64
	if arch == domain.AMD64 {
		expectedMachine = elf.EM_X86_64
	}

	validType := executable.Type == elf.ET_EXEC || executable.Type == elf.ET_DYN
	if executable.Class != elf.ELFCLASS64 || executable.Data != elf.ELFDATA2LSB ||
		executable.Machine != expectedMachine || !validType {
		return ErrArchitecture
	}

	return nil
}
