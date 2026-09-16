// Package vm coordinates VM lifecycle operations across host services.
//
// [Manager] orders effects; it does not implement the Lima backend, filesystem
// records, or guest provisioning commands. [Dependencies] makes those boundaries
// explicit. Use [New]; missing or typed-nil services are programming errors,
// while operations return input, host, and backend failures.
//
// # Create and update
//
//	Create                              Update
//	config + preflight                  config + preflight
//	        ↓                                   ↓
//	VM lock                             VM lock
//	        ↓                                   ↓
//	generation + home                   identity + disk checks
//	        ↓                                   ↓
//	save creating                       prepare generation → save updating
//	        ↓                                   ↓
//	Lima create                         stop if running → edit
//	        ↓                                   ↓
//	start → guest apply                 start → guest apply
//	        ↓                                   ↓
//	save ready                          save ready → prune old inputs
//
// Create and Update load configuration and run preflight before taking the
// VM lock. Update checks saved identity and backend disk size under that lock,
// then rechecks the live backend before editing. It preserves identity and home
// and refuses a disk shrink or an unknown current disk size.
//
// A generation materializes configuration as a Lima template, NixOS flake,
// selected module snapshots, and runtime ENV files. Module-source leases are
// held while those snapshots are copied.
//
// # Failure and deletion boundaries
//
// Creation rolls back locally prepared resources before backend creation when
// possible. Once backend operations have begun, failures retain ownership and
// inputs for recovery. Update discards uncommitted inputs but preserves a
// generation that may already be referenced by state. Old inputs are pruned
// only after a successful ready record.
//
// Delete acquires the VM lock and reads identity independently of runtime state.
// It removes the backend first, then either preserves home ownership or removes
// the exact managed home, and finally removes VM records. Force and home removal
// are separate choices; force does not imply deleting the home.
//
// # Queries and entry points
//
// [Manager.FetchAll] combines state records with one backend listing and bounded
// parallel address probes, preserving row order and damaged records. [Manager.Shell]
// does not acquire the exclusive operation lock. Lima power state and Limanix
// operation state remain separate in [Info].
//
// Read create.go, update.go, and delete.go for scenarios; generation.go for local
// inputs and cleanup; lifecycle.go for start, stop, and shell; and list.go for
// the read model. Error causes are in error.go; runtime diagnostic details are
// returned to callers rather than copied into persisted recovery messages.
package vm
