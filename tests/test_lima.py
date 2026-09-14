"""Check Lima configuration boundaries and subprocess behavior without a VM."""

import json
import signal
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from limanix.config import Config, Mount, Resources, User
from limanix.domain import Architecture, EnvName, EnvValue, GuestPath, Username
from limanix.lima import (
    LimaClient,
    LimaError,
    LimaInstance,
    LimaNetwork,
    LimaStatus,
    render_lima,
)


class TemplateTests(unittest.TestCase):
    def test_native_template_exposes_only_requested_mounts_and_no_env(self) -> None:
        config = Config(
            user=User(
                name=Username("developer"),
                home=GuestPath("/home/developer"),
                sudo=False,
            ),
            mounts=[
                Mount(source="/projects/service", target=GuestPath("/workspace")),
                Mount(source="/keys/git", target=GuestPath("/mnt/keys"), mode="ro"),
            ],
            env={EnvName("API_TOKEN"): EnvValue("not-for-the-lima-config")},
        )
        rendered = render_lima(
            config,
            Path("/managed/home"),
            Path("/managed/runtime"),
            host_arch="arm64",
            host_uid=501,
        )
        template = json.loads(rendered)
        self.assertEqual(template["vmType"], "vz")
        self.assertEqual(template["arch"], "aarch64")
        self.assertEqual(template["mountType"], "virtiofs")
        self.assertEqual(template["networks"], [{"vzNAT": True}])
        mounts = {mount["mountPoint"]: mount for mount in template["mounts"]}
        self.assertEqual(
            set(mounts), {"/home/developer", "/mnt/limanix", "/workspace", "/mnt/keys"}
        )
        self.assertTrue(mounts["/home/developer"]["writable"])
        self.assertEqual(mounts["/home/developer"]["location"], "/managed/home")
        self.assertTrue(mounts["/workspace"]["writable"])
        self.assertFalse(mounts["/mnt/keys"]["writable"])
        self.assertFalse(mounts["/mnt/limanix"]["writable"])
        self.assertNotIn("API_TOKEN", rendered)
        self.assertNotIn("not-for-the-lima-config", rendered)
        self.assertFalse(template["propagateProxyEnv"])
        self.assertFalse(template["ssh"]["forwardAgent"])
        self.assertTrue(template["portForwards"][0]["ignore"])
        self.assertFalse(template["portForwards"][0]["guestIPMustBeZero"])

    def test_foreign_guest_uses_qemu_shared_network_and_a_pinned_image(self) -> None:
        config = Config(resources=Resources(arch=Architecture.AMD64), mounts=[])
        template = json.loads(
            render_lima(
                config,
                Path("/managed/home"),
                Path("/managed/runtime"),
                host_arch="aarch64",
                host_uid=501,
            )
        )
        self.assertEqual(template["vmType"], "qemu")
        self.assertEqual(template["mountType"], "9p")
        self.assertEqual(template["networks"], [{"lima": "shared"}])
        image = template["images"][0]
        self.assertEqual(image["arch"], "x86_64")
        self.assertIn("/v0.2.1/nixos-lima-v0.2.1-x86_64.qcow2", image["location"])
        self.assertEqual(len(image["digest"].removeprefix("sha256:")), 64)

    def test_management_user_does_not_share_the_development_uid(self) -> None:
        for uid in (501, 1000):
            with self.subTest(uid=uid):
                template = json.loads(
                    render_lima(
                        Config(user=User(sudo=False), mounts=[]),
                        Path("/managed/home"),
                        Path("/managed/runtime"),
                        host_arch="arm64",
                        host_uid=uid,
                    )
                )
                self.assertEqual(template["user"]["name"], "limanix-admin")
                self.assertNotEqual(template["user"]["uid"], uid)
                self.assertTrue(template["user"]["passwordlessSudo"])

    def test_parent_mounts_precede_nested_mounts(self) -> None:
        config = Config(
            mounts=[
                Mount(source="/nested", target=GuestPath("/workspace/data")),
                Mount(source="/project", target=GuestPath("/workspace")),
            ]
        )
        template = json.loads(
            render_lima(
                config,
                Path("/managed/home"),
                Path("/managed/runtime"),
                host_arch="arm64",
                host_uid=501,
            )
        )
        targets = [mount["mountPoint"] for mount in template["mounts"]]
        self.assertLess(targets.index("/workspace"), targets.index("/workspace/data"))


class ClientTests(unittest.TestCase):
    def test_fetch_all_decodes_only_typed_instance_metadata(self) -> None:
        client = LimaClient()
        for output, expected in (
            ("", []),
            (
                '{"name":"limanix-one","status":"Stopped"}\n'
                '{"name":"limanix-two","status":"Running","disk":10737418240,'
                '"config":{"env":{"SECRET":"not-returned"},"networks":['
                '{"macAddress":"52:55:55:AA:BB:CC","vzNAT":true},'
                '{"macAddress":"52:55:55:DD:EE:FF","lima":"shared"},'
                '{"macAddress":"52:55:55:00:11:22","lima":"user-v2"}]}}\n',
                [
                    LimaInstance("limanix-one", LimaStatus.STOPPED),
                    LimaInstance(
                        "limanix-two",
                        LimaStatus.RUNNING,
                        10737418240,
                        (
                            LimaNetwork("52:55:55:aa:bb:cc", True),
                            LimaNetwork("52:55:55:dd:ee:ff", True),
                            LimaNetwork("52:55:55:00:11:22", False),
                        ),
                    ),
                ],
            ),
        ):
            with (
                self.subTest(output=output),
                patch(
                    "limanix.lima.client.subprocess.run",
                    return_value=subprocess.CompletedProcess([], 0, output, ""),
                ),
            ):
                self.assertEqual(client.fetch_all(), expected)

    def test_fetch_all_ignores_unowned_status_and_network_shapes(self) -> None:
        output = (
            '{"name":"colima","status":"FutureState",'
            '"config":{"networks":"plugin-managed"}}\n'
            '{"name":"other-limanix-vm","status":"Running",'
            '"config":{"networks":[{"macAddress":42,"vzNAT":"on"}]}}\n'
            '{"name":"limanix-owned","status":"Stopped"}\n'
        )
        with patch(
            "limanix.lima.client.subprocess.run",
            return_value=subprocess.CompletedProcess([], 0, output, ""),
        ):
            self.assertEqual(
                LimaClient().fetch_all(),
                [LimaInstance("limanix-owned", LimaStatus.STOPPED)],
            )

    def test_fetch_all_rejects_malformed_records_and_unknown_statuses(self) -> None:
        for output in (
            "not-json",
            "[]",
            '{"status":"Stopped"}',
            '{"name":42,"status":"Stopped"}',
            '{"name":"limanix-one"}',
            '{"name":"limanix-one","status":"FutureState"}',
            '{"name":"limanix-one","status":"Running","disk":true}',
            '{"name":"limanix-one","status":"Running","disk":"10GiB"}',
            '{"name":"limanix-one","status":"Running","config":{"networks":[1]}}',
            '{"name":"limanix-one","status":"Running",'
            '"config":{"networks":"plugin-managed"}}',
        ):
            with (
                self.subTest(output=output),
                patch(
                    "limanix.lima.client.subprocess.run",
                    return_value=subprocess.CompletedProcess([], 0, output, ""),
                ),
                self.assertRaises(LimaError),
            ):
                LimaClient().fetch_all()

    def test_all_upstream_status_values_are_preserved(self) -> None:
        for status in LimaStatus:
            output = json.dumps({"name": "limanix-one", "status": status.value})
            with (
                self.subTest(status=status),
                patch(
                    "limanix.lima.client.subprocess.run",
                    return_value=subprocess.CompletedProcess([], 0, output, ""),
                ),
            ):
                self.assertEqual(LimaClient().fetch_all()[0].status, status)

    def test_guest_arguments_are_not_interpreted_by_a_host_shell(self) -> None:
        command = ["printf", "%s", 'spaces; $(touch /tmp/unwanted) "quotes"']
        with patch(
            "limanix.lima.client.subprocess.run",
            return_value=subprocess.CompletedProcess([], 0, "result", ""),
        ) as run:
            self.assertEqual(LimaClient().run("sandbox", command), "result")
        arguments = run.call_args.args[0]
        self.assertEqual(arguments[-len(command) :], command)
        self.assertIn("--", arguments)
        self.assertFalse(run.call_args.kwargs.get("shell", False))
        self.assertIsNone(run.call_args.kwargs["timeout"])

    def test_failure_includes_stderr_but_does_not_echo_guest_arguments(self) -> None:
        with (
            patch(
                "limanix.lima.client.subprocess.run",
                return_value=subprocess.CompletedProcess(
                    [], 4, "", "guest unavailable\n"
                ),
            ),
            self.assertRaises(LimaError) as raised,
        ):
            LimaClient().run("sandbox", ["command", "secret-argument"])
        self.assertIn("limactl shell", str(raised.exception))
        self.assertIn("guest unavailable", str(raised.exception))
        self.assertNotIn("secret-argument", str(raised.exception))

    def test_edit_uses_a_data_expression_without_reading_host_files_through_yq(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            template = Path(directory) / "template.yaml"
            document = {"mounts": [{"location": '/path/with "quotes" and $characters'}]}
            template.write_text(json.dumps(document), encoding="utf-8")
            with patch(
                "limanix.lima.client.subprocess.run",
                return_value=subprocess.CompletedProcess([], 0, "", ""),
            ) as run:
                LimaClient().edit("sandbox", template)
        arguments = run.call_args.args[0]
        expression = arguments[arguments.index("--set") + 1]
        self.assertEqual(json.loads(expression.removeprefix(". = ")), document)
        self.assertNotIn("load(", expression)

    def test_qemu_network_preflight_reports_required_setup(self) -> None:
        with (
            patch("limanix.lima.client.platform.system", return_value="Darwin"),
            patch("limanix.lima.client.shutil.which", return_value="/installed/tool"),
            patch("limanix.lima.client.uses_vz", return_value=False),
            patch.object(
                LimaClient,
                "_execute",
                side_effect=[
                    subprocess.CompletedProcess([], 0, "2.2.0", ""),
                    LimaError("sudoers unavailable"),
                ],
            ),
            self.assertRaises(LimaError) as raised,
        ):
            LimaClient().preflight(Config(resources=Resources(arch=Architecture.AMD64)))
        self.assertIn("socket_vmnet", str(raised.exception))
        self.assertIn("limactl sudoers --check", str(raised.exception))

    def test_interactive_shell_inherits_streams_and_returns_signal_status(self) -> None:
        with (
            patch("limanix.lima.client.subprocess.Popen") as spawn,
            patch(
                "limanix.lima.client.signal.signal",
                return_value=signal.default_int_handler,
            ) as handler,
        ):
            process = spawn.return_value.__enter__.return_value
            process.wait.return_value = -signal.SIGTERM
            result = LimaClient().shell("sandbox", ["sudo", "-iu", "dev"])
        self.assertEqual(result, 128 + signal.SIGTERM)
        self.assertEqual(spawn.call_args.kwargs, {})
        self.assertEqual(
            spawn.call_args.args[0],
            [
                "limactl",
                "shell",
                "--workdir",
                "/",
                "sandbox",
                "--",
                "sudo",
                "-iu",
                "dev",
            ],
        )
        self.assertEqual(
            handler.call_args.args, (signal.SIGINT, signal.default_int_handler)
        )


if __name__ == "__main__":
    unittest.main()
