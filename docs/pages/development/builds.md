+++
title = "Building Limanix"
description = "Build native binaries and update pinned runtime dependencies."
weight = 40
+++

Build on macOS with the Go toolchain declared in `go.mod`, Task, and the Xcode
command-line tools installed:

```console
task --yes ci/build
```

The task prepares embedded assets, builds `bin/limanix-arm64` and
`bin/limanix-amd64`, ad-hoc signs both with `.github/assets/vz.entitlements`,
and verifies their signatures and Mach-O deployment target. CGO and Apple's SDK
are required; Docker is not used by this task.

`RELEASE_TAG` sets the embedded release version. Without it, the binary keeps
the development version. Run [source checks](contributing.md#check-a-change)
separately; a successful build does not imply that tests or vulnerability scans
have run.

## Embedded assets

`cmd/bundle-guestagent` builds Linux agents from the repository's pinned Lima
dependency. It verifies an existing bundle before reuse and rebuilds when its
dependencies, compiler, build settings, or contents differ.

`cmd/bundle-socketvmnet` downloads pinned upstream archives and verifies their
SHA-256, byte size, Mach-O architecture, library dependencies, and minimum macOS
version. It does not compile the helper's C source locally.

The generated archives and manifests are ignored build artifacts. `ci/test`
prepares them in the Go container; `ci/build` prepares them with the host Go
toolchain before compiling the application.

## Build inputs

| Input | Source of truth |
| --- | --- |
| Go compiler | `go.mod` and `docs/go.mod` toolchain declarations; shared Go image pin. |
| Lima integration | Go module dependency in `go.mod`. |
| macOS minimum | `macos_version` in `Taskfile.yml`. |
| socket_vmnet version, archive hashes, and byte sizes | `socket_vmnet` map in `Taskfile.yml`. |
| nixos-lima integration and image release | `internal/nixos/resources/base/flake.nix` and `flake.lock`. |
| Base-image hashes | `internal/nixos/image.go`. |

Taskfile's `build_flags` pass the macOS baseline and helper pins through linker
`-X` to the application, tests, and helper generator. `macos_version` also sets
the compiler deployment target. Go checks the complete major/minor/patch version;
a helper archive cannot silently raise the baseline.

Use the configured tasks for release builds. Direct
`go run ./cmd/bundle-socketvmnet` without those flags fails with an explicit
error. An unconfigured application can show help and version, but VM preflight
and helper installation reject missing build inputs.

## Update socket_vmnet

1. Select an [upstream release](https://github.com/lima-vm/socket_vmnet/releases).
2. Update `socket_vmnet.version` and both architectures' digests and sizes in
   Taskfile. Verify the digests against `SHA256SUMS` and the downloaded archives.
3. Run `task --yes ci/test ci/build`. The generator derives archive URLs and
   filenames from the version and rejects stale or mismatched bytes.
4. Verify QEMU startup, guest networking, and shutdown on a real Mac before
   publishing a release.

An existing secure host helper remains administrator-managed. Packaging a newer
helper does not make setup overwrite that installation.

## Change the macOS minimum

Update `macos_version`, run the packaging/build checks, and review the host
requirements in [Installation](/installation.md). Changing this value does not
make an upstream helper support an older OS; its executable must meet the chosen
baseline too.

Protocol constants remain in Go: Mach-O load commands and platform identifiers,
CPU mappings, and upstream archive naming. Their relationships are documented
in `internal/buildinfo/platform.go` and `internal/bundle/socketvmnet_*.go`.
A release that changes these contracts needs adapter changes, not just new pins.

## Update the guest image

The nixos-lima release is a guest integration dependency. Update its input in
`internal/nixos/resources/base/flake.nix`, regenerate `flake.lock`, and update
both image digests in `internal/nixos/image.go`. Go derives the image download
directory and filename from that locked release; it has no separate version
string.

Review `resources/base/platform.nix` for partition, filesystem, bootloader, and
NixOS compatibility. Verify first boot, update, and disk growth on real guests
before publishing. The same update contract is documented beside `BaseImage`.
