package guest

import (
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"time"

	"github.com/mr-chelyshkin/limanix/internal/lima"
)

const addressProbeTimeout = 5 * time.Second

// interfaceAddresses is the subset of "ip -j address show" needed to find the
// shared interface. Interface names are intentionally not assumed.
type interfaceAddresses struct {
	MACAddress string             `json:"address"`
	Addresses  []interfaceAddress `json:"addr_info"`
}

type interfaceAddress struct {
	Family string `json:"family"`
	Scope  string `json:"scope"`
	Local  string `json:"local"`
}

// Address selects a global IPv4 address by the configured shared interface MAC.
// An unavailable probe returns an empty address without failing VM listing.
func (guest *Guest) Address(ctx context.Context, instance lima.Instance) string {
	if instance.Status != lima.Running {
		return ""
	}

	shared := sharedMACs(instance.Networks)
	if len(shared) == 0 {
		return ""
	}

	probe, cancel := context.WithTimeout(ctx, addressProbeTimeout)
	defer cancel()

	output, err := guest.client.Run(probe, instance.Name, []string{"ip", "-j", "address", "show"}, true)
	if err != nil {
		return ""
	}

	var interfaces []interfaceAddresses

	if err := json.Unmarshal([]byte(output), &interfaces); err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if !shared[strings.ToLower(iface.MACAddress)] {
			continue
		}

		if address := iface.globalIPv4(); address != "" {
			return address
		}
	}

	return ""
}

func sharedMACs(networks []lima.Network) map[string]bool {
	result := make(map[string]bool, len(networks))

	for _, network := range networks {
		if network.Shared && network.MACAddress != "" {
			result[strings.ToLower(network.MACAddress)] = true
		}
	}

	return result
}

func (iface interfaceAddresses) globalIPv4() string {
	for _, address := range iface.Addresses {
		if address.Family != "inet" || address.Scope != "global" {
			continue
		}

		ip, err := netip.ParseAddr(address.Local)
		if err == nil && ip.Is4() {
			return ip.String()
		}
	}

	return ""
}
