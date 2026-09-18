# Sandbox configurations

| Example | Purpose |
| --- | --- |
| [rust-service.toml](rust-service.toml) | Git, Rust and Neovim; writable project mount; guest TCP port 8080. |
| [custom-module.toml](custom-module.toml) | Git and an imported [dev-tools module](modules/dev-tools/default.nix). |

Copy a configuration into your project directory as `limanix.toml`. Both examples
mount that directory at `/workspace`; the source `.` is relative to the config,
independent of the directory where you run Limanix. Change `resources.arch` to
`amd64` on an Intel Mac. The default development user is `dev` and its writable
host home is allocated under `~/.limanix`.

For example, run from your Rust project directory, replacing `/path/to/limanix`
with this checkout:

```console
cp /path/to/limanix/examples/rust-service.toml limanix.toml
limanix create --config limanix.toml
limanix shell rust-service -- bash -lc 'cd /workspace && cargo check'
```

Cargo's build output goes to the managed home through `CARGO_TARGET_DIR`. Project
files stay on the host. Opening TCP port 8080 allows access through the guest IP;
your service must also listen on a guest network interface.

Import the custom module before selecting it:

```console
limanix modules add dev-tools /path/to/limanix/examples/modules/dev-tools
cp /path/to/limanix/examples/custom-module.toml limanix.toml
limanix create --config limanix.toml
limanix shell custom-tools -- jq --version
```

Imports copy the module directory. Changing the original files does not change
the registered module or an existing VM. After replacing the registry copy,
apply your VM configuration with `limanix update --config limanix.toml`.

See [the examples guide](../docs/pages/guide/examples.md) for project workflows
and cleanup. Keep configurations containing private paths or environment values
in your own project or this checkout's ignored `vm/` directory.
