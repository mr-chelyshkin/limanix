+++
title = "Installation"
description = "Install Limanix on macOS and prepare QEMU for another guest architecture."
weight = 10
prev = "/"
next = "getting-started"
+++

Limanix requires **macOS 26 or newer**. Use the native binary for your Mac:

| Mac | Release binary | Native guest architecture |
| --- | --- | --- |
| Apple Silicon | `limanix-arm64` | `arm64` |
| Intel | `limanix-amd64` | `amd64` |

The binary includes Lima's VM integration, Linux guest agents for both
architectures, and the `socket_vmnet` network helper. Sessions use the system
SSH client. Host installations of Lima, Nix, and Python are not required.

## Install a release binary

Download the matching binary from
[GitHub Releases](https://github.com/mr-chelyshkin/limanix/releases). In the
directory containing the downloaded file, run this for Apple Silicon:

```console
mkdir -p ~/.local/bin
install -m 755 limanix-arm64 ~/.local/bin/limanix
~/.local/bin/limanix --help
```

For an Intel Mac, replace `limanix-arm64` with `limanix-amd64`. Add
`~/.local/bin` to your shell's `PATH` if it is not already there. The remaining
documentation assumes the binary is available as `limanix`.

{{< callout type="warning" >}}
VM commands do not support the Intel binary running under Rosetta on Apple
Silicon. Install `limanix-arm64` on those Macs.
{{< /callout >}}

### macOS download warning

Release builds are ad-hoc signed with the entitlements required by Lima's VZ
driver. They are not Developer ID signed or notarized by Apple. Gatekeeper may
block a file downloaded through your browser.

If macOS blocks it, try launching `limanix --help`, then follow
[Apple's instructions](https://support.apple.com/en-us/102445) to allow the
application in **System Settings → Privacy & Security → Open Anyway**.

For a binary you downloaded from this project's release and trust, another
option is to remove the quarantine attribute from that file only, if present:

```console
xattr -d com.apple.quarantine ~/.local/bin/limanix
~/.local/bin/limanix --help
```

This removes the download quarantine marker. It does not verify the publisher
or notarize the binary.

## Choose the guest architecture

Set `resources.arch` in your TOML configuration:

```toml
[resources]
arch = "arm64"
```

When the guest architecture matches your Mac, Limanix uses **VZ** with `vzNAT`
networking. This is the path used in [Getting started](getting-started.md).

A different guest architecture uses **QEMU**. For example, an `amd64` guest on
Apple Silicon needs the additional setup below. The generated configuration
defaults to `arm64`; it does not detect your Mac's architecture.

### Prepare QEMU

Lima provides the QEMU driver, but not the QEMU executable. Install QEMU
separately and make it available in `PATH`. With Homebrew:

```console
brew install qemu
```

Use a QEMU build compatible with your macOS version. Then prepare its shared
network:

```console
limanix network setup
```

Run this as your regular user in an interactive terminal. Review the displayed
paths and group, confirm installation, and enter your administrator password
when `sudo` requests it. Limanix and QEMU continue running as your regular user;
only the network helper is privileged.

The first QEMU operation can also offer this setup. Once it matches Lima's
configuration, later operations reuse it without a setup prompt. See
[QEMU networking](/guide/networking.md#qemu-network-setup) for installed files,
existing Lima configurations, and shared-helper limitations.

## Build from source

On macOS, install Task and the Xcode command-line tools, and use the Go toolchain
declared in `go.mod`. From a source checkout:

```console
task --yes ci/build
./bin/limanix-arm64 --help
```

Use `./bin/limanix-amd64` on Intel. The task prepares the embedded agents and
helper, builds both macOS binaries, and ad-hoc signs them. It does not require
Docker. See [Building Limanix](/development/builds.md) for build inputs and
dependency updates.

With Limanix installed, continue to [Getting started](getting-started.md).
