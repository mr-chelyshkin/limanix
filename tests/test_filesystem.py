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
    require_writable_directory,
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

    def test_writability_probe_leaves_no_files(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.assertEqual(require_writable_directory(root), root.resolve())
            self.assertEqual(list(root.iterdir()), [])

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

    @unittest.skipIf(os.geteuid() == 0, "Root bypasses Unix permission checks")
    def test_directory_without_write_access_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "readonly"
            target.mkdir(mode=0o500)
            try:
                with self.assertRaises(FilesystemError) as raised:
                    require_writable_directory(target)
                self.assertIn("Permission denied", raised.exception.reason)
            finally:
                target.chmod(0o700)


class AtomicWriteTests(unittest.TestCase):
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

            stream.write = partial_write
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
