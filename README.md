# Limanix

[![License: Apache-2.0](https://img.shields.io/github/license/mr-chelyshkin/limanix?label=license)](LICENSE)

<p align="center">
  <img src=".github/assets/readme-header.png"
       alt="Limanix"
       width="800">
</p>

Linux development environments on macOS, configured with TOML and NixOS modules.

Keep your project and editor on your Mac; run tools, services, and builds inside
a Linux VM. Limanix uses Lima for virtualization and NixOS to configure the
guest. You do not need to install Lima or Nix on the host.

[Documentation](docs/pages/_index.md) ·
[Releases](https://github.com/mr-chelyshkin/limanix/releases) ·
[Module catalog](https://github.com/mr-chelyshkin/limanix-modules)

## Installation

Requires **macOS 26 or newer**. Use `limanix-arm64` on Apple Silicon and
`limanix-amd64` on Intel; VM commands do not support running under Rosetta.

From this source checkout, with the Go version declared in [go.mod](go.mod),
Task, and Xcode command-line tools installed:

```console
task --yes ci/build
mkdir -p ~/.local/bin
install -m 755 bin/limanix-arm64 ~/.local/bin/limanix
export PATH="$HOME/.local/bin:$PATH"
```

On Intel, replace `limanix-arm64` with `limanix-amd64`. Keep `~/.local/bin` in
your shell's `PATH` for future sessions. The build prepares embedded assets and
signs both binaries; Docker is not required.

A guest matching your Mac's architecture uses Apple's Virtualization.framework.
Running another architecture requires an external QEMU installation and
`limanix network setup` with administrator approval. See
[Installation](docs/pages/installation.md) for QEMU setup and macOS download
warnings; builds are ad-hoc signed, not Apple-notarized.

## Quick start

Save the following as `limanix.toml` in your project directory. It creates a
VM with Git, Node.js, and npm, and shares your project at `/workspace`.

```toml
schema_version = 1
name = "dev-box"
env = {}

[resources]
arch = "arm64"
cpu = 2
mem = "4GiB"
disk = "16GiB"

[nixos]
modules = ["lmx:git", "lmx:nodejs"]

[network.ports]
tcp = [3000]
udp = []

[[mounts]]
source = "."
target = "/workspace"
```

On Intel, set `arch = "amd64"`. Run on your Mac:

```console
limanix create --config limanix.toml
limanix shell dev-box
```

The first creation downloads the base image and Nix dependencies. Once the
shell opens, run inside the VM:

```console
cd /workspace
git --version
node --version
npm --version
```

The default guest account is `dev`, with passwordless sudo inside the VM.
Its home is stored under `~/.limanix` on your Mac. The project mount is
read-write: changes and deletions in `/workspace` affect the same host files.

To access a development server, make it listen on `0.0.0.0:3000` in the guest.
Run `limanix list` on your Mac and open `http://<ADDRESS>:3000`, using the VM's
reported address. Ports are opened in the guest firewall, not forwarded to
your Mac's `localhost`.

## Modules and updates

Standard modules come from [limanix-modules](https://github.com/mr-chelyshkin/limanix-modules)
and are embedded in the binary. Inspect its available selectors:

```console
limanix modules list
```

Edit `nixos.modules` in your configuration to change tools. Versioned selectors
such as `lmx:go-1.24` are listed alongside module defaults such as `lmx:go`.
An empty list selects no optional modules; Limanix still provides the base system.

Apply configuration changes from your Mac:

```console
limanix update --config limanix.toml
```

Updates restart the VM and preserve its disk and managed home. Finish running
jobs before updating. A failed update does not automatically roll back every
change already applied.

Use `limanix stop dev-box` and `limanix start dev-box` between sessions.
When you no longer need the VM, `limanix delete dev-box` removes its disk but
retains its managed home and leaves external project directories in place.

## Learn more

- [Configuration reference](docs/pages/reference/configuration.md) — resources,
  users, mounts, environment, and firewall ports.
- [NixOS modules](docs/pages/guide/modules.md) — version selection and importing
  your own trusted modules.
- [Manage VMs](docs/pages/guide/lifecycle.md) — lifecycle commands, disk growth,
  and deletion options.
- [Troubleshooting](docs/pages/troubleshooting.md) — connection and update failures.
- [Development](docs/pages/development/contributing.md) — contributing and running checks.
