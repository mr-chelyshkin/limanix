# Generated Lima guest agents

`go run ./cmd/agentgen` builds the pinned Lima guest agent for Linux arm64 and
amd64, then writes deterministic gzip archives and an integrity manifest here.
Generated archives and the manifest are build inputs supplied before CI and
release builds. They are not tracked in Git.

The guest-agent command is declared as a Go tool in `go.mod`, so dependency
updates retain its Linux-specific package graph and the generated executables
use the same pinned modules as Limanix.

This file keeps the embedded resource directory present in a clean checkout.
It is not an executable payload. VM creation requires the generated archives.
