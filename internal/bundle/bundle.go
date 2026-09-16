package bundle

import (
	"context"
	"embed"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Embedding the directory excludes dot-prefixed build and atomic-write leftovers.
//
//go:embed resources
var resources embed.FS

// Cache materializes compressed guest agents in a host-only, content-addressed directory.
type Cache struct {
	root string
}

// New selects a cache beneath Limanix's runtime storage.
func New(root string) *Cache {
	return &Cache{
		root: root,
	}
}

// Path returns the private gzip archive accepted by Lima's native StartWithPaths.
func (cache *Cache) Path(ctx context.Context, arch domain.Architecture) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	archive, err := embeddedArchive(arch)
	if err != nil {
		return "", err
	}

	directory, err := cache.prepareDirectory(archive.digest)
	if err != nil {
		return "", err
	}

	destination := filepath.Join(directory, archive.name)
	if err = ensureCachedArchive(ctx, destination, archive.data); err != nil {
		return "", err
	}

	if err = ctx.Err(); err != nil {
		return "", err
	}

	return destination, nil
}
