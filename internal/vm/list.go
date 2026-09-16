package vm

import (
	"context"

	"github.com/mr-chelyshkin/limanix/internal/lima"
)

// FetchAll lists every saved VM, including damaged or interrupted records.
func (m *Manager) FetchAll(ctx context.Context) ([]Info, error) {
	entries, err := m.store.FetchAll()
	if err != nil {
		return nil, err
	}

	backendInstances := make(map[string]lima.Instance)
	for _, entry := range entries {
		if entry.Identity != nil {
			instances, err := m.backend.FetchAll(ctx)
			if err != nil {
				return nil, err
			}
			for _, instance := range instances {
				backendInstances[instance.Name] = instance
			}
			break
		}
	}
	
	result := make([]Info, 0, len(entries))
	for _, entry := range entries {
		info := Info{Name: entry.Name, Error: entry.Error}
		if entry.Identity != nil {
			identity := entry.Identity
			name := identity.LimaName()
			info.Home, info.Arch, info.LimaName = &identity.Home, &identity.Arch, &name
			if instance, exists := backendInstances[name]; exists {
				info.BackendStatus = &instance.Status
				info.Address = m.guest.Address(ctx, instance)
			}
		}
		if entry.Instance != nil {
			info.OperationStatus = &entry.Instance.Status
			if info.Error == nil {
				info.Error = entry.Instance.Error
			}
		}
		result = append(result, info)
	}
	return result, nil
}
