# Getting started

## 1. Create a config

Run in any folder:

```console
limanix first-config
```

This writes `limanix.toml` with the **default values**.

To use another folder, run:

```console
limanix first-config ~/projects/my-project
```

## 2. Edit your configuration

Open `limanix.toml` and choose what your sandbox needs:

| Setting         | What it controls                                               |
|-----------------|----------------------------------------------------------------|
| `nixos.modules` | Packages and system settings. Modules can change the whole VM. |
| `mounts`        | Local folders (`source`) and their paths in the VM (`target`). |
| `home.root`     | Local folder where Limanix keeps VM home directories.          |
| `env`           | Variables for VM login sessions and system and user services.  |
| `network`       | VM network and firewall ports for your services.               |

> **Module paths start from the config folder.** 
> The pattern `./modules/*.nix` selects NixOS files in a `modules/` folder next to `limanix.toml`.

See the [configuration reference](configuration.md) for all fields and defaults.

## 3. Create the sandbox

```console
limanix create --config limanix.toml
```

See the [CLI reference](cli.md) for command options.
