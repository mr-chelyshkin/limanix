package managedhome

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

func fixture(t *testing.T) domain.Identity {
	t.Helper()
	root, err := filesystem.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := domain.Identity{
		Name: domain.VMName("sandbox"), ID: "012345abcdef", Arch: domain.ARM64,
		Username: domain.Username("dev"), UserHome: domain.GuestPath("/home/dev"),
		HomeRoot: filepath.Join(root, "homes"), CreatedAt: "2026-09-13T12:00:00+00:00",
	}
	identity.Home, err = domain.HomePath(identity.HomeRoot, identity.Name, identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestPrivateHomeAndGuestMarkersCannotChangeRemoval(t *testing.T) {
	identity := fixture(t)
	home, err := (&Manager{}).Create(identity)
	if err != nil || home != identity.Home {
		t.Fatalf("home %q: %v", home, err)
	}
	info, err := os.Stat(home)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("private home mode: %v", err)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("home not empty: %v %v", entries, err)
	}
	foreign := t.TempDir()
	keep := filepath.Join(foreign, "keep")
	if err := os.WriteFile(keep, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".limanix-owner.json"), []byte(`{"home":"`+foreign+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, filepath.Join(home, "outside-link")); err != nil {
		t.Fatal(err)
	}
	if err := (&Manager{}).Remove(identity); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("home not removed: %v", err)
	}
	data, err := os.ReadFile(keep)
	if err != nil || string(data) != "original" {
		t.Fatalf("followed guest link: %q %v", data, err)
	}
	if err := (&Manager{}).Remove(identity); err != nil {
		t.Fatal("missing home removal:", err)
	}
	if _, err := os.Stat(identity.HomeRoot); err != nil {
		t.Fatal("removed parent root:", err)
	}
}

func TestPreexistingHomeIsNeverAdopted(t *testing.T) {
	identity := fixture(t)
	if err := os.MkdirAll(identity.Home, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(identity.Home, "keep")
	if err := os.WriteFile(keep, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Manager{}).Create(identity); err == nil {
		t.Fatal("adopted an existing home")
	}
	data, err := os.ReadFile(keep)
	if err != nil || string(data) != "original" {
		t.Fatalf("changed preexisting home: %q %v", data, err)
	}
}

func TestMismatchedAllocationCannotRemoveHome(t *testing.T) {
	identity := fixture(t)
	if _, err := (&Manager{}).Create(identity); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(identity.Home, "keep")
	if err := os.WriteFile(keep, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*domain.Identity){
		func(i *domain.Identity) { i.Home = i.HomeRoot },
		func(i *domain.Identity) { i.HomeRoot = i.Home },
		func(i *domain.Identity) { i.Name = domain.VMName("other") },
		func(i *domain.Identity) { i.ID = "abcdef012345" },
	} {
		changed := identity
		change(&changed)
		if err := (&Manager{}).Remove(changed); err == nil {
			t.Fatalf("removed mismatching allocation: %+v", changed)
		}
		if _, err := os.Stat(keep); err != nil {
			t.Fatal("changed home:", err)
		}
	}
}

func TestSymlinkHomeAndParentAreRefused(t *testing.T) {
	for _, where := range []string{"home", "root"} {
		t.Run(where, func(t *testing.T) {
			identity := fixture(t)
			foreign := t.TempDir()
			path := identity.HomeRoot
			if where == "home" {
				if err := os.Mkdir(identity.HomeRoot, 0o700); err != nil {
					t.Fatal(err)
				}
				path = identity.Home
			}
			if err := os.Symlink(foreign, path); err != nil {
				t.Fatal(err)
			}
			if _, err := (&Manager{}).Create(identity); err == nil {
				t.Fatal("created through symbolic link")
			}
			if err := (&Manager{}).Remove(identity); err == nil {
				t.Fatal("removed through symbolic link")
			}
			if _, err := os.Stat(foreign); err != nil {
				t.Fatal("changed symlink target:", err)
			}
		})
	}
}

func TestOrdinaryFileIsNotRemovedAsHome(t *testing.T) {
	identity := fixture(t)
	if err := os.Mkdir(identity.HomeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity.Home, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&Manager{}).Remove(identity); err == nil {
		t.Fatal("removed ordinary file")
	}
	data, err := os.ReadFile(identity.Home)
	if err != nil || string(data) != "keep" {
		t.Fatalf("changed ordinary file: %q %v", data, err)
	}
}
