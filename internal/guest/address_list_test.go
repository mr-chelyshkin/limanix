package guest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/guest"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/managedhome"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
	"github.com/mr-chelyshkin/limanix/internal/state"
	"github.com/mr-chelyshkin/limanix/internal/vm"
)

type addressListBackend struct {
	*lima.Client
	instances   []lima.Instance
	unreachable string
}

func (backend *addressListBackend) FetchAll(context.Context) ([]lima.Instance, error) {
	return backend.instances, nil
}

func (backend *addressListBackend) Run(ctx context.Context, name string, _ []string, _ bool) (string, error) {
	if name == backend.unreachable {
		return "", context.DeadlineExceeded
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return `[{"address":"52:55:55:aa:bb:cc","addr_info":[{"family":"inet","scope":"global","local":"192.0.2.10"}]}]`, nil
}

func TestVMListRetainsHealthyRowsAfterAddressProbeFailure(t *testing.T) {
	store, err := state.NewStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &addressListBackend{}
	for index, name := range []domain.VMName{"a-unreachable", "b-healthy"} {
		identifier := []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb"}[index]
		root := filepath.Join(store.Root(), "homes")
		home, err := domain.HomePath(root, name, identifier)
		if err != nil {
			t.Fatal(err)
		}
		identity := domain.Identity{
			Name: name, ID: identifier, Username: "dev", UserHome: "/home/dev", Arch: domain.ARM64,
			HomeRoot: root, Home: home, CreatedAt: "2026-09-15T00:00:00Z",
		}
		if err := store.Save(domain.Instance{Identity: identity, Status: domain.Ready, Generation: "cccccccccccc"}); err != nil {
			t.Fatal(err)
		}
		instance := lima.Instance{
			Name: identity.LimaName(), Status: lima.Running,
			Networks: []lima.Network{{MACAddress: "52:55:55:aa:bb:cc", Shared: true}},
		}
		backend.instances = append(backend.instances, instance)
		if index == 0 {
			backend.unreachable = instance.Name
		}
	}
	ctx := context.Background()
	metadata, err := nixos.SystemModules()
	if err != nil {
		t.Fatal(err)
	}

	entries, err := vm.New(vm.Dependencies{
		Modules: modules.NewRegistry(store, metadata),
		Homes:   &managedhome.Manager{},
		Guest:   guest.New(backend),
		Backend: backend,
		Store:   store,
		HostUID: 501,
	}).FetchAll(ctx)
	if err != nil || len(entries) != 2 {
		t.Fatalf("an unavailable address blocked VM listing: entries=%v, error=%v", entries, err)
	}
	if entries[0].Name != "a-unreachable" || entries[0].Address != "" || entries[0].BackendStatus == nil || *entries[0].BackendStatus != lima.Running || entries[0].Error != nil {
		t.Fatalf("unreachable VM row lost its valid state: %#v", entries[0])
	}
	if entries[1].Name != "b-healthy" || entries[1].Address != "192.0.2.10" || entries[1].Error != nil {
		t.Fatalf("failed address probe prevented healthy VM listing: %#v", entries[1])
	}
}
