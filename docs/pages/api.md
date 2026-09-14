# Python API

## Configuration model

The configuration models define the input structure, default values, and field
documentation. The [configuration reference](configuration.md) presents the
same information in TOML terms.

```{eval-rst}
.. automodule:: limanix.config
   :members: Config, Resources, User, Home, NixOS, Network, Ports, Mount
   :imported-members:
   :member-order: bysource
```

## Loading configuration

The `limanix.config` package also exports:

- `parse_config(data)` validates a decoded dictionary and returns `Config`.
  It fills omitted values from the model defaults without reading files.
- `load_config(path)` reads a TOML file and returns `Config` with host paths
  resolved relative to the file. It expands `~` and keeps environment variable
  references literal.
- `ConfigError` reports invalid configuration. Messages identify the field or
  file problem without including environment values.
- `config_to_dict(config)` serializes the model to the public configuration
  representation, including whole GiB strings for memory and disk sizes.

Module identifiers are checked for syntax while parsing; registry lookup and
mount source existence checks happen when preparing a VM operation.

## Domain values

Names, guest paths, module identifiers, environment values, and architectures
share validated types. Configuration and VM identity records use the same
`Username` and `GuestPath` validation. Memory and disk sizes are `ByteSize`
integer byte counts internally;
`ByteSize.parse("10GiB")` converts configuration text and `.to_gib()` formats it.

```{eval-rst}
.. automodule:: limanix.domain
   :members: VMName, Username, GuestPath, ModuleName, ModuleId, EnvName, EnvValue, Architecture, ByteSize
```

## CLI definition

The parser factory returns an
[`argparse.ArgumentParser`](https://docs.python.org/3/library/argparse.html#argparse.ArgumentParser).

```{eval-rst}
.. autofunction:: limanix.cli.build_parser
```

The CLI entry point remains `limanix.cli.main(argv=None)`. VM lifecycle and backend
responsibilities are described in [Source architecture](architecture.md).
