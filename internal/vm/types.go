package vm

import (
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/lima"
)

// Info combines persisted ownership with live backend status for a CLI listing.
// Damaged records remain visible without hiding healthy VMs.
//
// OperationStatus describes Limanix's workflow; BackendStatus describes Lima's
// lifecycle. Nil ownership and status fields indicate unavailable source data.
// An empty Address means that no address was discovered, not necessarily that
// the VM failed.
// Error carries a state-read diagnostic or the persisted recovery message.
type Info struct {
	Name            string               `json:"name"`
	Address         string               `json:"address"`
	Home            *string              `json:"home"`
	OperationStatus *domain.Status       `json:"state"`
	Arch            *domain.Architecture `json:"arch"`
	BackendStatus   *lima.Status         `json:"status"`
	LimaName        *string              `json:"lima_name"`
	Error           *string              `json:"error"`
}
