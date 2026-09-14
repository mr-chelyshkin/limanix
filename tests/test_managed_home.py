"""Verify host-owned home allocation and deletion boundaries."""

import stat
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path

from limanix.domain import Architecture, GuestPath, Username, VMName
from limanix.managed_home import ManagedHome, ManagedHomeError
from limanix.state import Identity, Instance, InstanceStatus, StateStore


class ManagedHomeTests(unittest.TestCase):
    def identity(self, directory: Path) -> Identity:
        root = directory.resolve() / "homes"
        return Identity(
            name=VMName("sandbox"),
            id="012345abcdef",
            arch=Architecture.ARM64,
            username=Username("dev"),
            user_home=GuestPath("/home/dev"),
            home_root=str(root),
            home=str(root / "sandbox-012345abcdef"),
            created_at="2026-09-13T12:00:00+00:00",
        )

    def test_create_private_empty_home_and_remove_using_host_identity(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            identity = self.identity(root)
            manager = ManagedHome()
            home = manager.create(identity)
            self.assertEqual(home, Path(identity.home))
            self.assertEqual(stat.S_IMODE(home.stat().st_mode), 0o700)
            self.assertEqual(list(home.iterdir()), [])
            (home / "work").mkdir()
            (home / "work/service.rs").write_text("fn main() {}")
            manager.remove(identity)
            self.assertFalse(home.exists())
            self.assertTrue(Path(identity.home_root).is_dir())
            manager.remove(identity)

    def test_preexisting_home_is_never_adopted_or_changed(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            identity = self.identity(Path(directory))
            home = Path(identity.home)
            home.mkdir(parents=True)
            sentinel = home / "keep"
            sentinel.write_text("original")
            with self.assertRaises(ManagedHomeError):
                ManagedHome().create(identity)
            self.assertEqual(sentinel.read_text(), "original")

    def test_forged_guest_marker_cannot_change_delete_target(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            identity = self.identity(root)
            manager = ManagedHome()
            home = manager.create(identity)
            outside = root / "outside"
            outside.mkdir()
            keep = outside / "keep"
            keep.write_text("original")
            (home / ".limanix-owner.json").write_text(
                '{"home": "' + str(outside) + '"}'
            )
            (home / "outside-link").symlink_to(outside, target_is_directory=True)
            manager.remove(identity)
            self.assertEqual(keep.read_text(), "original")
            self.assertFalse(home.exists())

    def test_corrupt_mutable_record_does_not_prevent_owned_home_removal(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            identity = self.identity(root)
            manager = ManagedHome()
            home = manager.create(identity)
            store = StateStore(root / "state")
            store.save(
                Instance(
                    identity=identity,
                    generation="abcdef012345",
                    status=InstanceStatus.READY,
                )
            )
            (store.instance_dir(identity.name) / "instance.json").write_text("{}")
            manager.remove(store.load_identity(identity.name))
            self.assertFalse(home.exists())

    def test_mismatched_root_name_or_id_does_not_remove_other_data(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            identity = self.identity(root)
            home = ManagedHome().create(identity)
            keep = home / "keep"
            keep.write_text("original")
            for changed in (
                replace(identity, home=str(root.resolve())),
                replace(identity, home_root=str(home)),
                replace(identity, name=VMName("other")),
                replace(identity, id="abcdef012345"),
            ):
                with (
                    self.subTest(identity=changed),
                    self.assertRaises(ManagedHomeError),
                ):
                    ManagedHome().remove(changed)
                self.assertEqual(keep.read_text(), "original")

    def test_symlink_home_and_parent_redirect_are_refused(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            identity = self.identity(root)
            outside = root / "outside"
            outside.mkdir()
            keep = outside / "keep"
            keep.write_text("original")
            Path(identity.home_root).mkdir()
            home = Path(identity.home)
            home.symlink_to(outside, target_is_directory=True)
            for method in (ManagedHome().create, ManagedHome().remove):
                with (
                    self.subTest(method=method),
                    self.assertRaisesRegex(ManagedHomeError, "symbolic link"),
                ):
                    method(identity)
            home.unlink()
            Path(identity.home_root).rmdir()
            Path(identity.home_root).symlink_to(outside, target_is_directory=True)
            for method in (ManagedHome().create, ManagedHome().remove):
                with (
                    self.subTest(method=method),
                    self.assertRaisesRegex(ManagedHomeError, "symbolic link"),
                ):
                    method(identity)
            self.assertEqual(keep.read_text(), "original")

    def test_non_directory_home_is_not_unlinked(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            identity = self.identity(Path(directory))
            home = Path(identity.home)
            home.parent.mkdir()
            home.write_text("keep")
            with self.assertRaisesRegex(ManagedHomeError, "not a directory"):
                ManagedHome().remove(identity)
            self.assertEqual(home.read_text(), "keep")


if __name__ == "__main__":
    unittest.main()
