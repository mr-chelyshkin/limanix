"""Materialize package resources and trusted modules for guest-side builds."""

import json
import shutil
from collections.abc import Mapping, Sequence
from importlib.resources import files
from importlib.resources.abc import Traversable
from pathlib import Path

from limanix.config import Config
from limanix.domain import EnvName, EnvValue
from limanix.filesystem import write_text_atomic

_BUILTINS = {
    "git": "Git version control.",
    "neovim": "Neovim editor.",
    "rust": "Rust compiler, Cargo, rustfmt, Clippy, and rust-analyzer.",
}


def builtin_modules() -> dict[str, str]:
    """Return bundled module identifiers and descriptions."""
    return dict(_BUILTINS)


def _copy_resource(source: Traversable, destination: Path) -> None:
    if source.is_dir():
        destination.mkdir(parents=True, exist_ok=True)
        for child in source.iterdir():
            _copy_resource(child, destination / child.name)
    else:
        destination.write_bytes(source.read_bytes())


def _quoted_environment_value(value: str) -> str:
    # EnvironmentFile and POSIX shell double quotes share these escape rules.
    for character in ("\\", '"', "$", "`"):
        value = value.replace(character, "\\" + character)
    return f'"{value}"'


def _environment_files(environment: Mapping[EnvName, EnvValue]) -> tuple[str, str]:
    lines = []
    for name, value in sorted(environment.items()):
        name, value = EnvName(name), EnvValue(value)
        lines.append(f"{name}={_quoted_environment_value(value)}")
    service = "".join(f"{line}\n" for line in lines)
    shell = "".join(f"export {line}\n" for line in lines)
    return service, shell


def prepare_bundle(
    config: Config,
    directory: Path,
    module_paths: Sequence[Path],
    *,
    uid: int,
) -> Path:
    """Write a self-contained flake plus environment files outside its source tree.

    Each module entry point belongs to a registry or packaged module directory.
    Its entire parent directory is copied once, retaining relative imports and assets.
    The returned path is the flake directory; sibling ``environment`` and
    ``environment.sh`` files must be installed in ``/etc/limanix`` at runtime.
    """
    if uid <= 0:
        raise ValueError("guest uid must be positive")
    service_environment, shell_environment = _environment_files(config.env)
    flake = directory / "flake"
    if flake.exists():
        raise FileExistsError(f"NixOS bundle already exists: {flake}")
    for path in module_paths:
        if not path.is_file() or path.suffix != ".nix":
            raise ValueError(f"module entry point is not a Nix file: {path}")
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    _copy_resource(files("limanix.nixos").joinpath("resources", "base"), flake)

    imports = []
    for index, source in enumerate(module_paths):
        target = flake / "modules" / f"{index:04d}"
        shutil.copytree(source.parent, target, symlinks=False)
        imports.append((target / source.name).relative_to(flake).as_posix())

    runtime = {
        "name": config.name,
        "arch": config.resources.arch,
        "user": {
            "name": config.user.name,
            "home": config.user.home,
            "sudo": config.user.sudo,
            "uid": uid,
        },
        "ports": {
            "tcp": config.network.ports.tcp,
            "udp": config.network.ports.udp,
        },
        "modules": imports,
    }
    write_text_atomic(
        flake / "runtime.json",
        json.dumps(runtime, indent=2, ensure_ascii=True) + "\n",
        mode=0o600,
    )
    for name, content in (
        ("environment", service_environment),
        ("environment.sh", shell_environment),
    ):
        write_text_atomic(directory / name, content, mode=0o600)
    return flake
