"""Exercise the installed CLI outside the source checkout."""

import subprocess
import sys
import tempfile
import tomllib
import unittest
from dataclasses import asdict
from importlib.metadata import version
from pathlib import Path

from limanix.config import Config
from limanix.config_template import render_config


class CliTests(unittest.TestCase):
    def run_cli(self, directory: Path, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [str(Path(sys.executable).with_name("limanix")), *args],
            cwd=directory,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_first_config_defaults_to_current_directory(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            result = self.run_cli(root, "first-config")
            self.assertEqual(result.returncode, 0, result.stderr)
            destination = root / "limanix.toml"
            self.assertEqual(list(root.iterdir()), [destination])
            self.assertEqual(
                tomllib.loads(destination.read_text(encoding="utf-8")), asdict(Config())
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


if __name__ == "__main__":
    unittest.main()
