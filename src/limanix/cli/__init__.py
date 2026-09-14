"""Public CLI entry point and parser factory."""

from limanix.cli.app import main
from limanix.cli.parser import build_parser

__all__ = ["build_parser", "main"]
