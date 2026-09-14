"""Exercise bundled resources and complete, independent third-party snapshots."""

import contextlib
import io
import json
import os
import select
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from limanix.cli.app import main
from limanix.config import Config
from limanix.domain import ModuleId
from limanix.modules import ModuleError, ModuleRegistry, copy_module_tree
from limanix.nixos import prepare_bundle
from limanix.state import StateStore


class ModuleRegistryTests(unittest.TestCase):
    def make_source(self, root: Path) -> Path:
        source = root / "third-party-source"
        source.mkdir()
        (source / "default.nix").write_text(
            "{ ... }: { imports = [ ./parts/editor.nix ]; }\n", encoding="utf-8"
        )
        parts = source / "parts"
        parts.mkdir()
        (parts / "editor.nix").write_text(
            "{ ... }: { programs.neovim.enable = true; }\n", encoding="utf-8"
        )
        (parts / "editor.conf").write_text("set number\n", encoding="utf-8")
        return source

    def test_builtin_catalog_and_selected_sources_work_outside_checkout(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            registry = ModuleRegistry(StateStore(root / "state"))
            catalog = registry.available()
            self.assertEqual(
                {entry.name for entry in catalog}, {"git", "rust", "neovim"}
            )
            self.assertTrue(all(entry.source == "bundled" for entry in catalog))
            self.assertTrue(all(entry.description for entry in catalog))
            with registry.sources([ModuleId("neovim"), ModuleId("git")]) as paths:
                self.assertEqual(len(paths), 2)
                self.assertIn("programs.neovim.enable", paths[0].read_text())
                self.assertIn("programs.git.enable", paths[1].read_text())
            self.assertEqual(list((registry.store.root / "modules").iterdir()), [])

    def test_damaged_catalog_entries_do_not_hide_healthy_modules(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            registry = ModuleRegistry(StateStore(root / "state"))
            source = self.make_source(root)
            registry.add("healthy", source)
            modules = registry.store.root / "modules"
            (modules / "BadName").mkdir()
            (modules / "BadName/default.nix").write_text("{}")
            (modules / "missing-entry").mkdir()
            (modules / "broken-link").symlink_to(root / "absent")
            (modules / "entry-link").mkdir()
            (modules / "entry-link/default.nix").symlink_to(source / "default.nix")
            (modules / "stray-file").write_text("unrelated")
            (modules / ".hidden-import").mkdir()

            catalog = {entry.name: entry for entry in registry.available()}
            healthy = {"git", "rust", "neovim", "third-party:healthy"}
            damaged = {
                "third-party:BadName",
                "third-party:missing-entry",
                "third-party:broken-link",
                "third-party:entry-link",
                "third-party:stray-file",
            }
            self.assertEqual(set(catalog), healthy | damaged)
            self.assertTrue(all(catalog[name].error is None for name in healthy))
            self.assertTrue(all(catalog[name].error for name in damaged))
            for name in damaged - {"third-party:BadName"}:
                with (
                    self.subTest(name=name),
                    self.assertRaises(ModuleError),
                    registry.sources([ModuleId(name)]),
                ):
                    pass
            with registry.sources([ModuleId("third-party:healthy")]) as paths:
                self.assertTrue(paths[0].is_file())

            with (
                patch("limanix.cli.app.StateStore", return_value=registry.store),
                contextlib.redirect_stdout(io.StringIO()) as output,
            ):
                self.assertEqual(main(["modules", "list"]), 0)
            self.assertIn("third-party:healthy", output.getvalue())
            for name in damaged:
                self.assertIn(
                    f"{name:<30} error: {catalog[name].error}", output.getvalue()
                )

            with (
                patch("limanix.cli.app.StateStore", return_value=registry.store),
                contextlib.redirect_stdout(io.StringIO()) as output,
            ):
                self.assertEqual(main(["modules", "list", "--json"]), 0)
            records = {entry["name"]: entry for entry in json.loads(output.getvalue())}
            self.assertEqual(set(records), set(catalog))
            self.assertTrue(all(records[name]["error"] is None for name in healthy))
            self.assertTrue(all(records[name]["error"] for name in damaged))

    def test_third_party_tree_retains_relative_imports_and_independent_snapshots(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = self.make_source(root)
            registry = ModuleRegistry(StateStore(root / "state"))
            registry.add("my-editor", source)
            imported = registry.store.root / "modules" / "my-editor"
            self.assertEqual(
                (imported / "default.nix").read_bytes(),
                (source / "default.nix").read_bytes(),
            )
            self.assertEqual(
                (imported / "parts/editor.conf").read_text(), "set number\n"
            )
            (source / "parts/editor.conf").write_text(
                "set nonumber\n", encoding="utf-8"
            )
            with registry.sources([ModuleId("third-party:my-editor")]) as paths:
                self.assertEqual(paths, (imported / "default.nix",))
                flake = prepare_bundle(Config(), root / "runtime", paths, uid=501)
            copied = flake / "modules" / "0000"
            self.assertEqual((copied / "parts/editor.conf").read_text(), "set number\n")
            self.assertEqual(
                (copied / "parts/editor.nix").read_bytes(),
                (imported / "parts/editor.nix").read_bytes(),
            )
            self.assertIn("./parts/editor.nix", (copied / "default.nix").read_text())
            (imported / "parts/editor.conf").write_text(
                "changed catalog\n", encoding="utf-8"
            )
            self.assertEqual((copied / "parts/editor.conf").read_text(), "set number\n")
            self.assertIn(
                "third-party:my-editor", {item.name for item in registry.available()}
            )

    def test_duplicate_import_does_not_replace_existing_tree(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = self.make_source(root)
            registry = ModuleRegistry(StateStore(root / "state"))
            registry.add("my-editor", source)
            (source / "default.nix").write_text("changed\n", encoding="utf-8")
            with self.assertRaisesRegex(ModuleError, "already exists"):
                registry.add("my-editor", source)
            imported = registry.store.root / "modules/my-editor/default.nix"
            self.assertIn("./parts/editor.nix", imported.read_text())

    def test_remove_preserves_existing_vm_snapshot(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = self.make_source(root)
            registry = ModuleRegistry(StateStore(root / "state"))
            registry.add("my-editor", source)
            with registry.sources([ModuleId("third-party:my-editor")]) as paths:
                flake = prepare_bundle(Config(), root / "runtime", paths, uid=501)
            entry = flake / "modules" / "0000" / "default.nix"
            content = entry.read_bytes()
            registry.remove("my-editor")
            self.assertEqual(entry.read_bytes(), content)
            self.assertTrue((entry.parent / "parts/editor.nix").is_file())
            self.assertTrue(source.is_dir())
            self.assertNotIn(
                "third-party:my-editor", {item.name for item in registry.available()}
            )
            with (
                self.assertRaises(ModuleError),
                registry.sources([ModuleId("third-party:my-editor")]),
            ):
                pass

    def test_unknown_modules_and_missing_imports_fail_explicitly(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            registry = ModuleRegistry(StateStore(root / "state"))
            for name in ("unknown", "third-party:missing"):
                with (
                    self.subTest(name=name),
                    self.assertRaises(ModuleError),
                    registry.sources([ModuleId(name)]),
                ):
                    pass
            with self.assertRaises(ModuleError):
                registry.remove("missing")
            with self.assertRaises(ModuleError):
                registry.add("missing", root / "does-not-exist")

    def test_module_requires_default_nix_and_rejects_links_and_special_files(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            registry = ModuleRegistry(StateStore(root / "state"))
            source = self.make_source(root)
            cases = ["file-link", "directory-link", "entry-link", "fifo", "no-entry"]
            for kind in cases:
                with self.subTest(kind=kind):
                    bad = root / kind
                    bad.mkdir()
                    (bad / "default.nix").write_text("{}\n", encoding="utf-8")
                    if kind == "file-link":
                        (bad / "linked.nix").symlink_to(source / "default.nix")
                    elif kind == "directory-link":
                        (bad / "parts").symlink_to(
                            source / "parts", target_is_directory=True
                        )
                    elif kind == "entry-link":
                        (bad / "default.nix").unlink()
                        (bad / "default.nix").symlink_to(source / "default.nix")
                    elif kind == "fifo":
                        os.mkfifo(bad / "pipe")
                    else:
                        (bad / "default.nix").unlink()
                    with self.assertRaises(ModuleError):
                        registry.add(kind, bad)
                    self.assertFalse((registry.store.root / "modules" / kind).exists())
                    self.assertEqual(
                        list(
                            (registry.store.root / "modules").glob(f".{kind}.import-*")
                        ),
                        [],
                    )

    def test_symlink_source_directory_produces_an_independent_snapshot(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = self.make_source(root)
            link = root / "source-link"
            link.symlink_to(source, target_is_directory=True)
            registry = ModuleRegistry(StateStore(root / "state"))
            registry.add("linked", link)
            imported = registry.store.root / "modules/linked"
            self.assertFalse(imported.is_symlink())
            self.assertEqual(
                (imported / "default.nix").read_bytes(),
                (source / "default.nix").read_bytes(),
            )
            link.unlink()
            (source / "default.nix").write_text("changed\n", encoding="utf-8")
            self.assertIn("./parts/editor.nix", (imported / "default.nix").read_text())

    def test_self_recursive_destination_is_rejected_before_copy_starts(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = self.make_source(root)
            destination = source / "nested" / "snapshot"
            with patch("limanix.modules.shutil.copytree") as copy:
                with self.assertRaises(ModuleError):
                    copy_module_tree(source, destination)
                copy.assert_not_called()
            self.assertFalse(destination.exists())

    def test_sources_allows_catalog_readers_and_holds_off_removal(self) -> None:
        reader_script = """
import sys
from pathlib import Path
from limanix.modules import ModuleRegistry
from limanix.state import StateStore
catalog = ModuleRegistry(StateStore(Path(sys.argv[1]))).available()
for item in catalog:
    print(item.name)
"""
        writer_script = """
import fcntl
import sys
from pathlib import Path
from unittest.mock import patch
from limanix.modules import ModuleRegistry
from limanix.state import StateStore
real_flock = fcntl.flock
reported = False
def observe_contention(descriptor, operation):
    global reported
    try:
        return real_flock(descriptor, operation)
    except BlockingIOError:
        if not reported:
            print('blocked', flush=True)
            reported = True
        raise
with patch('limanix.state.fcntl.flock', side_effect=observe_contention):
    ModuleRegistry(StateStore(Path(sys.argv[1]))).remove('my-editor')
print('removed', flush=True)
"""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            registry = ModuleRegistry(StateStore(root / "state"))
            registry.add("my-editor", self.make_source(root))
            writer = None
            try:
                with registry.sources([ModuleId("third-party:my-editor")]) as paths:
                    reader = subprocess.run(
                        [sys.executable, "-c", reader_script, str(registry.store.root)],
                        capture_output=True,
                        text=True,
                        timeout=5,
                    )
                    self.assertEqual(reader.returncode, 0, reader.stderr)
                    self.assertIn("third-party:my-editor", reader.stdout.splitlines())
                    writer = subprocess.Popen(
                        [sys.executable, "-c", writer_script, str(registry.store.root)],
                        stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE,
                        text=True,
                    )
                    assert writer.stdout is not None
                    ready, _, _ = select.select([writer.stdout], [], [], 5)
                    self.assertTrue(ready, "Remover did not attempt the registry lock")
                    self.assertEqual(writer.stdout.readline().strip(), "blocked")
                    self.assertIn("./parts/editor.nix", paths[0].read_text())
                stdout, stderr = writer.communicate(timeout=5)
                self.assertEqual(writer.returncode, 0, stderr)
                self.assertEqual(stdout.strip(), "removed")
                self.assertFalse((registry.store.root / "modules/my-editor").exists())
            finally:
                if writer is not None:
                    if writer.poll() is None:
                        writer.kill()
                    writer.communicate(timeout=5)


if __name__ == "__main__":
    unittest.main()
