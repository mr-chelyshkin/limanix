package lima

import "github.com/lima-vm/lima/v2/pkg/limatype"

// Status is Lima's upstream instance status, restricted at the adapter boundary.
type Status string

// Recognized backend statuses mirror Lima's lifecycle, not domain.Status.
// Unknown is an upstream value; an unrecognized string is rejected separately.
const (
	Unknown       Status = Status(limatype.StatusUnknown)
	Uninitialized Status = Status(limatype.StatusUninitialized)
	Installing    Status = Status(limatype.StatusInstalling)
	Broken        Status = Status(limatype.StatusBroken)
	Stopped       Status = Status(limatype.StatusStopped)
	Running       Status = Status(limatype.StatusRunning)
)

func (status Status) valid() bool {
	switch status {
	case Unknown, Uninitialized, Installing, Broken, Stopped, Running:
		return true
	default:
		return false
	}
}

// Network identifies a guest interface without exposing arbitrary Lima configuration.
// Address discovery uses MACAddress to match the interface inside the guest
// and Shared to distinguish a host-reachable shared network from other links.
type Network struct {
	MACAddress string
	Shared     bool
}

// Instance is the metadata needed by Limanix's VM orchestration.
// Name is the generated backend name, not the public VM name. Disk is measured
// in bytes; nil means unavailable metadata, distinct from a reported zero.
// Networks supplies interface identities for best-effort address discovery.
type Instance struct {
	Name     string
	Status   Status
	Disk     *int64
	Networks []Network
}
