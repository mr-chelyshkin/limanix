"""Apply the guest configuration and connect as its development user."""

import json
from collections.abc import Sequence
from ipaddress import AddressValueError, IPv4Address

from limanix.lima import LimaClient, LimaError, LimaInstance, LimaStatus


class Guest:
    """Guest-side operations over Lima's management connection."""

    def __init__(self, client: LimaClient) -> None:
        self.client = client

    @staticmethod
    def _user_command(user: str, command: Sequence[str]) -> list[str]:
        """Enter the user's home without reinterpreting command arguments."""
        bash = "/run/current-system/sw/bin/bash"
        # sudo --login reconstructs a shell command and re-expands its arguments.
        # Pass a fixed script and positional arguments to an explicit guest shell.
        login = ["--login"] if command else []
        target = list(command) if command else [bash, "--login"]
        return [
            "sudo",
            "--set-home",
            "--user",
            user,
            "--",
            bash,
            *login,
            "-c",
            'cd -- "$HOME" && exec "$@"',
            "limanix-command",
            *target,
        ]

    def apply(self, lima_name: str, user: str) -> None:
        """Install runtime ENV, build NixOS, and boot the selected generation."""
        self.client.run(
            lima_name, ["sudo", "install", "-d", "-m", "0755", "/etc/limanix"]
        )
        for name in ("environment", "environment.sh"):
            self.client.run(
                lima_name,
                [
                    "sudo",
                    "install",
                    "-m",
                    "0644",
                    f"/mnt/limanix/{name}",
                    f"/etc/limanix/{name}",
                ],
            )
        self.client.run(
            lima_name,
            [
                "sudo",
                "nixos-rebuild",
                "boot",
                "--flake",
                "path:/mnt/limanix/flake#runtime",
                "--no-write-lock-file",
            ],
            capture=False,
        )
        self.client.stop(lima_name)
        self.client.start(lima_name)
        self.client.run(lima_name, self._user_command(user, ["true"]))

    def address(self, instance: LimaInstance) -> str:
        """Find the shared interface's global IPv4 address by its configured MAC."""
        if instance.status != LimaStatus.RUNNING:
            return ""
        shared_macs = {
            network.mac_address.lower()
            for network in instance.networks
            if network.shared and network.mac_address
        }
        if not shared_macs:
            return ""
        try:
            interfaces: object = json.loads(
                self.client.run(instance.name, ["ip", "-j", "address", "show"])
            )
        except LimaError, ValueError:
            return ""
        if not isinstance(interfaces, list):
            return ""
        for interface in interfaces:
            if not isinstance(interface, dict):
                continue
            mac = interface.get("address")
            addresses = interface.get("addr_info", [])
            if (
                not isinstance(mac, str)
                or mac.lower() not in shared_macs
                or not isinstance(addresses, list)
            ):
                continue
            for address in addresses:
                if not isinstance(address, dict):
                    continue
                local = address.get("local")
                if (
                    address.get("family") == "inet"
                    and address.get("scope") == "global"
                    and isinstance(local, str)
                ):
                    try:
                        return str(IPv4Address(local))
                    except AddressValueError:
                        continue
        return ""

    def shell(self, lima_name: str, user: str, command: Sequence[str] = ()) -> int:
        """Run a command or attach a login shell as the development user."""
        return self.client.shell(lima_name, self._user_command(user, command))
