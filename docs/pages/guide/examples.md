+++
title = "Examples"
description = "Complete configurations for Rust development and an imported NixOS module."
weight = 50
url = "/examples/"
next = "reference"
+++

These examples are rendered directly from the repository's `examples/` files.
Download one into your project directory as `limanix.toml`. Its `source = "."`
mounts that directory at `/workspace`, even when you launch Limanix elsewhere.

Use `arch = "amd64"` for a native guest on an Intel Mac. Omitted settings use the
[configuration defaults](/reference/configuration.md): the development user is
`dev`, its home is `/home/dev`, and passwordless guest sudo is enabled.

## Rust development

This configuration installs Git, Rust tools, and Neovim. It opens TCP port
8080 and keeps Cargo's build output in the managed home.

{{< example "rust-service.toml" "toml" >}}

From your Mac:

```console
limanix create --config limanix.toml
limanix shell rust-service
```

Inside the VM, in a project that contains `Cargo.toml`:

```console
cd /workspace
cargo check
nvim .
```

For a build without an interactive session:

```console
limanix shell rust-service -- bash -lc 'cd /workspace && cargo check'
```

To reach your service from the Mac, configure it to listen on `0.0.0.0:8080`,
find the guest address with `limanix list`, and use
`http://<guest-address>:8080`. Opening the firewall port does not start the
service or forward a Mac port. See [Networking](networking.md).

## Import a NixOS module

Follow [Import your own module](modules.md#import-your-own-module) to register
the example `dev-tools` directory. It installs `jq` and `ripgrep` and is selected
as `third-party:dev-tools` in this complete configuration:

{{< example "custom-module.toml" "toml" >}}

After importing the module and saving the configuration:

```console
limanix create --config limanix.toml
limanix shell custom-tools -- jq --version
limanix shell custom-tools -- rg --version
```

Editing the original module does not change the imported copy. Follow the
[module update procedure](modules.md#apply-changes-to-an-imported-module) to
replace it and rebuild the VM.

## Finish working

Stop the VM to keep it for later:

```console
limanix stop rust-service
```

Or delete it with `limanix delete rust-service`. Deletion removes the guest disk
and retains the managed home; your external project mount is not deleted.
See [deletion options](lifecycle.md#delete-a-vm) before using `--remove-home` or
`--force`. Substitute `custom-tools` when cleaning up the custom-module example.
