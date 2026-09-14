"""Store VM identity, operation state, and file locks on the host.

Host layout::

    <state root>/
    ├── instances/<name>/
    │   ├── identity.json          VM and managed-home ownership
    │   └── instance.json          generation ID, status, error
    ├── homes/<name>-<id>.json     retained HOME ownership
    ├── modules/                   imports managed by ModuleRegistry
    └── locks/
        ├── instances/<name>.lock  per-VM operations and listing
        └── registry.lock          module registry readers/writers

macOS root: ~/Library/Application Support/Limanix; LIMANIX_HOME overrides it.
HOME records store ownership only; this module does not create or remove HOME.
"""

import fcntl
import json
import math
import os
import re
import stat
import sys
import time
from collections.abc import Iterator
from contextlib import AbstractContextManager, ExitStack, contextmanager
from dataclasses import asdict, dataclass, fields, replace
from enum import StrEnum
from pathlib import Path
from typing import Any

from limanix.domain import Architecture, DomainError, GuestPath, Username, VMName
from limanix.filesystem import FilesystemError, write_text_atomic

_SCHEMA_VERSION = 1
_LOCK_POLL_INTERVAL = 0.05
_REGISTRY_LOCK_TIMEOUT = 30.0


class StateError(Exception):
    """Invalid or unavailable local state."""


class _LockBusy(StateError):
    """A valid lock is held by another operation."""


class InstanceStatus(StrEnum):
    """Limanix operation states; interrupted is computed only for listing."""

    CREATING = "creating"
    READY = "ready"
    UPDATING = "updating"
    ERROR = "error"
    DELETING = "deleting"
    INTERRUPTED = "interrupted"


_IN_FLIGHT_STATUSES = frozenset(
    (InstanceStatus.CREATING, InstanceStatus.UPDATING, InstanceStatus.DELETING)
)


@dataclass(frozen=True, kw_only=True)
class Identity:
    """VM identity and owned host HOME, stored once in identity.json."""

    user_home: GuestPath
    username: Username
    arch: Architecture
    name: VMName
    created_at: str
    home_root: str
    home: str
    id: str

    @property
    def lima_name(self) -> str:
        """Return the generated Lima name: limanix-<name>-<id>."""
        return f"limanix-{self.name}-{self.id}"

    @staticmethod
    def home_path(root: str, name: VMName, identifier: str) -> Path:
        """Validate ownership fields and return root/<name>-<id>."""
        VMName(name)
        _token(identifier, "identity id")

        parent = Path(root)
        if not parent.is_absolute() or ".." in parent.parts or str(parent) != root:
            raise ValueError("invalid managed-home root")
        return parent / f"{name}-{identifier}"

    def home_paths(self) -> tuple[Path, Path]:
        """Return (root, home)."""
        home = self.home_path(self.home_root, self.name, self.id)
        if str(home) != self.home:
            raise ValueError("invalid managed-home identity")
        return home.parent, home


@dataclass(kw_only=True)
class Instance:
    """VM identity paired with an operation status, error, and host generation ID."""

    identity: Identity
    status: InstanceStatus
    generation: str
    error: str | None = None

    @property
    def name(self) -> VMName:
        """Return the public VM name from its identity."""
        return self.identity.name

    @property
    def lima_name(self) -> str:
        """Return the generated Lima name from its identity."""
        return self.identity.lima_name


@dataclass(frozen=True, kw_only=True)
class StateEntry:
    """Listing result; readable identity remains available if runtime is damaged."""

    instance: Instance | None
    identity: Identity | None
    name: str
    error: str | None


def default_state_root() -> Path:
    """Return LIMANIX_HOME or the platform default without creating directories."""
    override = os.environ.get("LIMANIX_HOME")
    if override:
        return Path(override).expanduser().resolve()
    if sys.platform == "darwin":
        return Path.home() / "Library" / "Application Support" / "Limanix"

    return (
        Path(os.environ.get("XDG_STATE_HOME", str(Path.home() / ".local/state")))
        / "limanix"
    )


def _name(value: str) -> VMName:
    """Validate a VM name, reporting invalid names as StateError."""
    try:
        return VMName(value)
    except DomainError as error:
        raise StateError(str(error)) from error


def _token(value: object, field: str) -> str:
    """Require exactly 12 lowercase hexadecimal characters."""
    if not isinstance(value, str) or not re.fullmatch(r"[a-f0-9]{12}", value):
        raise ValueError(f"invalid {field}")
    return value


def _record(data: Any, field_names: set[str]) -> dict[str, Any]:
    """Require schema version 1 and exactly the expected record fields."""
    if (
        not isinstance(data, dict)
        or type(data.get("schema_version")) is not int
        or data["schema_version"] != _SCHEMA_VERSION
    ):
        raise ValueError("unsupported state schema")
    if set(data) != field_names | {"schema_version"}:
        raise ValueError("unexpected record fields")
    return {name: data[name] for name in field_names}


def _identity(data: Any, name: str) -> Identity:
    """Decode identity and check its directory name and managed-home allocation."""
    values = _record(data, {field.name for field in fields(Identity)})
    for field_name, value in values.items():
        if not isinstance(value, str) or not value:
            raise ValueError(f"invalid {field_name}")

    if values["name"] != name:
        raise ValueError("identity name differs from its directory")

    values["name"] = VMName(values["name"])
    values["arch"] = Architecture(values["arch"])
    values["username"] = Username(values["username"])
    values["user_home"] = GuestPath(values["user_home"])

    identity = Identity(**values)
    identity.home_paths()
    return identity


class StateStore:
    """Read/write host JSON records and provide VM/registry lock contexts."""

    def __init__(self, root: Path | None = None) -> None:
        """Resolve the supplied or default root; initialize creates directories."""
        self.root = (
            (root if root is not None else default_state_root()).expanduser().resolve()
        )

    def initialize(self) -> None:
        """Create state and lock directories; new directories use mode 0700."""
        try:
            self.root.mkdir(mode=0o700, parents=True, exist_ok=True)
            for relative in (
                "instances",
                "modules",
                "homes",
                "locks",
                "locks/instances",
            ):
                directory = self.root / relative
                self._check_directory(directory)
                directory.mkdir(mode=0o700, exist_ok=True)
        except OSError as error:
            raise StateError(
                f"Cannot initialize state at {self.root}: {error.strerror}"
            ) from error

    def instance_lock(self, name: str) -> AbstractContextManager[None]:
        """Exclude concurrent operations on one VM without waiting."""
        return self._vm_lock(name, shared=False)

    def registry_lock(
        self, *, shared: bool = False, timeout: float = _REGISTRY_LOCK_TIMEOUT
    ) -> AbstractContextManager[None]:
        """Lock the module registry, waiting up to 30 seconds by default."""
        if not math.isfinite(timeout) or timeout < 0:
            raise ValueError("Registry lock timeout must be finite and nonnegative.")
        return self._lock(
            self.root / "locks" / "registry.lock",
            f"Timed out after {timeout:g}s waiting for the module registry lock. "
            "Try again after the other module operation finishes.",
            shared=shared,
            timeout=timeout,
        )

    def instance_dir(self, name: str) -> Path:
        """Return the checked root/instances/<name> path without creating it."""
        validated = _name(name)
        self._check_directory(self.root / "instances")
        directory = self.root / "instances" / validated
        self._check_directory(directory)
        return directory

    def save(self, instance: Instance) -> None:
        """Write identity.json and instance.json under instances/<name>/."""
        identity_data = {"schema_version": _SCHEMA_VERSION, **asdict(instance.identity)}
        directory = self.instance_dir(instance.name)
        mutable_data = {
            "schema_version": _SCHEMA_VERSION,
            **{
                field.name: getattr(instance, field.name)
                for field in fields(Instance)
                if field.name != "identity"
            },
        }

        try:
            _identity(identity_data, instance.name)
            self._instance(mutable_data, instance.identity)
            self.initialize()

            directory.mkdir(mode=0o700, exist_ok=True)
            identity_path = directory / "identity.json"
            if identity_path.exists() or identity_path.is_symlink():
                if self.load_identity(instance.name) != instance.identity:
                    raise StateError(
                        f"VM '{instance.name}' identity cannot be changed."
                    )
            else:
                write_text_atomic(
                    identity_path,
                    json.dumps(identity_data, indent=2) + "\n",
                    mode=0o600,
                )
            write_text_atomic(
                directory / "instance.json",
                json.dumps(mutable_data, indent=2) + "\n",
                mode=0o600,
            )
        except (OSError, ValueError, DomainError, FilesystemError) as error:
            raise StateError(
                f"Cannot save VM record '{instance.name}': {error}"
            ) from error

    def preserve_home(self, identity: Identity) -> Path:
        """Archive ownership in homes/<name>-<id>.json and return that path."""
        data = {"schema_version": _SCHEMA_VERSION, **asdict(identity)}
        try:
            _identity(data, identity.name)
            self.initialize()
            destination = self._preserved_home_path(identity)

            if destination.exists() or destination.is_symlink():
                saved = _identity(self._read_json(destination), identity.name)
                if saved != identity:
                    raise StateError(
                        f"Preserved home '{destination.stem}' ownership "
                        "cannot be changed."
                    )
            else:
                write_text_atomic(
                    destination, json.dumps(data, indent=2) + "\n", mode=0o600
                )

            return destination
        except (OSError, ValueError, TypeError, DomainError, FilesystemError) as error:
            raise StateError(
                f"Cannot preserve home ownership for VM '{identity.name}': {error}"
            ) from error

    def forget_home(self, identity: Identity) -> None:
        """Remove matching homes/<name>-<id>.json; do not remove HOME itself."""
        try:
            destination = self._preserved_home_path(identity)
            self._check_directory(destination.parent)
            try:
                saved = _identity(self._read_json(destination), identity.name)
            except FileNotFoundError:
                return
            if saved != identity:
                raise StateError(
                    f"Preserved home '{destination.stem}' ownership "
                    "differs from the VM."
                )
            destination.unlink()
        except (OSError, ValueError, TypeError, DomainError) as error:
            raise StateError(
                f"Cannot remove preserved home ownership "
                f"for VM '{identity.name}': {error}"
            ) from error

    def load_identity(self, name: str) -> Identity:
        """Read instances/<name>/identity.json independently of runtime state."""
        path = self.instance_dir(name) / "identity.json"

        try:
            return _identity(self._read_json(path), name)
        except FileNotFoundError as error:
            raise StateError(f"VM '{name}' has no managed identity.") from error
        except (OSError, ValueError, TypeError, DomainError) as error:
            raise StateError(f"Cannot read VM identity '{name}': {error}") from error

    def load(self, name: str) -> Instance:
        """Read and validate identity.json and instance.json for one VM."""
        identity = self.load_identity(name)
        return self._load_instance(identity)

    def fetch_all(self) -> list[StateEntry]:
        """List saved VMs, keeping damaged records with per-entry error messages."""
        directory = self.root / "instances"
        self._check_directory(directory)

        try:
            paths = sorted(directory.iterdir())
        except FileNotFoundError:
            return []
        except OSError as error:
            raise StateError(f"Cannot list VM records: {error}") from error

        entries: list[StateEntry] = []
        for path in paths:
            if not path.is_dir() and not path.is_symlink():
                continue

            error_text = None
            identity = None
            instance = None
            try:
                identity = self.load_identity(path.name)
                instance = self._load_instance(identity)

                if instance.status in _IN_FLIGHT_STATUSES:
                    with self._snapshot_lock(path.name) as acquired:
                        if acquired:
                            identity = None
                            identity = self.load_identity(path.name)
                            instance = self._load_instance(identity)

                            if instance.status in _IN_FLIGHT_STATUSES:
                                instance = replace(
                                    instance, status=InstanceStatus.INTERRUPTED
                                )
            except StateError as failure:
                instance = None
                error_text = str(failure)

            entries.append(
                StateEntry(
                    name=path.name,
                    identity=identity,
                    instance=instance,
                    error=error_text,
                )
            )
        return entries

    @contextmanager
    def _lock(
        self, path: Path, busy: str, *, shared: bool = False, timeout: float = 0
    ) -> Iterator[None]:
        """Hold a shared/exclusive flock and close its descriptor on block exit."""
        self.initialize()
        try:
            descriptor = os.open(path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        except OSError as error:
            raise StateError(
                f"Cannot open state lock {path}: {error.strerror}"
            ) from error
        try:
            try:
                if not stat.S_ISREG(os.fstat(descriptor).st_mode):
                    raise StateError(f"State lock is not a regular file: {path}")

                os.fchmod(descriptor, 0o600)
                operation = fcntl.LOCK_SH if shared else fcntl.LOCK_EX
                deadline = time.monotonic() + timeout

                while True:
                    try:
                        fcntl.flock(descriptor, operation | fcntl.LOCK_NB)
                        break
                    except BlockingIOError as error:
                        remaining = deadline - time.monotonic()
                        if remaining <= 0:
                            raise _LockBusy(busy) from error
                        time.sleep(min(_LOCK_POLL_INTERVAL, remaining))

            except OSError as error:
                raise StateError(
                    f"Cannot acquire state lock {path}: {error.strerror}"
                ) from error
            yield
        finally:
            os.close(descriptor)

    @contextmanager
    def _snapshot_lock(self, name: str) -> Iterator[bool]:
        """Yield True under a shared VM lock, or False if an operation holds it."""
        with ExitStack() as locks:
            try:
                locks.enter_context(self._vm_lock(name, shared=True))
            except _LockBusy:
                acquired = False
            else:
                acquired = True
            yield acquired

    @staticmethod
    def _check_directory(directory: Path) -> None:
        """Reject a directory path that is itself a symlink."""
        if directory.is_symlink():
            raise StateError(f"State directory is a symbolic link: {directory}")

    @staticmethod
    def _read_json(path: Path) -> Any:
        """Read a regular JSON file, rejecting a symlink at the file path."""
        descriptor = os.open(path, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
        with os.fdopen(descriptor, encoding="utf-8") as stream:
            if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
                raise ValueError("record is not a regular file")
            return json.load(stream)

    @staticmethod
    def _instance(data: Any, identity: Identity) -> Instance:
        """Decode runtime state; reject interrupted as a persisted status."""
        values = _record(
            data, {field.name for field in fields(Instance)} - {"identity"}
        )

        _token(values["generation"], "generation")
        if not isinstance(values["status"], str):
            raise ValueError("invalid lifecycle state")
        if values["error"] is not None and not isinstance(values["error"], str):
            raise ValueError("invalid error")

        values["status"] = InstanceStatus(values["status"])
        if values["status"] is InstanceStatus.INTERRUPTED:
            raise ValueError("interrupted is a computed listing status")
        return Instance(identity=identity, **values)

    def _vm_lock(self, name: str, *, shared: bool) -> AbstractContextManager[None]:
        """Return a lock for locks/instances/<name>.lock; shared allows readers."""
        validated = _name(name)
        return self._lock(
            self.root / "locks" / "instances" / f"{validated}.lock",
            f"Another operation is running for VM '{validated}'. "
            "Try again after it finishes.",
            shared=shared,
        )

    def _preserved_home_path(self, identity: Identity) -> Path:
        """Return the archive path for this identity's managed-home allocation."""
        _, home = identity.home_paths()
        return self.root / "homes" / f"{home.name}.json"

    def _load_instance(self, identity: Identity) -> Instance:
        """Read instance.json and attach the already loaded identity."""
        path = self.instance_dir(identity.name) / "instance.json"
        try:
            return self._instance(self._read_json(path), identity)
        except (OSError, ValueError, TypeError) as error:
            raise StateError(
                f"Cannot read VM record '{identity.name}': {error}"
            ) from error
