"""Exercise independent host identity, corruption isolation, and process locks."""

import errno
import json
import select
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import unittest
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import FrozenInstanceError, asdict, replace
from pathlib import Path
from unittest.mock import patch

from limanix.domain import Architecture, GuestPath, Username, VMName
from limanix.state import Identity, Instance, InstanceStatus, StateError, StateStore

_REGISTRY_WORKER = """
import fcntl
import sys
from pathlib import Path
from unittest.mock import patch
from limanix.state import StateError, StateStore

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

store = StateStore(Path(sys.argv[1]))
try:
    with patch('limanix.state.fcntl.flock', side_effect=observe_contention):
        with store.registry_lock(
            shared=sys.argv[2] == 'shared', timeout=float(sys.argv[3])
        ):
            print('acquired', flush=True)
except StateError as error:
    print(str(error), flush=True)
    sys.exit(12)
"""


class StateStoreTests(unittest.TestCase):
    def make_instance(self, root: Path, name: str = "sandbox") -> Instance:
        return Instance(
            identity=Identity(
                name=VMName(name),
                id="012345abcdef",
                arch=Architecture.ARM64,
                username=Username("dev"),
                user_home=GuestPath("/home/dev"),
                home_root=str(root.resolve() / "homes"),
                home=str(root.resolve() / f"homes/{name}-012345abcdef"),
                created_at="2026-09-13T12:00:00+00:00",
            ),
            generation="abcdef012345",
            status=InstanceStatus.READY,
        )

    def test_identity_and_runtime_roundtrip_are_separate_and_private(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            original = self.make_instance(root)
            with store.instance_lock(original.name):
                store.save(original)
            record = store.instance_dir(original.name) / "instance.json"
            identity = record.with_name("identity.json")
            self.assertEqual(
                json.loads(record.read_text()),
                {
                    "schema_version": 1,
                    "generation": original.generation,
                    "status": "ready",
                    "error": None,
                },
            )
            self.assertEqual(
                json.loads(identity.read_text()),
                {"schema_version": 1, **asdict(original.identity)},
            )
            self.assertEqual(store.load(original.name), original)
            for path in (record, identity):
                self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            self.assertEqual(stat.S_IMODE(store.root.stat().st_mode), 0o700)
            self.assertEqual(stat.S_IMODE(record.parent.stat().st_mode), 0o700)
            self.assertEqual(original.lima_name, "limanix-sandbox-012345abcdef")
            identity_inode = identity.stat().st_ino
            original.status = InstanceStatus.ERROR
            original.error = "guest rebuild failed"
            record.chmod(0o644)
            store.save(original)
            self.assertEqual(store.load(original.name), original)
            self.assertEqual(identity.stat().st_ino, identity_inode)
            self.assertEqual(stat.S_IMODE(record.stat().st_mode), 0o600)

    def test_identity_is_frozen_and_existing_identity_cannot_be_replaced(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            original = self.make_instance(root)
            store.save(original)
            with self.assertRaises(FrozenInstanceError):
                # Deliberately violate frozen typing to verify the runtime guard.
                original.identity.username = Username("another")  # type: ignore[misc]
            changed = replace(
                original,
                identity=replace(original.identity, username=Username("another")),
            )
            with self.assertRaisesRegex(StateError, "identity cannot be changed"):
                store.save(changed)
            self.assertEqual(store.load(original.name), original)

    def test_preserved_home_ownership_is_private_and_independent_of_vm_state(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            original = self.make_instance(root)
            store.save(original)
            record = store.preserve_home(original.identity)
            self.assertEqual(record, store.root / "homes" / "sandbox-012345abcdef.json")
            self.assertEqual(stat.S_IMODE(record.stat().st_mode), 0o600)
            self.assertEqual(
                json.loads(record.read_text()),
                {"schema_version": 1, **asdict(original.identity)},
            )
            shutil.rmtree(store.instance_dir(original.name))
            self.assertEqual(store.preserve_home(original.identity), record)
            self.assertTrue(record.is_file())
            store.forget_home(original.identity)
            self.assertFalse(record.exists())
            store.forget_home(original.identity)

    def test_preserved_home_record_cannot_be_replaced_by_different_ownership(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            original = self.make_instance(root).identity
            record = store.preserve_home(original)
            before = record.read_bytes()
            changed = replace(original, username=Username("another"))
            with self.assertRaises(StateError):
                store.preserve_home(changed)
            with self.assertRaises(StateError):
                store.forget_home(changed)
            self.assertEqual(record.read_bytes(), before)

    def test_missing_identity_and_path_traversal_cannot_be_loaded(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state")
            self.assertEqual(store.fetch_all(), [])
            with self.assertRaisesRegex(StateError, "no managed identity"):
                store.load("unknown")
            for name in ("../escape", "/tmp/escape", "has/slash", ".", ""):
                with self.subTest(name=name), self.assertRaises(StateError):
                    store.load(name)
                with self.subTest(lock_name=name), self.assertRaises(StateError):
                    store.instance_lock(name)
            self.assertFalse(store.root.exists())

    def test_identity_uses_username_and_guest_path_validation(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            instance = self.make_instance(root)
            store.save(instance)
            identity = store.instance_dir(instance.name) / "identity.json"
            valid = json.loads(identity.read_text())
            for field, values in (
                (
                    "username",
                    ["root", "limanix-admin", "BadName", "two words", "dev\n"],
                ),
                (
                    "user_home",
                    ["relative", "/home/../etc", "//home/dev", "/home/two words", "/"],
                ),
            ):
                for value in values:
                    with self.subTest(field=field, value=value):
                        identity.write_text(json.dumps({**valid, field: value}))
                        with self.assertRaises(StateError):
                            store.load_identity(instance.name)
            identity.write_text(json.dumps(valid))
            loaded = store.load_identity(instance.name)
            self.assertIsInstance(loaded.username, Username)
            self.assertIsInstance(loaded.user_home, GuestPath)

    def test_invalid_mutable_record_preserves_independent_identity(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            instance = self.make_instance(root)
            store.save(instance)
            record = store.instance_dir(instance.name) / "instance.json"
            valid = json.loads(record.read_text())
            cases: list[object] = [
                [],
                None,
                {},
                {**valid, "schema_version": 2},
                {**valid, "schema_version": True},
                {**valid, "generation": "../escape"},
                {**valid, "status": "unknown"},
                {**valid, "status": "interrupted"},
                {**valid, "status": 1},
                {**valid, "error": {"message": "failed"}},
                {**valid, "config": {"schema_version": 999}},
                {key: value for key, value in valid.items() if key != "generation"},
            ]
            for data in cases:
                with self.subTest(data=data):
                    record.write_text(json.dumps(data), encoding="utf-8")
                    with self.assertRaises(StateError):
                        store.load(instance.name)
                    self.assertEqual(
                        store.load_identity(instance.name), instance.identity
                    )
                    (entry,) = store.fetch_all()
                    self.assertEqual(entry.identity, instance.identity)
                    self.assertIsNone(entry.instance)
                    self.assertIsNotNone(entry.error)

    def test_invalid_identity_is_not_reconstructed_from_mutable_state(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            instance = self.make_instance(root)
            store.save(instance)
            identity = store.instance_dir(instance.name) / "identity.json"
            valid = json.loads(identity.read_text())
            for data in (
                {},
                {**valid, "name": "different"},
                {**valid, "id": "../escape"},
                {**valid, "arch": "unknown"},
                {**valid, "home": str(root)},
                {**valid, "username": 1},
                {**valid, "unexpected": True},
            ):
                with self.subTest(data=data):
                    identity.write_text(json.dumps(data), encoding="utf-8")
                    with self.assertRaises(StateError):
                        store.load_identity(instance.name)
                    (entry,) = store.fetch_all()
                    self.assertIsNone(entry.identity)
                    self.assertIsNone(entry.instance)
                    self.assertIsNotNone(entry.error)
            identity.unlink()
            with self.assertRaises(StateError):
                store.load(instance.name)

    def test_corrupted_json_is_reported_without_executing_payload(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            instance = self.make_instance(root)
            marker = root / "unexpected-code-execution"
            payload = f"__import__('pathlib').Path({str(marker)!r}).touch()"
            instance.error = payload
            store.save(instance)
            self.assertEqual(store.load(instance.name).error, payload)
            self.assertFalse(marker.exists())
            record = store.instance_dir(instance.name) / "instance.json"
            for content in (
                payload,
                "{ broken json",
                "null",
                '{"__reduce__": "os.system"}',
            ):
                record.write_text(content, encoding="utf-8")
                with self.subTest(content=content), self.assertRaises(StateError):
                    store.load(instance.name)
            self.assertFalse(marker.exists())

    def test_symbolic_instance_directory_or_identity_is_refused(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            store.initialize()
            foreign = root / "foreign"
            foreign.mkdir()
            link = store.root / "instances" / "sandbox"
            link.symlink_to(foreign, target_is_directory=True)
            with self.assertRaisesRegex(StateError, "symbolic link"):
                store.load("sandbox")
            with self.assertRaisesRegex(StateError, "symbolic link"):
                store.save(self.make_instance(root))
            self.assertEqual(list(foreign.iterdir()), [])
            (entry,) = store.fetch_all()
            self.assertIsNone(entry.identity)
            link.unlink()
            original = self.make_instance(root)
            store.save(original)
            identity = store.instance_dir(original.name) / "identity.json"
            target = foreign / "identity.json"
            identity.rename(target)
            identity.symlink_to(target)
            with self.assertRaises(StateError):
                store.load_identity(original.name)

    def test_fetch_all_keeps_healthy_records_when_another_is_corrupt(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            for name in ("zeta", "alpha", "broken"):
                store.save(self.make_instance(root, name))
            (store.instance_dir("broken") / "instance.json").write_text("{}")
            (store.root / "instances" / "unfinished").mkdir()
            (store.root / "instances" / "unrelated-file").touch()
            entries = store.fetch_all()
            self.assertEqual(
                [entry.name for entry in entries],
                ["alpha", "broken", "unfinished", "zeta"],
            )
            self.assertIsNotNone(entries[0].instance)
            self.assertIsNotNone(entries[1].identity)
            self.assertIsNone(entries[1].instance)
            self.assertIsNone(entries[2].identity)
            self.assertIsNotNone(entries[3].instance)

    def test_real_process_locks_are_per_vm_and_registry_is_independent(self) -> None:
        script = """
import sys
from pathlib import Path
from limanix.state import StateError, StateStore
store = StateStore(Path(sys.argv[1]))
try:
    lock = (
        store.registry_lock(timeout=0)
        if sys.argv[2] == 'registry' else store.instance_lock(sys.argv[2])
    )
    with lock:
        print('acquired')
except StateError as error:
    print(str(error))
    sys.exit(12)
"""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            store.save(self.make_instance(root))

            def attempt(name: str) -> subprocess.CompletedProcess[str]:
                return subprocess.run(
                    [sys.executable, "-c", script, str(store.root), name],
                    capture_output=True,
                    text=True,
                    timeout=10,
                )

            with store.instance_lock("sandbox"):
                busy = attempt("sandbox")
                self.assertEqual(busy.returncode, 12, busy.stderr)
                self.assertIn("VM 'sandbox'", busy.stdout)
                self.assertEqual(attempt("other-vm").returncode, 0)
                self.assertEqual(attempt("registry").returncode, 0)
                shutil.rmtree(store.instance_dir("sandbox"))
                self.assertEqual(attempt("sandbox").returncode, 12)
            self.assertEqual(attempt("sandbox").returncode, 0)
            with store.registry_lock():
                self.assertEqual(attempt("registry").returncode, 12)
                self.assertEqual(attempt("sandbox").returncode, 0)
            self.assertEqual(attempt("registry").returncode, 0)
            lock_path = store.root / "locks/instances/sandbox.lock"
            self.assertTrue(lock_path.exists())
            self.assertEqual(stat.S_IMODE(lock_path.stat().st_mode), 0o600)

    def test_listing_marks_killed_operations_without_changing_saved_state(self) -> None:
        script = """
import sys
from pathlib import Path
from limanix.state import InstanceStatus, StateStore
store = StateStore(Path(sys.argv[1]))
with store.instance_lock('sandbox'):
    instance = store.load('sandbox')
    instance.status = InstanceStatus(sys.argv[2])
    store.save(instance)
    print('active', flush=True)
    sys.stdin.read(1)
"""
        for status in (
            InstanceStatus.CREATING,
            InstanceStatus.UPDATING,
            InstanceStatus.DELETING,
        ):
            with (
                self.subTest(status=status),
                tempfile.TemporaryDirectory() as directory,
            ):
                root = Path(directory)
                store = StateStore(root / "state")
                instance = self.make_instance(root)
                store.save(instance)
                record = store.instance_dir(instance.name) / "instance.json"
                identity_path = record.with_name("identity.json")
                child = subprocess.Popen(
                    [sys.executable, "-c", script, str(store.root), status.value],
                    stdin=subprocess.PIPE,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                )
                try:
                    assert child.stdout is not None
                    ready, _, _ = select.select([child.stdout], [], [], 5)
                    self.assertTrue(ready, "Child did not save its active operation")
                    self.assertEqual(child.stdout.readline().strip(), "active")
                    before_record = record.read_bytes()
                    before_identity = identity_path.read_bytes()
                    (active,) = store.fetch_all()
                    assert active.instance is not None
                    self.assertEqual(active.instance.status, status)
                    self.assertIsNone(active.error)
                    child.kill()
                    child.communicate(timeout=5)
                    self.assertEqual(child.returncode, -signal.SIGKILL)
                    (interrupted,) = store.fetch_all()
                    assert interrupted.instance is not None
                    self.assertEqual(
                        interrupted.instance.status, InstanceStatus.INTERRUPTED
                    )
                    self.assertIsNone(interrupted.error)
                    self.assertEqual(store.load(instance.name).status, status)
                    self.assertEqual(record.read_bytes(), before_record)
                    self.assertEqual(identity_path.read_bytes(), before_identity)
                finally:
                    if child.poll() is None:
                        child.kill()
                    child.communicate(timeout=5)

    def test_listing_reads_finished_state_after_acquiring_vm_lock(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            instance = self.make_instance(root)
            instance.status = InstanceStatus.UPDATING
            store.save(instance)
            vm_lock = store._vm_lock

            @contextmanager
            def finish_before_snapshot(name: str, *, shared: bool) -> Iterator[None]:
                with vm_lock(name, shared=False):
                    instance.status = InstanceStatus.READY
                    store.save(instance)
                with vm_lock(name, shared=shared):
                    yield

            with patch.object(store, "_vm_lock", side_effect=finish_before_snapshot):
                (entry,) = store.fetch_all()
            assert entry.instance is not None
            self.assertEqual(entry.instance.status, InstanceStatus.READY)
            self.assertIsNone(entry.error)

    def test_listing_reports_lock_io_failure_and_busy_record_corruption(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            store = StateStore(root / "state")
            instance = self.make_instance(root)
            instance.status = InstanceStatus.CREATING
            store.save(instance)
            with patch(
                "limanix.state.fcntl.flock",
                side_effect=OSError(errno.EIO, "I/O failure"),
            ):
                (entry,) = store.fetch_all()
            self.assertIsNone(entry.instance)
            self.assertIn("Cannot acquire state lock", entry.error or "")
            self.assertIn("I/O failure", entry.error or "")
            self.assertEqual(store.load(instance.name).status, InstanceStatus.CREATING)
            with store.instance_lock(instance.name):
                (store.instance_dir(instance.name) / "instance.json").write_text("{}")
                (corrupt,) = store.fetch_all()
                self.assertIsNone(corrupt.instance)
                self.assertEqual(corrupt.identity, instance.identity)
                self.assertIn("Cannot read VM record", corrupt.error or "")

    def registry_process(
        self, store: StateStore, *, shared: bool, timeout: float
    ) -> subprocess.Popen[str]:
        process = subprocess.Popen(
            [
                sys.executable,
                "-c",
                _REGISTRY_WORKER,
                str(store.root),
                "shared" if shared else "exclusive",
                str(timeout),
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )

        def cleanup() -> None:
            if process.poll() is None:
                process.kill()
            process.communicate(timeout=5)

        self.addCleanup(cleanup)
        return process

    def assert_waiting(self, process: subprocess.Popen[str]) -> None:
        assert process.stdout is not None
        ready, _, _ = select.select([process.stdout], [], [], 5)
        self.assertTrue(ready, "Child did not report its lock attempt")
        self.assertEqual(process.stdout.readline().strip(), "blocked")

    def test_registry_shared_readers_acquire_simultaneously(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state")
            with store.registry_lock(shared=True):
                reader = self.registry_process(store, shared=True, timeout=1)
                stdout, stderr = reader.communicate(timeout=5)
                self.assertEqual(reader.returncode, 0, stderr)
                self.assertEqual(stdout.strip(), "acquired")

    def test_registry_readers_and_writers_wait_then_acquire_after_release(self) -> None:
        for holder_shared, waiter_shared in (
            (True, False),
            (False, True),
            (False, False),
        ):
            with self.subTest(holder_shared=holder_shared, waiter_shared=waiter_shared):
                with tempfile.TemporaryDirectory() as directory:
                    store = StateStore(Path(directory) / "state")
                    with store.registry_lock(shared=holder_shared):
                        waiter = self.registry_process(
                            store, shared=waiter_shared, timeout=5
                        )
                        # The child reports a real failed flock before we release ours.
                        self.assert_waiting(waiter)
                    stdout, stderr = waiter.communicate(timeout=5)
                    self.assertEqual(waiter.returncode, 0, stderr + stdout)
                    self.assertEqual(stdout.strip(), "acquired")

    def test_registry_timeout_reports_a_bounded_wait(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state")
            with store.registry_lock(shared=True):
                writer = self.registry_process(store, shared=False, timeout=0.1)
                stdout, stderr = writer.communicate(timeout=5)
                self.assertEqual(writer.returncode, 12, stderr)
                self.assertIn("blocked", stdout.splitlines())
                self.assertIn(
                    "Timed out after 0.1s waiting for the module registry lock", stdout
                )
            with store.registry_lock(shared=False, timeout=0):
                pass

    def test_registry_timeout_rejects_unbounded_or_negative_values(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state")
            for timeout in (-1, float("inf"), float("nan")):
                with self.subTest(timeout=timeout), self.assertRaises(ValueError):
                    store.registry_lock(timeout=timeout)
            self.assertFalse(store.root.exists())


if __name__ == "__main__":
    unittest.main()
