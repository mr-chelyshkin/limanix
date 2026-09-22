+++
title = "CLI"
description = "Command syntax, flags, output, and exit status."
weight = 20
url = "/cli/"
+++

Run `limanix --help` for the command list, or add `--help` to a command for its
usage. The command definitions below are generated from the same Cobra tree
used by the binary.

## Command arguments

`create` and `update` require `--config PATH`. VM operations such as `start`,
`stop`, `shell`, and `delete` use the configuration's public VM name.

```console
limanix create --config ./limanix.toml
limanix shell example-box -- git --version
```

For `shell`, arguments after the VM name belong to the guest command; an
optional `--` separates them from the name. Limanix preserves those arguments
and returns the command's exit status. See
[Running commands](/guide/lifecycle.md#open-a-shell-or-run-a-command) for shell expressions.

## Output and diagnostics

`list --json` and `modules list --json` emit JSON arrays; an empty result is
`[]`. A damaged record appears with its own `error` instead of hiding healthy
entries. See [listing fields](storage.md#listing-fields) for VM output.

Use `limanix --debug COMMAND ...` to enable internal Lima diagnostic logging.
Cleanup warnings have the `limanix: warning:` prefix. A warning after a completed
update does not change that update's successful exit status.

## Exit status

| Status | Meaning |
| --- | --- |
| `0` | Success, including help and version output. |
| `1` | Operational failure. |
| `2` | Command usage error. |
| `130` | Canceled VM operation. |
| Guest command's status | Returned by `shell NAME -- COMMAND ...`. |

In an interactive shell, **Ctrl+C** is handled by the foreground SSH session.
It does not stop the VM.

{{% include "cli.md" %}}
