"""The Lima instance data used by Limanix."""

from dataclasses import dataclass
from enum import StrEnum


class LimaStatus(StrEnum):
    """Instance states defined by Lima's limatype.Status contract."""

    UNKNOWN = ""
    UNINITIALIZED = "Uninitialized"
    INSTALLING = "Installing"
    BROKEN = "Broken"
    STOPPED = "Stopped"
    RUNNING = "Running"


@dataclass(frozen=True)
class LimaNetwork:
    """The guest interface identity and whether it uses shared networking."""

    mac_address: str
    shared: bool


@dataclass(frozen=True)
class LimaInstance:
    """Selected instance metadata, excluding the full Lima configuration."""

    name: str
    status: LimaStatus
    disk: int | None = None
    networks: tuple[LimaNetwork, ...] = ()
