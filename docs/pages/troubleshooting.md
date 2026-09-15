+++
title = "Troubleshooting"
weight = 35
+++

# Troubleshooting

Start with the reported error and the independent state and backend fields:

```console
limanix list
limanix list --json
```

| JSON field | Meaning |
| --- | --- |
| `state` | Limanix operation status, or `null` when its runtime record cannot be read. |
| `status` | Matched Lima backend status, or `null` when no backend status is available. |
| `error` | A saved operation diagnostic or a record-reading error. |
| `address` | Discovered shared IPv4 address, or an empty string. |

The table displays missing backend status as `Missing` and unreadable runtime
state as `corrupt`; those are not the corresponding JSON values.

## Interrupted or failed operations

`creating`, `updating`, and `deleting` become `interrupted` in listing when the
VM's operation lock is free but the saved operation is unfinished. Listing does
not rewrite that record. This says nothing about whether Lima is `Running` or
`Stopped`; inspect `status` separately.

Once backend creation starts, a failed `create` retains ownership records and
the managed home. Running `create` again with that name reports that state already
exists. Fix the reported problem before choosing a recovery operation.

If the recorded Lima instance still exists and both records are readable, apply
the configuration with `limanix update --config limanix.toml`. Keep the original
`name`, architecture, username, guest home, and host home root. Update cannot
shrink the disk or recreate a missing backend instance.

If ownership is readable but its Lima instance is missing, delete the saved VM
and create a new one. For a configuration named `example-box`:

```console
limanix delete example-box
limanix create --config limanix.toml
```

Deletion retains the old managed home by default. The new VM receives a separate
home allocation; it does not automatically reuse the retained directory.

## Damaged records and deletion

A damaged `instance.json` appears with its own error without hiding healthy VMs.
An intact `identity.json` still allows `limanix delete NAME`; deletion does not
require a readable runtime record.

If `identity.json` is damaged, the CLI cannot validate ownership and deletion
fails. There is no identity-validation bypass. The user configuration is not a
replacement for that immutable ownership record.

`delete --force` permits forced Lima termination and preserves the home.
`delete --remove-home` removes the owned home and its contents. Combine the flags
only when both actions are intended. Neither flag deletes external mount sources;
`--force` does not bypass ownership validation. See [deletion](getting-started.md#5-delete-the-sandbox).

## Empty address or unreachable service

An empty address does not establish that a VM is stopped. Address discovery needs
a running backend, its shared interface MAC, and a valid global IPv4 result from
the guest. Its probe has a five-second limit; a failed probe leaves the address
empty while listing continues.

Inspect the backend status separately. For service access, use the guest IP,
open its port in `network.ports`, and bind the service to a guest interface, such
as `0.0.0.0`. These ports are not forwarded to the Mac's `localhost`. See
[Network access](configuration.md#network-access).

## Configuration and path errors

`--config` must identify a regular UTF-8 TOML file. Relative host paths resolve
from that file's real directory, including when the configuration is a symlink.
Mount sources must exist as directories at create or update time. `~` expands
to the host home; `$HOME` and other environment references remain literal.

Mount destinations cannot overlap explicit mounts, hide the managed home, or
cover reserved guest system trees such as `/etc/ssh` or `/nix/store`. Read-only
mounts follow the same destination rules. See [Home, mounts, and VM state](configuration.md#home-mounts-and-vm-state).

Use `--debug` before the command to enable internal Lima diagnostic logging.
