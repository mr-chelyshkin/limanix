"""Manage host VM state, Lima, and guest-side NixOS builds.

The process stages a new input directory per create/update::

    <state root>/instances/<name>/
    ├── identity.json          VM and managed-home identity
    ├── instance.json          operation state and generation ID
    └── generations/<id>/
        ├── lima.yaml          Lima configuration
        ├── environment        service ENV
        ├── environment.sh     login-shell ENV
        └── flake/             NixOS sources and copied modules

    Mac generation ─► read-only /mnt/limanix in VM ─► NixOS build

macOS state root: ~/Library/Application Support/Limanix, or LIMANIX_HOME.
Generations are build inputs; old ones are pruned after successful updates.
The managed user HOME is stored separately under config.home.root.
"""

import logging
import os
import shutil
import uuid
from collections.abc import Sequence
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path

from limanix.config import Config, load_config
from limanix.domain import Architecture
from limanix.filesystem import require_directory, write_text_atomic
from limanix.guest import Guest
from limanix.lima import LimaClient, LimaInstance, LimaStatus, render_lima
from limanix.managed_home import ManagedHome
from limanix.modules import ModuleRegistry
from limanix.nixos import prepare_bundle
from limanix.state import Identity, Instance, InstanceStatus, StateError, StateStore

_logger = logging.getLogger(__name__)


class VMError(Exception):
    """A VM operation cannot proceed."""


@dataclass(frozen=True, kw_only=True)
class VMInfo:
    """VM summary."""

    name: str
    address: str
    home: str | None

    state: InstanceStatus | None
    arch: Architecture | None
    status: LimaStatus | None

    lima_name: str | None
    error: str | None = None


class VMManager:
    """Order backend operations under independent per-VM locks."""

    def __init__(
        self, store: StateStore | None = None, lima: LimaClient | None = None
    ) -> None:
        self.store = store if store is not None else StateStore()
        self.lima = lima if lima is not None else LimaClient()

        self.modules = ModuleRegistry(self.store)
        self.guest = Guest(self.lima)
        self.homes = ManagedHome()

    def _preflight(self, config: Config) -> None:
        """Check Lima prerequisites."""
        if os.getuid() == 0:
            raise VMError("Run Limanix as your regular host user, not root.")

        self.lima.preflight(config)
        for mount in config.mounts:
            require_directory(Path(mount.source))


    def _generation(self, instance: Instance) -> Path:
        """Return this generation's host path."""
        return (
            self.store.instance_dir(instance.name) / "generations" / instance.generation
        )

    def _prepare(self, instance: Instance, config: Config) -> Path:
        """Copy NixOS inputs, write ENV and Lima config, validate with Lima."""
        directory = self._generation(instance)
        with self.modules.sources(config.nixos.modules) as paths:
            prepare_bundle(config, directory, paths, uid=os.getuid())

        template = directory / "lima.yaml"
        write_text_atomic(
            template,
            render_lima(config, Path(instance.identity.home), directory),
            mode=0o600,
        )

        self.lima.validate(template)
        return template

    def _failed(self, instance: Instance) -> None:
        """Save ERROR status and a generic message in host state."""
        instance.status = InstanceStatus.ERROR
        instance.error = (
            "Operation failed. "
            "VM state file: "
            f"'{self.store.instance_dir(instance.name) / 'instance.json'}'. "
            "Fix the reported issue before retrying update or delete."
        )
        self.store.save(instance)

    def _prune_generations(self, instance: Instance) -> None:
        """Prune older inputs."""
        try:
            current = self._generation(instance)
            candidates = list(current.parent.iterdir())
        except (OSError, StateError) as error:
            _logger.warning(
                "VM '%s' was updated, but old generations could not be listed: %s",
                instance.name,
                error,
            )
            return
        for candidate in candidates:
            if candidate == current:
                continue
            try:
                if candidate.is_dir() and not candidate.is_symlink():
                    shutil.rmtree(candidate)
                else:
                    candidate.unlink()
            except OSError as error:
                _logger.warning(
                    "VM '%s' was updated, but old generation '%s' "
                    "could not be removed: %s",
                    instance.name,
                    candidate,
                    error,
                )

    def _require_lima(self, identity: Identity) -> LimaInstance:
        """Find the exact generated Lima name recorded in host identity."""
        for instance in self.lima.fetch_all():
            if instance.name == identity.lima_name:
                return instance
        raise VMError(
            f"Lima instance for '{identity.name}' is missing. Delete its saved record."
        )

    def create(self, config_path: Path) -> Instance:
        """Prepare inputs/home, save ownership, create Lima, then apply NixOS."""
        config = load_config(config_path)

        self._preflight(config)
        with self.store.instance_lock(config.name):
            directory = self.store.instance_dir(config.name)
            if directory.exists():
                raise VMError(
                    f"VM '{config.name}' already has state. Use update or delete."
                )

            identifier = uuid.uuid4().hex[:12]
            identity = Identity(
                id=identifier,
                name=config.name,
                arch=config.resources.arch,
                username=config.user.name,
                user_home=config.user.home,
                home_root=config.home.root,
                created_at=datetime.now(UTC).isoformat(),
                home=str(Identity.home_path(config.home.root, config.name, identifier)),
            )
            instance = Instance(
                identity=identity,
                status=InstanceStatus.CREATING,
                generation=uuid.uuid4().hex[:12],
            )
            home_created = False

            try:
                template = self._prepare(instance, config)
                self.homes.create(identity)
                home_created = True
                self.store.save(instance)
            except Exception, KeyboardInterrupt:
                if home_created:
                    self.homes.remove(identity)
                if directory.exists():
                    shutil.rmtree(directory)
                raise
            try:
                self.lima.create(instance.lima_name, template)
                self.lima.start(instance.lima_name)
                self.guest.apply(instance.lima_name, identity.username)
            except Exception, KeyboardInterrupt:
                self._failed(instance)
                raise

            instance.status = InstanceStatus.READY
            self.store.save(instance)
            return instance

    def update(self, config_path: Path) -> Instance:
        """Apply new inputs while keeping identity, disk, and managed home."""
        config = load_config(config_path)

        self._preflight(config)
        with self.store.instance_lock(config.name):
            instance = self.store.load(config.name)
            identity = instance.identity
            if (
                config.resources.arch,
                config.user.name,
                config.user.home,
                config.home.root,
            ) != (
                identity.arch,
                identity.username,
                identity.user_home,
                identity.home_root,
            ):
                raise VMError(
                    "An update cannot change architecture, username, "
                    "or managed-home paths. "
                    "Create a new VM for these changes."
                )

            actual = self._require_lima(identity)
            if actual.disk is None:
                raise VMError(
                    "Lima did not report the disk size; update was not started."
                )
            if config.resources.disk < actual.disk:
                raise VMError("Shrinking the guest disk is not supported.")

            instance.generation = uuid.uuid4().hex[:12]
            instance.status = InstanceStatus.UPDATING
            instance.error = None
            try:
                template = self._prepare(instance, config)
                self.store.save(instance)
            except Exception, KeyboardInterrupt:
                try:
                    unreferenced = (
                        self.store.load(config.name).generation != instance.generation
                    )
                except StateError:
                    unreferenced = False
                if unreferenced and self._generation(instance).exists():
                    shutil.rmtree(self._generation(instance))
                raise
            try:
                actual = self._require_lima(identity)
                if actual.status is LimaStatus.RUNNING:
                    self.lima.stop(instance.lima_name)

                self.lima.edit(instance.lima_name, template)
                self.lima.start(instance.lima_name)
                self.guest.apply(instance.lima_name, identity.username)
            except Exception, KeyboardInterrupt:
                self._failed(instance)
                raise

            instance.status = InstanceStatus.READY
            self.store.save(instance)
            self._prune_generations(instance)
            return instance

    def fetch_all(self) -> list[VMInfo]:
        """List saved VMs with their Lima status and guest IP where available."""
        entries = self.store.fetch_all()

        actual = (
            {instance.name: instance for instance in self.lima.fetch_all()}
            if any(entry.identity is not None for entry in entries)
            else {}
        )

        result = []
        for entry in entries:
            identity, record = entry.identity, entry.instance
            backend = actual.get(identity.lima_name) if identity else None

            result.append(
                VMInfo(
                    name=entry.name,
                    status=backend.status if backend else None,
                    home=identity.home if identity else None,
                    arch=identity.arch if identity else None,
                    state=record.status if record else None,
                    address=self.guest.address(backend) if backend else "",
                    lima_name=identity.lima_name if identity else None,
                    error=entry.error or (record.error if record else None),
                )
            )
        return result

    def start(self, name: str) -> None:
        """Start the recorded Lima instance."""
        with self.store.instance_lock(name):
            identity = self.store.load_identity(name)
            self._require_lima(identity)
            self.lima.start(identity.lima_name)

    def stop(self, name: str) -> None:
        """Stop the recorded Lima instance."""
        with self.store.instance_lock(name):
            identity = self.store.load_identity(name)
            self._require_lima(identity)
            self.lima.stop(identity.lima_name)

    def shell(self, name: str, command: Sequence[str]) -> int:
        """Run as the development user."""
        identity = self.store.load_identity(name)
        actual = self._require_lima(identity)

        if actual.status is not LimaStatus.RUNNING:
            raise VMError(f"VM '{name}' is not running. Use limanix start {name}.")

        return self.guest.shell(identity.lima_name, identity.username, command)

    def delete(
        self, name: str, *, force: bool = False, remove_home: bool = False
    ) -> str:
        """Delete Lima and instance state."""
        with self.store.instance_lock(name):
            identity = self.store.load_identity(name)
            try:
                record = self.store.load(name)
            except StateError:
                record = None

            if record is not None:
                record.status = InstanceStatus.DELETING
                self.store.save(record)

            try:
                actual = next(
                    (
                        item
                        for item in self.lima.fetch_all()
                        if item.name == identity.lima_name
                    ),
                    None,
                )
                if actual:
                    if actual.status is LimaStatus.RUNNING and not force:
                        self.lima.stop(identity.lima_name)
                    self.lima.delete(identity.lima_name, force=force)
                if remove_home:
                    self.homes.remove(identity)
                    self.store.forget_home(identity)
                else:
                    self.store.preserve_home(identity)

                shutil.rmtree(self.store.instance_dir(name))
            except Exception, KeyboardInterrupt:
                if record is not None:
                    self._failed(record)
                raise
            return identity.home
