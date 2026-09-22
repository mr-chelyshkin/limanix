// Package nixos materializes embedded NixOS configuration and selected module snapshots.
//
// [Prepare] creates the host-side inputs for one VM generation. It does not run Nix, connect to a guest, or change VM state.
// internal/vm supplies the generation directory and holds module-source leases while copying; internal/guest later
// applies the result through the read-only /mnt/limanix mount.
//
// # Generation contents
//
//	<generation>/
//	├─ flake/
//	│  ├─ flake.nix + flake.lock + base NixOS modules
//	│  ├─ runtime.json            user, architecture, ports, module imports
//	│  └─ modules/<index>/        selected bundled or imported module trees
//	├─ environment                systemd-compatible runtime assignments
//	└─ environment.sh             login-shell exports
//
// Base flake files and their lock file are embedded in the executable and copied into each generation.
// [BaseImage] derives the first-boot disk URL from the same locked nixos-lima release. Image digests live in image.go;
// changing that dependency requires reviewing the partition and boot configuration in resources/base/platform.nix.
//
// Standard modules come from the limanix-modules release selected in Taskfile, not Go declarations.
// cmd/bundle-modules packages their trees and module.toml metadata as resources/modules.zip before compilation.
// The executable reads this archive in memory; no catalog is downloaded or installed at runtime.
//
//	Taskfile tag → limanix-modules archive → bundle-modules → embedded modules.zip
//	                                                            ↓ lmx:NAME[-VERSION]
//	                                                     VM generation snapshot
//
// Each selected module becomes a separate snapshot with a generated import path. Explicit versions select
// versions/<version>.nix from that tree; names without a version retain default.nix.
// [SystemModules] returns an independent metadata map for the registry. An empty selection adds no optional modules;
// base files are still copied.
//
// # Runtime environment and failure contract
//
// ENV values are written literally with target-specific escaping. They remain beside the flake and are absent
// from runtime.json and flake source inputs. They are plaintext runtime configuration, not encrypted secret storage;
// guest installation makes them guest-wide environment settings.
//
// Prepare requires a fresh flake destination and a positive host UID. It may leave partial output on failure;
// the VM generation owner decides whether to discard it or retain it for recovery. Individual writes do not make
// an entire generation transactional.
//
// Read bundle.go for ordering, modules.go and resources.go for copying, runtime.go for the flake JSON contract,
// and environment.go for ENV encoding.
package nixos
