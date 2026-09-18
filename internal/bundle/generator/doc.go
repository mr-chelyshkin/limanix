// Package generator builds the Linux guest-agent archives embedded by internal/bundle.
//
// [Generate] is the implementation behind cmd/bundle-guestagent.
// It runs at development/build time, not when a user starts a VM.
//
// # Build and reuse pipeline
//
//	root module → selected Go toolchain → Lima version + per-target package graph
//	                                              ↓
//	                                  expected build metadata
//	                                              ↓
//	                               check manifest + gzip + ELF
//	                                 ├→ matching: reuse assets
//	                                 └→ mismatch: log reason and rebuild
//	                                                     ↓
//	                                temporary binaries → validate → gzip
//	                                                     ↓
//	                                 publish archives → publish manifest last
//
// Lima must be a versioned, unreplaced module. Expected dependencies come from the guest-agent package graph, not a digest
// of the application's whole go.mod. Compiler discovery may select a Go toolchain; subsequent commands pin its executable,
// GOROOT, and GOTOOLCHAIN=local. Build environment overrides are filtered explicitly in environment.go.
//
// Both Linux arm64 and amd64 executables are checked for ELF architecture and absence of PT_INTERP.
// Go version, dependencies, and build settings are read from each executable's build information and compared with the plan.
// An unverifiable source checksum prevents asset reuse.
//
// # Publication and diagnostics
//
// Builds use a temporary directory outside embedded resources. All targets are built and validated before publication begins.
// Each archive is replaced atomically and the manifest is written last; this is not a transaction across all files.
// Run generation before compiling the embedding application.
//
// A reuse failure is reported through the supplied diagnostic writer and triggers rebuilding.
// A build or publication failure returns an error.
// Runtime cache validation belongs to the parent bundle package.
//
// Read generate.go for ordering, toolchain.go and environment.go for compiler selection, source.go and plan.go for
// expected inputs, executable.go and metadata.go for validation, and manifest.go for reuse and publication.
package generator
