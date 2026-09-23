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

`cmd/bundle-modules` downloads the `limanix_modules_version` tag from Taskfile.
It validates module entry points, metadata, archive paths, and file types,
then packages the catalog and upstream license as `internal/nixos/resources/modules.zip`.
An existing valid archive for the same repository and tag is reused without a download.

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
| Standard NixOS module catalog | `limanix_modules_version` Git tag in `Taskfile.yml`. |
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

## Update the standard modules

1. Publish a new tag in [limanix-modules](https://github.com/mr-chelyshkin/limanix-modules).
   Each module lives in `modules/<name>/default.nix`, with its description in
   `module.toml` alongside it. Names and descriptions are read from this catalog.
   Versioned modules also declare `default` and `versions` in that file, with
   entry points at `modules/<name>/versions/<version>.nix`.
2. The module release sends a `limanix-modules-release` repository dispatch with
   `client_payload.tag`. Limanix opens a separate `deps/limanix-modules/<tag>` PR
   that updates only `limanix_modules_version` in Taskfile.
3. The release bot verifies the published module release and the exact PR diff,
   then squash-merges it without running the full PR checks. The commit and PR
   link to the module release; the PR is included in generated release notes.
4. The bot creates the next patch tag on that merge commit: `v0.3.99` becomes
   `v0.3.100`. The existing tag workflow builds the binaries and documentation
   before publishing. A failed build leaves the tag but publishes no new binary.

The `release-modules.yml` workflow has two jobs: `update` merges the
catalog PR and passes its commit SHA to `tag`. Dispatches are processed one at
a time; equal or older catalog versions leave `main` unchanged.

If `tag` fails after merge, choose **Re-run failed jobs** to retry tagging the
same commit without repeating the update. An existing release tag is not moved
or duplicated. Sending another dispatch does not recover a merged update:
the catalog version is already current. If `update` fails after the PR has
merged but before its SHA was saved, inspect the merge commit before creating
its release tag manually. If the tag exists and its build fails, rerun that
tag's release workflow instead.

### Release bot setup

Install a dedicated GitHub App on `mr-chelyshkin/limanix` with **Contents: write**
and **Pull requests: write**. In Limanix's Actions settings, configure variables
`RELEASE_APP_CLIENT_ID` and `RELEASE_APP_SLUG`, plus the PEM private key as secret
`RELEASE_APP_PRIVATE_KEY`. The modules repository needs the same client ID and
private key for dispatching; it requests only Contents permission on Limanix.

Add this App to the `main` ruleset's bypass list with **For pull requests only**.
Do not grant an unconditional bypass or administrative App permissions.
Ordinary PRs keep the existing CI and protection rules. Bot-owned module branches
are reserved for this workflow; user-authored PR events still run the normal CI.

No Go change is needed to add a module that follows this contract. Changing the
catalog format or guest integration contract may require code changes.

The source is pinned by tag, not SHA. Do not move published tags: cached builds
reuse the catalog for that tag, while a fresh checkout downloads its current
contents. The ZIP comment records the source repository: archives from another
repository are not reused. ZIP integrity checks detect corruption, not upstream tag changes.

The archive is a build artifact, not a separately installed user catalog.
`ci/lint`, `ci/vuln`, `ci/test`, `ci/docs`, and `docs/preview` prepare it in the Go
container; `ci/build` prepares it natively. A direct Go build requires the
archive to have been generated first.

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
