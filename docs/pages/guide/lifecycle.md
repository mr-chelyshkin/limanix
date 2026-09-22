+++
title = "Manage VMs"
description = "Run commands, update resources, and control what survives VM deletion."
weight = 10
prev = "guide"
+++

`create` and `update` take a configuration file. Other VM commands take the
public name from that file's `name` field.

## List your VMs

```console
limanix list
limanix list --json
```

`STATUS` comes from Lima and tells you whether the VM is running. `STATE` comes
from Limanix and describes its last configuration operation. A stopped VM can
still be `ready`: its configuration was applied successfully, but it is not
currently running. Listing does not start a VM or its network.

See [State and storage](/reference/storage.md#listing-fields) for the JSON
fields, or [Troubleshooting](/troubleshooting.md) for failed and interrupted states.

## Open a shell or run a command

For an interactive session:

```console
limanix shell example-box
```

The VM must already be running. The shell uses the configured development user
and starts in that user's home, not in the host's current working directory.
Exit the session with `exit`; the VM keeps running.

To run a single command and return its exit status:

```console
limanix shell example-box -- git --version
```

Arguments after `--` are passed to the guest command. For shell syntax such as
`cd`, pipes, or `&&`, invoke a guest shell explicitly:

```console
limanix shell example-box -- bash -lc 'cd /workspace && git status'
```

The single quotes keep your Mac's shell from interpreting that expression first.

## Stop and start

```console
limanix stop example-box
limanix start example-box
```

Stopping keeps the disk and managed home. Starting boots the existing guest;
it does not apply edits to `limanix.toml`. Use `update` to apply those edits.

## Apply configuration changes

Edit your configuration, then run:

```console
limanix update --config limanix.toml
```

The `name` selects an existing VM. An update prepares a new module snapshot,
applies Lima settings, builds the guest's NixOS configuration, and reboots into
it. A stopped VM is started as part of the update. Its disk and managed home
are retained.

{{< callout type="warning" >}}
Updates interrupt running sessions and services. Save your work and finish jobs
before applying a change. A failed update is not an automatic rollback of every
change already made; use the reported error and VM state to choose recovery.
{{< /callout >}}

| Setting | Update behavior |
| --- | --- |
| CPU and memory | Can change. |
| Disk | Can grow, but cannot shrink. |
| Modules, mounts, environment, firewall ports | Replaced by the selected configuration. |
| `user.sudo` | Can change without removing Limanix's management access. |
| Architecture, `user.name`, `user.home`, `home.root` | Fixed for this VM; create another VM to change them. |
| `name` | Selects the VM; it does not rename one. |

After success, Limanix removes old configuration generations. A cleanup failure
prints a warning while leaving the VM `ready`; later successful updates retry
the remaining cleanup.

### Grow the disk

Increase `disk` in the existing `[resources]` section, for example from
`"10GiB"` to `"20GiB"`, and run `update`:

```toml
[resources]
arch = "arm64"
cpu = 2
mem = "4GiB"
disk = "20GiB"
```

Keep your existing architecture, CPU, and memory values if they differ from
this example. During boot, the guest expands its root partition and filesystem.
Inspect the filesystem from your Mac with:

```console
limanix shell example-box -- df -h /
```

## Delete a VM

```console
limanix delete example-box
```

Deletion removes the VM disk and its Limanix records, but keeps the managed
host home and prints its path. To delete that home and all its contents as well:

```console
limanix delete example-box --remove-home
```

These are alternatives, not consecutive cleanup steps: `--remove-home` must be
used while deleting the VM. There is no separate CLI command for removing a
home retained after an earlier deletion.

For an unresponsive VM, `--force` permits forced Lima termination. It does not
remove the home unless `--remove-home` is also supplied, and it does not bypass
ownership validation.

| Data | Stop or update | Delete | Delete with `--remove-home` |
| --- | --- | --- | --- |
| Guest disk | Retained | Removed | Removed |
| Managed host home | Retained | Retained | Removed |
| External mount sources outside that home | Retained | Retained | Retained |

A retained home has an ownership archive in Limanix's
[state directory](/reference/storage.md). Creating another VM with the same
name allocates a new home; it does not automatically reuse the retained one.
