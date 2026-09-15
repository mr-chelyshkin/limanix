package hostagent

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

func TestHostAgentAcceptsOnlyGeneratedOwnedNamesAndPaths(t *testing.T) {
	root, err := filesystem.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIMA_HOME", root)
	name := "limanix-rust-box-abcdef123456"
	directory := filepath.Join(root, name)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	pid := filepath.Join(directory, filenames.HostAgentPID)
	socket := filepath.Join(directory, filenames.HostAgentSock)
	if err := validatePaths(name, pid, socket); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{"foreign", "limanix-box", "limanix-box-../123456", "limanix-Box-abcdef123456", "limanix-box-ABCDEF123456", "limanix--abcdef123456", "limanix-box-abcdef123456/child"} {
		if err := validateName(candidate); err == nil {
			t.Errorf("accepted %q", candidate)
		}
	}
	if err := validatePaths(name, filepath.Join(root, "other.pid"), socket); err == nil {
		t.Fatal("foreign PID file accepted")
	}
	if err := validatePaths(name, pid, filepath.Join(root, "other.sock")); err == nil {
		t.Fatal("foreign socket accepted")
	}
	command := Command()
	if !command.Hidden {
		t.Fatal("internal process interface became public")
	}
	command.SetArgs([]string{"--pidfile", pid, "--socket", socket, name})
	command.SetContext(context.Background())
	command.SilenceUsage, command.SilenceErrors = true, true
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "guest-agent") {
		t.Fatalf("missing embedded agent accepted: %v", err)
	}
	if _, err := os.Stat(pid); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed preflight wrote PID file")
	}
}

func TestPIDLeaseExcludesAnotherProcessAndPreservesRedirects(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "ha.pid")
	lease, err := acquirePID(filename)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != strconv.Itoa(os.Getpid())+"\n" {
		t.Fatal("wrong PID written")
	}
	info, err := os.Stat(filename)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("PID file is not private")
	}
	if second, err := acquirePID(filename); err == nil {
		_ = second.Close()
		t.Fatal("another host agent acquired PID lease")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filename); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("PID file was retained after shutdown")
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filename); err != nil {
		t.Fatal(err)
	}
	if lease, err := acquirePID(filename); err == nil {
		_ = lease.Close()
		t.Fatal("PID symlink accepted")
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != "keep" {
		t.Fatal("PID redirect target changed")
	}
}

func TestStalePIDCanBeReclaimedAndLivePIDCannot(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 0")
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "ha.pid")
	if err := os.WriteFile(filename, []byte(strconv.Itoa(command.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lease, err := acquirePID(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if lease, err := acquirePID(filename); err == nil {
		_ = lease.Close()
		t.Fatal("live PID reclaimed")
	}
}

func TestSocketCleanupRefusesOtherFiles(t *testing.T) {
	// Unix socket paths have a much smaller limit than ordinary filesystem paths.
	directory, err := os.MkdirTemp("", "ha-sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	filename := filepath.Join(directory, "ha.sock")
	if err := os.WriteFile(filename, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeSocket(filename); err == nil {
		t.Fatal("regular file removed as socket")
	}
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if err := removeSocket(filename); err != nil {
		t.Fatal(err)
	}
}

type readyListener struct {
	net.Listener
	ready chan struct{}
	once  sync.Once
}

func (listener *readyListener) Accept() (net.Conn, error) {
	listener.once.Do(func() { close(listener.ready) })
	return listener.Listener.Accept()
}

func TestSocketClosePreservesRegularReplacement(t *testing.T) {
	directory, err := os.MkdirTemp("", "ha-sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	socket := filepath.Join(directory, "ha.sock")
	listener, err := listenSocket(context.Background(), socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	ready := make(chan struct{})
	serveErrors := make(chan error, 1)
	apiServer := &http.Server{}
	defer func() { _ = apiServer.Close() }()
	go func() {
		serveErrors <- apiServer.Serve(&readyListener{Listener: listener, ready: ready})
	}()
	<-ready
	if err := os.Remove(socket); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(socket, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := apiServer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveErrors; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("HTTP server shutdown failed: %v", err)
	}
	if err := removeSocket(socket); err == nil {
		t.Fatal("regular replacement accepted as an owned socket")
	}
	data, err := os.ReadFile(socket)
	if err != nil || string(data) != "keep" {
		t.Fatalf("socket shutdown changed the replacement: %q, %v", data, err)
	}
}
