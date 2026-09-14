# Configuration

The TOML configuration is Limanix's input contract. Field types, descriptions,
and defaults come from {py:class}`limanix.config.Config` and its nested models.

Download {download}`limanix.example.toml <../../limanix.example.toml>` for the full
configuration with default values and comments.

## Validation and host paths

Missing fields use the model defaults. Unknown fields, invalid types, unsupported
schema versions, and invalid mount destinations are rejected before VM creation.
CPU counts and whole GiB sizes must be positive; ports are integers from 1 to
65535.

`home.root` and mount sources support `~`. Relative paths resolve from the
configuration file's directory. Environment variables in paths and `[env]`
values stay literal: writing `$HOME` does not interpolate the host environment.
Mount sources must exist as directories when creating or updating a VM.

Guest mount paths (`user.home` and `mounts.target`) cannot contain spaces, tabs,
or line breaks: the NixOS Lima integration writes these paths into `fstab`
without escaping them. Host paths such as `home.root` and `mounts.source` can
contain spaces.

## NixOS modules

Select modules by their registry identifiers:

```toml
[nixos]
modules = ["git", "rust", "neovim", "third-party:tools"]
```

`git`, `rust`, and `neovim` are bundled with Limanix. To register your own module,
prepare a directory containing `default.nix`, then import it:

```console
limanix modules list
limanix modules add tools ./modules/tools
```

Import copies the directory tree into Limanix's local registry. Keep relative
imports and required assets inside that tree; symlinks and special files inside
it are rejected. Configurations refer to the imported entry as
`third-party:tools`.

List each module identifier once. Pass module source directories to
`limanix modules add` before selecting their imported identifiers in the config.

Each VM configuration generation receives its own module snapshot. Editing the
original directory does not change an imported module or an existing VM.
To replace the registry copy, remove it and import the updated directory:

```console
limanix modules remove tools
limanix modules add tools ./modules/tools
```

Removing a registry entry leaves existing VM snapshots intact. A subsequent VM
update resolves its selected modules from the registry again.

A damaged registry entry appears with its own error in `modules list`; healthy
entries remain available. JSON output includes an `error` field for every entry.
Selecting a damaged module for a VM still fails with its validation error.

Modules are trusted NixOS configuration and can change the whole guest system.

## Home, mounts, and VM state

`home.root` defaults to `~/.limanix` and contains Limanix-owned directories named
`<name>-<id>`. The selected directory is mounted read-write at `user.home`. Other
mounts default to `rw`; set `mode = "ro"` for read-only access.

Explicit mounts cannot overlap each other or hide the managed home. Mounting a
subdirectory inside the home, such as `/home/dev/.config/nvim`, is supported.
The guest root and the entire `/etc`, `/boot`, `/usr`, `/var`, `/nix`, `/run`,
`/dev`, `/proc`, `/sys`, `/bin`, and `/sbin` subtrees are reserved. The
`/mnt/limanix` and `/home/limanix-admin` subtrees are also reserved for management.
An ancestor of the management user's home, such as `/home`, cannot be mounted
over it. These restrictions also apply to `user.home`.

VM records, configuration generations, and imported modules are stored under
`~/Library/Application Support/Limanix` on macOS. Set `LIMANIX_HOME` to override
this location. Records use JSON; a process lock serializes operations that change
state for the same VM. Different VMs have independent locks. The module registry
uses shared locks for listing and copying sources, and exclusive locks for
imports and removals. A conflicting registry operation waits for up to 30 seconds
before reporting a timeout. The Lima VM disk is managed separately by Lima.

The host-only `identity.json` records the immutable Lima identity and managed-home
location. `instance.json` stores only lifecycle status and generation metadata;
it does not contain or revalidate the user configuration. A damaged runtime
record appears with an error in `list`, while `delete` can still use the intact
ownership record. Files inside the guest home are not used as proof of ownership.
Guest usernames and paths use the same domain validation in configuration and
identity records. The identity also derives and checks its managed-home path.

For `creating`, `updating`, or `deleting`, `list` checks the VM operation lock.
If no operation holds it and the saved status is still unfinished, the displayed
state is `interrupted`. Listing does not rewrite the saved record or stop the VM.

Stopping or updating a VM retains its disk, including manually installed packages.
Normal deletion removes the disk and VM state but keeps the managed host home.
Before removing the VM state, Limanix preserves its home ownership under
`homes/<name>-<id>.json` in the state directory. Different VM instances with the
same public name retain separate home records. If that write fails, the VM
identity remains available for a repeated deletion attempt.
`delete --remove-home` also removes that owned home and all its contents.
`--force` controls forced Lima termination independently and preserves the home
unless `--remove-home` is supplied. Mount sources outside that home are never
deletion targets.

## Environment and user access

`[env]` supplies environment variables for login sessions and system and user
services. These values are intentionally readable by all users inside the VM;
host staging files are private (`0600`). Values are staged as runtime files
outside the generated flake and installed under `/etc/limanix` before the guest
rebuild. The generated Nix source does not embed these values in `/nix/store`;
they remain readable inside this development VM.

Updates reboot the guest after applying configuration, giving new sessions and
services the updated environment. This scope does not rewrite the environment
of an already running process before that reboot.

`user.sudo` controls passwordless sudo for the development user. Limanix manages
the guest through a separate `limanix-admin` account with management privileges.
`limanix shell` switches to the configured development user.

The development user has systemd lingering enabled. Its user manager can keep
services active after a shell session closes, while the VM is running. Limanix
sessions set the user's runtime directory and D-Bus address for `systemctl --user`:

```console
limanix shell example-box -- systemctl --user list-units --type=service
```

Define system or user services through your trusted NixOS modules.

## Network access

`network.mode = "shared"` uses a guest IP reachable from the Mac. Native VZ
guests use `vzNAT`; QEMU guests use `lima:shared` and require the host's
[socket_vmnet setup](https://lima-vm.io/docs/config/network/vmnet/#socket_vmnet).

`network.ports.tcp` and `.udp` open guest firewall ports. A service must listen
on a guest network interface, for example `0.0.0.0:8080`, to be reached as
`<guest-ip>:8080` from the Mac. These settings do not publish a port on the Mac's
`localhost`. Automatic service port forwarding is disabled; Lima's management
SSH connection remains available.

## Updates

`limanix update --config PATH` can change CPU, memory, modules, mounts,
environment, firewall ports, and the development user's sudo setting. The disk
can grow but cannot shrink.

The VM name, architecture, `user.name`, `user.home`, and `home.root` cannot change
in place. Create a new VM for those changes.

After a successful update, Limanix attempts to remove all previous configuration
generations, including those retained after earlier failed operations. A cleanup
failure emits a warning; the VM remains `ready` and `update` succeeds. Cleanup
continues for the remaining generations and retries leftovers on the next
successful update.

## Fields and defaults

```{include} ../_generated/configuration.md
```
