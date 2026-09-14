"""Exercise the installed CLI outside the source checkout."""

import contextlib
import io
import json
import os
import subprocess
import sys
import tempfile
import textwrap
import tomllib
import unittest
from importlib.metadata import version
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from limanix.cli import main
from limanix.config import Config, config_to_dict
from limanix.config.template import render_config


class CliTests(unittest.TestCase):
    def run_cli(self, directory: Path, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [str(Path(sys.executable).with_name("limanix")), *args],
            cwd=directory,
            capture_output=True,
            text=True,
            check=False,
            env={**os.environ, "LIMANIX_HOME": str(directory / "state")},
        )

    def test_first_config_defaults_to_current_directory(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            result = self.run_cli(root, "first-config")
            self.assertEqual(result.returncode, 0, result.stderr)
            destination = root / "limanix.toml"
            self.assertEqual(list(root.iterdir()), [destination])
            self.assertEqual(
                tomllib.loads(destination.read_text(encoding="utf-8")),
                config_to_dict(Config()),
            )

    def test_first_config_accepts_relative_and_absolute_directories(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / "project configs"
            target.mkdir()
            for argument in (target.name, str(target)):
                with self.subTest(path=argument):
                    result = self.run_cli(root, "first-config", argument)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(
                        (target / "limanix.toml").read_text(encoding="utf-8"),
                        render_config(),
                    )
                    self.assertFalse((root / "limanix.toml").exists())

    def test_first_config_replaces_existing_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            destination = root / "limanix.toml"
            destination.write_text("# old configuration\n" * 1000, encoding="utf-8")
            result = self.run_cli(root, "first-config")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(destination.read_text(encoding="utf-8"), render_config())

    def test_first_config_reports_write_error(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / "file"
            target.write_text("keep", encoding="utf-8")
            result = self.run_cli(root, "first-config", str(target))
            self.assertEqual(result.returncode, 1)
            self.assertIn("path is not a directory", result.stderr)
            self.assertNotIn("Traceback", result.stderr)
            self.assertEqual(result.stdout, "")
            self.assertEqual(target.read_text(encoding="utf-8"), "keep")

    def test_help_and_version(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            result = self.run_cli(root, "first-config", "--help")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("[PATH]", result.stdout)
            self.assertIn("current directory", result.stdout)
            self.assertIn("overwritten", result.stdout)
            result = self.run_cli(root, "--version")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.strip(), version("limanix"))
            self.assertEqual(list(root.iterdir()), [])

    def test_module_commands_use_packaged_resources_and_imported_copy(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "custom-module"
            source.mkdir()
            (source / "default.nix").write_text("{ ... }: {}\n", encoding="utf-8")
            result = self.run_cli(root, "modules", "add", "custom", str(source))
            self.assertEqual(result.returncode, 0, result.stderr)
            (source / "default.nix").unlink()
            result = self.run_cli(root, "modules", "list", "--json")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                {entry["name"] for entry in json.loads(result.stdout)},
                {"git", "neovim", "rust", "third-party:custom"},
            )
            self.assertTrue((root / "state/modules/custom/default.nix").is_file())
            result = self.run_cli(root, "modules", "remove", "custom")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((root / "state/modules/custom").exists())

    def test_create_reports_invalid_config_before_accessing_lima(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = root / "invalid.toml"
            config.write_text('name = "../invalid"\n', encoding="utf-8")
            result = self.run_cli(root, "create", "--config", str(config))
            self.assertEqual(result.returncode, 1)
            self.assertIn("name", result.stderr)
            self.assertNotIn("Traceback", result.stderr)
            self.assertFalse((root / "state").exists())

    def test_module_list_reports_a_bad_entry_and_keeps_healthy_entries(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "state/modules/BAD ENTRY").mkdir(parents=True)
            result = self.run_cli(root, "modules", "list", "--json")
            self.assertEqual(result.returncode, 0, result.stderr)
            entries = {entry["name"]: entry for entry in json.loads(result.stdout)}
            self.assertEqual(
                set(entries), {"git", "rust", "neovim", "third-party:BAD ENTRY"}
            )
            self.assertIsNone(entries["git"]["error"])
            self.assertIn(
                "Invalid module name", entries["third-party:BAD ENTRY"]["error"]
            )
            result = self.run_cli(root, "modules", "list")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("third-party:BAD ENTRY", result.stdout)
            self.assertIn("Invalid module name", result.stdout)

    def test_delete_flags_are_dispatched_independently(self) -> None:
        for force in (False, True):
            for remove_home in (False, True):
                with (
                    self.subTest(force=force, remove_home=remove_home),
                    patch("limanix.cli.app.VMManager") as factory,
                    contextlib.redirect_stdout(io.StringIO()) as output,
                ):
                    factory.return_value.delete.return_value = "/owned/home"
                    arguments = ["delete", "sandbox"]
                    if force:
                        arguments.append("--force")
                    if remove_home:
                        arguments.append("--remove-home")
                    self.assertEqual(main(arguments), 0)
                    factory.return_value.delete.assert_called_once_with(
                        "sandbox", force=force, remove_home=remove_home
                    )
                    self.assertEqual(
                        "Preserved managed home" in output.getvalue(), not remove_home
                    )

    def test_update_dispatches_only_the_config_path(self) -> None:
        with (
            patch("limanix.cli.app.VMManager") as factory,
            contextlib.redirect_stdout(io.StringIO()),
        ):
            factory.return_value.update.return_value = SimpleNamespace(name="sandbox")
            self.assertEqual(main(["update", "--config", "sandbox.toml"]), 0)
            factory.return_value.update.assert_called_once_with(Path("sandbox.toml"))

    def test_update_warning_has_cli_prefix_without_failing_the_command(self) -> None:
        script = textwrap.dedent("""\
            import logging
            from types import SimpleNamespace
            from unittest.mock import patch
            from limanix.cli import main

            def update(config):
                logging.getLogger("limanix.vm").warning("Old generation retained.")
                return SimpleNamespace(name="sandbox")

            with patch("limanix.cli.app.VMManager") as factory:
                factory.return_value.update.side_effect = update
                raise SystemExit(main(["update", "--config", "sandbox.toml"]))
            """)
        with tempfile.TemporaryDirectory() as directory:
            result = subprocess.run(
                [sys.executable, "-c", script],
                cwd=directory,
                capture_output=True,
                text=True,
                check=False,
            )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "Updated sandbox.\n")
        self.assertEqual(result.stderr, "limanix: warning: Old generation retained.\n")


if __name__ == "__main__":
    unittest.main()
