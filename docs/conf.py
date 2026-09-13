"""Sphinx configuration and generated configuration reference."""

import sys
from importlib.metadata import version as package_version
from pathlib import Path

from sphinx.application import Sphinx

sys.path.insert(0, str(Path(__file__).parent))

project = "Limanix"
author = "mr-chelyshkin"
release = package_version("limanix")
version = release

extensions = [
    "myst_parser",
    "sphinx.ext.autodoc",
    "sphinxarg.ext",
    "sphinxcontrib.mermaid",
]
source_suffix = {".md": "markdown"}
root_doc = "index"
exclude_patterns = ["_generated"]
nitpicky = True
nitpick_ignore = [("py:class", "argparse.ArgumentParser")]

autodoc_member_order = "bysource"
autodoc_typehints = "description"
smartquotes = False

html_theme = "sphinx_book_theme"
html_theme_options = {"navbar_persistent": []}
html_title = "Limanix documentation"
html_show_sourcelink = False
html_static_path = ["_static"]
html_css_files = ["mermaid.css"]

mermaid_light_theme = "neutral"
mermaid_height = "auto"
mermaid_fullscreen = False
mermaid_init_config = {
    "startOnLoad": False,
    "fontFamily": "var(--pst-font-family-base)",
    "flowchart": {
        "useMaxWidth": True,
        "curve": "linear",
        "htmlLabels": False,
        "nodeSpacing": 24,
        "rankSpacing": 32,
    },
}


def generate_configuration_reference(app: Sphinx) -> None:
    """Refresh the model reference before Sphinx discovers source documents."""
    from generate import render_reference

    destination = Path(app.confdir) / "_generated" / "configuration.md"
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(render_reference(), encoding="utf-8")


def setup(app: Sphinx) -> None:
    """Register generation of configuration documentation."""
    app.connect("builder-inited", generate_configuration_reference)
