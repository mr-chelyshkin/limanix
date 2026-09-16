+++
title = "Source architecture"
weight = 50
+++

# Source architecture

Limanix separates its command-line interface, configuration handling, VM
lifecycle, host state, and Lima/NixOS integration into Go packages.

```text
cmd/
├── limanix/             Application entry point
├── bundle-guestagent/   Guest-agent generator entry point
├── bundle-socketvmnet/  Network-helper packaging entry point
└── docsgen/             Documentation generator entry point
internal/
├── buildinfo/           Release identity and macOS compatibility baseline
├── bundle/              Embedded agents, macOS helper, and guest-agent cache
│   ├── generator/       Build-time guest-agent generation and verification
│   └── vmnetgen/        Pinned network-helper download and verification
├── cli/                 Cobra commands and output
├── config/              TOML model, validation, and rendering
├── docs/
│   └── generator/       CLI/configuration references, example, and version metadata
├── domain/              Shared validated values
├── filesystem/          Checked host paths and atomic writes
├── guest/               Guest configuration, sessions, and addresses
├── hostagent/           Persistent Lima subprocess and API socket
├── lima/                Lima schema, validation, and host operations
├── managedhome/         Managed host home ownership
├── modules/             Trusted third-party module registry
├── nixos/               Embedded sources and generation assembly
│   └── resources/
│       ├── base/
│       └── modules/
├── state/               Identity/runtime records and operation locks
├── vm/                  Lifecycle orchestration
└── vmnet/               Confirmed privileged setup of Lima networking
```

## Dependency boundaries

- `domain` defines values used by configuration and identity records.
- `config.Parse` validates TOML without filesystem access. `config.Load` reads a
  regular file and resolves host paths relative to its real location. Rendering
  produces TOML and Markdown without reading or writing files.
- `filesystem` checks paths and performs atomic writes independently of the CLI
  and configuration model.
- `state` stores identity, runtime, and preserved-home records independently of
  the user configuration parser. It owns per-VM locks and module-registry locks.
- `modules` imports trusted source trees and resolves selected paths. `nixos`
  assembles guest generations and embeds bundled modules and base NixOS sources.
- `lima` translates Limanix configuration into Lima's Go schema, validates it
  using Lima's loader, and enumerates instance names through Lima's store API.
  It registers the upstream VZ and QEMU drivers and calls Lima's lifecycle APIs
  directly. Lima starts the same Limanix executable as its persistent host-agent
  subprocess. Sessions use the system SSH client and Lima's SSH configuration.
- `hostagent` implements that hidden subprocess command. It owns its PID lease,
  API socket, synchronized JSON logging, and signal-driven shutdown.
- `bundle` supplies compressed Linux agents built from the pinned Lima
  dependency. It verifies their contents and caches the selected architecture
  under the host state directory before VM startup.
- `bundle/generator` builds and verifies those assets before the application is
  compiled. Its `Generate` function is called by `cmd/bundle-guestagent`; the
  runtime `bundle` package does not depend on the generator.
- `bundle/vmnetgen` verifies and packages the pinned upstream macOS helper release.
  `bundle` decodes the selected host executable and its license in memory.
- `vmnet` checks Lima's network setup and requests administrator approval when
  needed. Its short-lived installer uses embedded bytes, protected paths, Lima's
  sudoers generator, and `visudo`. Lima retains daemon and socket lifecycle ownership.
- `guest` applies guest configuration, opens development-user sessions, and
  discovers shared-network addresses.
- `vm` orders those operations and decides when to persist records or remove
  managed storage. Its backend interface allows lifecycle tests without Lima.
- `cli` defines Cobra commands and displays results and errors. Dependencies are
  constructed lazily; help and reference generation do not open host state.

Release binaries target macOS 26 or newer on `amd64` and `arm64`. They include the
VZ driver; building them requires CGO and Apple's SDK. The native build task signs
them with Lima's virtualization and network entitlements. QEMU remains an external prerequisite for foreign
architectures. The `socket_vmnet` helper is embedded and installed through an
explicitly confirmed privileged setup. NixOS files, bundled modules, compressed
Linux guest agents and original socket_vmnet release archives use `go:embed`.

Helper validation includes its Mach-O deployment target: an upstream archive
cannot raise the host minimum silently. Existing secure helpers are checked for
host executability and remain administrator-managed. Lima owns their network
lifecycle; Limanix adds a per-home lock around its own start/stop/delete calls,
not a persistent network service. See the
[shared-network limitations](getting-started.md#shared-network-lifecycle) when
using external `limactl` or multiple Lima homes.

`internal/bundle/generator` builds Linux `amd64` and `arm64` agents with CGO disabled and
deterministic gzip compression. Its manifest records the Lima dependency,
compiler and linked modules read from each executable, build settings, command
flags, and archive hashes. Before reuse, these are compared with the current
agent dependency graph and selected compiler. The generator also checks ELF
architecture and rejects binaries requiring a dynamic loader. Raw executables
are built in a system temporary directory outside the embedded resources.

See [Go packages](api.md) for the configuration and CLI surfaces.

## Runtime ownership

The default macOS storage locations separate user files from management records:

| Location | Owner and contents |
| --- | --- |
| `~/.limanix/<name>-<id>/` | Managed host home, mounted read-write as the guest user's home. |
| `~/Library/Application Support/Limanix/instances/<name>/` | Immutable identity, mutable runtime record, and configuration generations. |
| `~/Library/Application Support/Limanix/modules/<name>/` | Imported third-party module trees. |
| `~/Library/Application Support/Limanix/homes/<name>-<id>.json` | Ownership archives for retained homes. |
| `~/Library/Application Support/Limanix/runtime/guestagents/` | Verified, content-addressed guest-agent cache. |
| `~/.lima/<lima-name>/` | Lima-managed instance files and VM disk. |

`home.root` overrides the managed-home parent. `LIMANIX_HOME` overrides Limanix's
state directory; Lima manages its own storage location separately. Explicit mount
sources remain at the locations supplied by the user configuration.

The host-only identity record connects a public VM name to its generated Lima
identity and managed home. `Identity` derives and validates the managed-home
allocation. Configuration and identity share `Username` and `GuestPath`
validation. A separate mutable record stores lifecycle status and the current
generation. Neither record depends on the configuration parser.

Before allocating a VM's home or records, Lima preflight validates the generated
instance identifier and the longest SSH socket path using Lima's own byte limit.
This includes SSH's temporary suffix and the resolved Lima storage directory;
it does not change the configuration name's 1-to-63-character domain rule.
Host architecture detection accounts for Rosetta, and VM commands reject a
translated Intel process on Apple Silicon in favor of the native ARM64 binary.

Generations contain a self-contained flake and selected module copies.
Environment files are staged beside the flake, keeping their values out of its
Nix store source.

Listing checks the VM operation lock to distinguish an operation still running
from one whose process exited without saving its result. An abandoned `creating`,
`updating`, or `deleting` status is displayed as `interrupted`; persisted records
remain unchanged. Damaged records appear with their own errors, preserving
visibility of healthy VMs.

Guest-address discovery uses a five-second child context. A failed probe leaves
that VM's address empty and listing continues. SSH connections use
`ConnectTimeout=10`; management commands and NixOS rebuilds have no added overall
execution deadline.

For a running VM, an update stops and starts Lima to apply resource and mount
changes, installs environment files, runs `nixos-rebuild boot`, and reboots into
the new system. The guest disk and host home persist across updates. Lima status
is read again after preparing the new generation, before stopping the VM and
applying changes.

The registry holds a shared lock while `nixos` copies selected source trees once
into the generation. Nix builds run after the lock has been released. Registry
readers can run concurrently; imports and removals use an exclusive lock. A
module import prepares its copy outside that lock and commits it under the
exclusive lock. Removal detaches the registry entry under the exclusive lock
before cleaning up its files.

Lifecycle changes use the VM's own exclusive lock and reject a concurrent
operation for that VM. Registry lock conflicts wait for up to 30 seconds.
A successful update attempts to prune all generations except the current one.
Cleanup failures produce warnings while the completed update remains successful.

Management commands use `limanix-admin`; interactive sessions switch to the
configured development user. Management access remains independent of the
user's configurable sudo permission.

Deletion targets the recorded Lima instance and Limanix state. `--force` permits
forced Lima termination. The independent `--remove-home` flag removes the exact
owned home and checks against redirected paths. External mount sources are not
deletion targets.

When the home is retained, its immutable identity is archived separately before
VM state is removed. A failed archive write retains the VM identity for retry.
Removing the home also clears a matching archive from a previous attempt.

## Validation boundaries

The Go tests exercise configuration/domain validation, embedded NixOS sources,
Lima translation and typed records, host ownership, locking, CLI output, and
lifecycle ordering with injected backends. The documentation task generates the
default example and references from Go sources, then builds the Hugo site.

The project's workflows run containerized source checks on Ubuntu through
`mr-chelyshkin/actions/invoke-taskfile@v1` and run `ci/build` directly on
`macos-26`. Each checkout generates or verifies its Linux guest-agent assets and
the pinned macOS network-helper archives.
The native build scans Darwin sources, then builds and signs both release
architectures with CGO enabled and Apple's installed SDK.
Guest boot, mounts, NixOS rebuilds, and host-to-guest networking are separate
runtime checks on a local Mac.
