// Package nixos materializes embedded NixOS configuration and selected module snapshots.
//
// [Prepare] creates the host-side inputs for one VM generation. It does not run
// Nix, connect to a guest, or change VM state. internal/vm supplies the generation
// directory and holds module-source leases while copying; internal/guest later
// applies the result through the read-only /mnt/limanix mount.
//
// # Generation contents
//
//	<generation>/
//	├─ flake/
//	│  ├─ flake.nix + flake.lock + base NixOS modules
//	│  ├─ runtime.json            user, architecture, ports, module imports
//	│  └─ modules/<index>/        selected bundled or imported module trees
//	├─ environment               systemd-compatible runtime assignments
//	└─ environment.sh            login-shell exports
//
// Base flake files and their lock file are embedded in the executable and copied
// into each generation. Each selected module becomes a separate snapshot with
// a generated import path. [BuiltinModules] returns an independent metadata map
// for the registry; it does not expose the embedded filesystem for modification.
//
// # Runtime environment and failure contract
//
// ENV values are written literally with target-specific escaping. They remain
// beside the flake and are absent from runtime.json and flake source inputs.
// They are plaintext runtime configuration, not encrypted secret storage;
// guest installation makes them guest-wide environment settings.
//
// Prepare requires a fresh flake destination and a positive host UID. It may
// leave partial output on failure; the VM generation owner decides whether to
// discard it or retain it for recovery. Individual writes do not make an entire
// generation transactional.
//
// Read bundle.go for ordering, modules.go and resources.go for copying,
// runtime.go for the flake JSON contract, and environment.go for ENV encoding.
package nixos
