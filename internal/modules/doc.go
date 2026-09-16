// Package modules imports trusted NixOS module trees and leases stable registry sources.
//
// Bundled modules are referenced by ID. Imported modules are copied beneath the
// state root and selected as third-party:NAME. [Registry] receives bundled
// metadata from internal/nixos; it does not embed or evaluate Nix code.
//
// # Import and consumption
//
//	original directory → CopyTree → hidden staging directory
//	                                       ↓ exclusive registry lock
//	                               modules/<name>
//	                                       ↓ shared source lease
//	                         independent copy in a VM generation
//
// [Registry.Add] copies the complete tree, preserving relative imports. The
// exclusive lock covers publication by rename, not the initial copy.
// [Registry.Sources] returns a [SourceSet] holding a shared lock: its caller must
// finish copying every selected source before calling [SourceSet.Close].
//
// [Registry.Remove] detaches an import by rename under an exclusive lock, then
// removes its files after releasing the lock. Existing VM generations retain
// their independent snapshots.
//
// # Validation and trust
//
// [ValidateDirectory] requires a real directory with a regular default.nix.
// [CopyTree] checks entries while copying, refuses symlinks and special files,
// and removes its destination on failure. A destination cannot already exist
// or be inside the source.
//
// These checks protect the file-copy contract; they are not a Nix sandbox or
// an audit of module contents. Registry locks coordinate Limanix operations,
// not unrelated processes editing the same files.
//
// Read registry.go and catalog.go for lookup, import.go and remove.go for
// publication, sources.go for lease lifetime, and tree.go for file handling.
// [Error] supplies module context while preserving its cause for errors.Is/As.
package modules
