+++
title = "Contributing"
description = "Set up the checkout, run checks, and verify a change at the right boundary."
weight = 10
url = "/contributing/"
prev = "development"
+++

Start with [Architecture](architecture.md) for package responsibilities.
Use this page to prepare a checkout and run the same tasks used by the project.

## Prepare the tools

Install Task at the version declared in `Taskfile.yml`. Containerized Go checks
and documentation tasks need Docker. Building a release binary also requires
macOS, the Xcode command-line tools, and the Go toolchain declared in `go.mod`.

The root Taskfile delegates shared Go tooling to the pinned
`mr-chelyshkin/tasks` include. Keep local checks behind these tasks rather than
maintaining a second set of tool commands.

## Check a change

From the repository root:

```console
task --yes ci/fmt ci/lint ci/test ci/vuln ci/docs
```

| Task | Checks |
| --- | --- |
| `ci/fmt` | Go formatting; it reports differences without rewriting files. |
| `ci/lint` | Go source and test linting. |
| `ci/test` | Tests with the race detector, after preparing ignored embedded assets. |
| `ci/vuln` | Limanix packages and the embedded Lima Linux guest-agent entry point. |
| `ci/docs` | Generated references and the Hugo site, with warnings treated as errors. |

Run the native build separately on macOS:

```console
task --yes ci/build
```

It builds and signs `bin/limanix-arm64` and `bin/limanix-amd64`. It does not run
the test or vulnerability tasks. See [Building Limanix](builds.md) for build
inputs and packaging checks.

### Container user lookup

Tests and documentation generation use `-tags=osusergo` and `USER=limanix`.
The shared image runs with the host UID, which may have no `/etc/passwd` entry.
Lima resolves the host user during package initialization; these settings let
Go's user lookup use the supplied username without adding a container account.

## Check VM behavior

Unit tests cover contracts and lifecycle ordering with injected backends. They
do not prove guest boot, NixOS rebuilds, mounts, or host-to-guest networking.
Verify affected behavior on a real Mac when changing those integrations.

The ignored `vm/` directory can hold local configurations and environment values.
Select a configuration explicitly, for example:

```console
./bin/limanix-arm64 create --config vm/my-check/limanix.toml
```

Use the Intel binary on Intel hosts. Review the configuration's mounts and
network settings before creating a VM, and keep private values out of tracked
examples. Verify the VZ or QEMU path affected by your change.

## Update documentation

User-facing command and configuration changes need matching guides and
examples. Edit CLI definitions in `internal/cli` and configuration types,
`doc` tags, and defaults in `internal/config`; their references are generated.
Do not patch the generated Markdown to describe behavior missing from the code.

Follow [Writing documentation](documentation.md) for content placement, style,
generation, and local preview.

## CI and releases

The PR workflow runs formatting, lint, race tests, vulnerability checks,
documentation, and a native macOS build. Container checks run on Ubuntu;
native binaries build on `macos-26`.

The tag workflow verifies that the commit belongs to `main`, runs formatting,
lint, and vulnerability checks, then builds release binaries. It also requires
a successful documentation build before publishing the binaries. It does not
rerun `ci/test`; those tests run in the PR workflow.

The workflow definitions in `.github/workflows/` are the source for job ordering
and permissions. Real guest checks remain a separate validation step.
