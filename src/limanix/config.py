"""Configuration fields and defaults for Limanix development sandboxes."""

from dataclasses import dataclass, field
from typing import Literal


@dataclass(kw_only=True)
class Resources:
    """Guest architecture and compute resources."""

    arch: Literal["arm64", "amd64"] = field(default="arm64", doc="Guest architecture.")
    disk: str = field(default="10GiB", doc="Guest system disk size in GiB.")
    cpu: int = field(default=4, doc="Guest CPU count, a positive integer.")
    mem: str = field(default="8GiB", doc="Guest memory size in GiB.")


@dataclass(kw_only=True)
class User:
    """Regular guest user and sudo access."""

    name: str = field(default="dev", doc="Regular guest username.")
    home: str = field(default="/home/dev", doc="Guest user's home directory.")
    sudo: bool = field(default=True, doc="Passwordless sudo inside the guest.")


@dataclass(kw_only=True)
class Home:
    """Host storage for the guest user's home directory."""

    root: str = field(
        default="/opt/limanix",
        doc=(
            "Host root for <root>/<name>-<id>. Limanix creates this directory "
            "on the Mac and mounts it at user.home with read-write access."
        ),
    )


@dataclass(kw_only=True)
class NixOS:
    """Trusted modules that configure the guest system."""

    modules: list[str] = field(
        default_factory=lambda: ["./modules/*.nix"],
        doc=(
            "Paths and glob patterns for trusted NixOS modules, "
            "relative to the configuration file."
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
    target: str = field(doc="Mount destination inside the guest.")


@dataclass(kw_only=True)
class Config:
    """Limanix v1 input configuration for a development sandbox."""

    schema_version: int = field(
        default=1,
        doc="Contract version, independent of the installed Limanix package version.",
    )
    name: str = field(default="example-box", doc="Sandbox name.")
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
    env: dict[str, str] = field(
        default_factory=lambda: {
            "APP_ENV": "development",
            "APP_LOG_LEVEL": "debug",
        },
        doc="Guest-wide environment for login sessions and system/user services.",
    )
    mounts: list[Mount] = field(
        default_factory=lambda: [
            Mount(source="~/projects/my-project", target="/workspace"),
            Mount(source="~/.ssh/limanix", target="/mnt/git-keys", mode="ro"),
            Mount(source="~/.config/nvim", target="/home/dev/.config/nvim", mode="ro"),
        ],
        doc="Host directories mounted inside the guest.",
    )
