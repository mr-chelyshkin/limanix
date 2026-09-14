# Limanix

![Turtle illustration](../../.github/assets/readme-header.png)

**Build and test in Linux. Keep your project on your Mac.**

Limanix is a command-line tool for creating development sandboxes. Each sandbox
is a Linux virtual machine (VM) with the tools and packages your project needs.

Choose your tools with NixOS modules and describe the sandbox in a TOML config.
Share your local project folder with the VM, then build, test, and run services
inside it.

## Why use Limanix?

- **Keep project dependencies off your Mac.** Build and run your project inside
  the VM.
- **Make the setup clear.** Define packages and system settings in NixOS modules.
- **Work with your local files.** Share project folders and tool configs with
  the VM.

## How it works

Your `limanix.toml` describes the sandbox: VM resources, user, NixOS modules,
shared folders, environment variables, and network settings.

```{mermaid}
:align: center
:config: {"flowchart": {"padding": 8, "rankSpacing": 24}}

flowchart LR
    accTitle: How Limanix creates a development sandbox
    accDescr: Limanix uses your TOML config and NixOS modules to create a Linux VM on your Mac. Your local project folder is mounted into the VM. You work with the same files.

    config["TOML config<br/>+ NixOS modules"] --> cli["Limanix"]:::primary
    cli -->|creates| vm["Linux VM<br/>NixOS"]
    project["Project on your Mac"] ---|mounted folder| vm
```

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

architecture
api
contributing
```
