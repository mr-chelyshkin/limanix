+++
title = "Sandbox examples"
weight = 25
+++

# Sandbox examples

The examples below are tracked configuration files, rendered here directly from
`examples/`. Copy one into your project directory as `limanix.toml`. Its `source
= "."` mounts that directory at `/workspace`, even when Limanix is launched
elsewhere. Use `arch = "amd64"` on an Intel Mac.

The default user is `dev`, with passwordless sudo. Limanix mounts a separate
host directory under `~/.limanix` at `/home/dev` with write access. Configuration
values omitted in these examples use the [model defaults](configuration.md).

## Rust development

{{< example "rust-service.toml" "toml" >}}

After saving the file in your Rust project:

```console
limanix create --config limanix.toml
limanix shell rust-service
```

Inside the VM:

```console
cd /workspace
cargo check
nvim .
```

For a single command from the Mac:

```console
limanix shell rust-service -- bash -lc 'cd /workspace && cargo check'
```

`CARGO_TARGET_DIR` puts Cargo's build output in the managed home. Editing source
files changes the project on your Mac. Run your service on `0.0.0.0:8080` and use
`limanix list` to find the guest IP, then open `http://<guest-ip>:8080`.
Opening a guest firewall port does not start the service or forward a Mac port.

## Import a NixOS module

Create a directory such as `modules/dev-tools` and save this file as its
`default.nix`:

{{< example "modules/dev-tools/default.nix" "nix" >}}

Register the directory:

```console
limanix modules add dev-tools ./modules/dev-tools
limanix modules list
```

The registered identifier is `third-party:dev-tools`. This configuration selects
the module and mounts the project:

{{< example "custom-module.toml" "toml" >}}

Save it as `limanix.toml`, then run:

```console
limanix create --config limanix.toml
limanix shell custom-tools -- jq --version
limanix shell custom-tools -- rg --version
```

The registry copies the whole directory, including relative imports and assets.
Keep them inside that directory; symlinks and special files are rejected.
These are trusted NixOS modules with access to configure the whole guest system.

To add this module to an existing VM, import it, append `third-party:dev-tools`
to that VM's `nixos.modules`, keep its `name` unchanged, and run:

```console
limanix update --config limanix.toml
```

To apply changes to an imported module:

```console
limanix modules remove dev-tools
limanix modules add dev-tools ./modules/dev-tools
limanix update --config limanix.toml
```

Removing the registry entry alone does not change installed packages or existing
VM generations. An update resolves the selected identifiers again. Import
the replacement before updating a configuration that still selects it.

## Mount editor configuration and Git keys

To use your host Neovim configuration and a dedicated SSH-key directory, append
these mounts to the Rust configuration. Both source directories must exist:

```toml
[[mounts]]
source = "~/.config/nvim"
target = "/home/dev/.config/nvim"
mode = "ro"

[[mounts]]
source = "~/.ssh/limanix"
target = "/mnt/git-keys"
mode = "ro"
```

Neovim reads its configuration inside the managed home. Configure SSH in the
guest to use the selected key at `/mnt/git-keys/<filename>`. Use `mode = "rw"`
if you intend to edit a mounted configuration from the guest. These mounts use
the default `user.home`; adjust the Neovim target if you choose another home.

For an existing VM, apply the new mounts with `limanix update --config limanix.toml`.

## Sessions, services, and persistent files

`[env]` values are available to guest login sessions and system and user services.
They remain literal strings; `$HOME` does not expand during configuration loading.
An update reboots the guest. Sessions and services receive the new values after
the reboot.

For example:

```console
limanix shell rust-service -- printenv CARGO_TARGET_DIR
limanix shell rust-service -- systemctl --user list-units --type=service
```

User services can continue after the shell closes while the VM is running because
the development user has systemd lingering enabled. Define their units in a
trusted NixOS module. See [environment and user access](configuration.md#environment-and-user-access).

Manual package installations are part of this VM's disk state. Files in its
managed home survive `stop` and `update`, and normal deletion retains that home.
Files in external project mounts belong to their host directories.

## Cleanup

For the Rust example:

```console
limanix stop rust-service
limanix delete rust-service
```

Deletion removes the VM disk and runtime records. It prints the retained home
path and archives its ownership in `homes/<name>-<id>.json` under the host state
directory. The current CLI has no command for removing archived homes; retained
directories can be managed separately on the host.

To delete a VM and its managed home together:

```console
limanix delete rust-service --remove-home
```

For a stuck VM, add `--force` to force Lima termination. Home removal remains
controlled independently by `--remove-home`. Project mount sources are not
deleted by these commands.
