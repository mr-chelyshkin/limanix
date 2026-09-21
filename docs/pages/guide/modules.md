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

Standard modules use the reserved `lmx:` prefix. Their sources and descriptions
come from [limanix-modules](https://github.com/mr-chelyshkin/limanix-modules), packaged into
Limanix at build time. Listing a module does not install it in a VM.

The default is `modules = []`: no optional modules are selected. The base NixOS
configuration is always supplied by Limanix, independently of this list.

For a Rust development environment,
set the existing `[nixos]` section to:

```toml
[nixos]
modules = ["lmx:git", "lmx:rust", "lmx:neovim"]
```

Create the VM, or apply the change to an existing one:

```console
limanix update --config limanix.toml
```

The array is the complete selection, not an addition to the previous selection.
To stop selecting a module, remove all occurrences of its identifier and update
the VM. Repeated identifiers are accepted and preserved.

Use the full identifier in every configuration: `git` must be written as
`lmx:git`. Updating the standard catalog requires a Limanix build containing
the new catalog; existing VM generations change only after an explicit update.

## Select a toolchain version

Versioned standard modules expose a default and explicit selectors:

```toml
[nixos]
modules = ["lmx:go-1.24", "lmx:python"]
```

`lmx:go` selects the module's default; `lmx:go-1.24` selects its declared
1.24 variant. Selectors may also use a major version, such as `lmx:nodejs-24`
or `lmx:docker-28`. The catalog owns package versions and Nix definitions;
Limanix selects an entry point without choosing package versions itself.

Use `limanix modules list` to inspect the catalog embedded in your binary.
Unknown selectors fail before VM preparation and direct you to that list.
There is no fallback to the default. Imported modules retain
their existing `default.nix` entry point; these version selectors belong to the
standard catalog.

Multiple versions can be selected together:

```toml
[nixos]
modules = ["lmx:go-1.24", "lmx:go-1.25"]
```

The standard toolchain modules provide versioned commands and make the newest
selected version the unqualified command:

```console
go-1.24 version
go-1.25 version
go version
```

In this example, `go` uses 1.25 regardless of selection order. This behavior
belongs to the modules, not to a version manager in Limanix. See the
[module catalog](https://github.com/mr-chelyshkin/limanix-modules/tree/main/modules)
for exact versions and tool-specific commands.

Docker configures one system Engine per VM: select `lmx:docker` or one explicit
Docker version, not multiple Engines together.

Repeated identifiers are accepted. Identical Nix package definitions still refer
to the same derivation, not independently patchable package copies.

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
modules = ["lmx:git", "third-party:dev-tools"]
```

After creating or updating the VM, verify a package provided by this module:

```console
limanix shell example-box -- jq --version
```

Import copies the whole directory. Keep relative imports and required assets
inside that tree; symlinks and special files are rejected. The original source
directory is not mounted or watched.

The imported namespace is currently `third-party:`. For example, importing a
module named `git` creates `third-party:git` and does not replace `lmx:git`.

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
