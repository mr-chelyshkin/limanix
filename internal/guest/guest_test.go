package guest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/lima"
)

type call struct {
	operation string
	args      []string
	capture   bool
}
type fakeClient struct {
	calls       []call
	output      string
	failureAt   int
	shellStatus int
	run         func(context.Context) (string, error)
}

func (client *fakeClient) Run(ctx context.Context, _ string, args []string, capture bool) (string, error) {
	client.calls = append(client.calls, call{operation: "run", args: args, capture: capture})
	if client.run != nil {
		return client.run(ctx)
	}
	if client.failureAt > 0 && len(client.calls) == client.failureAt {
		return "", errors.New("build failed")
	}
	return client.output, nil
}

func (client *fakeClient) Start(context.Context, string) error {
	client.calls = append(client.calls, call{operation: "start"})
	return nil
}

func (client *fakeClient) Stop(context.Context, string) error {
	client.calls = append(client.calls, call{operation: "stop"})
	return nil
}

func (client *fakeClient) Shell(_ context.Context, _ string, args []string) (int, error) {
	client.calls = append(client.calls, call{operation: "shell", args: args})
	return client.shellStatus, nil
}

func TestApplyInstallsEnvironmentAndRebootsOnlyAfterBuild(t *testing.T) {
	client := &fakeClient{run: func(ctx context.Context) (string, error) {
		if deadline, exists := ctx.Deadline(); exists {
			t.Fatalf("guest configuration command acquired a probe deadline: %v", deadline)
		}
		return "", nil
	}}
	guest := New(client)
	if err := guest.Apply(context.Background(), "sandbox", "developer"); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != 7 {
		t.Fatalf("unexpected apply sequence: %v", client.calls)
	}
	for index, file := range []string{"environment", "environment.sh"} {
		if !reflect.DeepEqual(client.calls[index+1].args, []string{"sudo", "install", "-m", "0644", "/mnt/limanix/" + file, "/etc/limanix/" + file}) {
			t.Fatal("runtime ENV must be available guest-wide before rebuild")
		}
	}
	if client.calls[3].capture || client.calls[3].args[1] != "nixos-rebuild" || client.calls[4].operation != "stop" || client.calls[5].operation != "start" || client.calls[6].args[len(client.calls[6].args)-1] != "true" {
		t.Fatalf("unexpected rebuild/reboot order: %v", client.calls)
	}
	client = &fakeClient{failureAt: 4}
	if err := New(client).Apply(context.Background(), "sandbox", "developer"); err == nil {
		t.Fatal("rebuild failure disappeared")
	}
	if len(client.calls) != 4 {
		t.Fatal("failed build rebooted the guest")
	}
}

func TestAddressMatchesSharedMAC(t *testing.T) {
	client := &fakeClient{output: `[{"address":"52:55:55:00:00:01","addr_info":[{"family":"inet","scope":"global","local":"192.168.5.15"}]},{"address":"52:55:55:AA:BB:CC","addr_info":[{"family":"inet6","scope":"global","local":"2001:db8::1"},{"family":"inet","scope":"global","local":"192.0.2.10"}]}]`}
	instance := lima.Instance{Name: "sandbox", Status: lima.Running, Networks: []lima.Network{{MACAddress: "52:55:55:aa:bb:cc", Shared: true}}}
	guest := New(client)
	if address := guest.Address(context.Background(), instance); address != "192.0.2.10" {
		t.Fatalf("wrong interface selected: %s", address)
	}
	for _, output := range []string{"not-json", "null", "[1]", `[{"address":null}]`, "[]", `[{"address":"52:55:55:aa:bb:cc","addr_info":[{"family":"inet","scope":"global","local":"invalid"}]}]`} {
		client.output = output
		if address := guest.Address(context.Background(), instance); address != "" {
			t.Fatalf("unexpected unavailable address: %s", address)
		}
	}
	client.calls = nil
	instance.Status = lima.Stopped
	if guest.Address(context.Background(), instance) != "" || len(client.calls) != 0 {
		t.Fatal("queried stopped instance")
	}
}

func TestAddressBoundsProbeWithoutCancelingCaller(t *testing.T) {
	caller, cancel := context.WithCancel(context.Background())
	defer cancel()
	var probe context.Context
	client := &fakeClient{run: func(ctx context.Context) (string, error) {
		probe = ctx
		deadline, exists := ctx.Deadline()
		if remaining := time.Until(deadline); !exists || remaining <= 0 || remaining > 5*time.Second {
			t.Fatalf("address probe has no bounded deadline: %v, %v", deadline, exists)
		}
		return "", context.DeadlineExceeded
	}}
	instance := lima.Instance{Name: "sandbox", Status: lima.Running, Networks: []lima.Network{{MACAddress: "52:55:55:aa:bb:cc", Shared: true}}}
	guest := New(client)
	if address := guest.Address(caller, instance); address != "" {
		t.Fatalf("failed probe returned an address: %q", address)
	}
	if probe == nil {
		t.Fatal("address query did not reach the client")
	}
	if probe == caller || !errors.Is(probe.Err(), context.Canceled) || caller.Err() != nil {
		t.Fatalf("probe context was not released independently: probe=%v, caller=%v", probe.Err(), caller.Err())
	}
	client.run = nil
	client.output = `[{"address":"52:55:55:aa:bb:cc","addr_info":[{"family":"inet","scope":"global","local":"192.0.2.10"}]}]`
	if address := guest.Address(caller, instance); address != "192.0.2.10" {
		t.Fatalf("failed probe prevented the next address query: %q", address)
	}
}

func TestAddressHonorsEarlierCallerDeadline(t *testing.T) {
	caller, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	callerDeadline, _ := caller.Deadline()
	client := &fakeClient{run: func(ctx context.Context) (string, error) {
		if deadline, exists := ctx.Deadline(); !exists || !deadline.Equal(callerDeadline) {
			t.Fatalf("address query extended the caller deadline: %v", deadline)
		}
		<-ctx.Done()
		return "", ctx.Err()
	}}
	instance := lima.Instance{Name: "sandbox", Status: lima.Running, Networks: []lima.Network{{MACAddress: "52:55:55:aa:bb:cc", Shared: true}}}
	if address := New(client).Address(caller, instance); address != "" || !errors.Is(caller.Err(), context.DeadlineExceeded) {
		t.Fatalf("deadline did not end address query: address=%q, caller=%v", address, caller.Err())
	}
}

func TestUserCommandPreservesArgumentsInRealShell(t *testing.T) {
	values := []string{"space $value 'double\" end", "", "line one\nline two", "* ; : # `printf unintended`", `\backslash\`, "unicode ✓"}
	command := userCommand("dev", append([]string{"printf", `%s\000`}, values...))
	index := slices.Index(command, "-c")
	if index < 0 {
		t.Fatal("missing fixed shell script")
	}
	directory := t.TempDir()
	process := exec.Command("/bin/sh", append([]string{"-c"}, command[index+1:]...)...)
	process.Env = []string{"HOME=" + directory, "PATH=/usr/bin:/bin"}
	output, err := process.Output()
	if err != nil {
		t.Fatal(err)
	}
	actual := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
	if !reflect.DeepEqual(actual, values) {
		encoded, _ := json.Marshal(actual)
		t.Fatalf("guest arguments were reinterpreted: %s", encoded)
	}
	command = userCommand("dev", []string{"pwd"})
	index = slices.Index(command, "-c")
	process = exec.Command("/bin/sh", append([]string{"-c"}, command[index+1:]...)...)
	process.Env = []string{"HOME=" + directory, "PATH=/usr/bin:/bin"}
	output, err = process.Output()
	if err != nil {
		t.Fatal(err)
	}
	actualDirectory, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatal(err)
	}
	expectedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if actualDirectory != expectedDirectory {
		t.Fatalf("guest command ran outside HOME: %s", actualDirectory)
	}
	if _, err := os.Stat(filepath.Join(directory, "unintended")); !os.IsNotExist(err) {
		t.Fatal("shell substitution executed")
	}
}

func TestInteractiveLoginOnceAndStatusPreserved(t *testing.T) {
	client := &fakeClient{shellStatus: 7}
	status, err := New(client).Shell(context.Background(), "sandbox", "dev", nil)
	if err != nil || status != 7 {
		t.Fatalf("shell status changed: %d %v", status, err)
	}
	count := 0
	for _, arg := range client.calls[0].args {
		if arg == "--login" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("interactive shell logged in %d times", count)
	}
}
