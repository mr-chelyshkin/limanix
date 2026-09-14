# Getting started

Create a sandbox from a TOML configuration, work in it, and keep its files across
updates. The workflow has been verified with native `arm64` guests on macOS;
see [runtime validation](architecture.md#runtime-validation) for its scope.

## Prerequisites

Run Limanix on macOS with Python 3.14 or newer and
[Lima installed](https://lima-vm.io/docs/installation/). `limactl` must be available
in `PATH`. Nix builds run in the guest; host Nix is not required.

For a source checkout, install its environment with `uv sync --locked` and prefix
the commands below with `uv run`.

Native guests use VZ and `vzNAT` networking on macOS 13 or newer. A foreign
architecture, such as an `amd64` guest on Apple Silicon, requires QEMU and the
[socket_vmnet setup](https://lima-vm.io/docs/config/network/vmnet/#socket_vmnet).
Limanix checks these prerequisites; it does not install privileged network tools
or modify sudoers files.

## 1. Create a config

Run in any folder:

```console
limanix first-config
```

This writes `limanix.toml` with the default values, replacing an existing file.

To use another folder, run:

```console
limanix first-config ~/projects/my-project
```

## 2. Edit your configuration

Open `limanix.toml` and choose what your sandbox needs:

| Setting | What it controls |
| --- | --- |
| `nixos.modules` | Bundled and imported NixOS modules. |
| `mounts` | Local folders (`source`) and their paths in the VM (`target`). |
| `home.root` | Host directory containing VM home directories. |
| `env` | Variables for login sessions and system and user services. |
| `network.ports` | Guest firewall ports for your services. |

For Rust development with Git and Neovim, set:

```toml
[nixos]
modules = ["git", "rust", "neovim"]
```

Replace the example mount sources with existing directories and remove mounts
you do not need. Relative host paths resolve from the config file's directory;
`~` expands to your host home.

The default `home.root` is `~/.limanix`, inside your host user's home directory:

```toml
[home]
root = "~/.limanix"
```

Limanix creates a separate `<name>-<id>` directory there and mounts it as the
configured guest user's home. This default requires no directory setup with
sudo. Run Limanix as your regular host user.

See the [configuration reference](configuration.md) for all fields and defaults.

## 3. Create the sandbox

```console
limanix create --config limanix.toml
```

Creation prepares the module snapshot, creates the Lima VM, builds its NixOS
configuration inside the guest, and reboots into that configuration.

Use the configuration's `name` for subsequent commands; the generated example
uses `example-box`:

```console
limanix list
limanix shell example-box
limanix shell example-box -- rustc --version
```

`shell` opens a session as the development user. A separate `limanix-admin`
account performs management operations, including when `user.sudo` is false.

To reach a service from your Mac, run `limanix list` to find the guest address.
Configure the service to listen on a guest network interface and include its
port in `network.ports`. For example, a service listening on `0.0.0.0:8080` is
available at `http://<guest-address>:8080`. See
[Network access](configuration.md#network-access) for the network settings.

## 4. Update or stop the sandbox

After editing the configuration, apply it to the VM named by `config.name`:

```console
limanix update --config limanix.toml
```

An update keeps the guest disk and host home. For a running VM, it stops and
starts the guest to apply Lima changes, then builds NixOS and reboots again.
See [Update rules](configuration.md#updates) for fields that require a new VM.

```console
limanix stop example-box
limanix start example-box
```

## 5. Delete the sandbox

```console
limanix delete example-box
```

This removes the VM disk and its Limanix state while preserving the managed host
home. To remove that home and all its contents too, use:

```console
limanix delete example-box --remove-home
```

`--force` allows Lima to stop and delete an unresponsive VM. It preserves the
managed home unless `--remove-home` is also supplied. The two flags are independent.

External mount source directories outside the managed home are preserved in both
cases. See the [CLI reference](cli.md) for all command options.
