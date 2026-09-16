// Package bundle supplies the embedded Linux guest agents and macOS network helper.
//
// Lima's startup API needs a file path, but the agents are stored inside the Limanix executable.
// [Cache] selects and validates the compressed payload, then makes it available as a private host file.
// Startup does not install or download an external guest agent.
//
// [SocketVMNet] selects a pinned upstream release by host architecture, verifies its archive, Mach-O architecture
// and deployment target, and returns executable/license bytes. Unlike
// guest agents, the network helper is never installed in a user cache: package
// vmnet owns its administrator-approved installation into a protected host path.
// cmd/bundle-socketvmnet and the vmnetgen subpackage prepare the release archives.
// Taskfile's socket_vmnet map owns the version, digests and sizes. Its shared
// linker flags supply the generator and application; [SocketVMNetTargets] checks
// these inputs and derives filenames. Updating only a version cannot validate
// an archive against the previous release's digest.
//
// # Build-time and runtime paths
//
//	pinned Lima dependency
//	        ↓
//	cmd/bundle-guestagent → generator → resources/*.gz + manifest.json
//	                                         ↓
//	                                  embedded resources
//	                                         ↓
//	                                   Limanix binary
//
//	embedded gzip → Cache.Path(ctx, guestArch)
//	                       ↓
//	           <root>/<sha256>/<archive>.gz → Lima StartWithPaths
//
// [New] performs no I/O. Embedded gzip contents and ELF architecture are validated
// once per architecture in each process. The compressed bytes and their SHA-256
// digest are then shared by cache instances.
//
// Every [Cache.Path] call checks the disk copy and its 0600 permissions. It
// reuses an identical archive or writes a missing one atomically. A mismatch or
// incorrect permissions returns an error instead of silently replacing the
// existing cache entry. The digest names the compressed payload, not the ELF.
//
// Guest-agent decompression is only used for validation; Lima receives gzip. The generator
// subpackage owns compilation and manifest checks. This runtime package neither
// rebuilds agents nor reads the build manifest.
//
// Read bundle.go for the runtime entry point, archive.go for embedded-payload
// validation and process-local reuse, and cache.go for on-disk checks.
package bundle
