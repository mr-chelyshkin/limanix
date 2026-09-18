+++
title = "NixOS modules"
description = "Select bundled tools and import trusted NixOS modules from your project."
weight = 30
+++

NixOS modules describe packages and guest system settings. Select them in
`nixos.modules`; Limanix includes their sources in the configuration it builds
inside the VM.

## Select bundled tools

List the available modules:

```console
limanix modules list
```

Limanix bundles `git`, `rust`, and `neovim`. For a Rust development environment,
set the existing `[nixos]` section to:

```toml
[nixos]
modules = ["git", "rust", "neovim"]
```

Create the VM, or apply the change to an existing one:

```console
limanix update --config limanix.toml
```

List each identifier once. The array is the complete selection, not an addition
to the previous selection. To stop selecting a module, remove its identifier
and update the VM.

## Import your own module

Create a directory named `modules/dev-tools` and save this as `default.nix`:

{{< example "modules/dev-tools/default.nix" "nix" >}}

Import the directory into the local registry:

```console
limanix modules add dev-tools ./modules/dev-tools
limanix modules list
```

Select the imported entry with its `third-party:` prefix:

```toml
[nixos]
modules = ["git", "third-party:dev-tools"]
```

After creating or updating the VM, verify a package provided by this module:

```console
limanix shell example-box -- jq --version
```

Import copies the whole directory. Keep relative imports and required assets
inside that tree; symlinks and special files are rejected. The original source
directory is not mounted or watched.

{{< callout type="warning" >}}
Import only modules you trust. A module can configure the entire guest system;
it is not restricted to installing the packages shown in this example.
{{< /callout >}}

## Apply changes to an imported module

There are three separate copies to consider:

```text
Source directory  →  Local registry  →  VM configuration generation
                 add              create / update
```

Editing the source directory changes neither the imported copy nor an existing
VM. To replace the registry copy and apply it:

```console
limanix modules remove dev-tools
limanix modules add dev-tools ./modules/dev-tools
limanix update --config limanix.toml
```

Import the replacement before updating a VM that selects it. Each update
resolves the selected module identifiers from the registry again.

## Remove an imported module

First remove `third-party:dev-tools` from the configurations that should no
longer select it and update those VMs. Then remove the registry entry:

```console
limanix modules remove dev-tools
```

Removing a registry entry alone does not alter an existing VM generation. It
does prevent later create or update operations from resolving that identifier.

A damaged registry entry is listed with its own error without hiding healthy
entries. Use `limanix modules list --json` for structured output. See the
[CLI reference](/reference/cli.md) for flags and the
[custom-module example](examples.md#import-a-nixos-module) for a full configuration.
