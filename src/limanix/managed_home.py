"""Create and remove managed host homes using immutable host ownership records."""

import shutil
from pathlib import Path

from limanix.domain import DomainError
from limanix.state import Identity


class ManagedHomeError(Exception):
    """A managed home cannot be created or removed at its recorded path."""


class ManagedHome:
    """Manage only the exact home allocated to a validated VM identity."""

    @staticmethod
    def _paths(identity: Identity) -> tuple[Path, Path]:
        try:
            root, home = identity.home_paths()
            if root.resolve() != root or home.is_symlink() or home.resolve() != home:
                raise ValueError("managed-home path redirects through a symbolic link")
            return root, home
        except (OSError, ValueError, RuntimeError, DomainError) as error:
            raise ManagedHomeError(
                f"Cannot use managed home '{identity.home}': {error}"
            ) from error

    def create(self, identity: Identity) -> Path:
        """Allocate a new private directory; existing paths are never reused."""
        root, home = self._paths(identity)
        try:
            root.mkdir(parents=True, exist_ok=True)
            # Revalidate after preparing the parent and before allocating the home.
            self._paths(identity)
            home.mkdir(mode=0o700, exist_ok=False)
        except OSError as error:
            raise ManagedHomeError(
                f"Cannot create managed home '{home}': {error.strerror}. "
                "Set home.root to a writable directory or prepare its permissions."
            ) from error
        return home

    def remove(self, identity: Identity) -> None:
        """Remove the recorded home without trusting files writable by the guest."""
        _, home = self._paths(identity)
        try:
            if not home.exists():
                return
            if not home.is_dir():
                raise ManagedHomeError(f"Managed home is not a directory: {home}")
            shutil.rmtree(home)
        except OSError as error:
            raise ManagedHomeError(
                f"Cannot remove managed home '{home}': {error.strerror}"
            ) from error
