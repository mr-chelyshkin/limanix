+++
title = "Guide"
description = "Practical instructions for working with Limanix VMs."
weight = 30
prev = "getting-started"
next = "guide/lifecycle"
+++

If this is your first VM, start with [Getting started](/getting-started.md).
These guides cover the tasks you will use after that first run.

| Task | Guide |
| --- | --- |
| Run commands, change resources, stop or delete a VM | [Manage VMs](lifecycle.md) |
| Mount a project, preserve files, or set environment variables | [Files and environment](files.md) |
| Install bundled tools or import your own NixOS configuration | [NixOS modules](modules.md) |
| Reach a guest service or prepare QEMU networking | [Networking](networking.md) |
| Adapt a complete configuration for your project | [Examples](examples.md) |

The guides use `example-box` as the VM name and `limanix.toml` as the
configuration file. Substitute your configuration's `name` and path.
For exact field types and flags, use the [Reference](/reference/_index.md).
