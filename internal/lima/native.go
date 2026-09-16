package lima

import (
	"context"

	"github.com/lima-vm/lima/v2/pkg/driver/external/server"
	"github.com/lima-vm/lima/v2/pkg/driverutil"
	"github.com/lima-vm/lima/v2/pkg/instance"
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/networks/reconcile"
	"github.com/lima-vm/lima/v2/pkg/store"
)

// nativeAPI isolates upstream entry points from the client policies: ownership,
// cancellation, configuration validation and packaged executable selection.
// Each function can be replaced in adapter tests without touching the Lima store.
type nativeAPI struct {
	instances      func() ([]string, error)
	inspect        func(context.Context, string) (*limatype.Instance, error)
	create         func(context.Context, string, []byte, bool) (*limatype.Instance, error)
	start          func(context.Context, *limatype.Instance, bool, bool, string, string) error
	stop           func(context.Context, *limatype.Instance, bool) error
	delete         func(context.Context, *limatype.Instance, bool) error
	reconcile      func(context.Context, string) error
	validateDriver func(context.Context, *limatype.Instance) error
}

func newNativeAPI() nativeAPI {
	return nativeAPI{
		instances:      store.Instances,
		inspect:        store.Inspect,
		create:         instance.Create,
		start:          instance.StartWithPaths,
		stop:           instance.StopGracefully,
		delete:         instance.Delete,
		reconcile:      reconcile.Reconcile,
		validateDriver: validateConfiguredDriver,
	}
}

func validateConfiguredDriver(ctx context.Context, inst *limatype.Instance) error {
	driver, err := driverutil.CreateConfiguredDriver(ctx, inst, 0)
	if err != nil {
		return err
	}

	defer server.Stop(inst.Dir, true)
	return driver.Validate(ctx)
}
