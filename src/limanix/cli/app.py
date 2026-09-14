"""Dispatch CLI commands and present their results and errors."""

import json
import logging
import sys
from collections.abc import Sequence
from dataclasses import asdict

from limanix.cli.parser import build_parser
from limanix.config.files import write_default_config
from limanix.config.parser import ConfigError
from limanix.domain import DomainError
from limanix.filesystem import FilesystemError
from limanix.lima import LimaError
from limanix.managed_home import ManagedHomeError
from limanix.modules import ModuleError, ModuleRegistry
from limanix.state import StateError, StateStore
from limanix.vm import VMError, VMManager


def main(argv: Sequence[str] | None = None) -> int:
    """Run the Limanix command-line client."""
    logging.basicConfig(format="limanix: warning: %(message)s")
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        if args.command == "first-config":
            destination = write_default_config(args.path)
            print(f"Wrote {destination}.")
            return 0
        if args.command == "modules":
            registry = ModuleRegistry(StateStore())
            if args.module_command == "list":
                modules = registry.available()
                if args.json:
                    print(json.dumps([asdict(module) for module in modules], indent=2))
                else:
                    for module in modules:
                        detail = (
                            f"error: {module.error}"
                            if module.error is not None
                            else module.description
                        )
                        print(f"{module.name:<30} {detail}")
            elif args.module_command == "add":
                registry.add(args.name, args.path)
                print(f"Imported third-party:{args.name}.")
            elif args.module_command == "remove":
                registry.remove(args.name)
                print(f"Removed third-party:{args.name} from the catalog.")
            return 0
        manager = VMManager()
        if args.command == "create":
            instance = manager.create(args.config)
            print(f"Created {instance.name}. Managed home: {instance.identity.home}")
        elif args.command == "update":
            instance = manager.update(args.config)
            print(f"Updated {instance.name}.")
        elif args.command == "list":
            entries = manager.fetch_all()
            if args.json:
                print(json.dumps([asdict(entry) for entry in entries], indent=2))
            elif entries:
                print(f"{'NAME':<24} {'STATUS':<12} {'STATE':<12} ADDRESS")
                for entry in entries:
                    status = (
                        entry.status.value if entry.status is not None else "Missing"
                    )
                    state = entry.state.value if entry.state is not None else "corrupt"
                    print(
                        f"{entry.name:<24} {status:<12} "
                        f"{state:<12} {entry.address or '-'}"
                    )
                    if entry.error:
                        print(f"  {entry.error}")
            else:
                print("No VMs managed by Limanix.")
        elif args.command == "start":
            manager.start(args.name)
            print(f"Started {args.name}.")
        elif args.command == "stop":
            manager.stop(args.name)
            print(f"Stopped {args.name}.")
        elif args.command == "delete":
            home = manager.delete(
                args.name, force=args.force, remove_home=args.remove_home
            )
            print(f"Deleted {args.name}.")
            if not args.remove_home:
                print(f"Preserved managed home: {home}")
        elif args.command == "shell":
            command = args.args[1:] if args.args[:1] == ["--"] else args.args
            return manager.shell(args.name, command)
        else:
            parser.print_help()
        return 0
    except (
        ConfigError,
        DomainError,
        FilesystemError,
        LimaError,
        ManagedHomeError,
        ModuleError,
        StateError,
        VMError,
    ) as error:
        parser.exit(1, f"limanix: {error}\n")
    except OSError as error:
        parser.exit(
            1, f"limanix: filesystem operation failed: {error.strerror or error}\n"
        )
    except KeyboardInterrupt:
        print("limanix: interrupted.", file=sys.stderr)
        return 130
