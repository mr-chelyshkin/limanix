"""Check path diagnostics and real directory permissions."""

import errno
import os
import stat
import tempfile
import unittest
from pathlib import Path
from typing import Any
from unittest.mock import patch

from limanix.filesystem import (
    FilesystemError,
    require_directory,
    write_text_atomic,
)


class DirectoryTests(unittest.TestCase):
    def test_resolves_directory_links_and_home(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / "project"
            target.mkdir()
            link = root / "link"
            link.symlink_to(target, target_is_directory=True)
            self.assertEqual(require_directory(link), target.resolve())
            self.assertEqual(require_directory(Path("~")), Path.home().resolve())

    def test_missing_directory_and_file_are_distinct_errors(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(FilesystemError) as raised:
                require_directory(root / "missing")
            self.assertEqual(raised.exception.reason, "path does not exist")
            target = root / "file"
            target.touch()
            with self.assertRaises(FilesystemError) as raised:
                require_directory(target)
            self.assertEqual(raised.exception.reason, "path is not a directory")

    def test_invalid_path_has_a_filesystem_error(self) -> None:
        with self.assertRaises(FilesystemError):
            require_directory(Path("invalid\x00path"))

    @unittest.skipIf(os.geteuid() == 0, "Root bypasses Unix permission checks")
    def test_inaccessible_directory_is_not_reported_as_missing(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory) / "private"
            parent.mkdir()
            child = parent / "child"
            child.mkdir()
            parent.chmod(0o000)
            try:
                with self.assertRaises(FilesystemError) as raised:
                    require_directory(child)
                self.assertEqual(raised.exception.reason, "permission denied")
            finally:
                parent.chmod(0o700)


class AtomicWriteTests(unittest.TestCase):
    @unittest.skipIf(os.geteuid() == 0, "Root bypasses Unix permission checks")
    def test_readonly_directory_refuses_write_and_preserves_existing_content(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "readonly"
            root.mkdir()
            destination = root / "config.toml"
            destination.write_text("keep\n")
            root.chmod(0o500)
            try:
                with self.assertRaises(FilesystemError):
                    write_text_atomic(destination, "new\n")
                self.assertEqual(destination.read_text(), "keep\n")
                self.assertEqual(list(root.iterdir()), [destination])
            finally:
                root.chmod(0o700)

    def test_explicit_private_mode_replaces_broader_existing_mode(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory) / "record.json"
            destination.write_text("old\n")
            destination.chmod(0o644)
            write_text_atomic(destination, "new\n", mode=0o600)
            self.assertEqual(stat.S_IMODE(destination.stat().st_mode), 0o600)

    def test_file_sync_replace_and_directory_sync_order(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            destination = root / "record.json"
            operations: list[str] = []
            real_fsync = os.fsync
            real_replace = os.replace

            def sync(descriptor: int) -> None:
                kind = (
                    "directory"
                    if stat.S_ISDIR(os.fstat(descriptor).st_mode)
                    else "file"
                )
                operations.append(f"sync {kind}")
                real_fsync(descriptor)

            def replace(source: Path, target: Path) -> None:
                operations.append("replace")
                real_replace(source, target)

            with (
                patch("limanix.filesystem.os.fsync", side_effect=sync),
                patch("limanix.filesystem.os.replace", side_effect=replace),
                patch(
                    "limanix.filesystem.tempfile.NamedTemporaryFile",
                    wraps=tempfile.NamedTemporaryFile,
                ) as temporary,
            ):
                write_text_atomic(destination, "new\n")
            self.assertEqual(operations, ["sync file", "replace", "sync directory"])
            temporary.assert_called_once()
            self.assertEqual(stat.S_IMODE(destination.stat().st_mode), 0o600)
            self.assertEqual(list(root.iterdir()), [destination])

    def test_directory_sync_failure_reports_already_replaced_content(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            destination = root / "record.json"
            destination.write_text("old\n")
            real_fsync = os.fsync

            def sync(descriptor: int) -> None:
                if stat.S_ISDIR(os.fstat(descriptor).st_mode):
                    raise OSError(errno.EIO, "directory sync failed")
                real_fsync(descriptor)

            with (
                patch("limanix.filesystem.os.fsync", side_effect=sync),
                self.assertRaisesRegex(FilesystemError, "directory sync failed"),
            ):
                write_text_atomic(destination, "new\n")
            self.assertEqual(destination.read_text(), "new\n")
            self.assertEqual(list(root.iterdir()), [destination])

    def test_invalid_file_path_has_a_filesystem_error(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(FilesystemError):
                write_text_atomic(root / "invalid\x00file", "new\n")
            self.assertEqual(list(root.iterdir()), [])

    def test_new_file_is_private_and_existing_mode_is_preserved(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory) / "config.toml"
            result = write_text_atomic(destination, "first\n")
            self.assertEqual(result, destination.resolve())
            self.assertEqual(stat.S_IMODE(destination.stat().st_mode), 0o600)
            destination.chmod(0o640)
            write_text_atomic(destination, "second\n")
            self.assertEqual(destination.read_text(), "second\n")
            self.assertEqual(stat.S_IMODE(destination.stat().st_mode), 0o640)
            self.assertEqual(list(Path(directory).iterdir()), [destination])

    def test_sync_and_replace_failures_keep_old_file_and_remove_temporary(self) -> None:
        for operation in ("os.fsync", "os.replace"):
            with self.subTest(operation=operation):
                with tempfile.TemporaryDirectory() as directory:
                    root = Path(directory)
                    destination = root / "config.toml"
                    destination.write_text("keep\n")
                    with (
                        patch(
                            f"limanix.filesystem.{operation}",
                            side_effect=OSError(
                                errno.ENOSPC, "No space left on device"
                            ),
                        ),
                        self.assertRaises(FilesystemError),
                    ):
                        write_text_atomic(destination, "new\n")
                    self.assertEqual(destination.read_text(), "keep\n")
                    self.assertEqual(list(root.iterdir()), [destination])

    def test_encoding_failure_keeps_old_file_and_removes_temporary(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            destination = root / "config.toml"
            destination.write_text("keep\n")
            with self.assertRaises(FilesystemError):
                write_text_atomic(destination, "invalid\ud800")
            self.assertEqual(destination.read_text(), "keep\n")
            self.assertEqual(list(root.iterdir()), [destination])

    def test_cleanup_failure_preserves_original_error_and_reports_temporary(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            destination = root / "config.toml"
            destination.write_text("keep\n")
            with (
                patch(
                    "limanix.filesystem.os.replace",
                    side_effect=OSError(errno.ENOSPC, "No space left on device"),
                ),
                patch.object(
                    Path, "unlink", side_effect=PermissionError(errno.EACCES, "denied")
                ),
                self.assertRaises(FilesystemError) as raised,
            ):
                write_text_atomic(destination, "new\n")
            self.assertEqual(destination.read_text(), "keep\n")
            temporary = [path for path in root.iterdir() if path != destination]
            self.assertEqual(len(temporary), 1)
            self.assertIn("No space left on device", raised.exception.reason)
            self.assertIn("cannot remove temporary file", raised.exception.reason)
            self.assertIn(str(temporary[0].resolve()), raised.exception.reason)
            self.assertIn("denied", raised.exception.reason)

    def test_partial_write_failure_keeps_old_file_and_removes_temporary(self) -> None:
        create_temporary = tempfile.NamedTemporaryFile

        def failing_writer(*args: Any, **kwargs: Any) -> Any:
            stream = create_temporary(*args, **kwargs)
            write = stream.write

            def partial_write(text: str) -> int:
                write(text[:3])
                stream.flush()
                raise OSError(errno.ENOSPC, "No space left on device")

            self.enterContext(patch.object(stream, "write", side_effect=partial_write))
            return stream

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            destination = root / "config.toml"
            destination.write_text("keep\n")
            with (
                patch(
                    "limanix.filesystem.tempfile.NamedTemporaryFile",
                    side_effect=failing_writer,
                ),
                self.assertRaises(FilesystemError),
            ):
                write_text_atomic(destination, "replacement\n")
            self.assertEqual(destination.read_text(), "keep\n")
            self.assertEqual(list(root.iterdir()), [destination])

    def test_symlinks_and_non_regular_files_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            original = root / "original.toml"
            original.write_text("keep\n")
            link = root / "linked.toml"
            link.symlink_to(original)
            broken = root / "broken.toml"
            broken.symlink_to(root / "missing")
            folder = root / "folder"
            folder.mkdir()
            fifo = root / "fifo"
            os.mkfifo(fifo)
            for destination in (link, broken, folder, fifo):
                with self.subTest(path=destination):
                    with self.assertRaises(FilesystemError):
                        write_text_atomic(destination, "new\n")
            self.assertTrue(link.is_symlink())
            self.assertTrue(broken.is_symlink())
            self.assertEqual(original.read_text(), "keep\n")
            self.assertEqual(len(list(root.iterdir())), 5)

    @unittest.skipIf(os.geteuid() == 0, "Root bypasses Unix permission checks")
    def test_readonly_file_is_not_replaced_even_with_writable_parent(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            destination = root / "config.toml"
            destination.write_text("keep\n")
            destination.chmod(0o400)
            with self.assertRaises(FilesystemError):
                write_text_atomic(destination, "new\n")
            self.assertEqual(destination.read_text(), "keep\n")
            self.assertEqual(list(root.iterdir()), [destination])


if __name__ == "__main__":
    unittest.main()
