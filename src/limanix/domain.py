"""Validated values shared by configuration, state, and VM integrations."""

import re
from enum import StrEnum
from pathlib import PurePosixPath
from typing import Self

_GIB = 1024**3


class DomainError(Exception):
    """A value does not satisfy its domain contract."""


def _text(value: object, *, empty: bool = False) -> str:
    if not isinstance(value, str):
        raise DomainError("expected a string")
    if "\x00" in value:
        raise DomainError("must not contain NUL characters")
    if not empty and not value:
        raise DomainError("must not be empty")
    return value


class VMName(str):
    """A lowercase hostname label used for the public VM identity."""

    def __new__(cls, value: object) -> Self:
        text = _text(value)
        if not re.fullmatch(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?", text):
            raise DomainError(
                "expected a lowercase hostname label of 1 to 63 characters"
            )
        return super().__new__(cls, text)


class Username(str):
    """The configurable Linux development username, excluding management accounts."""

    def __new__(cls, value: object) -> Self:
        text = _text(value)
        if not re.fullmatch(r"[a-z_][a-z0-9_-]{0,31}", text) or text in (
            "root",
            "limanix-admin",
        ):
            raise DomainError(
                "expected a non-reserved Linux username of 1 to 32 characters"
            )
        return super().__new__(cls, text)


class GuestPath(str):
    """An absolute, normalized path supported by guest mount configuration.

    System-directory reservation and overlapping mounts are configuration policy,
    separate from this path's syntax and normalization.
    """

    def __new__(cls, value: object) -> Self:
        text = _text(value)
        if any(character in text for character in " \t\r\n"):
            raise DomainError(
                "guest mount paths cannot contain spaces, tabs, or line breaks "
                "because NixOS Lima does not escape them in fstab"
            )
        if not text.startswith("/") or text.startswith("//") or ".." in text.split("/"):
            raise DomainError("expected an absolute guest path without '..'")
        normalized = str(PurePosixPath(text))
        if normalized == "/":
            raise DomainError("the guest root directory is not allowed")
        return super().__new__(cls, normalized)


class ModuleName(str):
    """A catalog entry name shared by bundled and imported modules."""

    def __new__(cls, value: object) -> Self:
        text = _text(value)
        if len(text) > 63 or not re.fullmatch(r"[a-z][a-z0-9]*(?:-[a-z0-9]+)*", text):
            raise DomainError(
                "expected a module name of 1 to 63 lowercase letters, digits, "
                "and single hyphens, starting with a letter"
            )
        return super().__new__(cls, text)


class ModuleId(str):
    """A bundled module name or a third-party:NAME catalog reference."""

    def __new__(cls, value: object) -> Self:
        text = _text(value)
        ModuleName(text.removeprefix("third-party:"))
        return super().__new__(cls, text)

    @property
    def name(self) -> ModuleName:
        return ModuleName(self.removeprefix("third-party:"))

    @property
    def is_third_party(self) -> bool:
        return self.startswith("third-party:")


class EnvName(str):
    """A POSIX environment variable name."""

    def __new__(cls, value: object) -> Self:
        text = _text(value)
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", text):
            raise DomainError("expected a POSIX environment variable name")
        return super().__new__(cls, text)


class EnvValue(str):
    """A literal value supported by guest shell and systemd environment files."""

    def __new__(cls, value: object) -> Self:
        text = _text(value, empty=True)
        if any(
            ord(character) == 0xFEFF
            or 0xD800 <= ord(character) <= 0xDFFF
            or 0xFDD0 <= ord(character) <= 0xFDEF
            or ord(character) & 0xFFFF in (0xFFFE, 0xFFFF)
            for character in text
        ):
            raise DomainError(
                "contains a character unsupported by guest environment files"
            )
        return super().__new__(cls, text)


class Architecture(StrEnum):
    """Public guest architecture identifiers and their Lima equivalents."""

    ARM64 = "arm64"
    AMD64 = "amd64"

    @property
    def lima_arch(self) -> str:
        return {Architecture.ARM64: "aarch64", Architecture.AMD64: "x86_64"}[self]


class ByteSize(int):
    """A positive byte count, with whole GiB text at configuration boundaries."""

    def __new__(cls, value: int) -> Self:
        if isinstance(value, bool) or not isinstance(value, int) or value <= 0:
            raise DomainError("expected a positive integer byte count")
        return super().__new__(cls, value)

    @classmethod
    def parse(cls, value: object) -> Self:
        text = _text(value)
        if not re.fullmatch(r"[1-9][0-9]*GiB", text):
            raise DomainError("expected a positive whole GiB size, such as 8GiB")
        try:
            return cls(int(text.removesuffix("GiB")) * _GIB)
        except ValueError as error:
            raise DomainError(
                "size exceeds the supported integer parsing limit"
            ) from error

    def to_gib(self) -> str:
        if self % _GIB:
            raise DomainError("byte count cannot be represented as a whole GiB size")
        return f"{self // _GIB}GiB"
