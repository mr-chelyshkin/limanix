"""Create configuration files from the default model."""

from pathlib import Path

from limanix.config.template import render_config
from limanix.filesystem import write_text_atomic


def write_default_config(directory: Path) -> Path:
    """Write or replace limanix.toml in an existing directory.

    Return its absolute path.
    Raise FilesystemError for invalid paths or I/O failures.
    """
    return write_text_atomic(directory / "limanix.toml", render_config())
