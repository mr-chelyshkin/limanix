// Package catalog reads the standard NixOS module catalog packaged with Limanix.
//
// A catalog is one ZIP containing its release tag, upstream LICENSE, and module trees.
// Its ZIP comment identifies [Repository]; archives from another source are rejected.
// Module names come from modules/<name>; descriptions come from module.toml.
// No module identifiers or Nix implementations are declared in Go.
//
// [Open] validates archive paths, file types, contents, and metadata before exposing
// read-only module filesystems. It neither evaluates Nix nor accesses the network.
// The repository marker and release tag identify the downloaded source. These metadata
// and ZIP integrity checks do not prove the authenticity of an upstream release.
package catalog
