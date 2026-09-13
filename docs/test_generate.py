"""Check generated artifacts against independent parsers."""

import contextlib
import io
import tempfile
import tomllib
import unittest
from dataclasses import asdict, replace
from pathlib import Path
from unittest.mock import patch

import generate
from markdown_it import MarkdownIt

from limanix.config import Config


class GenerationTests(unittest.TestCase):
    def test_empty_defaults_preserve_entry_documentation(self) -> None:
        config = Config(mounts=[], env={})
        self.assertEqual(tomllib.loads(generate.render_example(config)), asdict(config))
        reference = generate.render_reference(config)
        self.assertIn("## `mounts`", reference)
        self.assertIn("Mount destination inside the guest.", reference)
        self.assertIn("Default: `[]`.", reference)

    def test_example_round_trip(self) -> None:
        config = Config()
        self.assertEqual(tomllib.loads(generate.render_example(config)), asdict(config))
        changed = replace(config, resources=replace(config.resources, cpu=7))
        self.assertEqual(
            tomllib.loads(generate.render_example(changed)), asdict(changed)
        )
        self.assertNotEqual(
            generate.render_reference(config), generate.render_reference(changed)
        )

    def test_strings_remain_literal(self) -> None:
        config = Config(
            env={"APP.NAME": '**bold** [link](https://example.org) `code` "\\\n\x7f'}
        )
        self.assertEqual(tomllib.loads(generate.render_example(config)), asdict(config))
        html = (
            MarkdownIt("commonmark")
            .enable("table")
            .render(generate.render_reference(config))
        )
        self.assertNotIn("<strong>", html)
        self.assertNotIn('<a href="https://example.org">', html)
        self.assertEqual(html.count("<code>"), html.count("</code>"))

    def test_check_detects_stale_example_without_overwriting(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            example = Path(directory) / "limanix.example.toml"
            example.write_text(generate.render_example())
            with (
                patch.object(generate, "EXAMPLE", example),
                patch("sys.argv", ["generate.py", "--check"]),
                contextlib.redirect_stdout(io.StringIO()),
            ):
                self.assertEqual(generate.main(), 0)
                example.write_text("# stale\n")
                with (
                    contextlib.redirect_stderr(io.StringIO()),
                    self.assertRaises(SystemExit) as raised,
                ):
                    generate.main()
                self.assertEqual(raised.exception.code, 1)
                self.assertEqual(example.read_text(), "# stale\n")


if __name__ == "__main__":
    unittest.main()
