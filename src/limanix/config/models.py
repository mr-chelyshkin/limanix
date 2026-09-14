"""Configuration models, defaults, and field documentation."""

from dataclasses import dataclass, field, fields, is_dataclass
from typing import Any, Literal, cast

from limanix.domain import (
    Architecture,
    ByteSize,
    EnvName,
    EnvValue,
    GuestPath,
    ModuleId,
    Username,
    VMName,
)


@dataclass(kw_only=True)
class Resources:
    """Guest architecture and compute resources."""

    arch: Architecture = field(default=Architecture.ARM64, doc="Guest architecture.")
    disk: ByteSize = field(
        default=ByteSize.parse("10GiB"), doc="Guest system disk size in GiB."
    )
    cpu: int = field(default=4, doc="Guest CPU count, a positive integer.")
    mem: ByteSize = field(
        default=ByteSize.parse("8GiB"), doc="Guest memory size in GiB."
    )


@dataclass(kw_only=True)
class User:
    """Regular guest user and sudo access."""

    name: Username = field(default=Username("dev"), doc="Regular guest username.")
    home: GuestPath = field(
        default=GuestPath("/home/dev"), doc="Guest user's home directory."
    )
    sudo: bool = field(default=True, doc="Passwordless sudo inside the guest.")


@dataclass(kw_only=True)
class Home:
    """Host storage for the guest user's home directory."""

    root: str = field(
        default="~/.limanix",
        doc=(
            "Host root for <root>/<name>-<id>. Limanix creates this directory "
            "on the Mac and mounts it at user.home with read-write access."
        ),
    )


@dataclass(kw_only=True)
class NixOS:
    """Trusted modules that configure the guest system."""

    modules: list[ModuleId] = field(
        default_factory=lambda: [ModuleId("git")],
        doc=(
            "Bundled module names (git, rust, neovim), or third-party:NAME "
            "for a module imported with limanix modules add."
        ),
    )


@dataclass(kw_only=True)
class Ports:
    """Inbound guest firewall ports, separate from host port forwarding."""

    tcp: list[int] = field(
        default_factory=lambda: [8080],
        doc=(
            "Inbound TCP ports in the guest firewall. Services listen on a guest "
            "network interface and are reached at <guest-ip>:<port> from the Mac."
        ),
    )
    udp: list[int] = field(
        default_factory=list, doc="Inbound UDP ports in the guest firewall."
    )


@dataclass(kw_only=True)
class Network:
    """Guest network and inbound firewall ports."""

    mode: Literal["shared"] = field(
        default="shared",
        doc="Shared/NAT network. The Mac reaches the guest by its own IP address.",
    )
    ports: Ports = field(default_factory=Ports, doc="Inbound guest firewall ports.")


@dataclass(kw_only=True)
class Mount:
    """A host directory mounted inside the guest."""

    mode: Literal["rw", "ro"] = field(default="rw", doc="Mount access: rw or ro.")
    source: str = field(doc="Host directory to mount inside the guest.")
    target: GuestPath = field(doc="Mount destination inside the guest.")


@dataclass(kw_only=True)
class Config:
    """Limanix v1 input configuration for a development sandbox."""

    schema_version: int = field(
        default=1,
        doc="Contract version, independent of the installed Limanix package version.",
    )
    name: VMName = field(default=VMName("example-box"), doc="Sandbox name.")
    user: User = field(default_factory=User, doc="Regular guest user and sudo access.")
    resources: Resources = field(
        default_factory=Resources, doc="Guest architecture and compute resources."
    )
    home: Home = field(
        default_factory=Home, doc="Host storage for the guest user's home directory."
    )
    nixos: NixOS = field(
        default_factory=NixOS, doc="Trusted modules that configure the guest system."
    )
    network: Network = field(
        default_factory=Network, doc="Guest network and inbound firewall ports."
    )
    env: dict[EnvName, EnvValue] = field(
        default_factory=lambda: {
            EnvName("APP_ENV"): EnvValue("development"),
            EnvName("APP_LOG_LEVEL"): EnvValue("debug"),
        },
        doc="Guest-wide environment for login sessions and system/user services.",
    )
    mounts: list[Mount] = field(
        default_factory=lambda: [
            Mount(source="~/projects/my-project", target=GuestPath("/workspace")),
            Mount(
                source="~/.ssh/limanix", target=GuestPath("/mnt/git-keys"), mode="ro"
            ),
            Mount(
                source="~/.config/nvim",
                target=GuestPath("/home/dev/.config/nvim"),
                mode="ro",
            ),
        ],
        doc="Host directories mounted inside the guest.",
    )


def _encode(value: Any) -> Any:
    if isinstance(value, ByteSize):
        return value.to_gib()
    if is_dataclass(value) and not isinstance(value, type):
        return {item.name: _encode(getattr(value, item.name)) for item in fields(value)}
    if isinstance(value, dict):
        return {str(key): _encode(item) for key, item in value.items()}
    if isinstance(value, list):
        return [_encode(item) for item in value]
    if isinstance(value, str):
        return str(value)
    return value


def config_to_dict(config: Config) -> dict[str, object]:
    """Serialize typed configuration to its public TOML/JSON representation."""
    return cast(dict[str, object], _encode(config))
