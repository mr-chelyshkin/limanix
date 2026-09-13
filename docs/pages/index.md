# Limanix

![Turtle illustration](../../.github/assets/readme-header.png)

**Linux development sandboxes on your Mac.**

Create your sandbox from the command line with Limanix.\
Each sandbox is a virtual machine (VM) with its own tools, packages, and system
settings.

## Why use Limanix?

- **Keep project dependencies off your Mac.** Build and run your project inside
  the VM.
- **Make the setup clear.** Define packages and system settings in NixOS modules.
- **Work with your local files.** Share project folders and tool configs with
  the VM.

## How it works

Your `limanix.toml` describes the sandbox: VM resources, user, NixOS modules,
shared folders, environment variables, and network settings.

| Part | Role |
| --- | --- |
| **Limanix** | Turns your TOML config into settings for Lima and NixOS. |
| **Lima** | Runs the Linux VM and provides shared folders and networking. |
| **NixOS** | Sets up the system and packages using your modules. |

Inside the VM, you work as a regular user with `sudo` access by default. You can
also install packages manually; these changes belong to that VM.

> **Shared files stay on your Mac.** Mounted folders and the VM user's home use
> local storage. Changes in a read-write mount are visible on both sides.

## Explore the docs

Start with **[Getting started](getting-started.md)** to create a config and set up
your sandbox.
The configuration reference lists all fields and defaults; the CLI reference
covers commands and options.

You can also download
{download}`limanix.example.toml <../../limanix.example.toml>` with all default values.

```{toctree}
:maxdepth: 1
:caption: Use Limanix

getting-started
configuration
cli
```

```{toctree}
:maxdepth: 1
:caption: Develop Limanix

api
contributing
```
