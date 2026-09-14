"""Exercise interrupted preparation and atomic metadata commit boundaries."""

import tempfile
import unittest
from pathlib import Path
from unittest.mock import create_autospec, patch

from limanix.config import Config, Home
from limanix.config.template import render_config
from limanix.lima import LimaClient, LimaInstance, LimaStatus
from limanix.state import Instance, StateError, StateStore
from limanix.vm import VMManager


class VMPreparationTests(unittest.TestCase):
    def setUp(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()
        self.store = StateStore(self.root / "state")
        self.lima = create_autospec(LimaClient, instance=True)
        self.lima.run.return_value = ""
        self.manager = VMManager(self.store, self.lima)
        self.enterContext(patch("limanix.vm.os.getuid", return_value=501))
        self.config = Config(
            home=Home(root=str(self.root / "homes")), mounts=[], env={}
        )
        self.config_path = self.root / "limanix.toml"
        self.config_path.write_text(render_config(self.config))

    def assert_clean_creation(self) -> None:
        self.lima.create.assert_not_called()
        self.assertFalse(self.store.instance_dir(self.config.name).exists())
        homes = self.root / "homes"
        self.assertEqual(list(homes.iterdir()) if homes.exists() else [], [])

    def create_existing(self) -> Instance:
        record = self.manager.create(self.config_path)
        self.lima.fetch_all.return_value = [
            LimaInstance(
                name=record.lima_name,
                status=LimaStatus.RUNNING,
                disk=10 * 1024**3,
                networks=(),
            )
        ]
        self.lima.reset_mock()
        return record

    def test_create_cleanup_on_interrupted_preparation(self) -> None:
        with (
            patch.object(self.lima, "validate", side_effect=KeyboardInterrupt()),
            self.assertRaises(KeyboardInterrupt),
        ):
            self.manager.create(self.config_path)
        self.assert_clean_creation()

    def test_first_record_failure_removes_created_home_and_state(self) -> None:
        for error in (StateError("disk full"), KeyboardInterrupt()):
            with (
                self.subTest(error=type(error).__name__),
                patch.object(self.store, "save", side_effect=error),
                self.assertRaises(type(error)),
            ):
                self.manager.create(self.config_path)
            self.assert_clean_creation()
        self.manager.create(self.config_path)

    def test_interrupted_first_commit_is_cleaned_before_lima_creation(self) -> None:
        original_save = self.store.save

        def save_then_interrupt(instance: Instance) -> None:
            original_save(instance)
            raise KeyboardInterrupt()

        with (
            patch.object(self.store, "save", side_effect=save_then_interrupt),
            self.assertRaises(KeyboardInterrupt),
        ):
            self.manager.create(self.config_path)
        self.assert_clean_creation()

    def test_update_record_failure_preserves_previous_generation(self) -> None:
        record = self.create_existing()
        for error in (StateError("disk full"), KeyboardInterrupt()):
            with (
                self.subTest(error=type(error).__name__),
                patch.object(self.store, "save", side_effect=error),
                self.assertRaises(type(error)),
            ):
                self.manager.update(self.config_path)
            self.assertEqual(self.store.load(record.name), record)
            self.assertEqual(
                [p.name for p in self.manager._generation(record).parent.iterdir()],
                [record.generation],
            )
            self.lima.edit.assert_not_called()
            self.lima.stop.assert_not_called()

    def test_update_interrupt_during_prepare_preserves_record(self) -> None:
        record = self.create_existing()
        with (
            patch.object(self.lima, "validate", side_effect=KeyboardInterrupt()),
            self.assertRaises(KeyboardInterrupt),
        ):
            self.manager.update(self.config_path)
        self.assertEqual(self.store.load(record.name), record)
        self.assertEqual(
            len(list(self.manager._generation(record).parent.iterdir())), 1
        )

    def test_interrupted_committed_update_retains_referenced_generation(self) -> None:
        record = self.create_existing()
        original_save = self.store.save

        def save_then_interrupt(instance: Instance) -> None:
            original_save(instance)
            raise KeyboardInterrupt()

        with (
            patch.object(self.store, "save", side_effect=save_then_interrupt),
            self.assertRaises(KeyboardInterrupt),
        ):
            self.manager.update(self.config_path)
        committed = self.store.load(record.name)
        self.assertNotEqual(committed.generation, record.generation)
        self.assertTrue(
            (self.manager._generation(committed) / "flake/flake.nix").is_file()
        )
        self.assertTrue(Path(committed.identity.home).exists())
        self.lima.edit.assert_not_called()


if __name__ == "__main__":
    unittest.main()
