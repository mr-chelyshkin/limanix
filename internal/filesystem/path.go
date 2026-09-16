package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// ExpandHome expands a leading tilde without cleaning symlinks followed by '..'.
// Named-user forms use the host user database.
func ExpandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	name, remainder, _ := strings.Cut(path[1:], string(filepath.Separator))
	var home string

	if name == "" {
		var err error

		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	} else {
		account, err := user.Lookup(name)
		if err != nil {
			return "", err
		}

		home = account.HomeDir
	}

	if remainder == "" {
		return home, nil
	}

	return home + string(filepath.Separator) + remainder, nil
}

// Resolve returns an absolute path, resolving existing ancestors even if its tail is absent.
func Resolve(path string) (string, error) {
	expanded, err := ExpandHome(path)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(expanded) {
		current, err := os.Getwd()
		if err != nil {
			return "", err
		}

		expanded = current + string(filepath.Separator) + expanded
	}

	return resolvePath(expanded, 0)
}

// Resolve each component before interpreting '..'; lexical cleanup would lose
// the parent selected by a symlink. Missing tails remain usable for later creation.
func resolvePath(value string, symlinkCount int) (string, error) {
	if symlinkCount > 40 {
		return "", ErrSymlinkLimit
	}

	current := string(filepath.Separator)
	components := strings.Split(strings.TrimPrefix(value, current), current)

	for index, component := range components {
		if component == "" || component == "." {
			continue
		}

		if component == ".." {
			current = filepath.Dir(current)
			continue
		}

		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return "", err
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			continue
		}

		target, err := os.Readlink(current)
		if err != nil {
			return "", err
		}

		if !filepath.IsAbs(target) {
			target = filepath.Dir(current) + string(filepath.Separator) + target
		}

		if index+1 < len(components) {
			target += string(filepath.Separator) + strings.Join(components[index+1:], string(filepath.Separator))
		}

		return resolvePath(target, symlinkCount+1)
	}

	return current, nil
}

// RequireDirectory resolves a directory path, including symlinks and a leading tilde.
func RequireDirectory(path string) (string, error) {
	resolved, err := Resolve(path)
	if err != nil {
		return "", wrapError(path, "use directory", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", wrapError(path, "use directory", err)
	}

	if !info.IsDir() {
		return "", wrapError(path, "use directory", ErrNotDirectory)
	}

	return resolved, nil
}

// CheckDirectory rejects existing symlinks and non-directories without creating a path.
func CheckDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", ErrDirectorySymlink, path)
	}

	if !info.IsDir() {
		return fmt.Errorf("%w: %s", ErrNotDirectory, path)
	}

	return nil
}
