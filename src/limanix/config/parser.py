"""Decode the public TOML contract and check cross-field configuration semantics."""

import posixpath
import stat
import tomllib
from dataclasses import MISSING, fields, is_dataclass
from functools import cache
from pathlib import Path, PurePosixPath
from typing import Any, Literal, cast, get_args, get_origin, get_type_hints

from limanix.config.models import Config, Mount
from limanix.domain import (
    Architecture,
    ByteSize,
    DomainError,
    EnvName,
    EnvValue,
    GuestPath,
    ModuleId,
    ModuleName,
    Username,
    VMName,
)


class ConfigError(Exception):
    """A configuration cannot be read or does not satisfy the input contract."""


def _table(value: object, path: str) -> dict[str, object]:
    if not isinstance(value, dict) or any(not isinstance(key, str) for key in value):
        raise ConfigError(f"{path}: expected a table")
    return cast(dict[str, object], value)


@cache
def _annotations(model: type) -> dict[str, Any]:
    return get_type_hints(model)


def _decode_model[T](model: type[T], value: object, path: str) -> T:
    if not is_dataclass(model):
        raise TypeError("configuration model must be a dataclass")
    data = _table(value, path)
    descriptors = {item.name: item for item in fields(model)}
    for key in sorted(data.keys() - descriptors.keys()):
        prefix = "" if path == "config" else f"{path}."
        raise ConfigError(f"{prefix}{key}: unknown field")
    annotations = _annotations(model)
    decoded: dict[str, Any] = {}
    for name, item in descriptors.items():
        field_path = name if path == "config" else f"{path}.{name}"
        if name in data:
            decoded[name] = _decode(data[name], annotations[name], field_path)
        elif item.default is MISSING and item.default_factory is MISSING:
            raise ConfigError(f"{field_path}: required field is missing")
    return model(**decoded)


def _decode(value: object, annotation: Any, path: str) -> Any:
    """Decode dataclasses and declared field types through one recursive path."""
    origin = get_origin(annotation)
    arguments = get_args(annotation)
    if origin is list:
        if not isinstance(value, list):
            raise ConfigError(f"{path}: expected an array")
        return [
            _decode(item, arguments[0], f"{path}[{index}]")
            for index, item in enumerate(value)
        ]
    if origin is dict:
        result = {}
        for key, item in _table(value, path).items():
            decoded_key = _decode(key, arguments[0], path)
            result[decoded_key] = _decode(item, arguments[1], f"{path}.{key}")
        return result
    if origin is Literal:
        if not any(
            type(value) is type(choice) and value == choice for choice in arguments
        ):
            raise ConfigError(
                f"{path}: expected one of {', '.join(map(str, arguments))}"
            )
        return value
    if isinstance(annotation, type) and is_dataclass(annotation):
        return _decode_model(annotation, value, path)
    try:
        if annotation is ByteSize:
            return ByteSize.parse(value)
        if annotation is Architecture:
            if not isinstance(value, str) or value not in Architecture:
                choices = ", ".join(item.value for item in Architecture)
                raise DomainError(f"expected one of {choices}")
            return Architecture(value)
        if annotation in (
            VMName,
            Username,
            GuestPath,
            ModuleName,
            ModuleId,
            EnvName,
            EnvValue,
        ):
            return annotation(value)
        if annotation is str:
            if not isinstance(value, str):
                raise DomainError("expected a string")
            if not value:
                raise DomainError("must not be empty")
            if "\x00" in value:
                raise DomainError("must not contain NUL characters")
            return value
        if annotation in (int, bool):
            if type(value) is not annotation:
                name = "an integer" if annotation is int else "a boolean"
                raise DomainError(f"expected {name}")
            return value
    except DomainError as error:
        raise ConfigError(f"{path}: {error}") from error
    raise TypeError(f"unsupported configuration annotation: {annotation}")


def _mount_target(path: str, field: str) -> PurePosixPath:
    target = PurePosixPath(path)
    reserved = (
        "/etc",
        "/boot",
        "/usr",
        "/var",
        "/nix",
        "/run",
        "/dev",
        "/proc",
        "/sys",
        "/bin",
        "/sbin",
        "/mnt/limanix",
        "/home/limanix-admin",
    )
    if any(target.is_relative_to(root) for root in reserved) or PurePosixPath(
        "/home/limanix-admin"
    ).is_relative_to(target):
        raise ConfigError(f"{field}: destination is reserved for the guest system")
    return target


def _validate_mounts(mounts: list[Mount], home: str) -> None:
    user_home = _mount_target(home, "user.home")
    targets: list[PurePosixPath] = []
    for index, mount in enumerate(mounts):
        field = f"mounts[{index}].target"
        target = _mount_target(mount.target, field)
        if user_home.is_relative_to(target):
            raise ConfigError(f"{field}: would hide the managed user home")
        if any(
            target.is_relative_to(previous) or previous.is_relative_to(target)
            for previous in targets
        ):
            raise ConfigError(f"{field}: overlaps another explicit mount")
        targets.append(target)


def _validate(config: Config) -> None:
    if config.schema_version != 1:
        raise ConfigError("schema_version: only version 1 is supported")
    if config.resources.cpu <= 0:
        raise ConfigError("resources.cpu: must be positive")
    if posixpath.normpath(config.home.root) in ("/", "//"):
        raise ConfigError("home.root: the host root directory is not allowed")
    for protocol in ("tcp", "udp"):
        for index, port in enumerate(getattr(config.network.ports, protocol)):
            if not 1 <= port <= 65535:
                raise ConfigError(
                    f"network.ports.{protocol}[{index}]: "
                    "port must be between 1 and 65535"
                )
    seen: set[ModuleId] = set()
    for index, module in enumerate(config.nixos.modules):
        if module in seen:
            raise ConfigError(f"nixos.modules[{index}]: duplicate module ID")
        seen.add(module)
    _validate_mounts(config.mounts, config.user.home)


def parse_config(data: dict[str, object]) -> Config:
    """Decode model types, fill defaults, and validate semantic constraints.

    This function performs no filesystem access. Host paths remain as supplied;
    load_config resolves them relative to the file. Errors identify fields and
    never include environment values. Registry lookup is a separate operation.
    """
    config = _decode_model(Config, data, "config")
    _validate(config)
    return config


def _resolve_host_path(value: str, parent: Path, field: str) -> str:
    try:
        path = Path(value).expanduser()
        if not path.is_absolute():
            path = parent / path
        return str(path.resolve())
    except (OSError, ValueError, RuntimeError) as error:
        raise ConfigError(f"{field}: host path cannot be resolved") from error


def load_config(path: Path) -> Config:
    """Read UTF-8 TOML and canonicalize host paths relative to its containing folder.

    Tilde expansion is supported; environment variables in paths and env values
    stay literal. Referenced mount sources do not have to exist when parsing.
    """
    try:
        path = path.expanduser().resolve()
        if not stat.S_ISREG(path.stat().st_mode):
            raise ConfigError("config: expected a regular TOML file")
        with path.open("rb") as stream:
            data = tomllib.load(stream)
    except tomllib.TOMLDecodeError as error:
        raise ConfigError(
            f"config: invalid TOML at line {error.lineno}, column {error.colno}"
        ) from error
    except UnicodeDecodeError as error:
        raise ConfigError("config: expected UTF-8 text") from error
    except (OSError, RuntimeError) as error:
        raise ConfigError("config: configuration file cannot be read") from error
    except ValueError as error:
        raise ConfigError("config: invalid configuration file path") from error

    config = parse_config(data)
    config.home.root = _resolve_host_path(config.home.root, path.parent, "home.root")
    if config.home.root == "/":
        raise ConfigError("home.root: the host root directory is not allowed")
    for index, mount in enumerate(config.mounts):
        mount.source = _resolve_host_path(
            mount.source, path.parent, f"mounts[{index}].source"
        )
    return config
