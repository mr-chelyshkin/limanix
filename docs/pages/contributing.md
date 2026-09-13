# Documentation development

## Sources

Documentation combines handwritten guides with reference material generated
from Python code:

| Content | Source |
| --- | --- |
| Guides | Markdown files in `docs/pages/` |
| Configuration reference and default TOML | `src/limanix/config.py` |
| CLI commands and arguments | `build_parser()` in `src/limanix/cli.py` |
| Python API | Python signatures and docstrings |

Sphinx builds the static site. MyST supplies Markdown support, `autodoc` reads
the Python API, and `sphinx-argparse` reads the CLI argument definitions.
The `docs` dependency group in `pyproject.toml` contains the documentation tools;
`uv.lock` records their resolved versions.

## Update the configuration contract

Edit a field's type, default, or description in `src/limanix/config.py`, then
regenerate the default configuration:

```console
task docs/generate
```

Commit `limanix.example.toml` alongside the model change. The configuration
reference is generated during each documentation build.

## Preview the site

Start the documentation server in the Python container:

```console
task docs/serve
```

Open <http://127.0.0.1:8070>. Changes to the docs and Python code rebuild the
site and refresh the browser. Press **Ctrl+C** to stop.

To use another port:

```console
task docs/serve DOCS_PORT=8001
```

## Check the site

Run the documentation build in the Python CI container:

```console
task ci/docs
```

Open `build/docs/index.html` to view the static site. Generated reference
fragments in `docs/_generated/` and HTML output in `build/docs/` are build
artifacts; the Markdown guides and Python sources are versioned.
