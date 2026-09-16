// Package domain defines shared values, immutable VM identity, and operation state.
//
// These types connect configuration, persistence, and runtime integrations without performing filesystem, process,
// or network operations. Host path checks here are lexical; filesystem resolution belongs to internal/filesystem.
//
// # Values and validation
//
// Constructors such as [NewVMName], [NewGuestPath], and [NewArchitecture] validate values at boundaries.
// A direct Go conversion to a named string type bypasses those constructors; callers receiving raw or
// persisted data must validate it.
//
// [ByteSize] stores bytes internally. Configuration text and JSON encode positive whole-GiB strings.
// [ModuleID] distinguishes bundled names from third-party:NAME; it does not establish that a module exists.
//
// # Identity and operation state
//
//	Identity.Name + Identity.ID → LimaName() → backend instance
//	Identity.HomeRoot + name + ID → HomePath() → exact host home
//	Identity + Status + Generation + Error → Instance
//
// [Identity] records the owner and exact home allocation. Its validation does not borrow defaults from a newer
// configuration schema. Persistence enforces immutability; the Go struct itself remains an ordinary value.
//
// [Status] describes Limanix operations, not whether Lima reports a running or stopped guest.
// [Interrupted] is a listing-time interpretation of an abandoned operation; it is not a persisted transition.
// [Instance] methods update only the value in memory and do not save records, acquire locks, or perform recovery.
//
// Read validation.go for names and paths, size.go for resource encoding,
// identity.go for ownership, and instance.go for lifecycle values.
// Package sentinel errors can be inspected with errors.Is.
package domain
