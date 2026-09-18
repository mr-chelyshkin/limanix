package nixos

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

//go:embed resources
var resources embed.FS

func copyResource(source, destination string) error {
	return fs.WalkDir(resources, source, func(resourcePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relative := strings.TrimPrefix(strings.TrimPrefix(resourcePath, source), "/")
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}

		data, err := resources.ReadFile(resourcePath)
		if err != nil {
			return err
		}

		return filesystem.WriteFileAtomic(target, data, 0o600)
	})
}
