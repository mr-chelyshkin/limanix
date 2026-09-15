package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/managedhome"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

type backendCall struct {
	Operation string
	Name      string
	Force     bool
	Arguments []string
}

type fakeBackend struct {
	instances         map[string]lima.Instance
	calls             []backendCall
	failOn            string
	shellExit         int
	onValidate        func()
	rejectStoppedStop bool
}

func (f *fakeBackend) called(operation, name string, force bool, args []string) error {
	f.calls = append(f.calls, backendCall{operation, name, force, append([]string(nil), args...)})
	if f.failOn == operation {
		return fmt.Errorf("backend diagnostic with sensitive-test-token: %s", operation)
	}
	return nil
}

func (f *fakeBackend) Preflight(_ context.Context, cfg config.Config) error {
	return f.called("preflight", string(cfg.Name), false, nil)
}

func (f *fakeBackend) FetchAll(context.Context) ([]lima.Instance, error) {
	if err := f.called("fetch_all", "", false, nil); err != nil {
		return nil, err
	}
	instances := make([]lima.Instance, 0, len(f.instances))
	for _, instance := range f.instances {
		instances = append(instances, instance)
	}
	return instances, nil
}

func (f *fakeBackend) Validate(_ context.Context, template string) error {
	if err := f.called("validate", "", false, nil); err != nil {
		return err
	}
	if _, err := templateDisk(template); err != nil {
		return err
	}
	if f.onValidate != nil {
		f.onValidate()
	}
	return nil
}

func templateDisk(path string) (*int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var template struct {
		Disk string `json:"disk"`
	}
	if err := json.Unmarshal(data, &template); err != nil {
		return nil, err
	}
	disk, err := domain.ParseByteSize(template.Disk)
	if err != nil {
		return nil, err
	}
	size := int64(disk)
	return &size, nil
}

func (f *fakeBackend) Create(_ context.Context, name, template string) error {
	if err := f.called("create", name, false, nil); err != nil {
		return err
	}
	disk, err := templateDisk(template)
	if err != nil {
		return err
	}
	f.instances[name] = lima.Instance{
		Name: name, Status: lima.Stopped, Disk: disk,
		Networks: []lima.Network{{MACAddress: "52:55:55:4a:e4:84", Shared: true}},
	}
	return nil
}

func (f *fakeBackend) Start(_ context.Context, name string) error {
	if err := f.called("start", name, false, nil); err != nil {
		return err
	}
	instance := f.instances[name]
	instance.Status = lima.Running
	f.instances[name] = instance
	return nil
}

func (f *fakeBackend) Stop(_ context.Context, name string) error {
	if err := f.called("stop", name, false, nil); err != nil {
		return err
	}
	instance := f.instances[name]
	if f.rejectStoppedStop && instance.Status == lima.Stopped {
		return errors.New("VM was stopped externally; redundant stop rejected")
	}
	instance.Status = lima.Stopped
	f.instances[name] = instance
	return nil
}

func (f *fakeBackend) Delete(_ context.Context, name string, force bool) error {
	if err := f.called("delete", name, force, nil); err != nil {
		return err
	}
	delete(f.instances, name)
	return nil
}

func (f *fakeBackend) Edit(_ context.Context, name, template string) error {
	if err := f.called("edit", name, false, nil); err != nil {
		return err
	}
	disk, err := templateDisk(template)
	if err != nil {
		return err
	}
	instance := f.instances[name]
	instance.Disk = disk
	f.instances[name] = instance
	return nil
}

func (f *fakeBackend) Run(_ context.Context, name string, args []string, _ bool) (string, error) {
	if err := f.called("run", name, false, args); err != nil {
		return "", err
	}
	if reflect.DeepEqual(args, []string{"ip", "-j", "address", "show"}) {
		return `[{"address":"52:55:55:4a:e4:84","addr_info":[{"family":"inet","scope":"global","local":"192.0.2.10"}]}]`, nil
	}
	return "", nil
}

func (f *fakeBackend) Shell(_ context.Context, name string, args []string) (int, error) {
	if err := f.called("shell", name, false, args); err != nil {
		return 1, err
	}
	return f.shellExit, nil
}

type fixtureData struct {
	root    string
	project string
	store   *state.Store
	backend *fakeBackend
	manager *Manager
}

func fixture(t *testing.T) fixtureData {
	t.Helper()
	root, err := filesystem.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "external-project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "keep.txt"), []byte("external data"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := state.NewStore(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{instances: make(map[string]lima.Instance)}
	manager := New(store, backend)
	manager.getUID = func() int { return 501 }
	return fixtureData{root, project, store, backend, manager}
}

func (f fixtureData) configuration(name string) config.Config {
	cfg := config.Default()
	cfg.Name = domain.VMName(name)
	cfg.Home.Root = filepath.Join(f.root, "homes")
	cfg.Env = map[domain.EnvName]domain.EnvValue{"TOKEN": "sensitive-test-token"}
	cfg.Mounts = []config.Mount{{Mode: "rw", Source: f.project, Target: "/workspace"}}
	return cfg
}

func (f fixtureData) writeConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	path := filepath.Join(f.root, string(cfg.Name)+".toml")
	data, err := config.Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSocketPreflightLeavesHomeAndStateUnallocated(t *testing.T) {
	for _, test := range []struct {
		length int
		error  string
	}{
		{50, "SSH socket path"},
		{63, "maximum length"},
	} {
		t.Run(fmt.Sprintf("name-length=%d", test.length), func(t *testing.T) {
			f := fixture(t)
			limaRoot := filepath.Join(f.root, "lima")
			t.Setenv("LIMA_HOME", limaRoot)
			manager := New(f.store, lima.NewClient(nil))
			manager.getUID = func() int { return 501 }
			cfg := f.configuration(strings.Repeat("n", test.length))
			_, err := manager.Create(context.Background(), f.writeConfig(t, cfg))
			if err == nil || !strings.Contains(err.Error(), test.error) {
				t.Fatalf("expected Lima name preflight rejection, got %v", err)
			}
			assertAbsent(t, cfg.Home.Root)
			assertAbsent(t, f.store.Root())
			assertAbsent(t, limaRoot)
		})
	}
}

func (f fixtureData) create(t *testing.T, name string) (state.Instance, string) {
	t.Helper()
	path := f.writeConfig(t, f.configuration(name))
	instance, err := f.manager.Create(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return instance, path
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected %q absent: %v", path, err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %q exists: %v", path, err)
	}
}

func (f fixtureData) instanceDirectory(t *testing.T, name domain.VMName) string {
	t.Helper()
	directory, err := f.store.InstanceDir(name)
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestCreateReadyKeepsSecretsOutOfStateAndCopiesModulesOnce(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	if instance.Status != state.Ready {
		t.Fatalf("state %s", instance.Status)
	}
	loaded, err := f.store.Load(instance.Identity.Name)
	if err != nil || loaded != instance {
		t.Fatalf("saved %+v: %v", loaded, err)
	}
	entries, err := os.ReadDir(instance.Identity.Home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("new home not empty: %v %v", entries, err)
	}
	directory := f.instanceDirectory(t, instance.Identity.Name)
	for _, name := range []string{"identity.json", "instance.json"} {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "sensitive-test-token") || strings.Contains(string(data), "config_path") {
			t.Fatalf("secret/config leaked into %s: %s", name, data)
		}
	}
	generation, err := f.manager.generationDir(instance)
	if err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, filepath.Join(generation, "sources"))
	assertExists(t, filepath.Join(generation, "flake", "modules", "0000", "default.nix"))
	infos, err := f.manager.FetchAll(context.Background())
	if err != nil || len(infos) != 1 || infos[0].Address != "192.0.2.10" {
		t.Fatalf("typed listing %+v: %v", infos, err)
	}
}

func TestCreateBeforeBackendFailureRemovesPreparedStateAndHome(t *testing.T) {
	for _, stage := range []string{"validate", "home"} {
		t.Run(stage, func(t *testing.T) {
			f := fixture(t)
			path := f.writeConfig(t, f.configuration("sandbox"))
			if stage == "validate" {
				f.backend.failOn = "validate"
			} else {
				f.manager.createHome = func(state.Identity) (string, error) { return "", errors.New("home allocation denied") }
			}
			instance, err := f.manager.Create(context.Background(), path)
			if err == nil {
				t.Fatal("creation succeeded despite preparation failure")
			}
			assertAbsent(t, f.instanceDirectory(t, instance.Identity.Name))
			assertAbsent(t, instance.Identity.Home)
			if len(f.backend.instances) != 0 {
				t.Fatal("backend was created before preparation completed")
			}
		})
	}
}

func TestCreateGuestFailureKeepsRecoverableOwnershipAndUpdateRecovers(t *testing.T) {
	f := fixture(t)
	path := f.writeConfig(t, f.configuration("sandbox"))
	f.backend.failOn = "run"
	if _, err := f.manager.Create(context.Background(), path); err == nil {
		t.Fatal("create succeeded despite rebuild failure")
	}
	instance, err := f.store.Load("sandbox")
	if err != nil || instance.Status != state.Error || instance.Error == nil {
		t.Fatalf("failed create not recoverable: %+v %v", instance, err)
	}
	if strings.Contains(*instance.Error, "sensitive-test-token") {
		t.Fatal("backend diagnostic leaked into persistent state")
	}
	assertExists(t, instance.Identity.Home)
	f.backend.failOn = ""
	updated, err := f.manager.Update(context.Background(), path)
	if err != nil || updated.Status != state.Ready || updated.Identity != instance.Identity {
		t.Fatalf("recovery update %+v: %v", updated, err)
	}
}

type firstSaveFailureStore struct {
	*state.Store
	failed  bool
	failure error
}

func (s *firstSaveFailureStore) Save(instance state.Instance) error {
	if !s.failed {
		s.failed = true
		return s.failure
	}
	return s.Store.Save(instance)
}

func TestFailedHomeCleanupRetainsIdentityForRecovery(t *testing.T) {
	f := fixture(t)
	saveFailure := errors.New("first state save unavailable")
	removeFailure := errors.New("home cleanup denied")
	f.manager.store = &firstSaveFailureStore{Store: f.store, failure: saveFailure}
	f.manager.removeHome = func(state.Identity) error { return removeFailure }
	path := f.writeConfig(t, f.configuration("sandbox"))
	result, err := f.manager.Create(context.Background(), path)
	if !errors.Is(err, saveFailure) || !errors.Is(err, removeFailure) {
		t.Fatalf("original errors not retained: %v", err)
	}
	loaded, err := f.store.Load(result.Identity.Name)
	if err != nil || loaded.Identity != result.Identity || loaded.Status != state.Error {
		t.Fatalf("cleanup orphaned home ownership: %+v %v", loaded, err)
	}
	assertExists(t, result.Identity.Home)
	if len(f.backend.instances) != 0 {
		t.Fatal("backend creation started after local preparation failure")
	}
	f.manager.removeHome = managedhome.Remove
	if _, err := f.manager.Delete(context.Background(), result.Identity.Name, false, true); err != nil {
		t.Fatal("retained identity cannot recover deletion:", err)
	}
	assertAbsent(t, result.Identity.Home)
}

func TestCanceledCreateCleansHomeBeforeBackendCreation(t *testing.T) {
	f := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.manager.createHome = func(identity state.Identity) (string, error) {
		home, err := managedhome.Create(identity)
		cancel()
		return home, err
	}
	result, err := f.manager.Create(ctx, f.writeConfig(t, f.configuration("sandbox")))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored: %v", err)
	}
	assertAbsent(t, result.Identity.Home)
	assertAbsent(t, f.instanceDirectory(t, result.Identity.Name))
	if len(f.backend.instances) != 0 {
		t.Fatal("backend was created after cancellation")
	}
}

func TestFailedUpdateRetriesPruneAllStaleGenerations(t *testing.T) {
	f := fixture(t)
	first, path := f.create(t, "sandbox")
	f.backend.failOn = "run"
	for range 2 {
		if _, err := f.manager.Update(context.Background(), path); err == nil {
			t.Fatal("update succeeded despite guest failure")
		}
	}
	generation, err := f.manager.generationDir(first)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(generation)
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 3 {
		t.Fatalf("failed generations lost: %v %v", entries, err)
	}
	f.backend.failOn = ""
	updated, err := f.manager.Update(context.Background(), path)
	if err != nil || updated.Status != state.Ready || updated.Identity != first.Identity {
		t.Fatalf("retry %+v: %v", updated, err)
	}
	entries, err = os.ReadDir(parent)
	if err != nil || len(entries) != 1 || entries[0].Name() != updated.Generation {
		t.Fatalf("stale generations remain: %v %v", entries, err)
	}
}

func TestSuccessfulUpdateCleanupFailureOnlyWarns(t *testing.T) {
	for _, failure := range []string{"remove", "listing"} {
		t.Run(failure, func(t *testing.T) {
			f := fixture(t)
			first, path := f.create(t, "sandbox")
			generation, err := f.manager.generationDir(first)
			if err != nil {
				t.Fatal(err)
			}
			parent := filepath.Dir(generation)
			var warnings []string
			f.manager.Warn = func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }
			if failure == "remove" {
				f.manager.removeAll = func(path string) error {
					if filepath.Dir(path) == parent {
						return errors.New("cleanup denied")
					}
					return os.RemoveAll(path)
				}
			} else {
				f.manager.readDir = func(path string) ([]os.DirEntry, error) {
					if path == parent {
						return nil, errors.New("listing denied")
					}
					return os.ReadDir(path)
				}
			}
			updated, err := f.manager.Update(context.Background(), path)
			if err != nil || updated.Status != state.Ready || updated.Error != nil || len(warnings) != 1 {
				t.Fatalf("cleanup changed operation outcome: %+v %v %v", updated, err, warnings)
			}
			loaded, err := f.store.Load(first.Identity.Name)
			if err != nil || loaded != updated {
				t.Fatalf("ready commit lost: %+v %v", loaded, err)
			}
			if !strings.Contains(warnings[0], "could not be") {
				t.Fatalf("unexpected warning %q", warnings[0])
			}
		})
	}
}

func TestForceAndRemoveHomeAreIndependent(t *testing.T) {
	for _, force := range []bool{false, true} {
		for _, removeHome := range []bool{false, true} {
			t.Run(fmt.Sprintf("force=%t/remove-home=%t", force, removeHome), func(t *testing.T) {
				f := fixture(t)
				instance, _ := f.create(t, "sandbox")
				if err := os.WriteFile(filepath.Join(instance.Identity.Home, "guest-file"), []byte("guest data"), 0o600); err != nil {
					t.Fatal(err)
				}
				f.backend.calls = nil
				home, err := f.manager.Delete(context.Background(), instance.Identity.Name, force, removeHome)
				if err != nil || home != instance.Identity.Home {
					t.Fatalf("delete %q: %v", home, err)
				}
				var sawStop, sawDelete bool
				for _, call := range f.backend.calls {
					if call.Operation == "stop" {
						sawStop = true
					}
					if call.Operation == "delete" {
						sawDelete = true
						if call.Force != force {
							t.Fatal("force option was not passed to Lima")
						}
					}
				}
				if !sawDelete || sawStop == force {
					t.Fatalf("incorrect backend deletion calls: %+v", f.backend.calls)
				}
				archive := filepath.Join(f.store.Root(), "homes", filepath.Base(instance.Identity.Home)+".json")
				if removeHome {
					assertAbsent(t, instance.Identity.Home)
					assertAbsent(t, archive)
				} else {
					assertExists(t, filepath.Join(instance.Identity.Home, "guest-file"))
					assertExists(t, archive)
				}
				assertAbsent(t, f.instanceDirectory(t, instance.Identity.Name))
				data, err := os.ReadFile(filepath.Join(f.project, "keep.txt"))
				if err != nil || string(data) != "external data" {
					t.Fatalf("removed external mount: %q %v", data, err)
				}
			})
		}
	}
}

func TestPreservedHomesStayKnownAfterNameReuse(t *testing.T) {
	f := fixture(t)
	var homes []string
	for range 2 {
		instance, _ := f.create(t, "sandbox")
		if _, err := f.manager.Delete(context.Background(), instance.Identity.Name, false, false); err != nil {
			t.Fatal(err)
		}
		homes = append(homes, instance.Identity.Home)
	}
	entries, err := os.ReadDir(filepath.Join(f.store.Root(), "homes"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("retained ownership records lost: %v %v", entries, err)
	}
	var recordedHomes []string
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(f.store.Root(), "homes", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var identity struct {
			Home string `json:"home"`
		}
		if err := json.Unmarshal(data, &identity); err != nil {
			t.Fatal(err)
		}
		recordedHomes = append(recordedHomes, identity.Home)
		assertExists(t, identity.Home)
	}
	sort.Strings(homes)
	sort.Strings(recordedHomes)
	if !reflect.DeepEqual(homes, recordedHomes) {
		t.Fatalf("recorded homes %v, want %v", recordedHomes, homes)
	}
}

type archiveFailureStore struct {
	*state.Store
	failure error
}

func (s *archiveFailureStore) PreserveHome(identity state.Identity) (string, error) {
	if s.failure != nil {
		return "", s.failure
	}
	return s.Store.PreserveHome(identity)
}

func TestFailedHomeArchiveKeepsIdentityForDeleteRetry(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	failure := errors.New("archive unavailable")
	store := &archiveFailureStore{f.store, failure}
	f.manager.store = store
	if _, err := f.manager.Delete(context.Background(), instance.Identity.Name, false, false); !errors.Is(err, failure) {
		t.Fatalf("delete ignored archival failure: %v", err)
	}
	if _, exists := f.backend.instances[instance.Identity.LimaName()]; exists {
		t.Fatal("backend was not deleted")
	}
	identity, err := f.store.LoadIdentity(instance.Identity.Name)
	if err != nil || identity != instance.Identity {
		t.Fatalf("lost retry identity: %+v %v", identity, err)
	}
	assertExists(t, instance.Identity.Home)
	store.failure = nil
	if _, err := f.manager.Delete(context.Background(), instance.Identity.Name, false, false); err != nil {
		t.Fatal("archive retry failed:", err)
	}
	assertAbsent(t, f.instanceDirectory(t, instance.Identity.Name))
	assertExists(t, filepath.Join(f.store.Root(), "homes", filepath.Base(instance.Identity.Home)+".json"))
}

func TestRemoveHomeClearsAnAlreadyArchivedIdentity(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	archive, err := f.store.PreserveHome(instance.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.Delete(context.Background(), instance.Identity.Name, false, true); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, instance.Identity.Home)
	assertAbsent(t, archive)
}

func TestForceDeleteRecoversFromFailedOrderlyStopAndKeepsHome(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	f.backend.failOn = "stop"
	if _, err := f.manager.Delete(context.Background(), instance.Identity.Name, false, false); err == nil {
		t.Fatal("orderly delete unexpectedly succeeded")
	}
	if _, err := f.manager.Delete(context.Background(), instance.Identity.Name, true, false); err != nil {
		t.Fatal("force delete could not recover:", err)
	}
	assertExists(t, instance.Identity.Home)
	if _, exists := f.backend.instances[instance.Identity.LimaName()]; exists {
		t.Fatal("force delete left backend instance")
	}
}

func TestCorruptRuntimeIsListedAndCanBeDeleted(t *testing.T) {
	f := fixture(t)
	broken, _ := f.create(t, "broken")
	healthy, _ := f.create(t, "healthy")
	if err := os.WriteFile(filepath.Join(f.instanceDirectory(t, broken.Identity.Name), "instance.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	infos, err := f.manager.FetchAll(context.Background())
	if err != nil || len(infos) != 2 || infos[0].Name != "broken" || infos[0].Error == nil || infos[0].OperationStatus != nil || infos[1].OperationStatus == nil || *infos[1].OperationStatus != state.Ready {
		t.Fatalf("corruption broke typed list: %+v %v", infos, err)
	}
	if _, err := f.manager.Delete(context.Background(), broken.Identity.Name, true, true); err != nil {
		t.Fatal("corrupt runtime blocked ownership deletion:", err)
	}
	assertAbsent(t, broken.Identity.Home)
	if _, exists := f.backend.instances[healthy.Identity.LimaName()]; !exists {
		t.Fatal("deleting corrupt VM affected a healthy VM")
	}
}

func TestUpdateRefreshesBackendStatusAfterPreparation(t *testing.T) {
	f := fixture(t)
	instance, path := f.create(t, "sandbox")
	f.backend.onValidate = func() {
		actual := f.backend.instances[instance.Identity.LimaName()]
		actual.Status = lima.Stopped
		f.backend.instances[actual.Name] = actual
	}
	f.backend.rejectStoppedStop = true
	updated, err := f.manager.Update(context.Background(), path)
	if err != nil || updated.Status != state.Ready || f.backend.instances[instance.Identity.LimaName()].Status != lima.Running {
		t.Fatalf("update used stale status: %+v %v", updated, err)
	}
}

func TestUpdateRechecksDiskBeforeStoppingVM(t *testing.T) {
	grownDisk := int64(20 * domain.GiB)
	for _, test := range []struct {
		name    string
		disk    *int64
		status  lima.Status
		message string
	}{
		{name: "grown", disk: &grownDisk, status: lima.Running, message: "shrinking"},
		{name: "unavailable", status: lima.Unknown, message: "did not report the disk size"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := fixture(t)
			instance, path := f.create(t, "sandbox")
			f.backend.calls = nil
			f.backend.onValidate = func() {
				actual := f.backend.instances[instance.Identity.LimaName()]
				actual.Disk = test.disk
				actual.Status = test.status
				f.backend.instances[actual.Name] = actual
			}
			_, err := f.manager.Update(context.Background(), path)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("update ignored changed disk metadata: %v", err)
			}
			for _, call := range f.backend.calls {
				if call.Operation == "stop" || call.Operation == "edit" || call.Operation == "start" {
					t.Fatalf("invalid disk update changed the backend: %+v", call)
				}
			}
			if f.backend.instances[instance.Identity.LimaName()].Status != test.status {
				t.Fatal("invalid disk update changed the reported VM status")
			}
			f.backend.onValidate = nil
			actual := f.backend.instances[instance.Identity.LimaName()]
			size := int64(20 * domain.GiB)
			actual.Disk = &size
			f.backend.instances[actual.Name] = actual
			cfg := f.configuration("sandbox")
			cfg.Resources.Disk = domain.ByteSize(size)
			updated, err := f.manager.Update(context.Background(), f.writeConfig(t, cfg))
			if err != nil || updated.Identity != instance.Identity || updated.Status != state.Ready {
				t.Fatalf("valid retry could not recover: %+v %v", updated, err)
			}
		})
	}
}

func TestUpdateAddsBundledAndImportedModulesToExistingVM(t *testing.T) {
	f := fixture(t)
	first, _ := f.create(t, "sandbox")
	keep := filepath.Join(first.Identity.Home, "manual-install-state")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(f.root, "custom-module")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "default.nix"), []byte("{ ... }: { services.openssh.enable = true; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.registry.Add(context.Background(), "custom", source); err != nil {
		t.Fatal(err)
	}
	cfg := f.configuration("sandbox")
	cfg.NixOS.Modules = []domain.ModuleID{"git", "rust", "third-party:custom"}
	updated, err := f.manager.Update(context.Background(), f.writeConfig(t, cfg))
	if err != nil || updated.Identity != first.Identity || updated.Status != state.Ready {
		t.Fatalf("module update %+v: %v", updated, err)
	}
	generation, err := f.manager.generationDir(updated)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"0000", "0001", "0002"} {
		assertExists(t, filepath.Join(generation, "flake", "modules", index, "default.nix"))
	}
	data, err := os.ReadFile(keep)
	if err != nil || string(data) != "keep" {
		t.Fatalf("module update discarded home state: %q %v", data, err)
	}
}

func TestUpdateChecksActualDiskAfterFailedGrowth(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	cfg := f.configuration("sandbox")
	cfg.Resources.Disk = domain.ByteSize(20 * domain.GiB)
	f.backend.failOn = "edit"
	if _, err := f.manager.Update(context.Background(), f.writeConfig(t, cfg)); err == nil {
		t.Fatal("failed disk edit unexpectedly succeeded")
	}
	if *f.backend.instances[instance.Identity.LimaName()].Disk != 10*domain.GiB {
		t.Fatal("failed edit changed actual disk")
	}
	f.backend.failOn = ""
	cfg.Resources.Disk = domain.ByteSize(10 * domain.GiB)
	if _, err := f.manager.Update(context.Background(), f.writeConfig(t, cfg)); err != nil {
		t.Fatal("failed growth blocked retry at actual size:", err)
	}
	actual := f.backend.instances[instance.Identity.LimaName()]
	size := int64(20 * domain.GiB)
	actual.Disk = &size
	f.backend.instances[actual.Name] = actual
	if _, err := f.manager.Update(context.Background(), f.writeConfig(t, cfg)); err == nil || !strings.Contains(err.Error(), "shrinking") {
		t.Fatalf("shrinking actual disk was accepted: %v", err)
	}
}

func TestUpdateCannotChangeImmutableIdentity(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	for _, change := range []func(*config.Config){
		func(c *config.Config) { c.User.Name = "another" },
		func(c *config.Config) { c.User.Home = "/home/another" },
		func(c *config.Config) { c.Home.Root = filepath.Join(f.root, "other") },
		func(c *config.Config) { c.Resources.Arch = domain.AMD64 },
	} {
		cfg := f.configuration("sandbox")
		change(&cfg)
		if _, err := f.manager.Update(context.Background(), f.writeConfig(t, cfg)); err == nil || !strings.Contains(err.Error(), "cannot change") {
			t.Fatalf("immutable update accepted: %v", err)
		}
		loaded, err := f.store.Load(instance.Identity.Name)
		if err != nil || loaded != instance {
			t.Fatalf("immutable check mutated state: %+v %v", loaded, err)
		}
	}
}

func TestLifecycleDoesNotNeedOriginalConfigurationOrMutableRecord(t *testing.T) {
	f := fixture(t)
	instance, path := f.create(t, "sandbox")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.instanceDirectory(t, instance.Identity.Name), "instance.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Stop(context.Background(), instance.Identity.Name); err != nil {
		t.Fatal("stop depended on mutable/config format:", err)
	}
	if err := f.manager.Start(context.Background(), instance.Identity.Name); err != nil {
		t.Fatal("start depended on mutable/config format:", err)
	}
	if _, err := f.manager.Delete(context.Background(), instance.Identity.Name, false, true); err != nil {
		t.Fatal("delete depended on mutable/config format:", err)
	}
}

func TestOtherVMAndRegistryOperateWhileFirstVMIsLocked(t *testing.T) {
	f := fixture(t)
	alpha, _ := f.create(t, "alpha")
	beta, _ := f.create(t, "beta")
	lock, err := f.store.InstanceLock(context.Background(), alpha.Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := f.manager.Stop(context.Background(), beta.Identity.Name); err != nil {
		t.Fatal("another VM was blocked by global lock:", err)
	}
	source := filepath.Join(f.root, "module")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "default.nix"), []byte("{ ... }: {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.registry.Add(context.Background(), "custom", source); err != nil {
		t.Fatal("module registry was blocked by VM lock:", err)
	}
	if err := f.manager.Stop(context.Background(), alpha.Identity.Name); !errors.Is(err, state.ErrLockBusy) {
		t.Fatalf("same VM did not respect operation lock: %v", err)
	}
}

func TestMissingBackendCanBeListedAndDeleted(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	clear(f.backend.instances)
	infos, err := f.manager.FetchAll(context.Background())
	if err != nil || len(infos) != 1 || infos[0].BackendStatus != nil {
		t.Fatalf("missing backend not visible: %+v %v", infos, err)
	}
	if _, err := f.manager.Delete(context.Background(), instance.Identity.Name, false, true); err != nil {
		t.Fatal("missing backend blocked deletion:", err)
	}
	infos, err = f.manager.FetchAll(context.Background())
	if err != nil || len(infos) != 0 {
		t.Fatalf("deleted records remain: %+v %v", infos, err)
	}
}

func TestListShowsInterruptedWithoutChangingPersistentState(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	instance.Status = state.Updating
	if err := f.store.Save(instance); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(f.instanceDirectory(t, instance.Identity.Name), "instance.json")
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := f.manager.FetchAll(context.Background())
	if err != nil || len(infos) != 1 || infos[0].OperationStatus == nil || *infos[0].OperationStatus != state.Interrupted || infos[0].BackendStatus == nil || *infos[0].BackendStatus != lima.Running {
		t.Fatalf("abandoned operation not represented: %+v %v", infos, err)
	}
	after, err := os.ReadFile(record)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("list changed saved record: %s %v", after, err)
	}
	lock, err := f.store.InstanceLock(context.Background(), instance.Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	infos, err = f.manager.FetchAll(context.Background())
	if err != nil || len(infos) != 1 || infos[0].OperationStatus == nil || *infos[0].OperationStatus != state.Updating {
		t.Fatalf("active operation misreported as interrupted: %+v %v", infos, err)
	}
}

func TestShellPreservesArgumentsAndChildExitStatus(t *testing.T) {
	f := fixture(t)
	instance, _ := f.create(t, "sandbox")
	f.backend.shellExit = 7
	code, err := f.manager.Shell(context.Background(), instance.Identity.Name, []string{"sh", "-c", "exit 7"})
	if err != nil || code != 7 {
		t.Fatalf("child exit status changed: %d %v", code, err)
	}
	call := f.backend.calls[len(f.backend.calls)-1]
	if !reflect.DeepEqual(call.Arguments[len(call.Arguments)-3:], []string{"sh", "-c", "exit 7"}) {
		t.Fatalf("shell arguments changed: %+v", call.Arguments)
	}
	if err := f.manager.Stop(context.Background(), instance.Identity.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.Shell(context.Background(), instance.Identity.Name, nil); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("entered a stopped VM: %v", err)
	}
}
