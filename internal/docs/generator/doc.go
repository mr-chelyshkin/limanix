// Package generator prepares model-derived inputs for the Hugo documentation site.
//
// [Generate] is the implementation behind cmd/docsgen. It renders configuration
// and command references from the same Go models and Cobra definitions used by
// the application. It does not start a VM, resolve application services, or
// render HTML.
//
// # Generated artifacts
//
//	config.Default + field tags → limanix.example.toml
//	                           └→ docs/_generated/configuration.md
//	cli.Command                → docs/_generated/cli.md
//	buildinfo.Version          → docs/_generated/metadata.json
//	                                      ↓
//	                          Hugo pages, shortcodes, and templates
//	                                      ↓
//	                             build/docs static site
//
// The example is also mounted as a downloadable Hugo asset. Handwritten pages,
// layouts, and site configuration remain under docs; generated references are
// included by those pages rather than maintained as separate copies.
//
// Generate renders all documents before writing them. Each file is replaced
// atomically, but the output set is not a multi-file transaction. Cancellation
// or an I/O error can leave a mixture of old and new files; rerunning Generate
// refreshes the set.
//
// The repository's ci/docs task runs generation before invoking [Hugo].
// This package does not embed website output into the Limanix runtime binary.
//
// Read render.go for the artifact list and source models, and generate.go for
// directory preparation, cancellation, writes, and diagnostics.
//
// [Hugo]: https://gohugo.io/
package generator
