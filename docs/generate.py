"""Render the example and configuration reference from the Python model."""

import argparse
import json
import re
import textwrap
from dataclasses import MISSING, fields, is_dataclass
from pathlib import Path
from typing import Any, Literal, get_args, get_origin, get_type_hints

from limanix.config import Config

PROJECT_ROOT = Path(__file__).resolve().parent.parent
EXAMPLE = PROJECT_ROOT / "limanix.example.toml"


def _toml(value: Any) -> str:
    if isinstance(value, str):
        return json.dumps(value, ensure_ascii=False).replace("\x7f", r"\u007f")
    if isinstance(value, bool):
        return str(value).lower()
    if isinstance(value, int):
        return str(value)
    if isinstance(value, list):
        return "[" + ", ".join(_toml(item) for item in value) + "]"
    raise TypeError(f"Unsupported example value: {type(value).__name__}")


def _key(name: str) -> str:
    return name if re.fullmatch(r"[A-Za-z0-9_-]+", name) else _toml(name)


def _comments(description: str) -> list[str]:
    return [f"# {line}" for line in textwrap.wrap(description, width=86)]


def _is_table_array(value: Any) -> bool:
    return isinstance(value, list) and bool(value) and is_dataclass(value[0])


def _has_table_entries(annotation: Any) -> bool:
    return get_origin(annotation) is list and is_dataclass(get_args(annotation)[0])


def _example_table(
    model: Any, path: str = "", description: str = "", *, array: bool = False
) -> list[str]:
    lines = []
    if path:
        lines.extend(["", f"[[{path}]]" if array else f"[{path}]"])
        lines.extend(_comments(description))
    for item in fields(model):
        value = getattr(model, item.name)
        if is_dataclass(value) or isinstance(value, dict) or _is_table_array(value):
            continue
        lines.extend(_comments(item.doc or ""))
        lines.append(f"{_key(item.name)} = {_toml(value)}")
    for item in fields(model):
        value = getattr(model, item.name)
        child = f"{path}.{_key(item.name)}" if path else _key(item.name)
        if is_dataclass(value):
            lines.extend(_example_table(value, child, item.doc or ""))
        elif isinstance(value, dict):
            lines.extend(["", f"[{child}]", *_comments(item.doc or "")])
            lines.extend(f"{_key(key)} = {_toml(val)}" for key, val in value.items())
        elif _is_table_array(value):
            for entry in value:
                lines.extend(_example_table(entry, child, item.doc or "", array=True))
    return lines


def _type_name(annotation: Any) -> str:
    origin = get_origin(annotation)
    args = get_args(annotation)
    if origin is Literal:
        return " or ".join(_toml(value) for value in args)
    if origin is list:
        return f"array[{_type_name(args[0])}]"
    if origin is dict:
        return f"table[{_type_name(args[0])}, {_type_name(args[1])}]"
    return {str: "string", int: "integer", bool: "boolean"}.get(
        annotation, annotation.__name__
    )


def _cell(value: str) -> str:
    return (
        value.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace("|", "&#124;")
        .replace("\n", " ")
    )


def _code(value: str) -> str:
    escaped = _cell(value)
    for char in "`*_[]\\":
        escaped = escaped.replace(char, f"&#{ord(char)};")
    return f"<code>{escaped}</code>"


def _reference_table(model: Any, path: str = "") -> list[str]:
    hints = get_type_hints(type(model))
    lines = [
        f"## `{path}`" if path else "## Top-level fields",
        "",
        "| Field | Type | Default | Description |",
        "| --- | --- | --- | --- |",
    ]
    for item in fields(model):
        value = getattr(model, item.name)
        if (
            is_dataclass(value)
            or isinstance(value, dict)
            or _has_table_entries(hints[item.name])
        ):
            continue
        lines.append(
            f"| {_code(item.name)} | {_code(_type_name(hints[item.name]))} "
            f"| {_code(_toml(value))} | {_cell(item.doc or '')} |"
        )
    lines.append("")
    for item in fields(model):
        value = getattr(model, item.name)
        child = f"{path}.{item.name}" if path else item.name
        if is_dataclass(value):
            lines.extend(_reference_table(value, child))
        elif isinstance(value, dict):
            lines.extend(
                [
                    f"## `{child}`",
                    "",
                    item.doc or "",
                    "",
                    f"Type: {_code(_type_name(hints[item.name]))}.",
                    "",
                    "| Default key | Default value |",
                    "| --- | --- |",
                ]
            )
            lines.extend(
                f"| {_code(key)} | {_code(_toml(val))} |" for key, val in value.items()
            )
            lines.append("")
        elif _has_table_entries(hints[item.name]):
            entry_type = get_args(hints[item.name])[0]
            entry_hints = get_type_hints(entry_type)
            entry_fields = fields(entry_type)
            lines.extend(
                [
                    f"## `{child}`",
                    "",
                    item.doc or "",
                    "",
                    "| Field | Type | Default per entry | Description |",
                    "| --- | --- | --- | --- |",
                ]
            )
            for entry_field in entry_fields:
                default = (
                    "Required"
                    if entry_field.default is MISSING
                    else _code(_toml(entry_field.default))
                )
                lines.append(
                    f"| {_code(entry_field.name)} "
                    f"| {_code(_type_name(entry_hints[entry_field.name]))} "
                    f"| {default} | {_cell(entry_field.doc or '')} |"
                )
            lines.extend(["", "### Default entries", ""])
            if not value:
                lines.extend(["Default: `[]`.", ""])
                continue
            lines.append("| " + " | ".join(_code(f.name) for f in entry_fields) + " |")
            lines.append("| " + " | ".join("---" for _ in entry_fields) + " |")
            for entry in value:
                lines.append(
                    "| "
                    + " | ".join(
                        _code(_toml(getattr(entry, f.name))) for f in entry_fields
                    )
                    + " |"
                )
            lines.append("")
    return lines


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="Check the saved example.")
    args = parser.parse_args()
    expected = render_example()
    if args.check:
        if not EXAMPLE.is_file() or EXAMPLE.read_text() != expected:
            parser.exit(1, "Example differs from the model. Run task docs/generate.\n")
        print("Example matches the configuration model.")
    else:
        EXAMPLE.write_text(expected)
        print(f"Generated {EXAMPLE.name} from limanix.config.Config.")
    return 0


def render_example(config: Config | None = None) -> str:
    """Return a commented TOML example with the model's defaults."""
    config = config if config is not None else Config()
    header = [
        "# Default Limanix configuration for a development sandbox.",
        "# Regenerate this file with: task docs/generate",
        "# Override these values in your own configuration when needed.",
        "#",
        "# Usage:",
        "#   mkdir -p vm",
        "#   cp limanix.example.toml vm/my-vm.toml",
        "#   limanix create --config vm/my-vm.toml",
        "",
    ]
    return "\n".join(header + _example_table(config)) + "\n"


def render_reference(config: Config | None = None) -> str:
    """Return Markdown field tables from types, defaults, and field descriptions."""
    config = config if config is not None else Config()
    return "\n".join(_reference_table(config))


if __name__ == "__main__":
    raise SystemExit(main())
