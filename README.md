# Limanix

<p align="center">
  <img src=".github/assets/readme-header.png"
       alt="l"
       width="800">
</p>

Development sandboxes with Lima and NixOS.

Limanix creates Linux development VMs on your Mac from a TOML configuration.
Choose bundled or imported NixOS modules, share your project with the guest,
and keep its home directory on the host.

## Run from source

On macOS, install Python 3.14 or newer, uv, and
[Lima](https://lima-vm.io/docs/installation/). Keep `limactl` available in `PATH`.
From this checkout, run:

```console
uv sync --locked
uv run limanix first-config
uv run limanix modules list
```

`first-config` writes `limanix.toml`, replacing an existing file. Before creating
a VM, edit that file: replace the example mount sources with existing directories,
remove unused mounts, and select your modules. By default, Limanix stores each
VM's home under `~/.limanix/<name>-<id>`; change `home.root` to use another location.

```console
uv run limanix create --config limanix.toml
uv run limanix list
uv run limanix shell example-box
```

Use the VM name from your configuration; the generated example uses
`example-box`. NixOS builds run inside the guest.

## Manage the VM

Apply an edited configuration, stop or start the VM, or delete it:

```console
uv run limanix update --config limanix.toml
uv run limanix stop example-box
uv run limanix start example-box
uv run limanix delete example-box
```

Deletion preserves the managed host home by default. Add `--remove-home` to
remove its contents too; `--force` controls forced Lima termination independently.

See [Getting started](docs/pages/getting-started.md) for the complete workflow,
[Configuration](docs/pages/configuration.md) for modules, mounts, ENV, and network
settings, [CLI](docs/pages/cli.md) for command options, and
[Source architecture](docs/pages/architecture.md) for implementation boundaries
and the verified runtime scope.
