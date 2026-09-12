"""Command-line entry point."""

import argparse
from collections.abc import Sequence
from importlib.metadata import version


def main(argv: Sequence[str] | None = None) -> int:
    """Run the Limanix command-line client."""
    parser = argparse.ArgumentParser(
        prog="limanix",
        description="Development sandboxes with Lima and NixOS.",
    )
    parser.add_argument("--version", action="version", version=version("limanix"))
    parser.parse_args(argv)
    print("Hello, world!")
    return 0
