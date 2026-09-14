"""Verify guest application order, shared addresses, and literal login arguments."""

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, call

from limanix.guest import Guest
from limanix.lima import LimaClient, LimaError, LimaInstance, LimaNetwork, LimaStatus


class GuestTests(unittest.TestCase):
    def test_apply_installs_guest_wide_env_before_rebuild_and_reboot(self) -> None:
        client = Mock(spec=LimaClient)
        Guest(client).apply("sandbox", "developer")
        calls = client.mock_calls
        rebuild = next(
            index for index, item in enumerate(calls) if "nixos-rebuild" in item.args[1]
        )
        installs = [
            item for item in calls[:rebuild] if item.args[1][:2] == ["sudo", "install"]
        ]
        self.assertEqual(len(installs), 3)
        for name, item in zip(
            ("environment", "environment.sh"), installs[1:], strict=True
        ):
            self.assertEqual(
                item.args[1][2:],
                ["-m", "0644", f"/mnt/limanix/{name}", f"/etc/limanix/{name}"],
            )
        self.assertEqual(calls[rebuild].kwargs, {"capture": False})
        self.assertEqual(
            calls[rebuild + 1 : rebuild + 3],
            [call.stop("sandbox"), call.start("sandbox")],
        )
        self.assertEqual(calls[-1].args[1][-1], "true")
        self.assertIn("developer", calls[-1].args[1])

    def test_failed_rebuild_does_not_reboot_the_guest(self) -> None:
        client = Mock(spec=LimaClient)
        client.run.side_effect = ["", "", "", LimaError("build failed")]
        with self.assertRaisesRegex(LimaError, "build failed"):
            Guest(client).apply("sandbox", "dev")
        client.stop.assert_not_called()
        client.start.assert_not_called()

    def test_shared_address_uses_mac_not_interface_name_or_first_ipv4(self) -> None:
        client = Mock(spec=LimaClient)
        client.run.return_value = json.dumps(
            [
                {
                    "ifname": "eth0",
                    "address": "52:55:55:00:00:01",
                    "addr_info": [
                        {"family": "inet", "scope": "global", "local": "192.168.5.15"}
                    ],
                },
                {
                    "ifname": "enp0s2",
                    "address": "52:55:55:AA:BB:CC",
                    "addr_info": [
                        {"family": "inet6", "scope": "global", "local": "2001:db8::1"},
                        {"family": "inet", "scope": "global", "local": "192.0.2.10"},
                    ],
                },
            ]
        )
        instance = LimaInstance(
            "sandbox",
            LimaStatus.RUNNING,
            networks=(LimaNetwork("52:55:55:aa:bb:cc", True),),
        )
        self.assertEqual(Guest(client).address(instance), "192.0.2.10")

    def test_absent_or_unavailable_addresses_are_empty(self) -> None:
        client = Mock(spec=LimaClient)
        guest = Guest(client)
        self.assertEqual(guest.address(LimaInstance("sandbox", LimaStatus.STOPPED)), "")
        self.assertEqual(guest.address(LimaInstance("sandbox", LimaStatus.RUNNING)), "")
        client.run.assert_not_called()
        instance = LimaInstance(
            "sandbox",
            LimaStatus.RUNNING,
            networks=(LimaNetwork("52:55:55:aa:bb:cc", True),),
        )
        for output in ("not-json", "null", "[1]", '[{"address":null}]', "[]"):
            with self.subTest(output=output):
                client.run.return_value = output
                self.assertEqual(guest.address(instance), "")
        client.run.side_effect = LimaError("guest unavailable")
        self.assertEqual(guest.address(instance), "")

    def test_shell_keeps_exit_status_and_opens_interactive_login_only_once(
        self,
    ) -> None:
        client = Mock(spec=LimaClient)
        client.shell.return_value = 7
        self.assertEqual(Guest(client).shell("sandbox", "dev"), 7)
        command = client.shell.call_args.args[1]
        self.assertEqual(command.count("--login"), 1)
        self.assertEqual(command[-2:], ["/run/current-system/sw/bin/bash", "--login"])

    def test_login_command_preserves_literal_arguments_and_enters_user_home(
        self,
    ) -> None:
        values = [
            "space $value 'double\" end",
            "",
            "line one\nline two",
            "* ; : # `printf unintended`",
            "\\backslash\\",
            "unicode ✓",
        ]
        probe = (
            "import json, os, sys; "
            "print(json.dumps({'cwd': os.getcwd(), 'args': sys.argv[1:]}))"
        )
        command = Guest._user_command("dev", [sys.executable, "-c", probe, *values])
        script = command.index("-c") + 1
        with tempfile.TemporaryDirectory() as directory:
            result = subprocess.run(
                ["/bin/sh", "-c", *command[script:]],
                env={"HOME": directory},
                capture_output=True,
                text=True,
                check=True,
            )
            output = json.loads(result.stdout)
            self.assertEqual(Path(output["cwd"]), Path(directory).resolve())
        self.assertEqual(output["args"], values)


if __name__ == "__main__":
    unittest.main()
