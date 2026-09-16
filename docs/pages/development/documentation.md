+++
title = "Writing documentation"
description = "Keep guides, generated references, and examples aligned with the code."
weight = 50
+++

The site uses Hugo and Hextra. Handwritten pages explain workflows; generated
references describe the actual command and configuration contracts.

## Choose the source

| Content | Edit here |
| --- | --- |
| Installation and first-run walkthrough | `docs/pages/installation.md`, `getting-started.md` |
| Task-oriented instructions and examples | `docs/pages/guide/` |
| Contract explanations and storage reference | `docs/pages/reference/` |
| Symptoms and recovery instructions | `docs/pages/troubleshooting.md` |
| Contributor and implementation documentation | `docs/pages/development/` |
| Configuration fields, descriptions, defaults | `internal/config/` |
| CLI commands and flags | `internal/cli/` |
| Downloadable project examples | `examples/` |
| Theme configuration, templates, assets | `docs/hugo.toml`, `docs/layouts/`, `docs/assets/` |

The documentation's structure follows the separation used by
[Task's documentation](https://taskfile.dev/docs/guide): installation, a first
working example, practical guides, and reference material. Contributor details
belong in Development, not in the first-run instructions.

## Write a guide

Start with what the reader wants to do. Show a small command or configuration,
explain the result, and put restrictions beside the affected step.

- Say whether a command runs on the Mac or inside the guest.
- Distinguish a complete configuration from a fragment that edits an existing section.
- Use the same VM name through a workflow; identify any prerequisites first.
- Explain what an operation changes and what it retains.
- Use warnings for overwritten files, interrupted sessions, privileges, and data loss.
- Link to exact fields or flags instead of duplicating generated tables.
- Describe verified behavior. Keep proposals and unvalidated behavior out of usage promises.

Page titles come from TOML front matter; do not repeat them as Markdown H1s.
Use `##` for task headings and `###` for related details. Keep navigation shallow.
Use a diagram only when it clarifies a relationship or sequence that prose does
not explain as directly.

## Build the site

```console
task --yes ci/docs
```

The task runs `cmd/docsgen`, then Hugo Extended, through `golang:_go/tool` in
the shared Go container. Docker is required; Node.js and npm are not.

```text
Go models + command tree → docsgen → reference fragments + example + metadata
Handwritten pages + generated content + Hextra → Hugo → build/docs/
```

`docsgen` writes `limanix.example.toml`, configuration and CLI fragments under
`docs/_generated/`, and version metadata. These files and `build/docs/` are
ignored artifacts. Edit their source, not generated output.

Hugo includes the generated references and downloadable examples in the static
site. Hextra provides navigation, search, light/dark styles, and highlighting.
Mermaid and FlexSearch scripts are downloaded at build time and served from
the built site. Their pins live in Hugo configuration and the theme dependency.

Taskfile pins Hugo Extended; `docs/go.mod` and `docs/go.sum` pin Hextra. Warnings
fail the site build. After moving a page, check its rendered links and anchors;
a successful Hugo build alone does not validate every handwritten link.

## Preview changes

```console
task docs/preview
```

Open <http://127.0.0.1:8060>. Hugo renders in memory and leaves `build/docs/`
unchanged. Markdown, template, and style edits rebuild and refresh the preview.
Press **Ctrl+C** to stop it.

Go references are generated once before the server starts. After changing a
Go model or CLI definition, restart the task to regenerate those fragments.
To choose another port:

```console
task docs/preview DOCS_PORT=8001
```
