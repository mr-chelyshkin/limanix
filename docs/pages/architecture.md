# Source architecture

The Python package separates the command-line interface, configuration handling,
VM lifecycle, host state, and Lima/NixOS integration.

```text
src/limanix/
├── __init__.py
├── __main__.py
├── cli/
│   ├── __init__.py
│   ├── app.py
│   └── parser.py
├── config/
│   ├── __init__.py
│   ├── models.py
│   ├── parser.py
│   ├── template.py
│   └── files.py
├── lima/
│   ├── __init__.py
│   ├── client.py
│   ├── models.py
│   └── template.py
├── nixos/
│   ├── __init__.py
│   ├── bundle.py
│   └── resources/
│       ├── base/
│       └── modules/
├── domain.py
├── filesystem.py
├── guest.py
├── managed_home.py
├── modules.py
├── state.py
└── vm.py
```

## Responsibilities

| Module | Responsibility |
| --- | --- |
| `__main__` | Entry point for `python -m limanix`. |
| `cli.parser` | Defines commands and arguments with `argparse`. |
| `cli.app` | Dispatches commands and displays results and operation errors. |
| `config.models` | Defines configuration fields, defaults, and documentation. |
| `config.parser` | Validates TOML data and resolves host paths at the file boundary. |
| `config.template` | Renders a configuration model as commented TOML. |
| `config.files` | Combines rendering and atomic writing of `limanix.toml`. |
| `domain` | Shared validated names, ENV values, architecture, and byte sizes. |
| `guest` | Runs guest configuration and user commands, and discovers the shared IP. |
| `managed_home` | Allocates and removes the exact home recorded in host ownership metadata. |
| `filesystem` | Checks local paths and performs atomic text writes. |
| `modules` | Imports trusted module trees, maintains the catalog, and provides selected source paths. |
| `state` | Stores identity, runtime, and preserved-home records with per-VM/registry locks. |
| `lima.client` | Invokes `limactl`, parses typed instance records, and reports failures. |
| `lima.models` | Defines Lima status, instance, and network types. |
| `lima.template` | Translates the validated configuration into Lima JSON/YAML. |
| `nixos.bundle` | Stages Nix sources and runtime files, copying selected modules once into the VM generation. |
| `vm` | Coordinates VM creation, updates, deletion, and managed host storage. |

The `cli` package exports `main` and `build_parser`. The `config` package exports
the configuration models and loading functions. These are the public imports in
the [Python API](api.md).

## Dependency boundaries

- `config.models` combines shared `domain` types with standard-library dataclasses.
- `parse_config` validates decoded data without filesystem access. `load_config`
  reads TOML and resolves host paths relative to its location.
- `config.template` works with models and TOML; it does not read or write files.
- `config.files` coordinates rendering and filesystem operations without CLI
  output or argument parsing.
- `filesystem` has no dependency on configuration or CLI code. It reports failures
  through `FilesystemError` for its callers to handle.
- `lima` handles host VM commands and Lima configuration. `nixos` prepares guest
  configuration; Nix builds run inside the VM. Lima instance records without the
  `limanix-` name prefix are filtered out before validating statuses and networks.
- `vm` owns operation ordering and persistence. The backends do not decide when
  to remove VM state or managed host storage.
- `cli.parser` defines the interface; `cli.app` calls the registry and VM manager,
  translating their results into terminal output and exit status.

## Runtime ownership

The host-only identity record connects a public VM name to its generated Lima
identity and managed home. `Identity` derives and validates the managed-home
allocation; configuration and identity share `Username` and `GuestPath` validation.
A separate mutable record stores lifecycle status and
current generation. Neither record depends on the configuration parser.
Generations contain a self-contained flake and module copies. Environment files
are staged beside the flake, keeping their values out of its Nix store source.

Listing uses the VM operation lock to distinguish a running operation from one
whose process exited without saving its result. An abandoned `creating`,
`updating`, or `deleting` status is displayed as `interrupted`; persisted records
remain unchanged.

For a running VM, an update stops and starts Lima to apply resource and mount
changes, installs the environment files, runs `nixos-rebuild boot`, and reboots
into the new system. The guest disk and host home persist across updates.
Lima status is read again after preparing the new generation, before stopping
the VM and applying the changes.

`ModuleRegistry` resolves selected source paths and holds a shared registry lock
while `nixos.bundle` copies their trees once into the VM generation. Nix builds
run after that lock has been released. Registry readers can run concurrently;
imports and removals use an exclusive lock.
A successful update attempts to prune all generations except the current one.
Cleanup failures produce warnings and leave the completed update successful.

Management commands use `limanix-admin`; interactive sessions switch to the
development user. This separates management access from the user's configurable
sudo permission.

Deletion targets the recorded Lima instance and Limanix state. `--force` permits
forced Lima termination. The independent `--remove-home` flag removes the home
using its host ownership record and checks against redirected paths.
External mount source directories are not deletion targets.
When the home is retained, its immutable identity is archived separately before
the VM state is removed. A failed archive write retains the VM record for retry;
removing the home also clears any matching archive left by a previous attempt.

## Runtime validation

Runtime checks used Lima 2.2.0 on macOS with a native `arm64` VZ guest, 2 CPUs,
4 GiB of memory, and a 10 GiB disk. They covered VM creation and boot, updates,
development-user sessions, read-write and read-only mounts, and file persistence
across updates. The bundled Git, Rust, and Neovim modules were installed; a Rust
program was compiled, and the Mac reached a guest HTTP service by its direct IP.

Checks also covered imported module copies with relative imports, literal ENV in
login sessions and system and user services, automatic service startup after
reboot, stop/start, and updates that replace mounts. Host paths containing spaces
were mounted successfully.

Live recovery checks confirmed that a failed update retained two generations and
a successful retry kept only the current generation. After replacing
`instance.json` with `{}`, `list` still displayed the VM and `delete --force`
completed while preserving its managed home and external mount source.

Unit tests cover all combinations of `--force` and `--remove-home`, including
preservation of external mount sources. The live forced-deletion check described
above exercised preservation of the managed home.

The `amd64` NixOS configuration was evaluated. QEMU guest boot and connectivity
have not been verified by these runtime checks.
