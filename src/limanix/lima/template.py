"""Translate the validated Limanix configuration into a Lima template."""

import json
import os
import platform
from pathlib import Path, PurePosixPath

from limanix.config import Config
from limanix.domain import Architecture

# Published at github.com/nixos-lima/nixos-lima/releases/expanded_assets/v0.2.1.
_IMAGE_DIGESTS = {
    "aarch64": "ebdf8363bcb51542892963790c08ddfbffe45443ab19c5e70275c4f0c0aa6f11",
    "x86_64": "967da3baf4ea410e728c751ca9e0a617299b6809297e4e50e773cae4ce79197d",
}
_IMAGE_RELEASE = "https://github.com/nixos-lima/nixos-lima/releases/download/v0.2.1"


def uses_vz(arch: Architecture, host_arch: str | None = None) -> bool:
    """Use VZ for a native guest; a foreign architecture requires QEMU."""
    host = host_arch if host_arch is not None else platform.machine()
    return host in (arch.value, arch.lima_arch)


def render_lima(
    config: Config,
    managed_home: Path,
    runtime_dir: Path,
    *,
    host_arch: str | None = None,
    host_uid: int | None = None,
) -> str:
    """Return JSON, a YAML subset, with only explicitly managed host mounts.

    The dedicated management user applies NixOS configuration independently of
    the development user's sudo setting. ENV files live outside the staged flake.
    """
    arch = config.resources.arch.lima_arch
    native = uses_vz(config.resources.arch, host_arch)
    uid = os.getuid() if host_uid is None else host_uid
    mounts = [
        {
            "location": str(managed_home),
            "mountPoint": config.user.home,
            "writable": True,
        },
        {
            "location": str(runtime_dir),
            "mountPoint": "/mnt/limanix",
            "writable": False,
        },
        *(
            {
                "location": mount.source,
                "mountPoint": mount.target,
                "writable": mount.mode == "rw",
            }
            for mount in config.mounts
        ),
    ]
    mounts.sort(key=lambda mount: len(PurePosixPath(str(mount["mountPoint"])).parts))
    document = {
        "vmType": "vz" if native else "qemu",
        "arch": arch,
        "images": [
            {
                "location": f"{_IMAGE_RELEASE}/nixos-lima-v0.2.1-{arch}.qcow2",
                "arch": arch,
                "digest": f"sha256:{_IMAGE_DIGESTS[arch]}",
            }
        ],
        "cpus": config.resources.cpu,
        "memory": config.resources.mem.to_gib(),
        "disk": config.resources.disk.to_gib(),
        "user": {
            "name": "limanix-admin",
            "home": "/home/limanix-admin",
            "uid": 1001 if uid == 1000 else 1000,
            "passwordlessSudo": True,
        },
        "mountType": "virtiofs" if native else "9p",
        "mounts": mounts,
        "networks": [{"vzNAT": True}] if native else [{"lima": "shared"}],
        "ssh": {
            "loadDotSSHPubKeys": False,
            "forwardAgent": False,
        },
        "propagateProxyEnv": False,
        "portForwards": [
            {
                "guestIP": "0.0.0.0",
                "guestIPMustBeZero": False,
                "guestPortRange": [1, 65535],
                "proto": "any",
                "ignore": True,
            }
        ],
        "containerd": {"system": False, "user": False},
    }
    return json.dumps(document, indent=2) + "\n"
