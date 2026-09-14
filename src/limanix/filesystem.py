"""Validate local paths and report filesystem failures to callers."""

import os
import stat
import tempfile
from pathlib import Path


class FilesystemError(Exception):
    """A failed filesystem operation with its path and human-readable reason."""

    def __init__(self, path: Path, operation: str, reason: str) -> None:
        self.operation = operation
        self.reason = reason
        self.path = path
        super().__init__(f"Cannot {operation} '{path}': {reason}.")


def require_directory(path: Path) -> Path:
    """Return an absolute, resolved path to an existing directory.

    Expand ``~`` and follow directory symlinks.
    Raise FilesystemError for missing paths, other file types, invalid paths, or access.
    """
    try:
        resolved = path.expanduser().resolve(strict=True)
        metadata = resolved.stat()
    except FileNotFoundError as error:
        raise FilesystemError(path, "use directory", "path does not exist") from error
    except NotADirectoryError as error:
        raise FilesystemError(
            path, "use directory", "path is not a directory"
        ) from error
    except PermissionError as error:
        raise FilesystemError(path, "use directory", "permission denied") from error
    except (OSError, ValueError, RuntimeError) as error:
        raise FilesystemError(path, "use directory", str(error)) from error
    if not stat.S_ISDIR(metadata.st_mode):
        raise FilesystemError(path, "use directory", "path is not a directory")
    return resolved


def _existing_file_mode(path: Path) -> int | None:
    try:
        metadata = path.lstat()
    except FileNotFoundError:
        return None
    if stat.S_ISLNK(metadata.st_mode):
        raise FilesystemError(path, "write file", "symbolic links are not allowed")
    if not stat.S_ISREG(metadata.st_mode):
        raise FilesystemError(path, "write file", "path is not a regular file")
    descriptor = os.open(path, os.O_WRONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
    try:
        opened = os.fstat(descriptor)
        if not stat.S_ISREG(opened.st_mode):
            raise FilesystemError(path, "write file", "path is not a regular file")
        return stat.S_IMODE(opened.st_mode)
    finally:
        os.close(descriptor)


def write_text_atomic(path: Path, text: str, *, mode: int | None = None) -> Path:
    """Write UTF-8 text through a temporary file in the same directory.

    Require an existing writable parent directory.
    Reject destination symlinks and non-regular files;
    check existing file write access without truncation.

    Preserve existing permission bits unless ``mode`` is supplied;
    new files are private (0600) by default.
    Other inode metadata is not copied.
    Return the absolute destination path.

    Write, flush, and sync the temporary file before replacing the destination,
    then sync the containing directory to persist the replacement.
    Failures before replacement leave the destination unchanged.
    A directory sync failure is reported after the new content is installed.
    """
    directory = require_directory(path.parent)
    destination = directory / path.name
    temporary: Path | None = None
    failure: FilesystemError | None = None
    try:
        existing_mode = _existing_file_mode(destination)
        with tempfile.NamedTemporaryFile(
            mode="w",
            encoding="utf-8",
            dir=directory,
            prefix=f".{destination.name}-",
            suffix=".tmp",
            delete=False,
        ) as stream:
            temporary = Path(stream.name)
            os.fchmod(
                stream.fileno(),
                mode
                if mode is not None
                else existing_mode
                if existing_mode is not None
                else 0o600,
            )
            stream.write(text)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, destination)
        temporary = None
        descriptor = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
    except FilesystemError as error:
        failure = error
        raise
    except (OSError, ValueError) as error:
        reason = error.strerror if isinstance(error, OSError) else str(error)
        failure = FilesystemError(destination, "write file", reason or str(error))
        raise failure from error
    finally:
        if temporary is not None:
            try:
                temporary.unlink(missing_ok=True)
            except OSError as error:
                reason = error.strerror or str(error)
                if failure is not None:
                    raise FilesystemError(
                        failure.path,
                        failure.operation,
                        f"{failure.reason}; cannot remove temporary file "
                        f"'{temporary}': {reason}",
                    ) from failure
                raise FilesystemError(
                    temporary, "remove temporary file", reason
                ) from error
    return destination
