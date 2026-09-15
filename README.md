# Limanix

<p align="center">
  <img src=".github/assets/readme-header.png"
       alt="Limanix"
       width="800">
</p>

Limanix creates NixOS development VMs on your Mac. Describe packages and system
settings with NixOS modules, mount your local project, and build and run it in
the guest. Each VM keeps its own disk and a managed home on the host.

Install the native macOS binary from
[GitHub Releases](https://github.com/mr-chelyshkin/limanix/releases) when available:
`limanix-darwin-arm64` for Apple Silicon or `limanix-darwin-amd64` for Intel.
For example, on Apple Silicon:

```console
mkdir -p ~/.local/bin
install -m 755 limanix-darwin-arm64 ~/.local/bin/limanix
```

Use macOS 13 or newer and add `~/.local/bin` to `PATH`. The binary includes Lima
and its Linux guest agents; host Lima, Nix, and Python are not required. See
[Getting started](docs/pages/getting-started.md) for building from source,
Gatekeeper instructions, and foreign-architecture QEMU prerequisites.

Create a configuration and inspect the available modules:

```console
limanix first-config
limanix modules list
```

Edit `limanix.toml`: replace or remove the example mounts, choose modules, and
set `resources.arch` to `arm64` on Apple Silicon or `amd64` on Intel for a native
guest. Then create and use the VM:

```console
limanix create --config limanix.toml
limanix shell example-box
limanix update --config limanix.toml
```

The managed home defaults to `~/.limanix/<name>-<id>`. Read-write mounts change
the same files on your Mac. Imported modules use `third-party:NAME`; apply changes
to an existing VM with `update`.

- [Getting started](docs/pages/getting-started.md)
- [Examples](examples/README.md)
- [Configuration](docs/pages/configuration.md) and [CLI](docs/pages/cli.md)
- [Source architecture](docs/pages/architecture.md) and [Go packages](docs/pages/api.md)

Development checks and documentation use Task and Docker:

```console
task ci/fmt ci/lint ci/test ci/vuln ci/docs
task docs/serve
```

The preview opens at <http://127.0.0.1:8070>. Native binary builds use the Go
toolchain in `go.mod` and the Xcode command-line tools: `task --yes ci/build`.
See [Development and documentation](docs/pages/contributing.md) for the tooling
and generated-source workflow.
