package state

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

func fixture(t *testing.T, name string) (*Store, Instance) {
	t.Helper()
	root, err := filesystem.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{
		Name: domain.VMName(name), ID: "012345abcdef", Arch: domain.ARM64,
		Username: domain.Username("dev"), UserHome: domain.GuestPath("/home/dev"),
		HomeRoot: filepath.Join(root, "homes"), CreatedAt: "2026-09-13T12:00:00+00:00",
	}
	identity.Home, err = HomePath(identity.HomeRoot, identity.Name, identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	return store, Instance{Identity: identity, Status: Ready, Generation: "abcdef012345"}
}

func TestSeparatePrivateRecordsAndImmutableIdentity(t *testing.T) {
	store, original := fixture(t, "sandbox")
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(original.Identity.Name)
	if err != nil || loaded != original {
		t.Fatalf("roundtrip %+v: %v", loaded, err)
	}
	directory, err := store.InstanceDir(original.Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(directory, "identity.json"), filepath.Join(directory, "instance.json")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private record %s: %v", path, err)
		}
	}
	runtimePath := filepath.Join(directory, "instance.json")
	data, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	var runtimeFields map[string]any
	if err := json.Unmarshal(data, &runtimeFields); err != nil {
		t.Fatal(err)
	}
	if len(runtimeFields) != 4 || runtimeFields["status"] != "ready" || runtimeFields["schema_version"] != float64(1) {
		t.Fatalf("runtime contains unexpected fields: %+v", runtimeFields)
	}
	identityPath := filepath.Join(directory, "identity.json")
	beforeIdentity, err := os.Stat(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Identity.Username = domain.Username("another")
	if err := store.Save(changed); err == nil {
		t.Fatal("replaced existing identity")
	}
	message := "guest rebuild failed"
	original.Status, original.Error = Error, &message
	if err := os.Chmod(runtimePath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	afterIdentity, err := os.Stat(identityPath)
	if err != nil || !os.SameFile(beforeIdentity, afterIdentity) {
		t.Fatalf("rewrote immutable identity: %v", err)
	}
	loaded, err = store.Load(original.Identity.Name)
	if err != nil || loaded.Status != Error || loaded.Error == nil || *loaded.Error != message {
		t.Fatalf("updated mutable record %+v: %v", loaded, err)
	}
	info, err := os.Stat(runtimePath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mutable record mode not reset: %v", err)
	}
	if original.Identity.LimaName() != "limanix-sandbox-012345abcdef" {
		t.Fatal("invalid backend name")
	}
}

func TestCorruptedRuntimeKeepsIndependentIdentity(t *testing.T) {
	store, original := fixture(t, "sandbox")
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	directory, err := store.InstanceDir(original.Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"schema_version":1,"generation":"abcdef012345","status":"ready","error":null}`
	for _, data := range []string{
		`[]`, `null`, `{}`, `{ broken json`,
		strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(valid, `"schema_version":1`, `"schema_version":true`, 1),
		strings.Replace(valid, `"schema_version":1`, `"schema_version":1.0`, 1),
		strings.Replace(valid, `"abcdef012345"`, `"../escape"`, 1),
		strings.Replace(valid, `"ready"`, `"unknown"`, 1),
		strings.Replace(valid, `"ready"`, `"interrupted"`, 1),
		strings.Replace(valid, `"ready"`, `1`, 1),
		strings.Replace(valid, `null`, `{"message":"bad"}`, 1),
		strings.Replace(valid, `"error":null`, `"error":null,"config":{}`, 1),
		strings.Replace(valid, `,"error":null`, ``, 1),
	} {
		t.Run(data, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(directory, "instance.json"), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Load(original.Identity.Name); err == nil {
				t.Fatal("accepted corrupt runtime")
			}
			identity, err := store.LoadIdentity(original.Identity.Name)
			if err != nil || identity != original.Identity {
				t.Fatalf("independent identity %+v: %v", identity, err)
			}
			entries, err := store.FetchAll()
			if err != nil || len(entries) != 1 || entries[0].Instance != nil || entries[0].Identity == nil || entries[0].Error == nil {
				t.Fatalf("corruption not isolated: %+v %v", entries, err)
			}
		})
	}
}

func TestIdentityValidationAndCorruptionIsolation(t *testing.T) {
	store, original := fixture(t, "sandbox")
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	directory, err := store.InstanceDir(original.Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(directory, "identity.json")
	valid, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	for field, values := range map[string][]any{
		"name": {"different", "../escape"}, "id": {"../escape", "invalid"}, "arch": {"unknown"},
		"username":  {"root", "limanix-admin", "BadName", "two words", "dev\n", 1},
		"user_home": {"relative", "/home/../etc", "//home/dev", "/home/two words", "/"},
		"home":      {store.Root()}, "unexpected": {true},
	} {
		for _, value := range values {
			t.Run(field+":"+stringValue(value), func(t *testing.T) {
				var data map[string]any
				if err := json.Unmarshal(valid, &data); err != nil {
					t.Fatal(err)
				}
				data[field] = value
				encoded, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(identityPath, encoded, 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := store.LoadIdentity(original.Identity.Name); err == nil {
					t.Fatal("accepted invalid identity")
				}
				entries, err := store.FetchAll()
				if err != nil || len(entries) != 1 || entries[0].Identity != nil || entries[0].Instance != nil || entries[0].Error == nil {
					t.Fatalf("invalid identity not isolated: %+v %v", entries, err)
				}
			})
		}
	}
}

func TestInvalidUTF8StateIsAnIsolatedDamagedRow(t *testing.T) {
	for _, recordName := range []string{"identity.json", "instance.json"} {
		t.Run(recordName, func(t *testing.T) {
			store, broken := fixture(t, "broken")
			if err := store.Save(broken); err != nil {
				t.Fatal(err)
			}
			healthy := broken
			healthy.Identity.Name = "healthy"
			var err error
			healthy.Identity.Home, err = HomePath(healthy.Identity.HomeRoot, healthy.Identity.Name, healthy.Identity.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Save(healthy); err != nil {
				t.Fatal(err)
			}
			directory, err := store.InstanceDir(broken.Identity.Name)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, recordName)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if recordName == "identity.json" {
				data = bytes.Replace(data, []byte(broken.Identity.CreatedAt), []byte{0xff}, 1)
			} else {
				data = bytes.Replace(data, []byte(`"error": null`), []byte{'"', 'e', 'r', 'r', 'o', 'r', '"', ':', ' ', '"', 0xff, '"'}, 1)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			entries, err := store.FetchAll()
			if err != nil || len(entries) != 2 || entries[0].Instance != nil || entries[0].Error == nil || entries[1].Instance == nil {
				t.Fatalf("invalid encoding not isolated: %+v %v", entries, err)
			}
			if recordName == "identity.json" && entries[0].Identity != nil {
				t.Fatal("decoded invalid UTF-8 identity")
			}
			if recordName == "instance.json" && entries[0].Identity == nil {
				t.Fatal("damaged runtime hid valid identity")
			}
			if !strings.Contains(*entries[0].Error, "UTF-8") {
				t.Fatalf("wrong encoding diagnostic: %s", *entries[0].Error)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatalf("listing rewrote damaged record: %s %v", after, err)
			}
		})
	}
}

func TestIdentityGuestPathNormalizesWithoutChangingCallerOrSavedIdentity(t *testing.T) {
	store, original := fixture(t, "sandbox")
	original.Identity.UserHome = "/home/dev//."
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	if original.Identity.UserHome != "/home/dev//." {
		t.Fatal("save mutated the caller's identity")
	}
	loaded, err := store.LoadIdentity(original.Identity.Name)
	if err != nil || loaded.UserHome != "/home/dev" {
		t.Fatalf("guest path not normalized: %+v %v", loaded, err)
	}
	directory, err := store.InstanceDir(original.Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(directory, "identity.json")
	data, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	// A legacy identity is normalized on read without changing its immutable file.
	legacy := bytes.Replace(data, []byte(`"user_home": "/home/dev"`), []byte(`"user_home": "/home/dev//."`), 1)
	if bytes.Equal(legacy, data) {
		t.Fatal("fixture did not find user_home field")
	}
	if err := os.WriteFile(identityPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadIdentity(original.Identity.Name)
	if err != nil || loaded.UserHome != "/home/dev" {
		t.Fatalf("legacy normalization failed: %+v %v", loaded, err)
	}
	if err := store.Save(original); err != nil {
		t.Fatal("equivalent path broke immutable comparison:", err)
	}
	after, err := os.ReadFile(identityPath)
	if err != nil || !bytes.Equal(legacy, after) {
		t.Fatalf("rewrote immutable identity: %s %v", after, err)
	}
	archive, err := store.PreserveHome(original.Identity)
	if err != nil {
		t.Fatal(err)
	}
	archiveData, err := os.ReadFile(archive)
	if err != nil || !bytes.Contains(archiveData, []byte(`"user_home": "/home/dev"`)) {
		t.Fatalf("archive not canonical: %s %v", archiveData, err)
	}
	if err := store.ForgetHome(loaded); err != nil {
		t.Fatal("archive comparison did not normalize consistently:", err)
	}
}

func TestIdentityRejectsInvalidTextBeforeIO(t *testing.T) {
	for _, field := range []string{"home_root", "created_at"} {
		t.Run(field, func(t *testing.T) {
			for _, invalid := range []struct{ name, value string }{
				{"non-UTF8", string([]byte{0xff})}, {"NUL", "\x00"},
			} {
				t.Run(invalid.name, func(t *testing.T) {
					store, original := fixture(t, "sandbox")
					expected := "invalid created_at"
					if field == "home_root" {
						original.Identity.HomeRoot += invalid.value
						original.Identity.Home = filepath.Join(original.Identity.HomeRoot, "sandbox-"+original.Identity.ID)
						expected = "invalid managed-home root"
						if _, err := HomePath(original.Identity.HomeRoot, original.Identity.Name, original.Identity.ID); err == nil {
							t.Fatal("accepted invalid allocation root")
						}
					} else {
						original.Identity.CreatedAt += invalid.value
					}
					if err := original.Identity.Validate(); err == nil || err.Error() != expected {
						t.Fatalf("identity text validation: %v", err)
					}
					if err := store.Save(original); err == nil {
						t.Fatal("saved invalid identity text")
					}
					if _, err := os.Stat(store.Root()); !errors.Is(err, fs.ErrNotExist) {
						t.Fatalf("invalid identity created state directories: %v", err)
					}
				})
			}
		})
	}
}

func TestUnicodeHomeOwnershipRoundTrip(t *testing.T) {
	store, original := fixture(t, "sandbox")
	original.Identity.HomeRoot = filepath.Join(filepath.Dir(store.Root()), "дома-🦀")
	original.Identity.CreatedAt = "legacy metadata 🦀"
	var err error
	original.Identity.Home, err = HomePath(original.Identity.HomeRoot, original.Identity.Name, original.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(original.Identity.Home, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(original.Identity.Home, "keep")
	if err := os.WriteFile(keep, []byte("данные"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(original.Identity.Name)
	if err != nil || loaded != original {
		t.Fatalf("Unicode identity changed during JSON round-trip: %+v %v", loaded, err)
	}
	archive, err := store.PreserveHome(loaded.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := store.PreserveHome(original.Identity); err != nil || again != archive {
		t.Fatalf("Unicode ownership archive cannot be retried: %q %v", again, err)
	}
	var record identityRecord
	if err := readRecord(archive, &record); err != nil || record.Identity != original.Identity {
		t.Fatalf("Unicode archive changed ownership: %+v %v", record, err)
	}
	directory, err := store.InstanceDir(original.Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "данные" {
		t.Fatalf("preserved Unicode home data changed: %q %v", data, err)
	}
}

func stringValue(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func TestFetchAllSortsAndKeepsHealthyRecords(t *testing.T) {
	store, original := fixture(t, "zeta")
	for _, name := range []string{"zeta", "alpha", "broken"} {
		instance := original
		instance.Identity.Name = domain.VMName(name)
		var err error
		instance.Identity.Home, err = HomePath(instance.Identity.HomeRoot, instance.Identity.Name, instance.Identity.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Save(instance); err != nil {
			t.Fatal(err)
		}
	}
	broken := filepath.Join(store.Root(), "instances", "broken", "instance.json")
	if err := os.WriteFile(broken, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(store.Root(), "instances", "unfinished"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root(), "instances", "unrelated-file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := store.FetchAll()
	if err != nil || len(entries) != 4 {
		t.Fatalf("list %+v: %v", entries, err)
	}
	for index, name := range []string{"alpha", "broken", "unfinished", "zeta"} {
		if entries[index].Name != name {
			t.Fatalf("entry %d: %+v", index, entries[index])
		}
	}
	if entries[0].Instance == nil || entries[1].Identity == nil || entries[1].Instance != nil || entries[2].Identity != nil || entries[3].Instance == nil {
		t.Fatalf("incorrect corruption rows: %+v", entries)
	}
}

func TestRetainedHomeArchiveSurvivesStateAndNameReuse(t *testing.T) {
	store, first := fixture(t, "sandbox")
	archive, err := store.PreserveHome(first.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if archive != filepath.Join(store.Root(), "homes", "sandbox-012345abcdef.json") {
		t.Fatalf("archive %q", archive)
	}
	info, err := os.Stat(archive)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode: %v", err)
	}
	changed := first.Identity
	changed.Username = domain.Username("another")
	if _, err := store.PreserveHome(changed); err == nil {
		t.Fatal("replaced ownership archive")
	}
	if err := store.ForgetHome(changed); err == nil {
		t.Fatal("deleted mismatching archive")
	}
	second := first.Identity
	second.ID = "abcdef012345"
	second.Home, err = HomePath(second.HomeRoot, second.Name, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondArchive, err := store.PreserveHome(second)
	if err != nil || secondArchive == archive {
		t.Fatalf("reused name archive %q: %v", secondArchive, err)
	}
	if err := store.ForgetHome(first.Identity); err != nil {
		t.Fatal(err)
	}
	if err := store.ForgetHome(first.Identity); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secondArchive); err != nil {
		t.Fatalf("deleted other ownership archive: %v", err)
	}
}

func TestStateRefusesSymlinkDirectoriesRecordsAndLocks(t *testing.T) {
	store, original := fixture(t, "sandbox")
	if err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	foreign := t.TempDir()
	instanceDirectory := filepath.Join(store.Root(), "instances", "sandbox")
	if err := os.Symlink(foreign, instanceDirectory); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(original); err == nil {
		t.Fatal("saved through symlink directory")
	}
	if err := os.Remove(instanceDirectory); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	identity := filepath.Join(instanceDirectory, "identity.json")
	target := filepath.Join(foreign, "identity.json")
	if err := os.Rename(identity, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, identity); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadIdentity(original.Identity.Name); err == nil {
		t.Fatal("read symlink identity")
	}
	lockPath := filepath.Join(store.Root(), "locks", "instances", "sandbox.lock")
	if err := os.Symlink(target, lockPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstanceLock(context.Background(), original.Identity.Name); err == nil || errors.Is(err, ErrLockBusy) {
		t.Fatalf("symlink lock misclassified: %v", err)
	}
}

func TestMissingStateAndInvalidNamesCreateNothing(t *testing.T) {
	store, _ := fixture(t, "sandbox")
	entries, err := store.FetchAll()
	if err != nil || len(entries) != 0 {
		t.Fatalf("missing list %+v: %v", entries, err)
	}
	for _, name := range []string{"unknown", "../escape", "/tmp/escape", "has/slash", ".", ""} {
		if _, err := store.Load(domain.VMName(name)); err == nil {
			t.Fatalf("loaded %q", name)
		}
	}
	if _, err := os.Stat(store.Root()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read created state root: %v", err)
	}
}

func TestDefaultStateRootResolvesHomeAndOverride(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "actual-home")
	if err := os.Mkdir(actual, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "home-link")
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", link)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("LIMANIX_HOME", "")
	defaultPath, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filesystem.Resolve(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore("")
	if err != nil || store.Root() != expected {
		t.Fatalf("default root %v: %v, want %q", store, err, expected)
	}
	t.Setenv("LIMANIX_HOME", "~/override")
	store, err = NewStore("")
	expected, resolveErr := filesystem.Resolve(filepath.Join(actual, "override"))
	if err != nil || resolveErr != nil || store.Root() != expected {
		t.Fatalf("override root %v: %v, want %q: %v", store, err, expected, resolveErr)
	}
}

func TestRegistryReadersWaitWritersCancelAndTimeout(t *testing.T) {
	store, _ := fixture(t, "sandbox")
	ctx := context.Background()
	first, err := store.RegistryLock(ctx, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	}()
	second, err := store.RegistryLock(ctx, true, 0)
	if err != nil {
		t.Fatal("reader blocked another reader:", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := store.RegistryLock(ctx, false, 80*time.Millisecond); !errors.Is(err, ErrLockBusy) {
		t.Fatalf("writer contention %v", err)
	}
	if time.Since(started) < 60*time.Millisecond {
		t.Fatal("writer did not wait for its timeout")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.RegistryLock(canceled, false, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lock: %v", err)
	}
	waiting, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if _, err := store.RegistryLock(waiting, false, time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lock: %v", err)
	}
	result := make(chan error, 1)
	go func() {
		lock, err := store.RegistryLock(ctx, false, time.Second)
		if err == nil {
			err = lock.Close()
		}
		result <- err
	}()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal("waiter did not acquire after release:", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter stuck")
	}
}

// The test binary doubles as a helper so locks are validated across real processes.
func TestLockHelperProcess(t *testing.T) {
	if os.Getenv("LIMANIX_STATE_HELPER") != "1" {
		return
	}
	store, err := NewStore(os.Getenv("LIMANIX_STATE_HELPER_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	mode := os.Getenv("LIMANIX_STATE_HELPER_MODE")
	var lock *Lock
	if mode == "registry" {
		lock, err = store.RegistryLock(context.Background(), false, 0)
	} else {
		lock, err = store.InstanceLock(context.Background(), domain.VMName(mode))
	}
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			os.Exit(12)
		}
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	if status := os.Getenv("LIMANIX_STATE_HELPER_STATUS"); status != "" {
		instance, err := store.Load(domain.VMName(mode))
		if err != nil {
			t.Fatal(err)
		}
		instance.Status = Status(status)
		if err := store.Save(instance); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stdout.WriteString("active\n"); err != nil {
			t.Fatal(err)
		}
		var input [1]byte
		if _, err := os.Stdin.Read(input[:]); err != nil {
			t.Fatal(err)
		}
	}
}

func helperCommand(store *Store, mode, status string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	command.Env = append(os.Environ(), "LIMANIX_STATE_HELPER=1", "LIMANIX_STATE_HELPER_ROOT="+store.Root(),
		"LIMANIX_STATE_HELPER_MODE="+mode, "LIMANIX_STATE_HELPER_STATUS="+status,
		"GORACE="+os.Getenv("GORACE")+" atexit_sleep_ms=0")
	return command
}

func TestRealProcessLocksArePerVMAndRegistryIndependent(t *testing.T) {
	store, original := fixture(t, "sandbox")
	lock, err := store.InstanceLock(context.Background(), original.Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	busy := helperCommand(store, "sandbox", "").Run()
	var exit *exec.ExitError
	if !errors.As(busy, &exit) || exit.ExitCode() != 12 {
		t.Fatalf("another process did not see VM lock: %v", busy)
	}
	for _, mode := range []string{"other-vm", "registry"} {
		if output, err := helperCommand(store, mode, "").CombinedOutput(); err != nil {
			t.Fatalf("independent %s lock failed: %s %v", mode, output, err)
		}
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if output, err := helperCommand(store, "sandbox", "").CombinedOutput(); err != nil {
		t.Fatalf("VM lock not released: %s %v", output, err)
	}
}

func TestKilledOperationsListInterruptedWithoutPersistingIt(t *testing.T) {
	for _, status := range []Status{Creating, Updating, Deleting} {
		t.Run(string(status), func(t *testing.T) {
			store, original := fixture(t, "sandbox")
			if err := store.Save(original); err != nil {
				t.Fatal(err)
			}
			command := helperCommand(store, "sandbox", string(status))
			stdin, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			command.Stderr = &stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = stdin.Close()
				if command.ProcessState == nil {
					_ = command.Process.Kill()
					_ = command.Wait()
				}
			}()
			ready := make(chan string, 1)
			go func() {
				line, _ := bufio.NewReader(stdout).ReadString('\n')
				ready <- line
			}()
			select {
			case line := <-ready:
				if line != "active\n" {
					t.Fatalf("helper did not save state: %q %s", line, stderr.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("helper did not acquire lock")
			}
			directory, err := store.InstanceDir(original.Identity.Name)
			if err != nil {
				t.Fatal(err)
			}
			record := filepath.Join(directory, "instance.json")
			before, err := os.ReadFile(record)
			if err != nil {
				t.Fatal(err)
			}
			entries, err := store.FetchAll()
			if err != nil || len(entries) != 1 || entries[0].Instance == nil || entries[0].Instance.Status != status {
				t.Fatalf("running operation incorrectly marked interrupted: %+v %v", entries, err)
			}
			if err := command.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := command.Wait(); err == nil {
				t.Fatal("killed process succeeded")
			}
			entries, err = store.FetchAll()
			if err != nil || len(entries) != 1 || entries[0].Instance == nil || entries[0].Instance.Status != Interrupted {
				t.Fatalf("abandoned operation not marked interrupted: %+v %v", entries, err)
			}
			after, err := os.ReadFile(record)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("listing rewrote state: %s %v", after, err)
			}
			loaded, err := store.Load(original.Identity.Name)
			if err != nil || loaded.Status != status {
				t.Fatalf("persisted lifecycle changed: %+v %v", loaded, err)
			}
		})
	}
}

func TestInterruptedRefreshReloadsCompletedOperation(t *testing.T) {
	for _, status := range []Status{Creating, Updating} {
		t.Run(string(status), func(t *testing.T) {
			store, original := fixture(t, "sandbox")
			original.Status = status
			writer, err := store.InstanceLock(context.Background(), original.Identity.Name)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			}()
			if err := store.Save(original); err != nil {
				t.Fatal(err)
			}
			stale, err := store.loadEntry(original.Identity.Name)
			if err != nil || stale.Instance == nil || stale.Instance.Status != status {
				t.Fatalf("active operation was not read: %+v %v", stale, err)
			}
			completed := original
			completed.Status = Ready
			completed.Generation = "fedcba987654"
			if err := store.Save(completed); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			// The operation completed between the initial read and the shared lock.
			refreshed, err := store.refreshInterruptedEntry(original.Identity.Name, stale)
			if err != nil || refreshed.Instance == nil || *refreshed.Instance != completed {
				t.Fatalf("refresh returned stale or interrupted state: %+v %v", refreshed, err)
			}
			if refreshed.Identity == nil || *refreshed.Identity != completed.Identity {
				t.Fatalf("refresh lost identity: %+v", refreshed)
			}
			if stale.Instance.Status != status || stale.Instance.Generation != original.Generation {
				t.Fatalf("refresh mutated the earlier entry: %+v", stale)
			}
			loaded, err := store.Load(original.Identity.Name)
			if err != nil || loaded != completed {
				t.Fatalf("refresh changed persisted state: %+v %v", loaded, err)
			}
		})
	}
}
