package nixos

import (
	"encoding/json"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

type runtimeUser struct {
	Name domain.Username  `json:"name"`
	Home domain.GuestPath `json:"home"`
	Sudo bool             `json:"sudo"`
	UID  int              `json:"uid"`
}

type runtimeConfig struct {
	Name    domain.VMName       `json:"name"`
	Arch    domain.Architecture `json:"arch"`
	User    runtimeUser         `json:"user"`
	Ports   config.Ports        `json:"ports"`
	Modules []string            `json:"modules"`
}

func writeRuntime(filename string, cfg config.Config, imports []string, uid int) error {
	record := runtimeConfig{
		Name: cfg.Name,
		Arch: cfg.Resources.Arch,
		User: runtimeUser{
			Name: cfg.User.Name,
			Home: cfg.User.Home,
			Sudo: cfg.User.Sudo,
			UID:  uid,
		},
		Ports:   cfg.Network.Ports,
		Modules: imports,
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return filesystem.WriteFileAtomic(filename, append(data, '\n'), 0o600)
}
