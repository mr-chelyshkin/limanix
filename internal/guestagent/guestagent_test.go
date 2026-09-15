package guestagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Cache tests need an ELF header, not the full Lima executable. The embedded
// payloads are exercised separately for both architectures below.
func cacheFixture(t *testing.T) *Cache {
	t.Helper()
	header := elf.Header64{
		Ident:   [elf.EI_NIDENT]byte{0x7f, 'E', 'L', 'F', byte(elf.ELFCLASS64), byte(elf.ELFDATA2LSB), byte(elf.EV_CURRENT)},
		Type:    uint16(elf.ET_EXEC),
		Machine: uint16(elf.EM_AARCH64),
		Version: uint32(elf.EV_CURRENT),
		Ehsize:  64,
	}
	var archive bytes.Buffer
	writer := gzip.NewWriter(&archive)
	if err := binary.Write(writer, binary.LittleEndian, header); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	cache := New(t.TempDir())
	cache.readFile = func(string) ([]byte, error) { return archive.Bytes(), nil }
	return cache
}

func TestEmbeddedAgentsMaterializePrivatelyForBothArchitectures(t *testing.T) {
	cache := New(t.TempDir())
	root, err := filesystem.Resolve(cache.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range domain.Architectures() {
		path, err := cache.Path(context.Background(), arch)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("agent cache permissions: %v, %v", info, err)
		}
		if !strings.HasSuffix(path, ".gz") || filepath.Dir(filepath.Dir(path)) != root {
			t.Fatalf("unexpected cached path: %s", path)
		}
	}
}

func TestMissingAndCanceledAgentsDoNotCreateCache(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	cache := New(root)
	cache.readFile = func(string) ([]byte, error) { return nil, fs.ErrNotExist }
	if _, err := cache.Path(context.Background(), domain.ARM64); err == nil || !strings.Contains(err.Error(), "assets/generate") {
		t.Fatalf("missing payload did not fail clearly: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.Path(ctx, domain.ARM64); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("missing/canceled resource created cache")
	}
}

func TestCacheReuseAndCorruptionIsolation(t *testing.T) {
	cache := cacheFixture(t)
	path, err := cache.Path(context.Background(), domain.ARM64)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := cache.Path(context.Background(), domain.ARM64)
	if err != nil || again != path {
		t.Fatalf("immutable cached path changed: %s, %v", again, err)
	}
	unchanged, err := os.Stat(again)
	if err != nil || unchanged.ModTime() != info.ModTime() {
		t.Fatal("existing agent was unnecessarily rewritten")
	}
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Path(context.Background(), domain.ARM64); err == nil {
		t.Fatal("corrupt cache was accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Path(context.Background(), domain.ARM64); err == nil {
		t.Fatal("redirected cache was accepted")
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != "keep" {
		t.Fatal("redirect target changed")
	}
}

func TestConcurrentMaterializationUsesTheSameContentAddress(t *testing.T) {
	cache := cacheFixture(t)
	var group sync.WaitGroup
	paths := make(chan string, 4)
	for range 4 {
		group.Go(func() {
			path, err := cache.Path(context.Background(), domain.ARM64)
			if err != nil {
				t.Error(err)
				return
			}
			paths <- path
		})
	}
	group.Wait()
	close(paths)
	first := ""
	for path := range paths {
		if first == "" {
			first = path
		}
		if path != first {
			t.Fatal("concurrent calls selected different payload paths")
		}
	}
}

func TestWrongArchitectureArchiveIsRejected(t *testing.T) {
	archive, err := cacheFixture(t).readFile("")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateArchive(archive, domain.AMD64); err == nil {
		t.Fatal("guest architecture mismatch was accepted")
	}
	if err := validateArchive([]byte("not gzip"), domain.ARM64); err == nil {
		t.Fatal("corrupt archive accepted")
	}
}
