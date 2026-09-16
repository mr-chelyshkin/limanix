// Package buildinfo holds the application version shared by CLI output and documentation.
//
// [Version] has a source default for development builds. The release build
// replaces it with the requested tag through Go's linker -X flag; there is no
// runtime Git lookup.
//
// The CLI reads it for limanix --version; docsgen writes it into
// docs/_generated/metadata.json for the Hugo site.
//
// This version identifies Limanix, not the pinned Lima module, the base guest
// image, or the persisted-state schema. Those versions are maintained by their
// respective packages.
package buildinfo
