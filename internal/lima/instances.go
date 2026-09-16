package lima

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/limatype/dirnames"
)

func validateOwnedInstanceName(name string) error {
	if !strings.HasPrefix(name, "limanix-") {
		return ErrForeignInstance
	}

	return dirnames.ValidateInstName(name)
}

// inspectInstance restricts inspection to Limanix names. Damaged metadata remains
// accessible to Delete, which may need it to clean up an incomplete instance.
func (client *Client) inspectInstance(ctx context.Context, name string) (*limatype.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := validateOwnedInstanceName(name); err != nil {
		return nil, err
	}

	query, cancel := queryContext(ctx)
	defer cancel()

	inst, err := client.native.inspect(query, name)
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
func (client *Client) FetchAll(ctx context.Context) (instances []Instance, failure error) {
	defer func() {
		failure = operationError(ctx, "list", failure)
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	names, err := client.native.instances()
	if err != nil {
		return nil, err
	}

	instances = []Instance{}

	for _, name := range names {
		if !strings.HasPrefix(name, "limanix-") {
			continue
		}

		upstream, err := client.inspectInstance(ctx, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, err
		}

		inst, err := instanceMetadata(upstream)
		if err != nil {
			return nil, err
		}

		instances = append(instances, inst)
	}

	return instances, nil
}

func instanceMetadata(upstream *limatype.Instance) (Instance, error) {
	if upstream == nil {
		return Instance{}, ErrMissingMetadata
	}

	if err := validateOwnedInstanceName(upstream.Name); err != nil {
		return Instance{}, err
	}

	switch {
	case !Status(upstream.Status).valid():
		return Instance{}, ErrInvalidStatus
	case upstream.Disk < 0:
		return Instance{}, ErrInvalidDisk
	}

	inst := Instance{
		Name:     upstream.Name,
		Status:   Status(upstream.Status),
		Networks: []Network{},
	}

	if upstream.Config != nil {
		disk := upstream.Disk
		inst.Disk = &disk
	}

	for _, network := range upstream.Networks {
		inst.Networks = append(inst.Networks, Network{
			MACAddress: strings.ToLower(network.MACAddress),
			Shared:     network.Lima == "shared" || enabled(network.VZNAT),
		})
	}

	return inst, nil
}

func inspectionErrors(inst *limatype.Instance) error {
	if len(inst.Errors) != 0 {
		return fmt.Errorf("errors inspecting instance: %w", errors.Join(inst.Errors...))
	}

	if inst.Config == nil {
		return ErrMissingConfig
	}

	return nil
}

func enabled(value *bool) bool {
	return value != nil && *value
}
