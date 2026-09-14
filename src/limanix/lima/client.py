"""Run Lima commands through argument vectors without a host shell."""

import json
import platform
import shutil
import signal
import subprocess
from collections.abc import Sequence
from pathlib import Path

from limanix.config import Config
from limanix.lima.models import LimaInstance, LimaNetwork, LimaStatus
from limanix.lima.template import uses_vz


class LimaError(Exception):
    """A Lima operation failed or its host prerequisites are unavailable."""


def _parse_instance(value: object) -> LimaInstance:
    if not isinstance(value, dict):
        raise ValueError("expected an instance object")
    record: dict[str, object] = value
    name = record.get("name")
    status = record.get("status")
    disk = record.get("disk")
    if not isinstance(name, str) or not name or not isinstance(status, str):
        raise ValueError("expected an instance name and status")
    if disk is not None and (type(disk) is not int or disk < 0):
        raise ValueError("expected a disk size in bytes")
    config = record.get("config", {})
    if not isinstance(config, dict):
        raise ValueError("expected an instance configuration object")
    configured_networks = config.get("networks", [])
    if not isinstance(configured_networks, list):
        raise ValueError("expected configured networks")
    networks = []
    for network in configured_networks:
        if not isinstance(network, dict):
            raise ValueError("expected a network object")
        mac = network.get("macAddress", "")
        vz_nat = network.get("vzNAT", False)
        lima_network = network.get("lima", "")
        if (
            not isinstance(mac, str)
            or type(vz_nat) is not bool
            or not isinstance(lima_network, str)
        ):
            raise ValueError("invalid network identity")
        networks.append(
            LimaNetwork(
                mac_address=mac.lower(), shared=vz_nat or lima_network == "shared"
            )
        )
    return LimaInstance(name, LimaStatus(status), disk, tuple(networks))


class LimaClient:
    """Invoke the installed limactl; lifecycle policy belongs to the caller."""

    def __init__(self, executable: str = "limactl") -> None:
        self.executable = executable

    def _execute(
        self,
        arguments: Sequence[str],
        *,
        capture: bool = True,
        timeout: float | None = None,
    ) -> subprocess.CompletedProcess[str]:
        operation = f"{Path(self.executable).name} {arguments[0]}"
        try:
            result = subprocess.run(
                [self.executable, "--tty=false", *arguments],
                stdin=subprocess.DEVNULL,
                capture_output=capture,
                text=True,
                check=False,
                timeout=timeout,
            )
        except FileNotFoundError as error:
            raise LimaError(
                f"Cannot run {operation}: install Lima and make limactl available."
            ) from error
        except subprocess.TimeoutExpired as error:
            raise LimaError(
                f"{operation} timed out after {timeout:g} seconds."
            ) from error
        except OSError as error:
            raise LimaError(
                f"Cannot run {operation}: {error.strerror or error}."
            ) from error
        if result.returncode:
            detail = (result.stderr or "").strip()
            suffix = f": {detail}" if detail else "."
            raise LimaError(
                f"{operation} exited with status {result.returncode}{suffix}"
            )
        return result

    def preflight(self, config: Config) -> None:
        """Check host tools and shared networking without starting a VM."""
        if platform.system() != "Darwin":
            raise LimaError("Limanix VM operations require macOS.")
        if shutil.which(self.executable) is None:
            raise LimaError("Install Lima and make limactl available in PATH.")
        self._execute(["--version"], timeout=30)
        if uses_vz(config.resources.arch):
            release = platform.mac_ver()[0]
            if release and int(release.split(".")[0]) < 13:
                raise LimaError("VZ shared networking requires macOS 13 or newer.")
            return
        qemu = f"qemu-system-{config.resources.arch.lima_arch}"
        if shutil.which(qemu) is None:
            raise LimaError(f"Install QEMU and make {qemu} available in PATH.")
        try:
            self._execute(["sudoers", "--check"], timeout=30)
        except LimaError as error:
            raise LimaError(
                "QEMU shared networking requires socket_vmnet and Lima sudo access. "
                "Complete the setup at "
                "https://lima-vm.io/docs/config/network/vmnet/#socket_vmnet "
                f"and verify it with 'limactl sudoers --check'. {error}"
            ) from error

    def fetch_all(self) -> list[LimaInstance]:
        """Decode metadata for instances with Limanix's name prefix."""
        result = self._execute(["list", "--json"], timeout=30)
        instances = []
        try:
            for line in result.stdout.splitlines():
                if not line.strip():
                    continue
                value: object = json.loads(line)
                if isinstance(value, dict):
                    name = value.get("name")
                    if isinstance(name, str) and not name.startswith("limanix-"):
                        continue
                instances.append(_parse_instance(value))
        except (json.JSONDecodeError, ValueError) as error:
            raise LimaError("limactl list returned invalid instance JSON.") from error
        return instances

    def validate(self, template: Path) -> None:
        """Check a template with Lima's own schema and validation rules."""
        self._execute(["validate", "--", str(template.resolve())], timeout=60)

    def create(self, name: str, template: Path) -> None:
        """Create an instance from the supplied template, without starting it."""
        self._execute(["create", "--name", name, "--", str(template.resolve())])

    def start(self, name: str) -> None:
        """Start an existing instance and wait for Lima's readiness checks."""
        self._execute(["start", "--", name], capture=False)

    def stop(self, name: str) -> None:
        """Request an orderly shutdown."""
        self._execute(["stop", "--", name], capture=False)

    def delete(self, name: str, *, force: bool = False) -> None:
        """Delete an instance, optionally allowing Lima to kill its processes."""
        arguments = ["delete"]
        if force:
            arguments.append("--force")
        self._execute([*arguments, "--", name])

    def edit(self, name: str, template: Path) -> None:
        """Replace a stopped instance's configuration through Lima's --set API."""
        try:
            document = json.loads(template.read_text(encoding="utf-8"))
            if not isinstance(document, dict):
                raise ValueError("expected a JSON object")
        except (OSError, ValueError) as error:
            raise LimaError(
                f"Cannot read generated Lima template '{template}'."
            ) from error
        expression = ". = " + json.dumps(document)
        self._execute(["edit", "--set", expression, "--", name])

    def run(self, name: str, command: Sequence[str], *, capture: bool = True) -> str:
        """Run a guest command as the Lima management user, without a timeout."""
        if not command:
            raise LimaError("A guest command is required.")
        result = self._execute(
            ["shell", "--workdir", "/", name, "--", *command], capture=capture
        )
        return result.stdout or ""

    def shell(self, name: str, command: Sequence[str] = ()) -> int:
        """Attach inherited terminal streams and let SSH handle terminal signals."""
        arguments = [self.executable, "shell", "--workdir", "/", name]
        if command:
            arguments.extend(["--", *command])
        try:
            with subprocess.Popen(arguments) as process:
                previous = signal.signal(signal.SIGINT, signal.SIG_IGN)
                try:
                    status = process.wait()
                finally:
                    signal.signal(signal.SIGINT, previous)
        except OSError as error:
            raise LimaError(
                f"Cannot run limactl shell: {error.strerror or error}."
            ) from error
        return status if status >= 0 else 128 - status
