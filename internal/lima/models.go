// Package lima translates Limanix configuration and operates Lima's native runtime.
package lima

import "github.com/lima-vm/lima/v2/pkg/limatype"

// Status is Lima's upstream instance status, restricted at the adapter boundary.
type Status string

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
