# Packaged Lima resources

`go run ./cmd/bundle-socketvmnet` downloads the pinned macOS helper release for
arm64 and amd64. Both original tar.gz archives are embedded with their upstream
license. Packaging verifies their pinned SHA-256 digests, sizes, Mach-O architecture,
macOS deployment target, and system-library dependencies. The supported host
minimum is macOS 26. Runtime installation needs administrator approval.

`go run ./cmd/bundle-guestagent` builds the pinned Lima guest agent for Linux arm64 and amd64, 
then writes deterministic gzip archives and an integrity manifest here. Packaged archives and
the manifest are build inputs supplied before CI and release builds. They are not tracked in Git.

The guest-agent command is declared as a Go tool in `go.mod`, dependency updates retain its Linux-specific 
package graph and the generated executables use the same pinned modules as Limanix.

The generator uses one resolved Go compiler with `GOENV=off`, `GOWORK=off`, empty `GOFLAGS` and `GOEXPERIMENT`, 
CGO disabled, and fixed CPU baselines. Explicit cache, proxy, and authentication environment settings are preserved.

This file keeps the embedded resource directory present in a clean checkout. The directory embed excludes dot-prefixed 
leftovers from interrupted writes. VM creation requires the generated archives.
