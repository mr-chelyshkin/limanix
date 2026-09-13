"""Command-line entry point."""

import argparse
from collections.abc import Sequence
from importlib.metadata import version
from pathlib import Path

from limanix.config_files import write_default_config
from limanix.filesystem import FilesystemError


def build_parser() -> argparse.ArgumentParser:
    """Describe the Limanix commands and their arguments."""
    parser = argparse.ArgumentParser(
        prog="limanix",
        description="Development sandboxes with Lima and NixOS.",
    )
    parser.add_argument("--version", action="version", version=version("limanix"))
    commands = parser.add_subparsers(dest="command", title="commands")
    first_config = commands.add_parser(
        "first-config",
        help="Write the default limanix.toml.",
        description=(
            "Write the default limanix.toml. The existing file is overwritten. "
            "Configuration symlinks are rejected."
        ),
    )
    first_config.add_argument(
        "path",
        metavar="PATH",
        type=Path,
        nargs="?",
        default=Path("."),
        help="Existing destination directory (default: current directory).",
    )
    create = commands.add_parser(
        "create",
        help="Create a development sandbox from a TOML configuration.",
        description="Create a development sandbox from a TOML configuration.",
    )
    create.add_argument(
        "--config",
        metavar="PATH",
        type=Path,
        required=True,
        help="Path to the VM configuration file.",
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    """Run the Limanix command-line client."""
    parser = build_parser()
    args = parser.parse_args(argv)
    if args.command == "first-config":
        try:
            destination = write_default_config(args.path)
        except FilesystemError as error:
            parser.exit(1, f"limanix: {error}\n")
        print(f"Wrote {destination}.")
        return 0
    if args.command == "create":
        parser.exit(2, "limanix: VM creation is not implemented.\n")
    parser.print_help()
    return 0
