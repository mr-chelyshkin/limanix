package lima

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/limatype/dirnames"
	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
)

func TestSSHHelper(t *testing.T) {
	index := 0
	for index < len(os.Args) && os.Args[index] != "--" {
		index++
	}
	if index+1 >= len(os.Args) {
		return
	}
	mode := os.Args[index+1]
	switch mode {
	case "arguments":
		_ = json.NewEncoder(os.Stdout).Encode(os.Args[index+2:])
	case "failure":
		_, _ = fmt.Fprintln(os.Stderr, "guest unavailable")
		os.Exit(17)
	case "signal":
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	case "ssh-version":
		_, _ = fmt.Fprintln(os.Stderr, "OpenSSH_9.9p1")
	case "wait":
		time.Sleep(time.Hour)
	case "wait-group-ready", "wait-ignore-ready":
		if index+2 >= len(os.Args) {
			os.Exit(2)
		}
		ready := os.Args[index+2]
		signal.Ignore(syscall.SIGTERM)
		if mode == "wait-group-ready" {
			child := exec.Command(os.Args[0], "-test.run=^TestSSHHelper$", "--", "wait-ignore-ready", ready+".child")
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
			if err := child.Start(); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
		}
		if err := os.WriteFile(ready+".tmp", []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := os.Rename(ready+".tmp", ready); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		time.Sleep(time.Hour)
	}
	os.Exit(0)
}

func TestRemoteScriptPreservesLiteralArguments(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "unwanted")
	values := []string{"", "space value", "line one\nline two", `quotes ' " $HOME`, "$(touch " + marker + "); touch " + marker}
	args := append([]string{os.Args[0], "-test.run=TestSSHHelper", "--", "arguments"}, values...)
	command := exec.Command("/bin/sh", "-c", remoteScript(args))
	command.Env = append(os.Environ(), "SHELL=/bin/sh", "HOME="+t.TempDir(), "GORACE="+os.Getenv("GORACE")+" atexit_sleep_ms=0")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	if err := json.Unmarshal(output, &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, values) {
		t.Fatalf("remote shell changed arguments: %#v", actual)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("guest argument executed as shell syntax: %v", err)
	}
}

func helperSSHClient(t *testing.T, mode string) *Client {
	t.Helper()
	client := NewClient(nil)
	client.Stdout, client.Stderr = io.Discard, io.Discard
	client.inspect = func(_ context.Context, name string) (*limatype.Instance, error) {
		return &limatype.Instance{Name: name, Status: limatype.StatusRunning}, nil
	}
	client.sshCommand = func(ctx context.Context, _ *limatype.Instance, args []string, _ bool) (*exec.Cmd, error) {
		command := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=TestSSHHelper", "--", mode}, args...)...)
		// Keep race detection enabled without its shutdown delay in helper processes.
		command.Env = append(os.Environ(), "GORACE="+os.Getenv("GORACE")+" atexit_sleep_ms=0")
		return command, nil
	}
	return client
}

func TestSSHRunReportsBoundedFailureWithoutArguments(t *testing.T) {
	client := helperSSHClient(t, "failure")
	_, err := client.Run(context.Background(), "limanix-test", []string{"command", "secret-argument"}, true)
	if err == nil || !strings.Contains(err.Error(), "guest unavailable") || !strings.Contains(err.Error(), "status 17") || strings.Contains(err.Error(), "secret-argument") {
		t.Fatalf("unexpected SSH error: %v", err)
	}
	buffer := &tailBuffer{limit: 8}
	_, _ = buffer.Write([]byte("old-data"))
	_, _ = buffer.Write([]byte("new"))
	if !bytes.Equal(buffer.data, []byte("-datanew")) {
		t.Fatalf("unexpected stderr tail: %s", buffer.data)
	}
	_, _ = buffer.Write([]byte("0123456789"))
	if !bytes.Equal(buffer.data, []byte("23456789")) {
		t.Fatalf("unexpected long stderr tail: %s", buffer.data)
	}
}

func TestSSHRunCancellation(t *testing.T) {
	client := helperSSHClient(t, "wait")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := client.Run(ctx, "limanix-test", []string{"wait"}, true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestSSHRunCancellationBeforeProcessStarts(t *testing.T) {
	client := helperSSHClient(t, "arguments")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	factory := client.sshCommand
	var command *exec.Cmd
	client.sshCommand = func(ctx context.Context, instance *limatype.Instance, args []string, interactive bool) (*exec.Cmd, error) {
		cancel()
		var err error
		command, err = factory(ctx, instance, args, interactive)
		return command, err
	}
	_, err := client.Run(ctx, "limanix-test", []string{"true"}, true)
	if !errors.Is(err, context.Canceled) || command == nil || command.Process != nil {
		t.Fatalf("pre-start cancellation lost its cause or started a process: command=%v error=%v", command, err)
	}
}

func TestSSHRunCancellationCleansProcessGroup(t *testing.T) {
	client := helperSSHClient(t, "wait-group-ready")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := filepath.Join(t.TempDir(), "ready")
	result := make(chan error, 1)
	go func() {
		_, err := client.Run(ctx, "limanix-test", []string{ready}, true)
		result <- err
	}()
	readPID := func(path string) (int, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		pid, err := strconv.Atoi(string(data))
		if err != nil || pid <= 0 {
			return 0, fmt.Errorf("helper reported invalid PID: %q, %v", data, err)
		}
		return pid, nil
	}
	defer func() {
		if pid, err := readPID(ready); err == nil {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
		if pid, err := readPID(ready + ".child"); err == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}()
	var parentPID, childPID int
	startup := time.NewTimer(3 * time.Second)
	defer startup.Stop()
	for parentPID == 0 || childPID == 0 {
		for path, target := range map[string]*int{ready: &parentPID, ready + ".child": &childPID} {
			pid, err := readPID(path)
			if err == nil {
				*target = pid
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
		}
		if parentPID != 0 && childPID != 0 {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("helper exited before becoming ready: %v", err)
		case <-startup.C:
			t.Fatal("SSH subprocess did not become ready")
		case <-time.After(5 * time.Millisecond):
		}
	}
	group, err := syscall.Getpgid(childPID)
	if err != nil || group != parentPID {
		t.Fatalf("helper child is outside the SSH process group: group=%d parent=%d error=%v", group, parentPID, err)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("running SSH cancellation lost its cause: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("running SSH subprocess did not stop after cancellation")
	}
	if err := syscall.Kill(parentPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("canceled SSH subprocess still exists: pid=%d, error=%v", parentPID, err)
	}
	reaped := time.NewTimer(2 * time.Second)
	defer reaped.Stop()
	for {
		if err := syscall.Kill(childPID, 0); errors.Is(err, syscall.ESRCH) {
			break
		} else if err != nil {
			t.Fatalf("check canceled SSH helper: %v", err)
		}
		select {
		case <-reaped.C:
			t.Fatalf("canceled SSH helper still exists: pid=%d group=%d", childPID, group)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestSSHRunDoesNotAddAnExecutionDeadline(t *testing.T) {
	client := helperSSHClient(t, "arguments")
	factory := client.sshCommand
	client.sshCommand = func(ctx context.Context, instance *limatype.Instance, args []string, interactive bool) (*exec.Cmd, error) {
		if deadline, exists := ctx.Deadline(); exists {
			t.Fatalf("management command acquired an overall execution deadline: %v", deadline)
		}
		return factory(ctx, instance, args, interactive)
	}
	if _, err := client.Run(context.Background(), "limanix-test", []string{"nixos-rebuild", "boot"}, true); err != nil {
		t.Fatal(err)
	}
}

func TestSSHCommandsUseAConnectionTimeout(t *testing.T) {
	t.Setenv("LIMA_HOME", t.TempDir())
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	t.Setenv("SSH", shellescape.Quote(os.Args[0])+" -test.run=TestSSHHelper -- ssh-version")
	configDir, err := dirnames.LimaConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, filenames.UserPrivateKey), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	instanceDir, err := os.MkdirTemp("", "ls-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(instanceDir); err != nil {
			t.Error(err)
		}
	})
	username := "limanix-admin"
	instance := &limatype.Instance{
		Name: "limanix-test", Dir: instanceDir, SSHAddress: "127.0.0.1", SSHLocalPort: 60022,
		Config: &limatype.LimaYAML{User: limatype.User{Name: &username}},
	}
	client := NewClient(nil)
	client.Stdout = io.Discard
	for _, interactive := range []bool{false, true} {
		command, err := client.newSSHCommand(context.Background(), instance, []string{"true"}, interactive)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for index := 0; index+1 < len(command.Args); index++ {
			if command.Args[index] == "-o" && command.Args[index+1] == "ConnectTimeout=10" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("SSH connection timeout missing (interactive=%v): %v", interactive, command.Args)
		}
	}
}

func TestSSHShellPreservesExitAndSignals(t *testing.T) {
	for _, test := range []struct {
		mode string
		code int
	}{{"failure", 17}, {"signal", 128 + int(syscall.SIGTERM)}} {
		client := helperSSHClient(t, test.mode)
		code, err := client.Shell(context.Background(), "limanix-test", []string{"command"})
		if err != nil || code != test.code {
			t.Fatalf("%s: code=%d error=%v", test.mode, code, err)
		}
	}
}

func TestSSHSessionRejectsStoppedAndForeignInstances(t *testing.T) {
	client := helperSSHClient(t, "arguments")
	client.inspect = func(_ context.Context, name string) (*limatype.Instance, error) {
		return &limatype.Instance{Name: name, Status: limatype.StatusStopped}, nil
	}
	if _, err := client.Run(context.Background(), "limanix-test", []string{"true"}, true); err == nil {
		t.Fatal("stopped VM accepted an SSH command")
	}
	if _, err := client.Run(context.Background(), "foreign", []string{"true"}, true); err == nil {
		t.Fatal("foreign VM accepted an SSH command")
	}
}
