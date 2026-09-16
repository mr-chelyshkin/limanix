package lima

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
	"github.com/lima-vm/lima/v2/pkg/limayaml"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

func TestFetchAllFiltersBeforeInspection(t *testing.T) {
	client := NewClient(nil)
	client.native.instances = func() ([]string, error) { return []string{"colima", "other-limanix-vm", "limanix-owned"}, nil }
	client.native.inspect = func(_ context.Context, name string) (*limatype.Instance, error) {
		if name != "limanix-owned" {
			t.Fatalf("foreign VM inspected: %s", name)
		}
		nat := true
		return &limatype.Instance{Name: name, Status: limatype.StatusRunning, Disk: 10 * domain.GiB, Config: &limatype.LimaYAML{}, Networks: []limatype.Network{{MACAddress: "52:55:55:AA:BB:CC", VZNAT: &nat}}}, nil
	}
	instances, err := client.FetchAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].Status != Running || *instances[0].Disk != 10*domain.GiB || !reflect.DeepEqual(instances[0].Networks, []Network{{MACAddress: "52:55:55:aa:bb:cc", Shared: true}}) {
		t.Fatalf("unexpected metadata: %#v", instances)
	}
	client.native.instances = func() ([]string, error) { return []string{"colima"}, nil }
	instances, err = client.FetchAll(context.Background())
	if err != nil || instances == nil || len(instances) != 0 {
		t.Fatalf("expected empty nonnil list: %v %v", instances, err)
	}
}

func TestNativeMetadataStatusesAndDamagedInstances(t *testing.T) {
	for _, status := range []Status{Unknown, Uninitialized, Installing, Broken, Stopped, Running} {
		upstream := &limatype.Instance{Name: "limanix-owned", Status: string(status), Errors: []error{errors.New("damaged YAML")}}
		inst, err := instanceMetadata(upstream)
		if err != nil || inst.Status != status || inst.Disk != nil {
			t.Fatalf("status %q failed: %#v %v", status, inst, err)
		}
	}
	for _, upstream := range []*limatype.Instance{nil, {Name: "foreign", Status: limatype.StatusRunning}, {Name: "limanix-owned", Status: "FutureState"}, {Name: "limanix-owned", Status: limatype.StatusStopped, Disk: -1}} {
		if _, err := instanceMetadata(upstream); err == nil {
			t.Fatalf("accepted invalid metadata: %#v", upstream)
		}
	}
}

func TestNativeInspectionSkipsDeletedAndRejectsForeignNames(t *testing.T) {
	client := NewClient(nil)
	client.native.instances = func() ([]string, error) { return []string{"limanix-gone", "limanix-owned"}, nil }
	client.native.inspect = func(_ context.Context, name string) (*limatype.Instance, error) {
		if name == "limanix-gone" {
			return nil, os.ErrNotExist
		}
		return &limatype.Instance{Name: name, Status: limatype.StatusStopped}, nil
	}
	inst, err := client.FetchAll(context.Background())
	if err != nil || len(inst) != 1 || inst[0].Name != "limanix-owned" {
		t.Fatalf("unexpected concurrent-delete result: %v %v", inst, err)
	}
	if _, err := client.inspectInstance(context.Background(), "colima"); err == nil {
		t.Fatal("foreign name reached native inspection")
	}
}

func nativeLifecycleClient(t *testing.T, arch string) (*Client, *limatype.Instance) {
	t.Helper()
	t.Setenv("LIMA_HOME", t.TempDir())
	inst := &limatype.Instance{Name: "limanix-owned", Status: limatype.StatusStopped, Arch: arch, Config: &limatype.LimaYAML{}}
	client := NewClient(func(_ context.Context, actual domain.Architecture) (string, error) {
		expected, err := guestArchitecture(arch)
		if err != nil || actual != expected {
			t.Fatalf("wrong guest agent architecture: %s", actual)
		}
		return "/private/agents/guest.gz", nil
	})
	client.native.inspect = func(context.Context, string) (*limatype.Instance, error) { return inst, nil }
	client.executable = func() (string, error) { return "/application/limanix", nil }
	client.native.reconcile = func(context.Context, string) error { return nil }
	return client, inst
}

func TestNativeStartUsesSelfAndPackagedAgent(t *testing.T) {
	for _, arch := range []string{limatype.AARCH64, limatype.X8664} {
		t.Run(arch, func(t *testing.T) {
			client, inst := nativeLifecycleClient(t, arch)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var childContext context.Context
			var calls []string
			client.native.reconcile = func(_ context.Context, name string) error { calls = append(calls, "network:"+name); return nil }
			client.native.start = func(startup context.Context, actual *limatype.Instance, foreground, progress bool, self, agent string) error {
				childContext = startup
				if actual != inst || foreground || progress || self != "/application/limanix" || agent != "/private/agents/guest.gz" {
					t.Fatalf("incorrect native startup parameters: %v %v %s %s", foreground, progress, self, agent)
				}
				calls = append(calls, "start")
				return nil
			}
			if err := client.Start(ctx, inst.Name); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, []string{"network:limanix-owned", "start"}) {
				t.Fatalf("incorrect lifecycle order: %v", calls)
			}
			cancel()
			select {
			case <-childContext.Done():
				t.Fatal("successful hostagent retained the CLI's cancellation")
			default:
			}
		})
	}
}

func TestNativeStartForwardsCancellationAndCleansFailure(t *testing.T) {
	for _, mode := range []string{"inflight", "cancel-at-success", "failure"} {
		t.Run(mode, func(t *testing.T) {
			client, inst := nativeLifecycleClient(t, limatype.AARCH64)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var child context.Context
			client.native.start = func(actual context.Context, _ *limatype.Instance, _ bool, _ bool, _ string, _ string) error {
				child = actual
				switch mode {
				case "inflight":
					cancel()
					select {
					case <-actual.Done():
						return actual.Err()
					case <-time.After(time.Second):
						t.Fatal("startup did not receive cancellation")
					}
				case "cancel-at-success":
					cancel()
					return nil
				case "failure":
					return errors.New("startup failed")
				}
				return nil
			}
			err := client.Start(ctx, inst.Name)
			if err == nil {
				t.Fatal("startup incorrectly succeeded")
			}
			if mode != "failure" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation cause: %v", err)
			}
			select {
			case <-child.Done():
			default:
				t.Fatal("unsuccessful startup retained hostagent context")
			}
		})
	}
}

func TestNativeStartDoesNotRestartRunningOrDamagedInstance(t *testing.T) {
	client, inst := nativeLifecycleClient(t, limatype.AARCH64)
	client.native.start = func(context.Context, *limatype.Instance, bool, bool, string, string) error {
		t.Fatal("unexpected startup")
		return nil
	}
	inst.Status = limatype.StatusRunning
	if err := client.Start(context.Background(), inst.Name); err != nil {
		t.Fatal(err)
	}
	inst.Errors = []error{errors.New("damaged config")}
	if err := client.Start(context.Background(), inst.Name); err == nil {
		t.Fatal("started damaged instance")
	}
}

func TestNativeCreatePassesBytesWithoutBrokenTemplateSideEffect(t *testing.T) {
	client := NewClient(nil)
	path := filepath.Join(t.TempDir(), "template.yaml")
	data := []byte("vmType: qemu\ncpus: 2\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	client.native.create = func(_ context.Context, name string, actual []byte, saveBroken bool) (*limatype.Instance, error) {
		if name != "limanix-owned" || !bytes.Equal(actual, data) || saveBroken {
			t.Fatalf("incorrect create parameters: %s %q %v", name, actual, saveBroken)
		}
		return &limatype.Instance{Name: name}, nil
	}
	if err := client.Create(context.Background(), "limanix-owned", path); err != nil {
		t.Fatal(err)
	}
	if err := client.Create(context.Background(), "foreign", path); err == nil {
		t.Fatal("created foreign instance")
	}
	client.native.create = func(context.Context, string, []byte, bool) (*limatype.Instance, error) {
		return nil, os.ErrPermission
	}
	err := client.Create(context.Background(), "limanix-owned", path)
	if !errors.Is(err, os.ErrPermission) || !strings.HasPrefix(err.Error(), "Lima create:") {
		t.Fatalf("unexpected operation error: %v", err)
	}
}

func TestNativeStopDeleteOrderingAndForce(t *testing.T) {
	client, inst := nativeLifecycleClient(t, limatype.AARCH64)
	var calls []string
	client.native.reconcile = func(_ context.Context, name string) error {
		if name != "" {
			t.Fatal("stopped instance reactivated networking")
		}
		calls = append(calls, "network")
		return nil
	}
	client.native.stop = func(_ context.Context, actual *limatype.Instance, restart bool) error {
		if actual != inst || restart {
			t.Fatal("incorrect graceful stop")
		}
		calls = append(calls, "stop")
		return nil
	}
	if err := client.Stop(context.Background(), inst.Name); err != nil {
		t.Fatal(err)
	}
	inst.Errors = []error{errors.New("damaged config")}
	client.native.delete = func(_ context.Context, actual *limatype.Instance, force bool) error {
		if actual != inst || !force {
			t.Fatal("force or damaged metadata lost")
		}
		calls = append(calls, "delete")
		return nil
	}
	if err := client.Delete(context.Background(), inst.Name, true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"stop", "network", "delete", "network"}) {
		t.Fatalf("incorrect ordering: %v", calls)
	}
}

func editFixture(t *testing.T) (*Client, *limatype.Instance, string, []byte) {
	t.Helper()
	t.Setenv("LIMA_HOME", t.TempDir())
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	runtimeDir := filepath.Join(dir, "runtime")
	for _, path := range []string{home, runtimeDir} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Default()
	cfg.Mounts = nil
	cfg.Resources.CPU = 2
	data, err := Render(cfg, home, runtimeDir, "arm64", 501)
	if err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(dir, filenames.LimaYAML)
	if err := os.WriteFile(yamlPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	yaml, err := limayaml.Load(context.Background(), data, yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	inst := &limatype.Instance{Name: "limanix-owned", Status: limatype.StatusStopped, Dir: dir, Config: yaml}
	client := NewClient(nil)
	client.native.inspect = func(context.Context, string) (*limatype.Instance, error) { return inst, nil }
	client.native.validateDriver = func(context.Context, *limatype.Instance) error { return nil }
	return client, inst, filepath.Join(dir, "replacement.yaml"), data
}

func TestNativeEditValidatesThenAtomicallyReplaces(t *testing.T) {
	client, inst, path, previous := editFixture(t)
	data := bytes.Replace(previous, []byte(`"cpus": 2`), []byte(`"cpus": 3`), 1)
	if bytes.Equal(data, previous) {
		t.Fatal("fixture CPU replacement failed")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	validated := false
	client.native.validateDriver = func(_ context.Context, actual *limatype.Instance) error {
		if *actual.Config.CPUs != 3 {
			t.Fatal("driver did not receive new template")
		}
		validated = true
		return nil
	}
	if err := client.Edit(context.Background(), inst.Name, path); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(inst.Dir, filenames.LimaYAML))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(inst.Dir, filenames.LimaYAML))
	if err != nil {
		t.Fatal(err)
	}
	if !validated || !bytes.Equal(actual, data) || info.Mode().Perm() != 0o600 {
		t.Fatalf("edit lost validation/data/permissions: %v %o", validated, info.Mode().Perm())
	}
}

func TestNativeEditFailurePreservesConfiguration(t *testing.T) {
	for _, mode := range []string{"running", "invalid-schema", "driver", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			client, inst, path, previous := editFixture(t)
			data := bytes.Replace(previous, []byte(`"cpus": 2`), []byte(`"cpus": 3`), 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "running":
				inst.Status = limatype.StatusRunning
			case "invalid-schema":
				data = []byte(`{"cpus":-1}`)
			case "driver":
				client.native.validateDriver = func(context.Context, *limatype.Instance) error { return errors.New("driver rejected changes") }
			case "cancel":
				client.native.validateDriver = func(context.Context, *limatype.Instance) error { cancel(); return nil }
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := client.Edit(ctx, inst.Name, path); err == nil {
				t.Fatal("rejected edit succeeded")
			}
			actual, err := os.ReadFile(filepath.Join(inst.Dir, filenames.LimaYAML))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, previous) {
				t.Fatal("rejected edit replaced persistent YAML")
			}
		})
	}
}

func TestNativeOperationsRejectAlreadyCanceledContext(t *testing.T) {
	client := NewClient(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client.native.instances = func() ([]string, error) { t.Fatal("enumerated after cancellation"); return nil, nil }
	client.native.inspect = func(context.Context, string) (*limatype.Instance, error) {
		t.Fatal("inspected after cancellation")
		return nil, nil
	}
	for _, call := range []func() error{func() error { _, err := client.FetchAll(ctx); return err }, func() error { return client.Start(ctx, "limanix-owned") }, func() error { return client.Stop(ctx, "limanix-owned") }, func() error { return client.Delete(ctx, "limanix-owned", true) }, func() error { return client.Create(ctx, "limanix-owned", "missing") }, func() error { return client.Edit(ctx, "limanix-owned", "missing") }} {
		if err := call(); !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation: %v", err)
		}
	}
}

func TestNativeStopPreservesCancellationCause(t *testing.T) {
	client, inst := nativeLifecycleClient(t, limatype.AARCH64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.native.stop = func(context.Context, *limatype.Instance, bool) error {
		cancel()
		return errors.New("timed out waiting for instance to shut down after 3 minutes")
	}
	if err := client.Stop(ctx, inst.Name); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost upstream shutdown cancellation: %v", err)
	}
}
