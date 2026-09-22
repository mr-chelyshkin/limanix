+++
title = "Networking"
description = "Reach services at the guest IP and understand Lima's shared QEMU networking."
weight = 40
+++

Limanix uses shared/NAT networking. Your Mac reaches a service at the **guest's
IP address**, not at the Mac's `localhost`.

## Reach a guest service

Open the service's port in the guest firewall:

```toml
[network.ports]
tcp = [8080]
udp = []
```

Apply the configuration with `limanix update --config limanix.toml`. Then start
your service inside the guest, configured to listen on a guest network
interface, for example `0.0.0.0:8080`.

Find the guest address:

```console
limanix list
```

Use `http://<guest-address>:8080` on your Mac, substituting the `ADDRESS` shown
for your VM. These settings open firewall ports; they do not start a service or
publish a port on the Mac. Automatic service port forwarding is disabled.
Lima's management SSH connection remains available.

For UDP services, put the port in `udp` instead. The arrays are the full
configured port lists, not incremental additions. A service bound only to
`127.0.0.1` inside the guest is not listening on its shared-network interface.

## VZ and QEMU networks

| Guest architecture | Driver | Network |
| --- | --- | --- |
| Same as the Mac | VZ | `vzNAT`; no socket_vmnet setup. |
| Different from the Mac | QEMU | `lima:shared`; requires QEMU and privileged socket_vmnet setup. |

See [Installation](/installation.md#prepare-qemu) for the external QEMU
prerequisite. Limanix embeds the network helper, not QEMU itself.

## QEMU network setup

```console
limanix network setup
```

Run this as your regular user in an interactive terminal. If setup is needed,
the command explains why and asks for confirmation before requesting
administrator authentication through `sudo`.

At Lima's standard paths, setup:

1. Installs a missing helper under `/opt/socket_vmnet`.
2. Generates sudoers rules with Lima's API for the actual network configuration.
3. Validates the rules with `visudo`, backs up an existing sudoers file, and
   atomically writes `/private/etc/sudoers.d/lima`.

The helper and its parent directories must pass root-ownership and permission
checks. User-writable paths and symlinks are refused. Limanix and QEMU remain
unprivileged. No Limanix launchd service is installed.

### Existing Lima installations

Setup reads `$LIMA_HOME/_config/networks.yaml` and preserves its selected group.
New configurations use the pinned Lima version's default (`admin` in Lima 2.2);
an existing `everyone` remains `everyone`. The confirmation shows the group.

A secure installed helper is reused, not overwritten. Setup checks whether it
can execute on the current Mac without starting a network. An incompatible
helper needs administrator repair. Automatic installation supports Lima's
standard paths; custom paths remain administrator-managed under
[Lima's instructions](https://lima-vm.io/docs/config/network/vmnet/#socket_vmnet).

A matching setup is reused without a prompt. If network configuration changes,
run setup again to check whether sudoers must be regenerated. In a
noninteractive environment, missing setup is reported as an error; prepare it
in an interactive terminal first.

## Shared-network lifecycle

Lima starts the helper when a QEMU VM needs it and stops it when no running VM
in the current `LIMA_HOME` uses the network. Before starting a stopped QEMU VM,
Lima starts a missing helper. `list` does not start networking, and `start` on
an already-running VM does not perform a network repair.

Limanix serializes its own start, stop, and delete operations within one
`LIMA_HOME`, including the startup image-download phase. External `limactl`
processes do not participate in that lock.

{{< callout type="warning" >}}
Different Lima homes can refer to the same system socket and PID files. Each
Lima reconciler sees only its own instances and can stop a helper used by
another home. Different homes do not isolate the network helper's lifecycle.
{{< /callout >}}

Use one Lima home for VMs sharing the managed network, and avoid concurrent
lifecycle operations through Limanix and external `limactl`. Do not rely on a
helper restart preserving active guest connections. Limanix does not silently
reboot an already-running VM to repair networking.

For an empty address or an unreachable service, continue to
[Troubleshooting](/troubleshooting.md#service-is-unreachable).
