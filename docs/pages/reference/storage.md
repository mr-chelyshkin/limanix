+++
title = "State and storage"
description = "Where Limanix keeps files and how to interpret VM state."
weight = 30
next = "troubleshooting"
+++

Limanix keeps guest user files, management records, and Lima disks separately.
These are the default macOS locations:

| Location | Contents |
| --- | --- |
| `~/.limanix/<name>-<id>/` | Managed host home, mounted at the guest's `user.home`. |
| `~/Library/Application Support/Limanix/instances/<name>/` | Identity, runtime record, and configuration generations. |
| `~/Library/Application Support/Limanix/modules/<name>/` | Imported NixOS module trees. |
| `~/Library/Application Support/Limanix/homes/<name>-<id>.json` | Ownership archive for a retained home. |
| `~/Library/Application Support/Limanix/runtime/guestagents/` | Verified guest-agent cache. |
| `~/.lima/<lima-name>/` | Lima instance files and guest disk. |

Explicit mount sources remain at the host locations in your configuration.

## Host environment overrides

| Setting | Controls | Does not control |
| --- | --- | --- |
| `LIMANIX_HOME` | Limanix records, generations, module registry, and runtime cache. | The managed home or Lima's disk location. |
| `LIMA_HOME` | Lima's instance store and network configuration. | Limanix records or the managed home. |
| `home.root` in TOML | The parent directory for managed guest homes. | Either application's management records. |

Changing a storage root selects a different location; it does not migrate
existing files. Use consistent overrides across commands for the same VMs.
Guest `[env]` values do not set these host variables.

Different `LIMA_HOME` directories can share system network helpers. Read the
[network lifecycle limits](/guide/networking.md#shared-network-lifecycle) before
using more than one home.

## Identity and runtime records

`identity.json` records the immutable VM identity, generated Lima name, guest
user, and owned host home. `instance.json` holds the mutable operation state
and configuration-generation metadata.

An intact identity can still support deletion when the runtime record is
damaged. A damaged identity cannot be replaced by the user configuration or
bypassed with `--force`. Files inside the guest home are not proof of ownership.

For an unfinished `creating`, `updating`, or `deleting` record, listing checks
whether an operation still holds the VM lock. If not, it displays `interrupted`
without rewriting the saved record.

## Listing fields

```console
limanix list --json
```

| JSON field | Meaning |
| --- | --- |
| `name` | Public VM name. |
| `lima_name` | Generated backend name, or `null` without a readable identity. |
| `arch` | Guest architecture, or `null` without a readable identity. |
| `home` | Managed host home, or `null` without a readable identity. |
| `state` | Limanix operation status, or `null` without a readable runtime record. |
| `status` | Matched Lima status, or `null` when unavailable. |
| `address` | Discovered shared IPv4 address, or an empty string. |
| `error` | Record-reading or saved operation error, or `null`. |

The human-readable table displays missing backend status as `Missing`,
unreadable runtime state as `corrupt`, and an empty address as `-`. These are
display labels, not the corresponding JSON values. A failed address probe
leaves that address empty while the remaining rows are still listed.

## Locks and retained homes

Changes to one VM use its operation lock; a concurrent operation for that VM is
rejected. The module registry allows concurrent readers and uses an exclusive
lock for imports and removals. A registry conflict waits for up to 30 seconds.

Normal VM deletion archives the managed home's ownership before removing its
VM records. Different instances with the same public name have separate home
allocations and archives. If archiving fails, the identity is retained for a
repeated deletion attempt.

See [Manage VMs](/guide/lifecycle.md#delete-a-vm) for deletion behavior and
[Troubleshooting](/troubleshooting.md) for recovery procedures.
