# Python API

## Configuration model

The configuration models define the input structure, default values, and field
documentation. The [configuration reference](configuration.md) presents the
same information in TOML terms.

```{eval-rst}
.. automodule:: limanix.config
   :members: Config, Resources, User, Home, NixOS, Network, Ports, Mount
   :member-order: bysource
```

## CLI definition

The parser factory returns an
[`argparse.ArgumentParser`](https://docs.python.org/3/library/argparse.html#argparse.ArgumentParser).

```{eval-rst}
.. autofunction:: limanix.cli.build_parser
```
