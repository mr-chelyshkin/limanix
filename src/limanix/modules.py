"""Catalog trusted NixOS modules and provide stable sources for VM bundles."""

import shutil
import stat
import tempfile
from collections.abc import Iterator, Sequence
from contextlib import ExitStack, contextmanager
from dataclasses import dataclass
from importlib.resources import as_file, files
from pathlib import Path
from typing import Literal

from limanix.domain import DomainError, ModuleId, ModuleName
from limanix.nixos import builtin_modules
from limanix.state import StateStore


class ModuleError(Exception):
    """An unavailable module or invalid module directory."""


@dataclass(frozen=True)
class ModuleInfo:
    """A public catalog entry."""

    name: str
    source: Literal["bundled", "third-party"]
    description: str
    error: str | None = None


def _name(value: str) -> ModuleName:
    try:
        return ModuleName(value)
    except DomainError as error:
        raise ModuleError(f"Invalid module name: {error}.") from error


def _validate_entry(directory: Path) -> None:
    """Require a real catalog directory with a regular Nix entry point."""
    try:
        metadata = directory.lstat()
        if stat.S_ISLNK(metadata.st_mode):
            raise ModuleError("Module directory must not be a symbolic link.")
        if not stat.S_ISDIR(metadata.st_mode):
            raise ModuleError("Module entry is not a directory.")
        entry = directory / "default.nix"
        if not stat.S_ISREG(entry.lstat().st_mode):
            raise ModuleError("Module entry point must be a regular default.nix file.")
    except OSError as error:
        raise ModuleError(f"Cannot read module entry: {error}.") from error


def copy_module_tree(source: Path, destination: Path) -> Path:
    """Copy a complete module tree, keeping relative imports inside its snapshot."""
    source = source.expanduser().resolve(strict=True)
    if destination.resolve().is_relative_to(source):
        raise ModuleError("A module cannot be copied into its own source directory.")
    if not source.is_dir() or not (source / "default.nix").is_file():
        raise ModuleError(
            "A third-party module must be a directory containing default.nix."
        )
    for path in source.rglob("*"):
        if path.is_symlink():
            raise ModuleError(
                f"Module symlinks are not supported: {path.relative_to(source)}"
            )
        if not path.is_file() and not path.is_dir():
            raise ModuleError(f"Unsupported file in module: {path.relative_to(source)}")
    shutil.copytree(source, destination)
    return destination / "default.nix"


class ModuleRegistry:
    """Keep imported source trees independent of the original checkout."""

    def __init__(self, store: StateStore) -> None:
        self.store = store

    def available(self) -> list[ModuleInfo]:
        """Return the catalog, retaining an error row for each invalid entry."""
        entries = [
            ModuleInfo(name, "bundled", description)
            for name, description in builtin_modules().items()
        ]
        directory = self.store.root / "modules"
        with self.store.registry_lock(shared=True):
            if directory.exists():
                for path in sorted(directory.iterdir()):
                    if path.name.startswith("."):
                        continue
                    error = None
                    try:
                        _name(path.name)
                        _validate_entry(path)
                    except ModuleError as failure:
                        error = str(failure)
                    entries.append(
                        ModuleInfo(
                            f"third-party:{path.name}",
                            "third-party",
                            "Locally imported NixOS module",
                            error=error,
                        )
                    )
        return entries

    def add(self, name: str, source: Path) -> None:
        name = _name(name)
        directory = self.store.root / "modules"
        destination = directory / name
        try:
            with self.store.registry_lock(shared=False):
                directory.mkdir(mode=0o700, exist_ok=True)
                if destination.exists() or destination.is_symlink():
                    raise ModuleError(
                        f"Module 'third-party:{name}' already exists. "
                        "Remove it before importing a replacement."
                    )
            with tempfile.TemporaryDirectory(
                prefix=f".{name}.import-", dir=directory
            ) as staging:
                imported = Path(staging) / "module"
                copy_module_tree(source, imported)
                with self.store.registry_lock(shared=False):
                    # Another importer may have committed while this tree copied.
                    if destination.exists() or destination.is_symlink():
                        raise ModuleError(
                            f"Module 'third-party:{name}' already exists. "
                            "Remove it before importing a replacement."
                        )
                    imported.rename(destination)
        except (OSError, ValueError) as error:
            raise ModuleError(f"Cannot import module '{name}': {error}") from error

    def remove(self, name: str) -> None:
        name = _name(name)
        directory = self.store.root / "modules"
        destination = directory / name
        try:
            with ExitStack() as cleanup:
                with self.store.registry_lock(shared=False):
                    if not destination.is_dir() or destination.is_symlink():
                        raise ModuleError(
                            f"Module 'third-party:{name}' is not installed."
                        )
                    staging = cleanup.enter_context(
                        tempfile.TemporaryDirectory(
                            prefix=f".{name}.remove-", dir=directory
                        )
                    )
                    destination.rename(Path(staging) / "module")
        except OSError as error:
            raise ModuleError(f"Cannot remove module '{name}': {error}") from error

    @contextmanager
    def sources(self, names: Sequence[ModuleId]) -> Iterator[tuple[Path, ...]]:
        """Keep source directories stable while the caller copies its VM bundle."""
        available = builtin_modules()
        with self.store.registry_lock(shared=True), ExitStack() as resources:
            result = []
            for value in names:
                try:
                    module_id = ModuleId(value)
                except DomainError as error:
                    raise ModuleError(f"Invalid module identifier: {error}.") from error
                if module_id.is_third_party:
                    source = self.store.root / "modules" / module_id.name
                    try:
                        _validate_entry(source)
                    except ModuleError as error:
                        raise ModuleError(f"Module '{module_id}': {error}") from error
                else:
                    if module_id not in available:
                        raise ModuleError(
                            f"Unknown bundled module '{module_id}'. "
                            "Use limanix modules list."
                        )
                    source = resources.enter_context(
                        as_file(
                            files("limanix.nixos").joinpath(
                                "resources", "modules", module_id
                            )
                        )
                    )
                result.append(source / "default.nix")
            yield tuple(result)
