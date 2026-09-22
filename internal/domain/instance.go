package domain

// Status describes a host-side operation, independently of the backend power state.
type Status string

const (
	// Creating records initial provisioning after local inputs have been prepared.
	Creating Status = "creating"

	// Ready records a completed create or update; the guest may be stopped.
	Ready Status = "ready"

	// Updating records a selected generation awaiting successful guest application.
	Updating Status = "updating"

	// Failed retains ownership and a recovery message after an operation failure.
	Failed Status = "error"

	// Deleting records an in-progress removal of the VM's owned resources.
	Deleting Status = "deleting"

	// Interrupted is computed for listings when an in-flight record has no writer.
	// It is never written as a persisted operation status.
	Interrupted Status = "interrupted"
)

// InFlight identifies statuses that require an active operation lock.
func (s Status) InFlight() bool {
	switch s {
	case Creating, Updating, Deleting:
		return true
	default:
		return false
	}
}

// Instance pairs immutable VM ownership with the current operation record.
type Instance struct {
	Identity   Identity
	Status     Status
	Generation string
	Error      *string
}

// BeginUpdate selects a fresh input generation and clears an earlier failure.
func (i *Instance) BeginUpdate(generation string) {
	i.Generation = generation
	i.Status = Updating
	i.Error = nil
}

// MarkReady records completion of the current create or update operation.
func (i *Instance) MarkReady() {
	i.Status = Ready
}

// MarkDeleting starts a new deletion attempt and clears an earlier failure.
func (i *Instance) MarkDeleting() {
	i.Status = Deleting
	i.Error = nil
}

// MarkFailed stores a safe recovery message, never raw guest command diagnostics.
func (i *Instance) MarkFailed(message string) {
	i.Status = Failed
	i.Error = &message
}
