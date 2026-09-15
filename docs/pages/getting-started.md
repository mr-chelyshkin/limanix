+++
title = "Getting started"
weight = 10
+++

# Getting started

Create a sandbox from a TOML configuration, work in it, and keep its files across
updates. Limanix runs on macOS and supports native VZ guests and
foreign-architecture QEMU guests.

## Prerequisites

Use macOS 13 or newer for native VZ guests. Limanix includes Lima's driver and
host-agent support, together with Linux guest agents for both supported
architectures. Sessions use the system SSH client. Host Lima, Nix, and Python
are not required; Nix builds run in the guest.

Release builds provide `limanix-darwin-arm64` for Apple Silicon and
`limanix-darwin-amd64` for Intel Macs. Install the matching artifact from
[GitHub Releases](https://github.com/mr-chelyshkin/limanix/releases) when available:

```console
mkdir -p ~/.local/bin
install -m 755 limanix-darwin-arm64 ~/.local/bin/limanix
```

For Intel Macs, use `limanix-darwin-amd64`. Add `~/.local/bin` to `PATH` if needed.

Release binaries are ad-hoc signed with the entitlements needed by Lima's VZ
driver. They are not Developer ID signed or notarized by Apple. Gatekeeper may
block a binary downloaded through a browser.

If macOS reports that the developer cannot be verified, try running
`limanix --help`, then follow Apple's instructions to allow that application in
**System Settings → Privacy & Security → Open Anyway**:
[Safely open apps on your Mac](https://support.apple.com/en-us/102445).

For a CLI binary that you downloaded from this project's release and trust, you
can instead remove the quarantine attribute from that file only, if present:

```console
xattr -d com.apple.quarantine ~/.local/bin/limanix
~/.local/bin/limanix --help
```

This removes the download quarantine marker; it does not notarize the binary or
verify its publisher.

VM commands require the native binary for your Mac. Running the Intel binary
under Rosetta on Apple Silicon is not supported; use `limanix-darwin-arm64`.

For a source checkout on macOS, install Task and the Xcode command-line tools,
and use the Go toolchain in `go.mod`:

```console
task --yes ci/build
./bin/limanix-darwin-arm64 --help
```

This builds Linux guest agents from the pinned Lima dependency, embeds their
compressed archives, and builds and signs both macOS binaries. Install the
matching binary as `limanix`, or prefix the commands below with
`./bin/limanix-darwin-arm64` on Apple Silicon and
`./bin/limanix-darwin-amd64` on Intel.

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
| `resources.arch` | Guest architecture: `arm64` or `amd64`. |
| `nixos.modules` | Bundled and imported NixOS modules. |
| `mounts` | Local folders (`source`) and their paths in the VM (`target`). |
| `home.root` | Host directory containing VM home directories. |
| `env` | Variables for login sessions and system and user services. |
| `network.ports` | Guest firewall ports for your services. |

The generated configuration defaults to `arm64`. For a native guest on an Intel
Mac, change `resources.arch` to `amd64`. Matching the host architecture uses VZ;
the other architecture requires the QEMU prerequisites described above.

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

See the [configuration reference](configuration.md) for all fields and defaults,
or start from the [development examples](examples.md).

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

To add tools to an existing VM, add their identifiers to `nixos.modules` and run
the same update command. Import a third-party module with `limanix modules add`
before selecting it. Removing an identifier from the config and updating applies
the new module selection too.

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

Retained homes have an ownership archive at `homes/<name>-<id>.json` under the
Limanix state directory described in [VM state](configuration.md#home-mounts-and-vm-state).
The current CLI has no separate command to remove a home after its VM was deleted.

External mount source directories outside the managed home are preserved in both
cases. See the [CLI reference](cli.md) for all command options.

If an operation fails or listing shows an interrupted or damaged record, see
[Troubleshooting](troubleshooting.md) for the existing recovery commands.
