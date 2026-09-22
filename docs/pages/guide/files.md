+++
title = "Files and environment"
description = "Share your project, choose read-only mounts, and configure guest-wide environment variables."
weight = 20
+++

Use mounts for files you want to keep on your Mac, and `[env]` for variables
needed by guest sessions and services. Apply changes with
`limanix update --config limanix.toml`.

## Mount your project

Add a mount to your configuration:

```toml
[[mounts]]
source = "."
target = "/workspace"
```

`source` is a host directory. Relative paths resolve from the configuration
file's directory, not from the directory where you launch Limanix. In this
example, `/workspace` contains the project beside `limanix.toml`.

Inside the VM:

```console
cd /workspace
```

Mounts default to `mode = "rw"`. File changes, including deletions, affect the
host directory directly; this is not a copy or a sync job.

### Mount a read-only directory

For example, share your existing Neovim configuration without allowing the guest
to edit that mount:

```toml
[[mounts]]
source = "~/.config/nvim"
target = "/home/dev/.config/nvim"
mode = "ro"
```

Create the source directory on your Mac before creating or updating the VM.
Adjust the target if your configuration uses a different `user.home`.

Host paths support `~` and spaces. `$HOME` and other environment references stay
literal. Guest mount destinations must be absolute and cannot contain spaces,
tabs, or line breaks. Destinations cannot overlap other explicit mounts, hide
the managed home, or cover reserved system paths. See the
[configuration rules](/reference/configuration.md#validation-rules) for the full list.

## Keep a persistent home

The guest user's home is backed by a dedicated host directory:

```text
Mac: ~/.limanix/<name>-<id>  →  Guest: /home/dev
```

`home.root` selects the host parent; `user.home` selects the guest destination.
The defaults need no directory setup with `sudo`. Limanix creates a separate
home for each VM and mounts it read-write.

Keep editor state and build caches in that home when you want them retained
after normal VM deletion. The VM disk survives stop and update, but is removed
by delete. External project mounts stay in their original host locations.

{{< callout type="warning" >}}
`delete --remove-home` removes the managed home and everything in it. A mount
whose source is inside that home is part of the same data being removed.
{{< /callout >}}

See [deletion options](lifecycle.md#delete-a-vm) before choosing how to clean up.

## Set guest environment variables

```toml
[env]
APP_ENV = "development"
RUST_LOG = "debug"
CARGO_TARGET_DIR = "/home/dev/.cache/cargo-target"
```

After applying the configuration, check a value from your Mac:

```console
limanix shell example-box -- printenv APP_ENV
```

Values are available to login sessions and system and user services. They are
literal strings: `"$HOME/cache"` does not expand to a home path. Use the intended
guest path explicitly.

Supplying `[env]` replaces the default environment map; it does not merge with
it. To configure no variables, use `env = {}` at the top level, before any
`[section]` header. Updates reboot the VM; new sessions and services then
receive the new values.

{{< callout type="warning" >}}
`[env]` is not a secret store. Its values are readable by all guest users.
Host staging files are private, and Limanix keeps these values outside the
generated Nix source, but installs readable files under `/etc/limanix`.
{{< /callout >}}

## Control guest sudo access

To remove passwordless sudo from the development user:

```toml
[user]
sudo = false
```

Keep your existing `user.name` and `user.home` if you have changed their
defaults. Limanix uses a separate `limanix-admin` account for management;
disabling development-user sudo does not prevent updates.

## Run user services

The development user has systemd lingering enabled. Its services can remain
active after a shell closes while the VM is running. Define service units in a
trusted [NixOS module](modules.md), then inspect them with:

```console
limanix shell example-box -- systemctl --user list-units --type=service
```

Limanix sessions set the runtime directory and D-Bus address used by
`systemctl --user`. Stopping or updating the VM still interrupts its services.
