package lima

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
	"github.com/lima-vm/lima/v2/pkg/osutil"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

func TestInstanceSocketByteBoundary(t *testing.T) {
	name := "limanix-sandbox-012345abcdef"
	for _, unicode := range []bool{false, true} {
		for _, length := range []int{osutil.UnixPathMax - 1, osutil.UnixPathMax} {
			rootLength := length - len(name) - len(filenames.LongestSock) - 2
			root := "/" + strings.Repeat("h", rootLength-1)
			if unicode {
				root = "/界" + strings.Repeat("h", rootLength-4)
			}
			t.Run(fmt.Sprintf("bytes=%d/unicode=%t", length, unicode), func(t *testing.T) {
				t.Setenv("LIMA_HOME", root)
				err := validateInstanceSocket(name)
				if length < osutil.UnixPathMax && err != nil {
					t.Fatalf("socket of %d bytes rejected: %v", length, err)
				}
				if length >= osutil.UnixPathMax && (err == nil || !strings.Contains(err.Error(), "SSH socket path")) {
					t.Fatalf("socket of %d bytes accepted: %v", length, err)
				}
			})
		}
	}
}

func TestInstanceSocketResolvesLimaHomeSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, strings.Repeat("h", osutil.UnixPathMax))
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "short")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIMA_HOME", alias)
	if err := validateInstanceSocket("limanix-sandbox-012345abcdef"); err == nil {
		t.Fatal("the symlink's short spelling hid an oversized real socket path")
	}
}

func TestPreflightSocketFailurePrecedesHostPrerequisites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "lima")
	t.Setenv("LIMA_HOME", root)
	cfg := config.Default()
	cfg.Name = domain.VMName(strings.Repeat("n", 50))
	err := NewClient(nil).Preflight(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "SSH socket path") {
		t.Fatalf("expected early socket failure, got %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight created the Lima directory: %v", err)
	}
}
