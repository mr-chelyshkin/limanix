package lima

import "github.com/lima-vm/lima/v2/pkg/limatype"

// Status is Lima's upstream instance status, restricted at the adapter boundary.
type Status string

const (
	// Unknown means Lima has not determined the instance's backend state.
	Unknown Status = Status(limatype.StatusUnknown)

	// Uninitialized means the Lima configuration exists but the backend instance has not been initialized.
	Uninitialized Status = Status(limatype.StatusUninitialized)

	// Installing means the backend is installing the guest, independently of Limanix provisioning.
	Installing Status = Status(limatype.StatusInstalling)

	// Broken means Lima detected an inspection error or an inconsistent backend state.
	Broken Status = Status(limatype.StatusBroken)

	// Stopped means Lima reports the instance as not running.
	Stopped Status = Status(limatype.StatusStopped)

	// Running means Lima reports the instance as running; it does not guarantee guest readiness.
	Running Status = Status(limatype.StatusRunning)
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
type Network struct {
	MACAddress string
	Shared     bool
}

// Instance is the metadata needed by Limanix's VM orchestration.
type Instance struct {
	Name     string
	Status   Status
	Disk     *int64
	Networks []Network
}
