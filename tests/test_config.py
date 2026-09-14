"""Validate configuration boundaries without a Lima installation or host mounts."""

import json
import os
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest.mock import patch

from limanix.config.models import Config, config_to_dict
from limanix.config.parser import ConfigError, load_config, parse_config
from limanix.config.template import render_config
from limanix.domain import (
    Architecture,
    ByteSize,
    DomainError,
    EnvName,
    EnvValue,
    GuestPath,
    ModuleId,
    ModuleName,
    Username,
    VMName,
)


class ConfigParserTests(unittest.TestCase):
    def test_domain_types_and_byte_sizes_have_a_stable_public_representation(
        self,
    ) -> None:
        config = parse_config(
            {
                "name": "rust-box",
                "resources": {"arch": "amd64", "disk": "20GiB", "mem": "16GiB"},
                "nixos": {"modules": ["git", "third-party:tools"]},
                "env": {"TOKEN": "literal $HOME"},
            }
        )
        self.assertIsInstance(config.name, VMName)
        self.assertIsInstance(config.user.name, Username)
        self.assertIsInstance(config.user.home, GuestPath)
        self.assertIsInstance(config.mounts[0].target, GuestPath)
        self.assertIs(config.resources.arch, Architecture.AMD64)
        self.assertIsInstance(config.resources.disk, ByteSize)
        self.assertEqual(config.resources.disk, 20 * 1024**3)
        self.assertEqual(config.resources.mem, 16 * 1024**3)
        self.assertIsInstance(config.nixos.modules[0], ModuleId)
        self.assertIsInstance(next(iter(config.env)), EnvName)
        self.assertIsInstance(next(iter(config.env.values())), EnvValue)
        encoded = config_to_dict(config)
        self.assertEqual(
            encoded["resources"],
            {"arch": "amd64", "disk": "20GiB", "cpu": 4, "mem": "16GiB"},
        )
        self.assertEqual(parse_config(json.loads(json.dumps(encoded))), config)
        self.assertEqual(parse_config(tomllib.loads(render_config(config))), config)

    def test_every_reserved_system_tree_rejects_descendant_mounts(self) -> None:
        for root in (
            "/etc",
            "/boot",
            "/usr",
            "/var",
            "/nix",
            "/run",
            "/dev",
            "/proc",
            "/sys",
            "/bin",
            "/sbin",
        ):
            for target in (root, root + "/child"):
                cases: tuple[dict[str, object], ...] = (
                    {"user": {"home": target}, "mounts": []},
                    {"mounts": [{"source": "./project", "target": target}]},
                )
                for data in cases:
                    with (
                        self.subTest(target=target, data=data),
                        self.assertRaises(ConfigError),
                    ):
                        parse_config(data)
        for target in ("/etc-project", "/var-project", "/home/dev/.config/nvim"):
            self.assertEqual(
                parse_config({"mounts": [{"source": "./project", "target": target}]})
                .mounts[0]
                .target,
                target,
            )

    def test_defaults_and_generated_template_round_trip(self) -> None:
        expected = config_to_dict(Config())
        self.assertEqual(config_to_dict(parse_config({})), expected)
        self.assertEqual(
            config_to_dict(parse_config(tomllib.loads(render_config()))), expected
        )

    def test_decoded_defaults_do_not_share_mutable_collections(self) -> None:
        changed = parse_config({})
        changed.nixos.modules.append(ModuleId("rust"))
        changed.env[EnvName("CUSTOM")] = EnvValue("value")
        changed.mounts[0].source = "./different-project"
        changed.network.ports.tcp.append(9090)
        fresh = parse_config({})
        self.assertEqual(config_to_dict(fresh), config_to_dict(Config()))

    def test_partial_tables_fill_defaults_and_empty_collections_stay_empty(
        self,
    ) -> None:
        config = parse_config(
            {
                "name": "rust-box-2",
                "resources": {"arch": "amd64", "cpu": 2},
                "nixos": {"modules": []},
                "network": {"ports": {"tcp": [], "udp": [1, 65535]}},
                "env": {},
                "mounts": [],
            }
        )
        self.assertEqual(config.resources.arch, "amd64")
        self.assertEqual(config.resources.cpu, 2)
        self.assertEqual(config.resources.mem, Config().resources.mem)
        self.assertEqual(config.env, {})
        self.assertEqual(config.mounts, [])
        self.assertEqual(config.nixos.modules, [])
        self.assertEqual(config.network.ports.tcp, [])
        self.assertEqual(config.network.ports.udp, [1, 65535])

    def test_unknown_fields_have_qualified_diagnostics(self) -> None:
        cases: list[tuple[dict[str, object], str]] = [
            ({"unknown": True}, "unknown"),
            ({"resources": {"cpus": 4}}, "resources.cpus"),
            ({"user": {"uid": 1000}}, "user.uid"),
            ({"home": {"path": "/opt/limanix"}}, "home.path"),
            ({"nixos": {"module": "git"}}, "nixos.module"),
            ({"network": {"bridge": "en0"}}, "network.bridge"),
            ({"network": {"ports": {"http": [80]}}}, "network.ports.http"),
            ({"mounts": [{"wrong": "mount"}]}, "mounts[0].wrong"),
        ]
        for data, field in cases:
            with self.subTest(data=data), self.assertRaises(ConfigError) as raised:
                parse_config(data)
            self.assertEqual(str(raised.exception), f"{field}: unknown field")

    def test_types_are_not_coerced(self) -> None:
        cases: list[tuple[dict[str, object], str]] = [
            ({"schema_version": True}, "schema_version"),
            ({"schema_version": "1"}, "schema_version"),
            ({"name": 1}, "name"),
            ({"resources": []}, "resources"),
            ({"resources": {"cpu": True}}, "resources.cpu"),
            ({"resources": {"cpu": 1.0}}, "resources.cpu"),
            ({"user": {"sudo": 1}}, "user.sudo"),
            ({"user": {"sudo": "false"}}, "user.sudo"),
            ({"network": {"ports": {"tcp": [True]}}}, "network.ports.tcp[0]"),
            ({"network": {"ports": {"tcp": "8080"}}}, "network.ports.tcp"),
            ({"nixos": {"modules": "git"}}, "nixos.modules"),
            ({"nixos": {"modules": [1]}}, "nixos.modules[0]"),
            ({"mounts": {}}, "mounts"),
            ({"mounts": ["/workspace"]}, "mounts[0]"),
            ({"env": []}, "env"),
            ({"env": {"PORT": 8080}}, "env.PORT"),
        ]
        for data, field in cases:
            with self.subTest(data=data), self.assertRaises(ConfigError) as raised:
                parse_config(data)
            self.assertTrue(str(raised.exception).startswith(f"{field}:"))

    def test_invalid_values_are_rejected_at_their_field(self) -> None:
        cases: list[tuple[dict[str, object], str]] = [
            ({"schema_version": 2}, "schema_version"),
            ({"name": "-box"}, "name"),
            ({"name": "box-"}, "name"),
            ({"name": "Box"}, "name"),
            ({"name": "box.local"}, "name"),
            ({"name": "x" * 64}, "name"),
            ({"resources": {"arch": "x86_64"}}, "resources.arch"),
            ({"resources": {"cpu": 0}}, "resources.cpu"),
            ({"resources": {"cpu": -1}}, "resources.cpu"),
            ({"resources": {"mem": "8GB"}}, "resources.mem"),
            ({"resources": {"mem": "0GiB"}}, "resources.mem"),
            ({"resources": {"disk": "1.5GiB"}}, "resources.disk"),
            ({"user": {"name": "root"}}, "user.name"),
            ({"user": {"name": "limanix-admin"}}, "user.name"),
            ({"user": {"name": "dev:1000"}}, "user.name"),
            ({"user": {"home": "home/dev"}}, "user.home"),
            ({"user": {"home": "/home/../dev"}}, "user.home"),
            ({"user": {"home": "/etc"}}, "user.home"),
            ({"user": {"home": "/home/limanix-admin"}}, "user.home"),
            ({"user": {"home": "/home"}}, "user.home"),
            ({"home": {"root": ""}}, "home.root"),
            ({"home": {"root": "/"}}, "home.root"),
            ({"home": {"root": "/opt/.."}}, "home.root"),
            ({"network": {"mode": "bridged"}}, "network.mode"),
            ({"network": {"ports": {"tcp": [0]}}}, "network.ports.tcp[0]"),
            ({"network": {"ports": {"udp": [65536]}}}, "network.ports.udp[0]"),
            ({"nixos": {"modules": ["./rust.nix"]}}, "nixos.modules[0]"),
            ({"nixos": {"modules": ["third-party:../rust"]}}, "nixos.modules[0]"),
        ]
        for data, field in cases:
            with self.subTest(data=data), self.assertRaises(ConfigError) as raised:
                parse_config(data)
            self.assertTrue(str(raised.exception).startswith(f"{field}:"))

    def test_module_identifiers_do_not_require_registry_access(self) -> None:
        modules = ["git", "rust", "neovim", "future-module", "third-party:my-rust"]
        config = parse_config({"nixos": {"modules": modules}})
        self.assertEqual(config.nixos.modules, modules)

    def test_duplicate_module_identifiers_are_rejected_before_snapshotting(
        self,
    ) -> None:
        for name in ("git", "third-party:my-editor"):
            with self.subTest(name=name), self.assertRaises(ConfigError) as raised:
                parse_config({"nixos": {"modules": [name, name]}})
            self.assertEqual(
                str(raised.exception), "nixos.modules[1]: duplicate module ID"
            )

    def test_environment_rejects_characters_unsupported_by_guest_files(self) -> None:
        for character in (
            "\ufeff",
            "\ud800",
            "\udfff",
            "\ufdd0",
            "\ufdef",
            "\ufffe",
            "\U0001ffff",
        ):
            with self.subTest(codepoint=hex(ord(character))):
                with self.assertRaises(ConfigError) as raised:
                    parse_config(
                        {"env": {"TOKEN": "never-display-this-value" + character}}
                    )
                self.assertTrue(str(raised.exception).startswith("env.TOKEN:"))
                self.assertNotIn("never-display-this-value", str(raised.exception))
        environment = {"TOKEN": "Привет 🐍\n\t\r"}
        self.assertEqual(parse_config({"env": environment}).env, environment)

    def test_environment_values_stay_literal_and_are_not_in_diagnostics(self) -> None:
        env = {"EMPTY": "", "TOKEN": "secret with $HOME\nand quotes '\""}
        self.assertEqual(parse_config({"env": env}).env, env)
        cases: tuple[dict[str, object], ...] = (
            {"env": {"BAD-NAME": "never-display-this-value"}},
            {"env": {"TOKEN": "never-display-this-value\x00"}},
        )
        for data in cases:
            with self.subTest(data=data), self.assertRaises(ConfigError) as raised:
                parse_config(data)
            self.assertNotIn("never-display-this-value", str(raised.exception))

    def test_mounts_require_source_target_and_rw_or_ro(self) -> None:
        for mount in (
            {"target": "/workspace"},
            {"source": "./project"},
            {"source": "", "target": "/workspace"},
            {"source": "./project", "target": "./workspace"},
            {"source": "./project", "target": "/workspace/../etc"},
            {"source": "./project", "target": "/workspace", "mode": "write"},
        ):
            with self.subTest(mount=mount), self.assertRaises(ConfigError):
                parse_config({"mounts": [mount]})
        config = parse_config(
            {"mounts": [{"source": "./project", "target": "/workspace/"}]}
        )
        self.assertEqual(config.mounts[0].mode, "rw")
        self.assertEqual(config.mounts[0].target, "/workspace")

    def test_guest_mount_paths_reject_unescaped_fstab_whitespace(self) -> None:
        for whitespace in (" ", "\t", "\n", "\r"):
            cases: tuple[tuple[dict[str, object], str], ...] = (
                ({"user": {"home": f"/home/dev{whitespace}home"}}, "user.home"),
                (
                    {
                        "mounts": [
                            {
                                "source": "./project",
                                "target": f"/workspace{whitespace}project",
                            }
                        ]
                    },
                    "mounts[0].target",
                ),
            )
            for data, field in cases:
                with self.subTest(whitespace=repr(whitespace), field=field):
                    with self.assertRaises(ConfigError) as raised:
                        parse_config(data)
                    self.assertTrue(str(raised.exception).startswith(f"{field}:"))
                    self.assertIn("fstab", str(raised.exception))

    def test_host_mount_paths_with_spaces_remain_supported(self) -> None:
        config = parse_config(
            {
                "home": {"root": "./managed homes"},
                "mounts": [{"source": "~/projects/my project", "target": "/workspace"}],
            }
        )
        self.assertEqual(config.home.root, "./managed homes")
        self.assertEqual(config.mounts[0].source, "~/projects/my project")

    def test_mounts_cannot_hide_managed_home_or_reserved_system_paths(self) -> None:
        for target in (
            "/",
            "/nix",
            "/nix/store",
            "/etc",
            "/etc/limanix",
            "/etc/limanix/environment",
            "/dev",
            "/proc",
            "/sys",
            "/run",
            "/mnt/limanix",
            "/mnt/limanix/config",
            "/home",
            "/home/dev",
            "/home/limanix-admin",
            "/home/limanix-admin/.ssh",
        ):
            with self.subTest(target=target), self.assertRaises(ConfigError):
                parse_config({"mounts": [{"source": "./project", "target": target}]})

    def test_nested_home_mounts_and_independent_user_directories_are_allowed(
        self,
    ) -> None:
        config = parse_config(
            {
                "mounts": [
                    {
                        "source": "./nvim",
                        "target": "/home/dev/.config/nvim",
                        "mode": "ro",
                    },
                    {"source": "./service", "target": "/opt/service"},
                ]
            }
        )
        self.assertEqual(len(config.mounts), 2)

    def test_explicit_mount_overlap_is_rejected_in_either_order(self) -> None:
        for targets in (
            ["/workspace", "/workspace"],
            ["/workspace", "/workspace/subdir"],
            ["/workspace/subdir", "/workspace"],
        ):
            with (
                self.subTest(targets=targets),
                self.assertRaises(ConfigError) as raised,
            ):
                parse_config(
                    {
                        "mounts": [
                            {"source": "./project", "target": target}
                            for target in targets
                        ]
                    }
                )
            self.assertIn("overlaps", str(raised.exception))

    def test_parse_does_not_expand_or_resolve_paths(self) -> None:
        with (
            patch.object(
                Path, "resolve", side_effect=AssertionError("filesystem access")
            ),
            patch.object(Path, "expanduser", side_effect=AssertionError("home lookup")),
        ):
            config = parse_config(
                {
                    "home": {"root": "./homes"},
                    "mounts": [
                        {"source": "~/$PROJECT", "target": "/workspace"},
                    ],
                }
            )
        self.assertEqual(config.home.root, "./homes")
        self.assertEqual(config.mounts[0].source, "~/$PROJECT")


class DomainValueTests(unittest.TestCase):
    def test_username_rules_are_shared_with_configuration_decoding(self) -> None:
        for text in ("dev", "_dev", "rust-dev_2", "a" * 32):
            with self.subTest(text=text):
                username = Username(text)
                self.assertEqual(username, text)
                self.assertEqual(
                    parse_config({"user": {"name": text}}).user.name, username
                )
        invalid: tuple[object, ...] = (
            "",
            "root",
            "limanix-admin",
            "2dev",
            "Dev",
            "dev:1000",
            "dev\x00",
            "a" * 33,
            1,
        )
        for invalid_value in invalid:
            with self.subTest(text=repr(invalid_value)):
                with self.assertRaises(DomainError):
                    Username(invalid_value)
                with self.assertRaises(ConfigError):
                    parse_config({"user": {"name": invalid_value}})

    def test_guest_path_normalization_is_shared_without_filesystem_access(self) -> None:
        with patch.object(
            Path, "resolve", side_effect=AssertionError("filesystem access")
        ):
            home = GuestPath("/home/./dev//")
            target = GuestPath("/workspace//project/./")
            config = parse_config(
                {
                    "user": {"home": "/home/./dev//"},
                    "mounts": [
                        {"source": "./project", "target": "/workspace//project/./"}
                    ],
                }
            )
        self.assertEqual(home, "/home/dev")
        self.assertEqual(target, "/workspace/project")
        self.assertEqual(config.user.home, home)
        self.assertEqual(config.mounts[0].target, target)
        self.assertEqual(parse_config(tomllib.loads(render_config(config))), config)

    def test_guest_path_syntax_and_reserved_mount_policy_remain_separate(self) -> None:
        invalid: tuple[object, ...] = (
            "",
            "/",
            "/./",
            "relative/path",
            "//home/dev",
            "/home/../dev",
            "/home/dev space",
            "/home/dev\tname",
            "/home/dev\nname",
            "/home/dev\rname",
            "/home/dev\x00name",
            1,
        )
        for value in invalid:
            with self.subTest(value=repr(value)):
                with self.assertRaises(DomainError):
                    GuestPath(value)
                with self.assertRaises(ConfigError):
                    parse_config({"user": {"home": value}})
                with self.assertRaises(ConfigError):
                    parse_config({"mounts": [{"source": "./project", "target": value}]})
        self.assertEqual(GuestPath("/etc/service"), "/etc/service")
        with self.assertRaises(ConfigError):
            parse_config(
                {"mounts": [{"source": "./project", "target": "/etc/service"}]}
            )

    def test_identity_values_validate_consistently_outside_the_parser(self) -> None:
        self.assertEqual(VMName("2-rust-box"), "2-rust-box")
        self.assertEqual(ModuleName("my-tools"), "my-tools")
        builtin = ModuleId("rust")
        imported = ModuleId("third-party:my-tools")
        self.assertFalse(builtin.is_third_party)
        self.assertEqual(builtin.name, ModuleName("rust"))
        self.assertTrue(imported.is_third_party)
        self.assertEqual(imported.name, ModuleName("my-tools"))
        for constructor, values in (
            (VMName, ("../box", "UPPER", "-name", "name-", "x" * 64)),
            (ModuleName, ("2tools", "a--b", "../tools")),
            (ModuleId, ("./tools.nix", "third-party:../tools", "third-party:")),
            (EnvName, ("1TOKEN", "TOKEN-NAME", "TOKEN\nNAME")),
            (EnvValue, ("secret\0", "secret\ufeff", "secret\ud800")),
        ):
            for value in values:
                with (
                    self.subTest(constructor=constructor.__name__, value=repr(value)),
                    self.assertRaises(DomainError),
                ):
                    constructor(value)

    def test_byte_sizes_are_numeric_and_architecture_mapping_is_explicit(self) -> None:
        size = ByteSize.parse("8GiB")
        self.assertEqual(int(size), 8 * 1024**3)
        self.assertEqual(size.to_gib(), "8GiB")
        self.assertGreater(size, ByteSize.parse("4GiB"))
        self.assertEqual(Architecture.ARM64.lima_arch, "aarch64")
        self.assertEqual(Architecture.AMD64.lima_arch, "x86_64")
        for value in (True, 0, -1):
            with self.subTest(value=value), self.assertRaises(DomainError):
                ByteSize(value)
        for serialized in ("0GiB", "1.5GiB", "8GB", 8, True):
            with self.subTest(value=serialized), self.assertRaises(DomainError):
                ByteSize.parse(serialized)
        with self.assertRaises(DomainError):
            ByteSize(1).to_gib()


class ConfigFileTests(unittest.TestCase):
    def test_resolves_host_paths_from_file_and_preserves_environment_literals(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config_file = root / "box.toml"
            config_file.write_text(
                '[home]\nroot = "./homes"\n'
                '[env]\nHOME_TOKEN = "$HOME"\n'
                '[[mounts]]\nsource = "./missing-project"\ntarget = "/workspace"\n'
                '[[mounts]]\nsource = "~/$PROJECT"\ntarget = "/mnt/other"\n',
                encoding="utf-8",
            )
            with patch.dict(os.environ, {"PROJECT": "should-not-be-expanded"}):
                config = load_config(config_file)
            self.assertEqual(config.home.root, str(root.resolve() / "homes"))
            self.assertEqual(
                config.mounts[0].source, str(root.resolve() / "missing-project")
            )
            self.assertEqual(
                config.mounts[1].source, str(Path.home().resolve() / "$PROJECT")
            )
            self.assertEqual(config.env, {"HOME_TOKEN": "$HOME"})

    def test_config_symlink_resolves_relative_to_real_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            configs = root / "configs"
            configs.mkdir()
            config_file = configs / "box.toml"
            config_file.write_text('[home]\nroot = "./homes"\n', encoding="utf-8")
            link = root / "link.toml"
            link.symlink_to(config_file)
            self.assertEqual(
                load_config(link).home.root, str(configs.resolve() / "homes")
            )

    def test_loading_resolves_host_paths_containing_spaces(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            source = root / "my project"
            source.mkdir()
            config_file = root / "box.toml"
            config_file.write_text(
                '[home]\nroot = "./managed homes"\n'
                '[[mounts]]\nsource = "./my project"\ntarget = "/workspace"\n',
                encoding="utf-8",
            )
            config = load_config(config_file)
            self.assertEqual(config.home.root, str(root / "managed homes"))
            self.assertEqual(config.mounts[0].source, str(source))

    def test_resolved_home_root_cannot_be_filesystem_root(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "root-link").symlink_to("/", target_is_directory=True)
            config_file = root / "box.toml"
            config_file.write_text('[home]\nroot = "./root-link"\n', encoding="utf-8")
            with self.assertRaisesRegex(ConfigError, "home.root"):
                load_config(config_file)

    def test_bad_toml_errors_do_not_include_secret_values(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            config_file = Path(directory) / "bad.toml"
            config_file.write_text(
                "[env]\nTOKEN = never-display-this-value\n", encoding="utf-8"
            )
            with self.assertRaises(ConfigError) as raised:
                load_config(config_file)
            self.assertIn("line 2", str(raised.exception))
            self.assertNotIn("never-display-this-value", str(raised.exception))

    def test_unreadable_non_regular_and_non_utf8_files_have_config_errors(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / "bad.toml"
            binary.write_bytes(b"\xff\xfe")
            fifo = root / "pipe"
            os.mkfifo(fifo)
            for path in (root / "missing", root, binary, fifo, root / "nul\x00path"):
                with self.subTest(path=path), self.assertRaises(ConfigError):
                    load_config(path)


if __name__ == "__main__":
    unittest.main()
