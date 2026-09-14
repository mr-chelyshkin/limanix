"""Exercise package resources and isolated guest configuration snapshots."""

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from typing import cast

from limanix.config import Config, NixOS
from limanix.domain import DomainError, EnvName, EnvValue, ModuleId, VMName
from limanix.modules import ModuleRegistry
from limanix.nixos import builtin_modules, prepare_bundle
from limanix.state import StateStore


class NixOSBundleTests(unittest.TestCase):
    def test_bundled_modules_copy_outside_the_package(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.assertEqual(set(builtin_modules()), {"git", "neovim", "rust"})
            registry = ModuleRegistry(StateStore(root / "state"))
            identifiers = [ModuleId(name) for name in builtin_modules()]
            with registry.sources(identifiers) as paths:
                flake = prepare_bundle(Config(), root / "runtime", paths, uid=501)
                expected = [path.read_text() for path in paths]
            for index, content in enumerate(expected):
                self.assertEqual(
                    (flake / "modules" / f"{index:04d}" / "default.nix").read_text(),
                    content,
                )

    def test_bundle_preserves_module_trees_and_excludes_environment(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            module = root / "registry" / "custom"
            (module / "nested").mkdir(parents=True)
            (module / "default.nix").write_text(
                "{ ... }: { imports = [ ./nested/feature.nix ]; }\n"
            )
            (module / "nested" / "feature.nix").write_text("{ ... }: {}\n")
            secret = "test-secret-absent-from-flake"
            config = Config(
                nixos=NixOS(modules=[ModuleId("custom")]),
                env={EnvName("TOKEN"): EnvValue(secret)},
            )
            flake = prepare_bundle(
                config, root / "runtime", [module / "default.nix"], uid=501
            )
            runtime = json.loads((flake / "runtime.json").read_text())
            self.assertEqual(runtime["modules"], ["modules/0000/default.nix"])
            self.assertEqual(runtime["user"]["uid"], 501)
            self.assertNotIn("gid", runtime["user"])
            self.assertNotIn("env", runtime)
            for path in flake.rglob("*"):
                if path.is_file():
                    self.assertNotIn(secret, path.read_text())
            imported = flake / "modules" / "0000" / "nested" / "feature.nix"
            self.assertEqual(imported.read_text(), "{ ... }: {}\n")
            (module / "nested" / "feature.nix").write_text("changed\n")
            self.assertEqual(imported.read_text(), "{ ... }: {}\n")
            self.assertEqual(
                (root / "runtime" / "environment").stat().st_mode & 0o777, 0o600
            )
            lock = json.loads((flake / "flake.lock").read_text())
            self.assertEqual(lock["nodes"]["nixos-lima"]["original"]["ref"], "v0.2.1")
            self.assertNotIn("runtimeSpec", lock["nodes"])

    def test_environment_preserves_literal_values_in_a_real_shell(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            values = {
                "LIMANIX_TEST_EMPTY": "",
                "LIMANIX_TEST_QUOTES": "single ' and double \"",
                "LIMANIX_TEST_LITERAL": "$(touch injected) `touch injected` $HOME",
                "LIMANIX_TEST_SPACES": "  a\tb  ",
                "LIMANIX_TEST_LINES": "line one\nline two\r\nend\n",
                "LIMANIX_TEST_BACKSLASH": "a\\b\\n\\\nlast\\",
                "LIMANIX_TEST_UNICODE": "Привет 🐍",
            }
            config = Config(
                env={EnvName(name): EnvValue(value) for name, value in values.items()}
            )
            prepare_bundle(config, root / "runtime", [], uid=501)
            shell = root / "runtime" / "environment.sh"
            script = '. "$1"; /usr/bin/env -0'
            result = subprocess.run(
                ["/bin/sh", "-c", script, "sh", str(shell)],
                cwd=root,
                env={"PATH": os.defpath},
                check=True,
                capture_output=True,
            )
            actual = dict(
                item.decode().split("=", 1)
                for item in result.stdout.split(b"\0")
                if item
            )
            self.assertEqual({key: actual[key] for key in values}, values)
            self.assertFalse((root / "injected").exists())
            service = (root / "runtime" / "environment").read_text()
            self.assertIn('LIMANIX_TEST_EMPTY=""\n', service)
            self.assertIn('LIMANIX_TEST_QUOTES="single \' and double \\""\n', service)

    def test_invalid_environment_is_rejected_before_writing(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for env in ({"X;touch pwned": "x"}, {"X": "nul\0value"}, {"X": "\ufeff"}):
                # Bypass constructors to test validation at the rendering boundary.
                invalid_environment = cast(dict[EnvName, EnvValue], env)
                with self.subTest(env=env), self.assertRaises(DomainError):
                    prepare_bundle(
                        Config(env=invalid_environment), root / "runtime", [], uid=501
                    )
                self.assertFalse((root / "runtime").exists())

    def test_existing_bundle_is_not_overwritten(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            flake = prepare_bundle(Config(), root, [], uid=501)
            original = (flake / "runtime.json").read_text()
            with self.assertRaises(FileExistsError):
                prepare_bundle(Config(name=VMName("new")), root, [], uid=501)
            self.assertEqual((flake / "runtime.json").read_text(), original)


if __name__ == "__main__":
    unittest.main()
