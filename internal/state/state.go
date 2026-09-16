package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Store owns a resolved state directory; construction does not create it.
type Store struct {
	root string
}

// DefaultRoot returns the platform state location or LIMANIX_HOME override.
func DefaultRoot() (string, error) {
	if override := os.Getenv("LIMANIX_HOME"); override != "" {
		return filesystem.Resolve(override)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Limanix"), nil
	}

	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		root = filepath.Join(home, ".local", "state")
	}

	return filesystem.Resolve(filepath.Join(root, "limanix"))
}

// NewStore resolves a supplied root or selects DefaultRoot when root is empty.
func NewStore(root string) (*Store, error) {
	var err error

	if root == "" {
		root, err = DefaultRoot()
	}

	if err != nil {
		return nil, fmt.Errorf("resolve state root: %w", err)
	}

	root, err = filesystem.Resolve(root)
	if err != nil {
		return nil, fmt.Errorf("resolve state root: %w", err)
	}

	return &Store{root: root}, nil
}

// Root returns the resolved root without modifying the filesystem.
func (s *Store) Root() string {
	return s.root
}

// Initialize creates private state directories while refusing redirected paths.
func (s *Store) Initialize() error {
	if err := filesystem.CheckDirectory(s.root); err != nil {
		return err
	}

	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("initialize state at %s: %w", s.root, err)
	}

	for _, relative := range []string{"instances", "modules", "homes", "locks", "locks/instances"} {
		directory := filepath.Join(s.root, relative)
		if err := filesystem.CheckDirectory(directory); err != nil {
			return err
		}

		if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("initialize state directory %s: %w", directory, err)
		}
	}

	return nil
}

// InstanceDir returns a checked VM state path without creating it.
func (s *Store) InstanceDir(name domain.VMName) (string, error) {
	if _, err := domain.NewVMName(string(name)); err != nil {
		return "", err
	}

	parent := filepath.Join(s.root, "instances")
	if err := filesystem.CheckDirectory(parent); err != nil {
		return "", err
	}

	directory := filepath.Join(parent, string(name))
	if err := filesystem.CheckDirectory(directory); err != nil {
		return "", err
	}

	return directory, nil
}

// RequireAbsent rejects an existing VM state directory before preparation starts.
func (s *Store) RequireAbsent(name domain.VMName) error {
	directory, err := s.InstanceDir(name)
	if err != nil {
		return err
	}

	_, err = os.Lstat(directory)

	switch {
	case err == nil:
		return fs.ErrExist
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return err
	}
}

// Remove discards the checked state directory of one VM, never its managed home.
func (s *Store) Remove(name domain.VMName) error {
	directory, err := s.InstanceDir(name)
	if err != nil {
		return err
	}

	return os.RemoveAll(directory)
}

// RecordPath identifies the mutable record used in recovery diagnostics.
func (s *Store) RecordPath(name domain.VMName) (string, error) {
	directory, err := s.InstanceDir(name)
	if err != nil {
		return "", err
	}

	return filepath.Join(directory, "instance.json"), nil
}

// GenerationDir locates one validated input generation beneath its VM directory.
func (s *Store) GenerationDir(instance domain.Instance) (string, error) {
	if !domain.ValidIdentifier(instance.Generation) {
		return "", ErrInvalidGeneration
	}

	directory, err := s.InstanceDir(instance.Identity.Name)
	if err != nil {
		return "", err
	}

	return filepath.Join(directory, "generations", instance.Generation), nil
}
