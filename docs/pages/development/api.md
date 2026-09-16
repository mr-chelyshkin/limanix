+++
title = "Go packages"
description = "Implementation contracts for configuration, commands, and runtime adapters."
weight = 30
url = "/api/"
+++

Limanix is a Go command-line application. Its implementation lives under
`internal/`, where Go limits imports to this repository's module. These packages
are implementation boundaries, rather than a separately published SDK.

## Configuration and domain values

`internal/config` owns the v1 TOML contract:

- `Default()` returns a configuration with independent collections.
- `Parse(data)` validates TOML and fills omitted defaults without reading host
  files. Host paths and environment values remain literal.
- `Load(path)` reads a regular UTF-8 TOML file and resolves host paths from its
  real directory, following configuration symlinks. Referenced mount directories
  do not need to exist at this stage.
- `Validate(config)` checks domain values and relationships, including mount
  reservations and overlaps, independently of the module registry.
- `Render(config)` and `RenderExample()` produce commented TOML. `RenderReference()`
  generates Markdown from the same struct fields, types, tags, and defaults.

`internal/domain` supplies shared validated names, usernames, guest paths,
module identifiers, environment values, architectures, and byte sizes.
Constructors such as `NewVMName`, `NewUsername`, and `NewGuestPath` enforce the
same rules in configuration and identity records. `GuestPath` normalizes absolute
paths without consulting the filesystem; reserved system trees are a separate
configuration policy.

`ByteSize` stores an `int64` byte count. `ParseByteSize("10GiB")` decodes a positive
whole-GiB size; `GiB()` formats it. TOML and public JSON use whole-GiB strings.
Configuration errors identify fields without printing environment values.

## CLI and generated references

`internal/cli.Command` constructs the Cobra command tree without opening the
state store or querying Lima. `Execute` dispatches commands, connects explicit
input/output streams, and returns an OS exit status. Its manager and registry
interfaces allow tests to exercise CLI behavior without a VM.

`cmd/docsgen` calls `internal/docs/generator.Generate`, which uses that command
tree and the configuration renderer to write the [CLI reference](/reference/cli.md),
[configuration reference](/reference/configuration.md), default TOML, and version metadata.
Hugo includes those generated files in the static documentation site.

## Runtime packages

| Package | Boundary |
| --- | --- |
| `internal/app` | Constructs and shares runtime services lazily. |
| `internal/vm` | Orders VM lifecycle operations, generation preparation, and persistence. |
| `internal/state` | Maintains immutable identity and mutable runtime records, preserved-home records, and locks. |
| `internal/managedhome` | Creates and removes the exact home derived from its host ownership record. |
| `internal/modules` | Imports trusted module trees and resolves source paths under registry locks. |
| `internal/lima` | Uses Lima's schema, store, driver, and lifecycle APIs directly; opens sessions with the system SSH client. |
| `internal/hostagent` | Runs the hidden Lima host-agent subprocess, its PID lease and API socket, logging, and signal-driven shutdown. |
| `internal/bundle` | Verifies and caches Linux guest agents; decodes embedded macOS helper archives. |
| `internal/vmnet` | Checks Lima networking and runs confirmed privileged setup. |
| `internal/nixos` | Embeds NixOS sources and bundled modules; copies selected modules into a generation. |
| `internal/guest` | Applies guest configuration, starts development-user sessions, and discovers the guest address. |
| `internal/filesystem` | Checks host paths and writes private, durable atomic files. |
| `internal/buildinfo` | Provides release metadata and validates the configured macOS baseline. |

See [Architecture](architecture.md) for operation ordering and ownership.
