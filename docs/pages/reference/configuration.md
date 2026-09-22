+++
title = "Configuration"
description = "TOML field types, defaults, and validation rules."
weight = 10
url = "/configuration/"
prev = "reference"
+++

Limanix reads a UTF-8 TOML file passed explicitly to `create --config` or
`update --config`. It does not search for a configuration based on the current
directory. The filename `limanix.toml` is a convention, not a required basename.

Download {{< download >}} for every field with comments and default values.
For a working project-sized example, start with
[Getting started](/getting-started.md) or [Examples](/guide/examples.md).

## Defaults and replacement

Omitted fields use the model defaults. A supplied section overrides its provided
fields; omitted fields within it retain their defaults. Arrays describe a
complete selection, not additions to the defaults or to the running VM.

Two defaults deserve attention when writing a small configuration:

- **Mounts:** omitting `mounts` retains the default example mounts. Provide your
  own `[[mounts]]` entries, or write `mounts = []` at the top level for no explicit
  project mounts. The managed home is still mounted.
- **Environment:** supplying `[env]` replaces the default map. Write `env = {}`
  at the top level to configure no environment variables.

Top-level assignments belong before the first `[section]` header in TOML.

## Host paths

`home.root` and `mounts.source` support `~`. Relative paths resolve from the
configuration file's real directory, including when the config is a symlink.
`$HOME` and other environment references remain literal, both in paths and in
`[env]` values.

Mount sources must exist as directories when creating or updating a VM. Host
paths can contain spaces. Guest paths cannot contain spaces, tabs, or line
breaks because the guest integration writes them into `fstab` without escaping.

## Validation rules

Unknown fields, invalid types, and unsupported schema versions are rejected.
CPU counts and whole-GiB sizes must be positive. Ports range from 1 to 65535.
Module identifiers must be unique within `nixos.modules`.

`name` is a lowercase hostname label of 1 to 63 characters. Creation also checks
the generated Lima name and SSH socket path before allocating the home or
records. On macOS, that path must be shorter than 104 bytes, including SSH's
temporary suffix. A long storage directory can therefore require a shorter VM
name; the preflight error identifies that constraint.

Mount destinations must not overlap one another or hide the managed home.
A mount inside that home, such as `/home/dev/.config/nvim`, is supported.
The following destinations are reserved:

- The guest root `/`.
- `/etc`, `/boot`, `/usr`, `/var`, `/nix`, `/run`, `/dev`, `/proc`, `/sys`, `/bin`,
  `/sbin`, and everything below them.
- `/mnt/limanix` and `/home/limanix-admin`, including their subtrees.
- Ancestors that would hide the management user's home, such as `/home`.

These destination rules also apply to `user.home`.

## Applying changes

Editing the file alone does not change a VM. Run
`limanix update --config limanix.toml` to apply it. Some fields are fixed for an
existing VM, and disks cannot shrink; see the
[update rules](/guide/lifecycle.md#apply-configuration-changes).

The tables below are generated from the runtime model's types, descriptions,
and defaults. They describe the full configuration, not just fields that can
be updated in place.

{{% include "configuration.md" %}}
