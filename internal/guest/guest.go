// Package guest applies NixOS generations and enters the development user session.
package guest

import (
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
)

// Client is the management connection needed for guest operations.
type Client interface {
	Run(context.Context, string, []string, bool) (string, error)
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Shell(context.Context, string, []string) (int, error)
}

const addressProbeTimeout = 5 * time.Second

type Guest struct{ client Client }

func New(client Client) *Guest { return &Guest{client: client} }

// userCommand changes to HOME and execs literal positional arguments.
// sudo --login reinterprets argument quoting, so a fixed explicit Bash script is used.
func userCommand(user domain.Username, command []string) []string {
	bash := "/run/current-system/sw/bin/bash"
	args := []string{"sudo", "--set-home", "--user", string(user), "--", bash}
	if len(command) > 0 {
		args = append(args, "--login")
	} else {
		command = []string{bash, "--login"}
	}
	args = append(args, "-c", `cd -- "$HOME" && exec "$@"`, "limanix-command")
	return append(args, command...)
}

// Apply installs global ENV before rebuilding; a failed rebuild never reboots.
func (guest *Guest) Apply(ctx context.Context, name string, user domain.Username) error {
	if _, err := guest.client.Run(ctx, name, []string{"sudo", "install", "-d", "-m", "0755", "/etc/limanix"}, true); err != nil {
		return err
	}
	for _, file := range []string{"environment", "environment.sh"} {
		if _, err := guest.client.Run(ctx, name, []string{"sudo", "install", "-m", "0644", "/mnt/limanix/" + file, "/etc/limanix/" + file}, true); err != nil {
			return err
		}
	}
	if _, err := guest.client.Run(ctx, name, []string{"sudo", "nixos-rebuild", "boot", "--flake", "path:/mnt/limanix/flake#runtime", "--no-write-lock-file"}, false); err != nil {
		return err
	}
	if err := guest.client.Stop(ctx, name); err != nil {
		return err
	}
	if err := guest.client.Start(ctx, name); err != nil {
		return err
	}
	_, err := guest.client.Run(ctx, name, userCommand(user, []string{"true"}), true)
	return err
}

// Address selects a global IPv4 address by the configured shared interface MAC.
func (guest *Guest) Address(ctx context.Context, instance lima.Instance) string {
	if instance.Status != lima.Running {
		return ""
	}
	sharedMACs := map[string]bool{}
	for _, network := range instance.Networks {
		if network.Shared && network.MACAddress != "" {
			sharedMACs[strings.ToLower(network.MACAddress)] = true
		}
	}
	if len(sharedMACs) == 0 {
		return ""
	}
	probe, cancel := context.WithTimeout(ctx, addressProbeTimeout)
	defer cancel()
	output, err := guest.client.Run(probe, instance.Name, []string{"ip", "-j", "address", "show"}, true)
	if err != nil {
		return ""
	}
	var interfaces []struct {
		MACAddress string `json:"address"`
		Addresses  []struct {
			Family string `json:"family"`
			Scope  string `json:"scope"`
			Local  string `json:"local"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal([]byte(output), &interfaces); err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if !sharedMACs[strings.ToLower(iface.MACAddress)] {
			continue
		}
		for _, address := range iface.Addresses {
			if address.Family != "inet" || address.Scope != "global" {
				continue
			}
			ip, err := netip.ParseAddr(address.Local)
			if err == nil && ip.Is4() {
				return ip.String()
			}
		}
	}
	return ""
}

func (guest *Guest) Shell(ctx context.Context, name string, user domain.Username, command []string) (int, error) {
	return guest.client.Shell(ctx, name, userCommand(user, command))
}
