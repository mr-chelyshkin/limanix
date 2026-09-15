package filesystem

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRequireDirectoryResolvesLinksAndMissingTail(t *testing.T) {
	root, err := RequireDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(root, "actual")
	if err := os.Mkdir(actual, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	resolved, err := RequireDirectory(link)
	if err != nil || resolved != actual {
		t.Fatalf("resolved %q: %v", resolved, err)
	}
	resolved, err = Resolve(filepath.Join(link, "new", "tail"))
	if err != nil || resolved != filepath.Join(actual, "new", "tail") {
		t.Fatalf("resolve missing tail %q: %v", resolved, err)
	}
	if _, err := RequireDirectory(filepath.Join(root, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RequireDirectory(filepath.Join(root, "file")); err == nil {
		t.Fatal("accepted ordinary file as directory")
	}
	if _, err := RequireDirectory("invalid\x00path"); err == nil {
		t.Fatal("accepted invalid path")
	}
}

func TestResolveAndAtomicWriteInterpretParentAfterSymlink(t *testing.T) {
	root, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(root, "outside", "child")
	if err := os.MkdirAll(actual, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"/..", "/../missing/tail", "/../missing/../tail"} {
		input := link + suffix
		resolved, err := Resolve(input)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Clean(actual + suffix)
		if resolved != want {
			t.Fatalf("resolve %q = %q, want %q", input, resolved, want)
		}
	}
	resolved, err := RequireDirectory(link + "/..")
	if err != nil || resolved != filepath.Dir(actual) {
		t.Fatalf("require directory %q: %v", resolved, err)
	}
	path := link + "/../record.json"
	resolved, err = WriteTextAtomic(path, "correct parent\n", 0)
	want := filepath.Join(root, "outside", "record.json")
	if err != nil || resolved != want {
		t.Fatalf("atomic write destination %q: %v, want %q", resolved, err, want)
	}
	data, err := os.ReadFile(want)
	if err != nil || string(data) != "correct parent\n" {
		t.Fatalf("wrong parent contents: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "record.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("wrote through prematurely cleaned parent: %v", err)
	}
	t.Setenv("HOME", root)
	expanded, err := ExpandHome("~/link/../missing/tail")
	if err != nil || expanded != root+"/link/../missing/tail" {
		t.Fatalf("expanded path cleaned too soon: %q %v", expanded, err)
	}
	resolved, err = Resolve("~/link/../missing/tail")
	if err != nil || resolved != filepath.Join(root, "outside", "missing", "tail") {
		t.Fatalf("home path resolved incorrectly: %q %v", resolved, err)
	}
}

func TestResolveDanglingLinksAndLoops(t *testing.T) {
	root, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink("missing/target", dangling); err != nil {
		t.Fatal(err)
	}
	resolved, err := Resolve(dangling + "/tail")
	if err != nil || resolved != filepath.Join(root, "missing", "target", "tail") {
		t.Fatalf("dangling link resolution %q: %v", resolved, err)
	}
	if _, err := RequireDirectory(dangling); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("require dangling link: %v", err)
	}
	loop := filepath.Join(root, "loop")
	if err := os.Symlink("loop", loop); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(loop); err == nil {
		t.Fatal("accepted a symbolic link loop")
	}
}

func TestAtomicWritePrivateAndPreservedMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if _, err := WriteTextAtomic(path, "first\n", 0); err != nil {
		t.Fatal(err)
	}
	assertMode(t, path, 0o600)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteTextAtomic(path, "second\n", 0); err != nil {
		t.Fatal(err)
	}
	assertMode(t, path, 0o640)
	if err := WriteFileAtomic(path, []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertMode(t, path, 0o600)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "private\n" {
		t.Fatalf("data %q: %v", data, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}

func TestAtomicWriteRejectsSymlinksAndSpecialFiles(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "original")
	if err := os.WriteFile(original, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	broken := filepath.Join(root, "broken")
	folder := filepath.Join(root, "folder")
	fifo := filepath.Join(root, "fifo")
	for linkPath, target := range map[string]string{link: original, broken: filepath.Join(root, "absent")} {
		if err := os.Symlink(target, linkPath); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, broken, folder, fifo} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := WriteTextAtomic(path, "new", 0); err == nil {
				t.Fatal("accepted nonregular destination")
			}
		})
	}
	data, err := os.ReadFile(original)
	if err != nil || string(data) != "keep" {
		t.Fatalf("original changed: %q %v", data, err)
	}
}

func TestAtomicWriteFailurePreservesFileAndCreatesNoTemporary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteTextAtomic(path, string([]byte{0xff}), 0); err == nil {
		t.Fatal("accepted invalid UTF-8")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep" {
		t.Fatalf("original changed: %q %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}

func TestReadonlyFilesAndDirectoryRefuseReplacement(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses Unix permission checks")
	}
	for _, readonly := range []string{"file", "directory"} {
		t.Run(readonly, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "config")
			if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			permissionPath, mode := path, fs.FileMode(0o400)
			if readonly == "directory" {
				permissionPath, mode = root, 0o500
			}
			if err := os.Chmod(permissionPath, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(permissionPath, 0o700); err != nil {
					t.Error(err)
				}
			})
			if _, err := WriteTextAtomic(path, "new", 0); err == nil {
				t.Fatal("replaced readonly file")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "keep" {
				t.Fatalf("original changed: %q %v", data, err)
			}
		})
	}
}

func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode %#o, want %#o", path, got, want)
	}
}
