+++
title = "Troubleshooting"
description = "Diagnose failed operations, configuration errors, and guest connectivity."
weight = 50
prev = "reference/storage"
next = "development"
+++

Start with the command's error and the current VM listing:

```console
limanix list
limanix list --json
```

`status` describes Lima's backend; `state` describes the Limanix operation.
Inspect both before choosing a recovery action. The JSON `error` field contains
record-reading or saved operation diagnostics. See
[listing fields](/reference/storage.md#listing-fields) for all values.

For more Lima diagnostics, rerun the relevant command with `--debug` before its
name, for example `limanix --debug list`.

## macOS blocks the binary

Use the native release binary for your Mac and macOS 26 or newer. On Apple
Silicon, use `limanix-arm64`, not the Intel binary under Rosetta.

A browser download can trigger Gatekeeper because releases are not Developer ID
signed or notarized. Follow the
[installation instructions](installation.md#macos-download-warning) for a
binary you trust; do not disable Gatekeeper globally.

## Configuration or mount validation fails

Check the field named by the error:

- Pass a regular UTF-8 TOML file with `--config`.
- Replace the generated example mount sources with existing host directories.
  To use no explicit mounts, put `mounts = []` before any section header.
- Resolve relative source paths from the config file's real directory, not your
  shell's working directory. `~` expands; `$HOME` remains literal.
- Use distinct guest destinations that do not hide the home or reserved system
  paths. Read-only mounts follow the same destination rules.

See [configuration rules](/reference/configuration.md#validation-rules). If the
error names the SSH socket path length, shorten the public VM name before
creation or choose a shorter Lima storage location. Moving existing VM storage
is not performed automatically by changing an environment variable.

## Create or update failed

Once backend creation begins, a failed `create` retains ownership records and
the managed home. Repeating `create` with the same name then reports existing
state. Fix the reported cause before selecting recovery.

| What the listing and records show | Recovery |
| --- | --- |
| Backend exists and both records are readable | Correct the configuration or module problem, then run `update`. |
| Backend is missing, identity is readable | Delete the saved VM, then create another one. |
| Runtime record is damaged, identity is readable | Deletion remains available through the identity record. |
| Identity is damaged | Ownership cannot be validated; there is no CLI bypass. |

### Retry with the existing backend

Keep the original name, architecture, username, guest home, and host home root:

```console
limanix update --config limanix.toml
```

Update cannot shrink the disk or recreate a missing Lima instance. A NixOS
build failure does not reboot into the failed generation, but earlier update
steps may already have changed Lima settings or runtime environment files.
Do not treat the failure as a full rollback.

### Replace a missing backend

For a configuration named `example-box`:

```console
limanix delete example-box
limanix create --config limanix.toml
```

Deletion retains the old managed home. The new VM receives a separate home
allocation; it does not automatically reuse the retained directory.

## State is interrupted

An unfinished `creating`, `updating`, or `deleting` operation is displayed as
`interrupted` when no process holds its VM operation lock. Listing does not
rewrite the record. The backend may still be `Running` or `Stopped`.

Inspect `status` and the diagnostic, then use the recovery choices above.
Starting a VM does not replay an interrupted configuration update.

## Another operation holds a lock

Limanix rejects concurrent changes to the same VM. Let the active operation
finish before retrying. Registry imports and removals also coordinate with
module readers; a conflicting registry operation waits for up to 30 seconds
before reporting a timeout.

Do not delete lock files as a recovery step while another process may still be
using them. Separate VM operation locks do not remove the shared
[network lifecycle coordination](/guide/networking.md#shared-network-lifecycle).

## QEMU network setup is required

First check that QEMU is installed and its executable is available in `PATH`.
Then run this as your regular user in an interactive terminal:

```console
limanix network setup
```

The command reports the missing or mismatched component and asks before making
privileged changes. Existing `everyone` or `admin` groups are preserved.
A mismatched sudoers file can be regenerated with confirmation and a backup.
An incompatible installed helper or custom installation paths require
administrator attention; setup does not overwrite them.

See [QEMU network setup](/guide/networking.md#qemu-network-setup). Running the
whole application with `sudo` is not the setup procedure.

## Service is unreachable

Check these conditions in order:

1. The VM is `Running`, independently of its Limanix `state`.
2. Your service is running inside the guest and listens on a guest interface,
   such as `0.0.0.0`, not only guest `127.0.0.1`.
3. Its port is listed under the correct TCP or UDP array in the applied config.
4. Your Mac connects to the listed **guest IP**, not the Mac's `localhost`.

An empty `address` does not prove that the VM is stopped. Discovery needs a
reachable guest, its shared interface MAC, and a global IPv4 address. Its probe
has a five-second limit; failure leaves that field empty while listing continues.

For QEMU, also check whether an external Lima operation or another `LIMA_HOME`
could have stopped the shared helper. Limanix does not repair networking by
silently rebooting an already-running VM. See the
[shared-helper limitations](/guide/networking.md#shared-network-lifecycle).

## A module cannot be found or has not changed

Use `limanix modules list` to check the selected identifier. Imported modules
use `third-party:NAME`; `modules add` and `modules remove` take the unprefixed
name. Import a directory containing `default.nix` before selecting it.

Editing the original module directory does not change its imported copy.
Replace the registry entry, then update the VM. See
[Applying module changes](/guide/modules.md#apply-changes-to-an-imported-module).

## Deletion fails or data needs to be retained

`--force` permits forced Lima termination; it does not bypass ownership checks.
A readable `identity.json` is required even when `instance.json` is damaged.

Normal deletion retains the managed home. `--remove-home` removes that home
and its contents. The flags are independent; combine them only when both
operations are intended. External sources outside the managed home are not
deleted. Review [deletion behavior](/guide/lifecycle.md#delete-a-vm) before retrying.
