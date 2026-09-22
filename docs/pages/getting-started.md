+++
title = "Getting started"
description = "Create a Linux VM, share your project directory, and run a command."
weight = 20
prev = "installation"
next = "guide"
+++

This walkthrough creates a VM with Git and mounts your project at `/workspace`.
[Install Limanix](installation.md) before continuing. Run the commands on your
Mac unless a step says otherwise.

## Create a configuration

Open a terminal in your project directory and run:

```console
limanix first-config
```

This writes a commented `limanix.toml` in the current directory. You can also
pass an existing directory: `limanix first-config ~/projects/my-project`.

{{< callout type="warning" >}}
`first-config` replaces an existing `limanix.toml`. Do not run it over a
configuration you want to keep.
{{< /callout >}}

For this walkthrough, replace the generated example with the following complete
configuration. It mounts the directory containing `limanix.toml`, without
requiring the placeholder directories in the generated example:

```toml
schema_version = 1
name = "example-box"
env = {}

[resources]
arch = "arm64"
cpu = 2
mem = "4GiB"
disk = "10GiB"

[nixos]
modules = ["lmx:git"]

[network.ports]
tcp = []
udp = []

[[mounts]]
source = "."
target = "/workspace"
```

On an **Intel Mac**, change `arch` to `"amd64"`. Matching your Mac's architecture
uses Apple's Virtualization.framework (VZ) and needs no QEMU installation.

The omitted user settings create a `dev` account with passwordless sudo. Its
home is mounted from a new directory under `~/.limanix` on your Mac. The project
mount is read-write by default. See the [configuration reference](/reference/configuration.md)
for every field and default.

## Create the VM

```console
limanix create --config limanix.toml
```

Limanix prepares the selected modules, creates the VM, builds its NixOS
configuration inside the guest, and reboots into it. The first creation needs
network access to download the base image and Nix dependencies.

When the command completes, inspect the VM:

```console
limanix list
```

The `NAME` is `example-box`, taken from your configuration. `STATUS` reports
Lima's VM status; `STATE` reports the Limanix operation state. A successful
creation leaves the VM `Running` and `ready`.

## Work inside the VM

```console
limanix shell example-box
```

The session starts in `/home/dev`. Inside the VM, run:

```console
cd /workspace
git --version
ls
exit
```

`/workspace` contains your host project files. Editing them inside the VM changes
the same files on your Mac. `exit` closes the shell; it does not stop the VM.

For a single command from your Mac:

```console
limanix shell example-box -- git --version
```

## Add a tool

Change the existing `[nixos]` section in `limanix.toml`:

```toml
[nixos]
modules = ["lmx:git", "lmx:neovim"]
```

Apply it, then check the installed tool:

```console
limanix update --config limanix.toml
limanix shell example-box -- nvim --version
```

An update restarts the VM. Its disk, managed home, and project files are retained.
Save your work and finish running jobs before updating.

## Stop or remove the VM

Keep the VM for later:

```console
limanix stop example-box
```

Resume it with `limanix start example-box`. If you no longer need the VM, use
`limanix delete example-box`. Deletion removes the VM disk and retains its
managed home; the project mount is not a deletion target.

## Next steps

- [Manage VMs](/guide/lifecycle.md): updates, disk growth, and deletion options.
- [Files and environment](/guide/files.md): mounts, persistent home, and services.
- [NixOS modules](/guide/modules.md): bundled tools and your own modules.
- [Examples](/guide/examples.md): complete Rust and custom-module configurations.
