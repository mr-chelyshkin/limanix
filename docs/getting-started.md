# Getting started

## Create your configuration

Copy the default configuration from the repository root:

```console
mkdir -p vm
cp limanix.example.toml vm/example-box.toml
```

Edit `vm/example-box.toml` to set your project paths, modules, and environment
variables. The values in `limanix.example.toml` are the defaults; the
[configuration reference](configuration.md) describes each field.

The `vm/` directory is ignored by Git and holds your local configurations.

## Define the development environment

NixOS modules describe the guest system and its packages. The `nixos.modules`
list accepts file paths and glob patterns relative to your configuration file.
These modules have permission to configure the entire guest system.

Mounts connect host directories to paths inside the guest. Each mount declares
its source, target, and access mode. The `home` section defines where Limanix
keeps the guest user's home directory on the host.

The `env` table defines environment variables for guest login sessions and
system and user services. The `network` section describes connectivity and the
guest firewall ports used by your services.

## Use the configuration

```console
limanix create --config vm/example-box.toml
```

See [CLI](cli.md) for the command arguments.
