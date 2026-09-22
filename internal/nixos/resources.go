package nixos

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/nixos/catalog"
)

//go:embed resources/base resources/modules.zip
var resources embed.FS

var systemCatalog = sync.OnceValues(func() (*catalog.Catalog, error) {
	data, err := resources.ReadFile("resources/modules.zip")
	if err != nil {
		return nil, err
	}

	return catalog.Open(data)
})

func copyResource(source, destination string) error {
	files, err := fs.Sub(resources, source)
	if err != nil {
		return err
	}

	return copyFiles(files, destination)
}

func copyFiles(files fs.FS, destination string) error {
	return fs.WalkDir(files, ".", func(resourcePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := filepath.Join(destination, filepath.FromSlash(resourcePath))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}

		data, err := fs.ReadFile(files, resourcePath)
		if err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		return filesystem.WriteFileAtomic(target, data, 0o600|info.Mode().Perm()&0o100)
	})
}
