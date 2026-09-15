// Package config owns the versioned public TOML contract, defaults and documentation.
package config

import "github.com/mr-chelyshkin/limanix/internal/domain"

type Resources struct {
	Arch domain.Architecture `toml:"arch" json:"arch" doc:"Guest architecture."`
	Disk domain.ByteSize     `toml:"disk" json:"disk" doc:"Guest system disk size in GiB."`
	Mem  domain.ByteSize     `toml:"mem" json:"mem" doc:"Guest memory size in GiB."`
	CPU  int                 `toml:"cpu" json:"cpu" doc:"Guest CPU count, a positive integer."`
}

type User struct {
	Name domain.Username  `toml:"name" json:"name" doc:"Regular guest username."`
	Home domain.GuestPath `toml:"home" json:"home" doc:"Guest user's home directory."`
	Sudo bool             `toml:"sudo" json:"sudo" doc:"Passwordless sudo inside the guest."`
}

type Home struct {
	Root string `toml:"root" json:"root" doc:"Host root for <root>/<name>-<id>. Limanix creates this directory on the Mac and mounts it at user.home with read-write access."`
}

type NixOS struct {
	Modules []domain.ModuleID `toml:"modules" json:"modules" doc:"Bundled module names (git, rust, neovim), or third-party:NAME for a module imported with limanix modules add."`
}

type Ports struct {
	TCP []int `toml:"tcp" json:"tcp" doc:"Inbound TCP ports in the guest firewall. Services listen on a guest network interface and are reached at <guest-ip>:<port> from the Mac."`
	UDP []int `toml:"udp" json:"udp" doc:"Inbound UDP ports in the guest firewall."`
}

type Network struct {
	Mode  string `toml:"mode" json:"mode" doc:"Shared/NAT network. The Mac reaches the guest by its own IP address." choices:"shared"`
	Ports Ports  `toml:"ports" json:"ports" doc:"Inbound guest firewall ports."`
}

type Mount struct {
	Mode   string           `toml:"mode" json:"mode" doc:"Mount access: rw or ro." choices:"rw,ro" default:"rw"`
	Source string           `toml:"source" json:"source" doc:"Host directory to mount inside the guest." required:"true"`
	Target domain.GuestPath `toml:"target" json:"target" doc:"Mount destination inside the guest." required:"true"`
}

type Config struct {
	SchemaVersion int                                `toml:"schema_version" json:"schema_version" doc:"Contract version, independent of the installed Limanix package version."`
	Name          domain.VMName                      `toml:"name" json:"name" doc:"Sandbox name."`
	User          User                               `toml:"user" json:"user" doc:"Regular guest user and sudo access."`
	Resources     Resources                          `toml:"resources" json:"resources" doc:"Guest architecture and compute resources."`
	Home          Home                               `toml:"home" json:"home" doc:"Host storage for the guest user's home directory."`
	NixOS         NixOS                              `toml:"nixos" json:"nixos" doc:"Trusted modules that configure the guest system."`
	Network       Network                            `toml:"network" json:"network" doc:"Guest network and inbound firewall ports."`
	Env           map[domain.EnvName]domain.EnvValue `toml:"env" json:"env" doc:"Guest-wide environment for login sessions and system/user services."`
	Mounts        []Mount                            `toml:"mounts" json:"mounts" doc:"Host directories mounted inside the guest."`
}

// Default returns independent collections for a new configuration.
func Default() Config {
	return Config{
		SchemaVersion: 1,
		Name:          "example-box",
		User:          User{Name: "dev", Home: "/home/dev", Sudo: true},
		Resources:     Resources{Arch: domain.ARM64, Disk: domain.ByteSize(10 * domain.GiB), CPU: 4, Mem: domain.ByteSize(8 * domain.GiB)},
		Home:          Home{Root: "~/.limanix"},
		NixOS:         NixOS{Modules: []domain.ModuleID{"git"}},
		Network:       Network{Mode: "shared", Ports: Ports{TCP: []int{8080}, UDP: []int{}}},
		Env:           map[domain.EnvName]domain.EnvValue{"APP_ENV": "development", "APP_LOG_LEVEL": "debug"},
		Mounts: []Mount{
			{Mode: "rw", Source: "~/projects/my-project", Target: "/workspace"},
			{Mode: "ro", Source: "~/.ssh/limanix", Target: "/mnt/git-keys"},
		},
	}
}
