"""Render the default configuration as commented TOML."""

import textwrap
from dataclasses import fields, is_dataclass
from typing import Any

import tomlkit
from tomlkit.items import Table

from limanix.config.models import Config, config_to_dict


def _comments(target: Table | tomlkit.TOMLDocument, description: str) -> None:
    for line in textwrap.wrap(description, width=86):
        target.add(tomlkit.comment(line))


def _is_table_array(value: Any) -> bool:
    return isinstance(value, list) and bool(value) and is_dataclass(value[0])


def _fill(
    target: Table | tomlkit.TOMLDocument, model: Any, data: dict[str, Any]
) -> None:
    for item in fields(model):
        value = getattr(model, item.name)
        if is_dataclass(value) or isinstance(value, dict) or _is_table_array(value):
            continue
        _comments(target, item.doc or "")
        target.add(item.name, tomlkit.item(data[item.name]))
    for item in fields(model):
        value = getattr(model, item.name)
        if is_dataclass(value):
            target.add(item.name, _table(value, item.doc or "", data[item.name]))
        elif isinstance(value, dict):
            section = tomlkit.table()
            _comments(section, item.doc or "")
            section.update(data[item.name])
            target.add(item.name, section)
        elif _is_table_array(value):
            entries = tomlkit.aot()
            for entry, encoded in zip(value, data[item.name], strict=True):
                entries.append(_table(entry, item.doc or "", encoded))
            target.add(item.name, entries)


def _table(model: Any, description: str, data: dict[str, Any]) -> Table:
    section = tomlkit.table()
    _comments(section, description)
    _fill(section, model, data)
    return section


def render_config(config: Config | None = None) -> str:
    """Return commented TOML using the model's defaults unless a config is given."""
    config = config if config is not None else Config()
    document = tomlkit.document()
    _comments(document, "Default Limanix configuration for a development sandbox.")
    _comments(document, "Edit these values to match your environment.")
    document.add(tomlkit.nl())
    _fill(document, config, config_to_dict(config))
    return tomlkit.dumps(document)
