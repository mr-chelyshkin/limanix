"""Define the CLI arguments used by execution, help, and documentation."""

import argparse
from importlib.metadata import version
from pathlib import Path


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
    update = commands.add_parser(
        "update", help="Apply a configuration and restart an existing VM."
    )
    update.add_argument("--config", metavar="PATH", type=Path, required=True)
    listing = commands.add_parser("list", help="List VMs managed by Limanix.")
    listing.add_argument(
        "--json", action="store_true", help="Print machine-readable JSON."
    )
    for name, help_text in (
        ("start", "Start an existing VM."),
        ("stop", "Stop a VM and preserve its disk and home."),
        ("delete", "Delete a VM; preserve its managed host home by default."),
    ):
        command = commands.add_parser(name, help=help_text)
        command.add_argument("name", metavar="NAME")
        if name == "delete":
            command.add_argument(
                "--force",
                action="store_true",
                help="Force Lima to stop and delete the VM.",
            )
            command.add_argument(
                "--remove-home",
                action="store_true",
                help="Also remove this VM's managed host home and its contents.",
            )
    shell = commands.add_parser(
        "shell", help="Connect as the configured development user."
    )
    shell.add_argument("name", metavar="NAME")
    shell.add_argument("args", metavar="COMMAND", nargs=argparse.REMAINDER)
    modules = commands.add_parser(
        "modules", help="Manage bundled and third-party NixOS modules."
    )
    module_commands = modules.add_subparsers(dest="module_command", required=True)
    module_list = module_commands.add_parser(
        "list", help="List available module identifiers."
    )
    module_list.add_argument("--json", action="store_true")
    module_add = module_commands.add_parser(
        "add", help="Import a directory containing default.nix."
    )
    module_add.add_argument("name", metavar="NAME")
    module_add.add_argument("path", metavar="DIRECTORY", type=Path)
    module_remove = module_commands.add_parser(
        "remove", help="Remove an imported module from the catalog."
    )
    module_remove.add_argument("name", metavar="NAME")
    return parser
