package vm

import (
	"context"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/state"
)

const addressProbeConcurrency = 4

// FetchAll lists every saved VM, including damaged or interrupted records.
func (m *Manager) FetchAll(ctx context.Context) ([]Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := m.store.FetchAll()
	if err != nil {
		return nil, err
	}

	instances, err := m.listBackend(ctx, entries)
	if err != nil {
		return nil, err
	}

	result := make([]Info, len(entries))
	var probes errgroup.Group
	probes.SetLimit(addressProbeConcurrency)

	for index, entry := range entries {
		probes.Go(func() error {
			result[index] = m.instanceInfo(ctx, entry, instances)
			return ctx.Err()
		})
	}

	if err = probes.Wait(); err != nil {
		return nil, err
	}

	return result, ctx.Err()
}

func (m *Manager) listBackend(ctx context.Context, entries []state.Entry) (map[string]lima.Instance, error) {
	var (
		result      = make(map[string]lima.Instance)
		hasIdentity = slices.ContainsFunc(entries, func(entry state.Entry) bool {
			return entry.Identity != nil
		})
	)
	if !hasIdentity {
		return result, nil
	}

	instances, err := m.backend.FetchAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		result[instance.Name] = instance
	}

	return result, nil
}

func (m *Manager) instanceInfo(ctx context.Context, entry state.Entry, instances map[string]lima.Instance) Info {
	info := Info{
		Name:  entry.Name,
		Error: entry.Error,
	}

	if identity := entry.Identity; identity != nil {
		name := identity.LimaName()
		info.Home = &identity.Home
		info.Arch = &identity.Arch
		info.LimaName = &name

		if instance, exists := instances[name]; exists {
			info.BackendStatus = &instance.Status
			info.Address = m.guest.Address(ctx, instance)
		}
	}

	if record := entry.Instance; record != nil {
		info.OperationStatus = &record.Status
		if info.Error == nil {
			info.Error = record.Error
		}
	}

	return info
}
