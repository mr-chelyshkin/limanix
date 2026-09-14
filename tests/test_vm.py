"""Exercise orchestration, deletion choices, recovery, and generation ownership."""

import contextlib
import io
import json
import shutil
import tempfile
import unittest
from collections.abc import Iterator, Sequence
from dataclasses import replace
from pathlib import Path
from unittest.mock import patch

from limanix.cli import main
from limanix.config import Config, Home, Mount, NixOS
from limanix.config.template import render_config
from limanix.domain import (
    ByteSize,
    EnvName,
    EnvValue,
    GuestPath,
    ModuleId,
    Username,
    VMName,
)
from limanix.lima import LimaClient, LimaError, LimaInstance, LimaNetwork, LimaStatus
from limanix.state import Instance, InstanceStatus, StateError, StateStore
from limanix.vm import VMError, VMManager


class FakeLima(LimaClient):
    def __init__(self) -> None:
        super().__init__()
        self.instances: dict[str, LimaInstance] = {}
        self.calls: list[tuple[object, ...]] = []
        self.fail_on: str | None = None
        self.shell_status = 0

    def called(self, operation: str, *args: object) -> None:
        self.calls.append((operation, *args))
        if self.fail_on == operation:
            raise LimaError("backend diagnostic with sensitive-test-token")

    def preflight(self, config: Config) -> None:
        self.called("preflight", config.name)

    def validate(self, template: Path) -> None:
        self.called("validate")
        json.loads(template.read_text())

    def fetch_all(self) -> list[LimaInstance]:
        self.called("fetch_all")
        return list(self.instances.values())

    def create(self, name: str, template: Path) -> None:
        self.called("create", name)
        data = json.loads(template.read_text())
        self.instances[name] = LimaInstance(
            name=name,
            status=LimaStatus.STOPPED,
            disk=int(ByteSize.parse(data["disk"])),
            networks=(LimaNetwork(mac_address="52:55:55:4a:e4:84", shared=True),),
        )

    def start(self, name: str) -> None:
        self.called("start", name)
        self.instances[name] = replace(self.instances[name], status=LimaStatus.RUNNING)

    def stop(self, name: str) -> None:
        self.called("stop", name)
        self.instances[name] = replace(self.instances[name], status=LimaStatus.STOPPED)

    def delete(self, name: str, *, force: bool = False) -> None:
        self.called("delete", name, force)
        del self.instances[name]

    def edit(self, name: str, template: Path) -> None:
        self.called("edit", name)
        disk = ByteSize.parse(json.loads(template.read_text())["disk"])
        self.instances[name] = replace(self.instances[name], disk=int(disk))

    def run(self, name: str, command: Sequence[str], *, capture: bool = True) -> str:
        self.called("run", name, tuple(command))
        if tuple(command) == ("ip", "-j", "address", "show"):
            return json.dumps(
                [
                    {
                        "address": "52:55:55:4a:e4:84",
                        "addr_info": [
                            {"family": "inet", "scope": "global", "local": "192.0.2.10"}
                        ],
                    }
                ]
            )
        return ""

    def shell(self, name: str, command: Sequence[str] = ()) -> int:
        self.called("shell", name, tuple(command))
        return self.shell_status


class VMManagerTests(unittest.TestCase):
    def setUp(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()
        self.project = self.root / "external-project"
        self.project.mkdir()
        (self.project / "keep.txt").write_text("external data")
        self.store = StateStore(self.root / "state")
        self.lima = FakeLima()
        self.manager = VMManager(self.store, self.lima)
        self.enterContext(patch("limanix.vm.os.getuid", return_value=501))

    def configuration(self, name: str = "sandbox") -> Config:
        return Config(
            name=VMName(name),
            home=Home(root=str(self.root / "homes")),
            env={EnvName("TOKEN"): EnvValue("sensitive-test-token")},
            mounts=[Mount(source=str(self.project), target=GuestPath("/workspace"))],
            nixos=NixOS(modules=[ModuleId("git")]),
        )

    def write_config(self, config: Config | None = None) -> Path:
        config = config or self.configuration()
        path = self.root / f"{config.name}.toml"
        path.write_text(render_config(config))
        return path

    def test_create_stores_runtime_metadata_without_config_or_guest_marker(
        self,
    ) -> None:
        record = self.manager.create(self.write_config())
        self.assertEqual(record.status, InstanceStatus.READY)
        self.assertEqual(record, self.store.load(record.name))
        home = Path(record.identity.home)
        self.assertEqual(list(home.iterdir()), [])
        directory = self.store.instance_dir(record.name)
        for metadata in (directory / "instance.json", directory / "identity.json"):
            self.assertNotIn("sensitive-test-token", metadata.read_text())
            self.assertNotIn("config_path", metadata.read_text())
        self.assertFalse((self.manager._generation(record) / "sources").exists())
        self.assertTrue(
            (
                self.manager._generation(record) / "flake/modules/0000/default.nix"
            ).is_file()
        )
        self.assertEqual(self.manager.fetch_all()[0].address, "192.0.2.10")

    def test_create_failure_retains_recoverable_owned_vm(self) -> None:
        self.lima.fail_on = "run"
        with self.assertRaises(LimaError):
            self.manager.create(self.write_config())
        record = self.store.load("sandbox")
        self.assertEqual(record.status, InstanceStatus.ERROR)
        self.assertNotIn("sensitive-test-token", record.error or "")
        self.assertTrue(Path(record.identity.home).exists())
        self.lima.fail_on = None
        self.assertEqual(
            self.manager.update(self.write_config()).status, InstanceStatus.READY
        )

    def test_failed_update_retries_prune_all_stale_generations(self) -> None:
        config_path = self.write_config()
        record = self.manager.create(config_path)
        self.lima.fail_on = "run"
        for _ in range(2):
            with self.assertRaises(LimaError):
                self.manager.update(config_path)
        generations = self.manager._generation(record).parent
        self.assertEqual(len(list(generations.iterdir())), 3)
        self.lima.fail_on = None
        updated = self.manager.update(config_path)
        self.assertEqual([p.name for p in generations.iterdir()], [updated.generation])
        self.assertEqual(updated.identity, record.identity)

    def test_cleanup_failure_warns_and_cli_update_succeeds(self) -> None:
        config_path = self.write_config()
        record = self.manager.create(config_path)
        generations = self.manager._generation(record).parent
        (generations / "000000000001").mkdir()
        failed: list[Path] = []
        remove_tree = shutil.rmtree

        def fail_first_generation(directory: Path) -> None:
            if directory.parent == generations and not failed:
                failed.append(directory)
                raise PermissionError("cleanup fixture: permission denied")
            remove_tree(directory)

        with (
            patch("limanix.vm.shutil.rmtree", side_effect=fail_first_generation),
            patch("limanix.cli.app.VMManager", return_value=self.manager),
            contextlib.redirect_stdout(io.StringIO()) as output,
            self.assertLogs("limanix.vm", level="WARNING") as warnings,
        ):
            self.assertEqual(main(["update", "--config", str(config_path)]), 0)

        updated = self.store.load(record.name)
        self.assertEqual(updated.status, InstanceStatus.READY)
        self.assertIsNone(updated.error)
        self.assertIn("Updated sandbox.", output.getvalue())
        self.assertIn("could not be removed", warnings.output[0])
        self.assertIn(str(failed[0]), warnings.output[0])
        self.assertEqual(len(warnings.records), 1)
        self.assertEqual(
            set(generations.iterdir()), {self.manager._generation(updated), failed[0]}
        )
        retried = self.manager.update(config_path)
        self.assertEqual(
            list(generations.iterdir()), [self.manager._generation(retried)]
        )

    def test_cleanup_listing_failure_keeps_updated_vm_ready(self) -> None:
        config_path = self.write_config()
        record = self.manager.create(config_path)
        generations = self.manager._generation(record).parent
        iterate = Path.iterdir

        def fail_generation_listing(directory: Path) -> Iterator[Path]:
            if directory == generations:
                raise PermissionError("cleanup fixture: listing denied")
            return iterate(directory)

        with (
            patch.object(Path, "iterdir", fail_generation_listing),
            self.assertLogs("limanix.vm", level="WARNING") as warnings,
        ):
            updated = self.manager.update(config_path)

        self.assertEqual(updated.status, InstanceStatus.READY)
        self.assertEqual(self.store.load(record.name), updated)
        self.assertIsNone(updated.error)
        self.assertTrue(self.manager._generation(updated).is_dir())
        self.assertIn("could not be listed", warnings.output[0])

    def test_force_and_remove_home_are_independent(self) -> None:
        for force in (False, True):
            for remove_home in (False, True):
                with self.subTest(force=force, remove_home=remove_home):
                    record = self.manager.create(self.write_config())
                    home = Path(record.identity.home)
                    (home / "guest-file").write_text("data")
                    self.lima.calls.clear()
                    self.manager.delete(
                        record.name, force=force, remove_home=remove_home
                    )
                    self.assertIn(("delete", record.lima_name, force), self.lima.calls)
                    self.assertEqual(
                        ("stop", record.lima_name) in self.lima.calls, not force
                    )
                    self.assertEqual(home.exists(), not remove_home)
                    self.assertEqual(
                        (self.project / "keep.txt").read_text(), "external data"
                    )
                    self.assertFalse(self.store.instance_dir(record.name).exists())
                    archived = self.store.root / "homes" / f"{home.name}.json"
                    self.assertEqual(archived.exists(), not remove_home)

    def test_preserved_homes_remain_known_after_reusing_a_vm_name(self) -> None:
        first = self.manager.create(self.write_config())
        self.manager.delete(first.name)
        second = self.manager.create(self.write_config())
        self.manager.delete(second.name)
        records = list((self.store.root / "homes").iterdir())
        self.assertEqual(
            {json.loads(record.read_text())["home"] for record in records},
            {first.identity.home, second.identity.home},
        )
        self.assertEqual(len(records), 2)
        self.assertTrue(Path(first.identity.home).is_dir())
        self.assertTrue(Path(second.identity.home).is_dir())

    def test_failed_home_archival_keeps_vm_identity_for_retry(self) -> None:
        record = self.manager.create(self.write_config())
        with (
            patch.object(
                self.store,
                "preserve_home",
                side_effect=StateError("Archive unavailable"),
            ),
            self.assertRaisesRegex(StateError, "Archive unavailable"),
        ):
            self.manager.delete(record.name)
        self.assertNotIn(record.lima_name, self.lima.instances)
        self.assertEqual(self.store.load_identity(record.name), record.identity)
        self.assertTrue(Path(record.identity.home).is_dir())
        self.manager.delete(record.name)
        self.assertFalse(self.store.instance_dir(record.name).exists())
        self.assertTrue(
            (
                self.store.root / "homes" / f"{Path(record.identity.home).name}.json"
            ).is_file()
        )

    def test_retry_with_remove_home_clears_already_archived_ownership(self) -> None:
        record = self.manager.create(self.write_config())
        archived = self.store.preserve_home(record.identity)
        self.manager.delete(record.name, remove_home=True)
        self.assertFalse(Path(record.identity.home).exists())
        self.assertFalse(archived.exists())

    def test_force_can_delete_when_orderly_stop_fails_and_preserve_home(self) -> None:
        record = self.manager.create(self.write_config())
        self.lima.fail_on = "stop"
        with self.assertRaises(LimaError):
            self.manager.delete(record.name)
        self.manager.delete(record.name, force=True)
        self.assertTrue(Path(record.identity.home).exists())
        self.assertNotIn(record.lima_name, self.lima.instances)

    def test_corrupt_mutable_record_is_listed_and_can_be_deleted(self) -> None:
        corrupt = self.manager.create(self.write_config())
        healthy = self.manager.create(self.write_config(self.configuration("healthy")))
        (self.store.instance_dir(corrupt.name) / "instance.json").write_text("{}")
        entries = {entry.name: entry for entry in self.manager.fetch_all()}
        self.assertIsNotNone(entries[corrupt.name].error)
        self.assertIsNone(entries[corrupt.name].state)
        self.assertEqual(entries[healthy.name].state, InstanceStatus.READY)
        self.manager.delete(corrupt.name, force=True, remove_home=True)
        self.assertFalse(Path(corrupt.identity.home).exists())
        self.assertIn(healthy.lima_name, self.lima.instances)

    def test_state_operations_do_not_parse_user_configuration(self) -> None:
        record = self.manager.create(self.write_config())
        with patch(
            "limanix.config.parser.parse_config",
            side_effect=AssertionError("current config validator called"),
        ):
            self.assertEqual(self.store.load(record.name), record)
            self.manager.stop(record.name)
            self.manager.start(record.name)
            self.manager.delete(record.name, remove_home=True)

    def test_cli_list_displays_interrupted_without_changing_vm_or_record(self) -> None:
        record = self.manager.create(self.write_config())
        record.status = InstanceStatus.UPDATING
        self.store.save(record)
        with (
            patch("limanix.cli.app.VMManager", return_value=self.manager),
            contextlib.redirect_stdout(io.StringIO()) as output,
        ):
            self.assertEqual(main(["list", "--json"]), 0)
        (entry,) = json.loads(output.getvalue())
        self.assertEqual(entry["state"], "interrupted")
        self.assertEqual(entry["status"], "Running")
        self.assertEqual(self.store.load(record.name).status, InstanceStatus.UPDATING)
        with self.store.instance_lock(record.name):
            self.assertEqual(self.manager.fetch_all()[0].state, InstanceStatus.UPDATING)

    def test_update_uses_actual_disk_size_after_failed_growth(self) -> None:
        config = self.configuration()
        record = self.manager.create(self.write_config(config))
        config.resources.disk = ByteSize.parse("20GiB")
        self.lima.fail_on = "edit"
        with self.assertRaises(LimaError):
            self.manager.update(self.write_config(config))
        self.assertEqual(
            self.lima.instances[record.lima_name].disk, ByteSize.parse("10GiB")
        )
        self.lima.fail_on = None
        config.resources.disk = ByteSize.parse("10GiB")
        self.manager.update(self.write_config(config))
        self.lima.instances[record.lima_name] = replace(
            self.lima.instances[record.lima_name], disk=int(ByteSize.parse("20GiB"))
        )
        with self.assertRaisesRegex(VMError, "Shrinking"):
            self.manager.update(self.write_config(config))

    def test_update_preserves_immutable_identity(self) -> None:
        record = self.manager.create(self.write_config())
        original = self.configuration()
        for config in (
            replace(original, user=replace(original.user, name=Username("other"))),
            replace(
                original, user=replace(original.user, home=GuestPath("/home/other"))
            ),
            replace(original, home=Home(root=str(self.root / "other"))),
        ):
            with self.assertRaisesRegex(VMError, "cannot change"):
                self.manager.update(self.write_config(config))
            self.assertEqual(self.store.load(record.name), record)

    def test_update_refreshes_lima_status_after_preparation(self) -> None:
        config_path = self.write_config()
        record = self.manager.create(config_path)
        prepare = self.manager._prepare
        stop = self.lima.stop

        def stop_during_preparation(instance: Instance, config: Config) -> Path:
            template = prepare(instance, config)
            self.lima.instances[record.lima_name] = replace(
                self.lima.instances[record.lima_name], status=LimaStatus.STOPPED
            )
            return template

        def reject_redundant_stop(name: str) -> None:
            if self.lima.instances[name].status is LimaStatus.STOPPED:
                raise LimaError("VM was already stopped outside Limanix")
            stop(name)

        with (
            patch.object(self.manager, "_prepare", side_effect=stop_during_preparation),
            patch.object(self.lima, "stop", side_effect=reject_redundant_stop),
        ):
            updated = self.manager.update(config_path)
        self.assertEqual(updated.status, InstanceStatus.READY)
        self.assertEqual(
            self.lima.instances[record.lima_name].status, LimaStatus.RUNNING
        )

    def test_update_uses_name_from_config(self) -> None:
        self.manager.create(self.write_config())
        with self.assertRaises(StateError):
            self.manager.update(self.write_config(self.configuration("different")))

    def test_other_vm_and_module_registry_are_not_blocked(self) -> None:
        alpha = self.manager.create(self.write_config())
        beta = self.manager.create(self.write_config(self.configuration("beta")))
        module = self.root / "module"
        module.mkdir()
        (module / "default.nix").write_text("{ ... }: {}")
        with self.store.instance_lock(alpha.name):
            self.manager.stop(beta.name)
            self.manager.modules.add("custom", module)
            with self.assertRaises(StateError):
                self.manager.stop(alpha.name)
        self.assertEqual(self.lima.instances[beta.lima_name].status, LimaStatus.STOPPED)

    def test_missing_backend_can_be_listed_and_deleted(self) -> None:
        record = self.manager.create(self.write_config())
        self.lima.instances.clear()
        self.assertIsNone(self.manager.fetch_all()[0].status)
        self.manager.delete(record.name, remove_home=True)
        self.assertEqual(self.manager.fetch_all(), [])

    def test_shell_checks_status_and_preserves_child_exit_code(self) -> None:
        record = self.manager.create(self.write_config())
        self.lima.shell_status = 7
        self.assertEqual(self.manager.shell(record.name, ["false"]), 7)
        self.manager.stop(record.name)
        with self.assertRaisesRegex(VMError, "not running"):
            self.manager.shell(record.name, [])

    def test_removing_guest_files_does_not_destroy_host_ownership(self) -> None:
        record = self.manager.create(self.write_config())
        home = Path(record.identity.home)
        (home / ".limanix-owner.json").write_text("guest-controlled")
        (home / ".limanix-owner.json").unlink()
        self.manager.delete(record.name, force=True, remove_home=True)
        self.assertFalse(home.exists())


if __name__ == "__main__":
    unittest.main()
