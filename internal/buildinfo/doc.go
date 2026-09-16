// Package buildinfo holds Limanix's release identity and host compatibility baseline.
//
// [Version] has a source default for development builds. The release build
// replaces it with the requested tag through Go's linker -X flag; there is no
// runtime Git lookup.
//
// The CLI reads it for limanix --version; docsgen writes it into
// docs/_generated/metadata.json for the Hugo site.
// [MinimumMacOSMajor] is shared by host checks and embedded-helper validation;
// the native build's deployment target in Taskfile.yml must match it.
//
// This version identifies Limanix, not the pinned Lima module, the base guest
// image, or the persisted-state schema. Those versions are maintained by their
// respective packages.
package buildinfo
