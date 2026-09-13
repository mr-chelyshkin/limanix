"""Render the default configuration as commented TOML."""

import textwrap
from dataclasses import fields, is_dataclass
from typing import Any

import tomlkit
from tomlkit.items import Table

from limanix.config import Config


def _comments(target: Table | tomlkit.TOMLDocument, description: str) -> None:
    for line in textwrap.wrap(description, width=86):
        target.add(tomlkit.comment(line))


def _is_table_array(value: Any) -> bool:
    return isinstance(value, list) and bool(value) and is_dataclass(value[0])


def _fill(target: Table | tomlkit.TOMLDocument, model: Any) -> None:
    for item in fields(model):
        value = getattr(model, item.name)
        if is_dataclass(value) or isinstance(value, dict) or _is_table_array(value):
            continue
        _comments(target, item.doc or "")
        target.add(item.name, tomlkit.item(value))
    for item in fields(model):
        value = getattr(model, item.name)
        if is_dataclass(value):
            target.add(item.name, _table(value, item.doc or ""))
        elif isinstance(value, dict):
            section = tomlkit.table()
            _comments(section, item.doc or "")
            section.update(value)
            target.add(item.name, section)
        elif _is_table_array(value):
            entries = tomlkit.aot()
            for entry in value:
                entries.append(_table(entry, item.doc or ""))
            target.add(item.name, entries)


def _table(model: Any, description: str) -> Table:
    section = tomlkit.table()
    _comments(section, description)
    _fill(section, model)
    return section


def render_config(config: Config | None = None) -> str:
    """Return commented TOML using the model's defaults unless a config is given."""
    config = config if config is not None else Config()
    document = tomlkit.document()
    _comments(document, "Default Limanix configuration for a development sandbox.")
    _comments(document, "Edit these values to match your environment.")
    document.add(tomlkit.nl())
    _fill(document, config)
    return tomlkit.dumps(document)
