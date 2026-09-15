+++
title = "CLI"
weight = 30
+++

# CLI

The command reference is generated from the same Cobra command tree and flags
as `limanix --help`. Run `task docs/generate` after changing command definitions.

Use `shell NAME` for an interactive session, or `shell NAME -- COMMAND ...` for
a guest command. Arguments after the VM name are passed to the guest command;
the optional `--` separates that command from the name. The command's exit status
is returned to the caller.

Usage errors return status `2`; operational failures return `1` and canceled
VM operations return `130`. In an interactive shell, **Ctrl+C** is handled by the
foreground SSH session. Cleanup warnings use the `limanix: warning:` prefix and
do not turn a completed update into a failure.

{{% include "cli.md" %}}
