package config

import (
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Validate checks domain values and cross-field policy without consulting the module registry.
func Validate(config Config) error {
	if err := validateValue(reflect.ValueOf(config), ""); err != nil {
		return err
	}

	switch {
	case config.SchemaVersion != 1:
		return fieldError("schema_version", "only version 1 is supported")
	case config.Resources.CPU <= 0:
		return fieldError("resources.cpu", "must be positive")
	case path.Clean(config.Home.Root) == "/":
		return fieldError("home.root", "the host root directory is not allowed")
	}

	if err := validatePorts(config.Network.Ports.TCP, "tcp"); err != nil {
		return err
	}

	if err := validatePorts(config.Network.Ports.UDP, "udp"); err != nil {
		return err
	}

	if err := validateModules(config.NixOS.Modules); err != nil {
		return err
	}

	return validateMounts(config.User.Home, config.Mounts)
}

func validatePorts(ports []int, protocol string) error {
	for index, port := range ports {
		if port < 1 || port > 65535 {
			field := fmt.Sprintf("network.ports.%s[%d]", protocol, index)

			return fieldError(field, "port must be between 1 and 65535")
		}
	}

	return nil
}

func validateModules(modules []domain.ModuleID) error {
	seen := make(map[domain.ModuleID]bool, len(modules))

	for index, module := range modules {
		if seen[module] {
			return fieldError(fmt.Sprintf("nixos.modules[%d]", index), "duplicate module ID")
		}

		seen[module] = true
	}

	return nil
}

func validateMounts(home domain.GuestPath, mounts []Mount) error {
	userHome, err := checkedGuestTarget(home, "user.home")
	if err != nil {
		return err
	}

	targets := make([]string, 0, len(mounts))

	for index, mount := range mounts {
		field := fmt.Sprintf("mounts[%d].target", index)

		target, err := checkedGuestTarget(mount.Target, field)
		if err != nil {
			return err
		}

		if isWithin(userHome, target) {
			return fieldError(field, "would hide the managed user home")
		}

		for _, previous := range targets {
			if isWithin(target, previous) || isWithin(previous, target) {
				return fieldError(field, "overlaps another explicit mount")
			}
		}

		targets = append(targets, target)
	}

	return nil
}

func checkedGuestTarget(value domain.GuestPath, field string) (string, error) {
	normalized, err := domain.NewGuestPath(string(value))
	if err != nil {
		return "", wrapField(field, err.Error(), err)
	}

	var (
		target   = string(normalized)
		reserved = []string{
			"/etc",
			"/boot",
			"/usr",
			"/var",
			"/nix",
			"/run",
			"/dev",
			"/proc",
			"/sys",
			"/bin",
			"/sbin",
			"/mnt/limanix",
			"/home/limanix-admin",
		}
	)

	for _, root := range reserved {
		if isWithin(target, root) {
			return "", fieldError(field, "destination is reserved for the guest system")
		}
	}

	if isWithin("/home/limanix-admin", target) {
		return "", fieldError(field, "destination is reserved for the guest system")
	}

	return target, nil
}

func isWithin(candidate, root string) bool {
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}
