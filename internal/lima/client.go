package lima

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lima-vm/lima/v2/pkg/instance"
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/limatype/dirnames"
	"github.com/lima-vm/lima/v2/pkg/limayaml"
	"github.com/lima-vm/lima/v2/pkg/networks/reconcile"
	"github.com/lima-vm/lima/v2/pkg/store"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Error identifies an operation without including guest command arguments.
type Error struct {
	Operation string
	Err       error
}

func (err *Error) Error() string { return "Lima " + err.Operation + ": " + err.Err.Error() }
func (err *Error) Unwrap() error { return err.Err }

// Some upstream shutdown paths report a timeout when their context is canceled.
// Preserve the caller's cancellation cause at our adapter boundary.
func operationError(ctx context.Context, operation string, err error) error {
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return &Error{Operation: operation, Err: err}
}

// Client operates Lima's store, drivers and instance lifecycle in-process. Lima
// starts this executable's hidden hostagent command; no limactl installation is
// involved. The guest-agent provider returns the packaged agent for each guest.
type Client struct {
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	sshCommand     func(context.Context, *limatype.Instance, []string, bool) (*exec.Cmd, error)
	agentPath      func(context.Context, domain.Architecture) (string, error)
	executable     func() (string, error)
	instances      func() ([]string, error)
	inspect        func(context.Context, string) (*limatype.Instance, error)
	create         func(context.Context, string, []byte, bool) (*limatype.Instance, error)
	start          func(context.Context, *limatype.Instance, bool, bool, string, string) error
	stop           func(context.Context, *limatype.Instance, bool) error
	delete         func(context.Context, *limatype.Instance, bool) error
	reconcile      func(context.Context, string) error
	validateDriver func(context.Context, *limatype.Instance) error
}

func NewClient(agentPath func(context.Context, domain.Architecture) (string, error)) *Client {
	return &Client{
		Stdin:          os.Stdin,
		Stdout:         os.Stdout,
		Stderr:         os.Stderr,
		agentPath:      agentPath,
		executable:     os.Executable,
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

func queryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}

func validateOwnedInstanceName(name string) error {
	if !strings.HasPrefix(name, "limanix-") {
		return errors.New("instance is not managed by Limanix")
	}
	return dirnames.ValidateInstName(name)
}

// inspectInstance is shared by lifecycle and SSH operations. Only Limanix's
// names reach upstream inspection, which may return metadata with Errors for a
// damaged instance. Delete must retain access to that metadata for cleanup.
func (client *Client) inspectInstance(ctx context.Context, name string) (*limatype.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateOwnedInstanceName(name); err != nil {
		return nil, err
	}
	query, cancel := queryContext(ctx)
	defer cancel()
	inst, err := client.inspect(query, name)
	if err != nil {
		return nil, err
	}
	if err := query.Err(); err != nil {
		return nil, err
	}
	return inst, nil
}

// FetchAll enumerates the native Lima store and filters foreign names before
// inspecting them. Upstream metadata is narrowed to the orchestration contract.
func (client *Client) FetchAll(ctx context.Context) ([]Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	names, err := client.instances()
	if err != nil {
		return nil, &Error{Operation: "list", Err: err}
	}
	result := []Instance{}
	for _, name := range names {
		if !strings.HasPrefix(name, "limanix-") {
			continue
		}
		upstream, err := client.inspectInstance(ctx, name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, &Error{Operation: "list", Err: err}
		}
		inst, err := instanceMetadata(upstream)
		if err != nil {
			return nil, &Error{Operation: "list", Err: err}
		}
		result = append(result, inst)
	}
	return result, nil
}

func instanceMetadata(upstream *limatype.Instance) (Instance, error) {
	if upstream == nil {
		return Instance{}, errors.New("expected instance metadata")
	}
	if err := validateOwnedInstanceName(upstream.Name); err != nil {
		return Instance{}, err
	}
	if !Status(upstream.Status).valid() {
		return Instance{}, errors.New("expected a known instance status")
	}
	if upstream.Disk < 0 {
		return Instance{}, errors.New("expected a nonnegative disk size in bytes")
	}
	inst := Instance{Name: upstream.Name, Status: Status(upstream.Status), Networks: []Network{}}
	if upstream.Config != nil {
		disk := upstream.Disk
		inst.Disk = &disk
	}
	for _, network := range upstream.Networks {
		inst.Networks = append(inst.Networks, Network{MACAddress: strings.ToLower(network.MACAddress), Shared: network.Lima == "shared" || network.VZNAT != nil && *network.VZNAT})
	}
	return inst, nil
}

// Validate loads defaults and validates through Lima's native schema.
func (client *Client) Validate(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return &Error{Operation: "validate", Err: err}
	}
	yaml, err := limayaml.Load(ctx, data, absolute)
	if err != nil {
		return &Error{Operation: "validate", Err: err}
	}
	if err := limayaml.Validate(yaml, false); err != nil {
		return &Error{Operation: "validate", Err: err}
	}
	return ctx.Err()
}
