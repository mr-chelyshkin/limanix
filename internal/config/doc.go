// Package config owns the public TOML configuration contract, defaults, and generated reference.
//
// [Config] and its nested models describe user input. Their fields and tags are shared by parsing, validation,
// commented TOML rendering, and Markdown reference generation. Configuration schema versioning is independent
// of the application release and the state-record schema.
//
// # Input pipeline
//
//	Parse: TOML → shape checks → defaults + decoding → Validate → Config
//	Load:  file → Parse → resolve host paths from the real file directory
//
// [Parse] performs no filesystem access. Unknown fields, invalid types, and missing required entry fields are rejected
// before defaults are applied. Omitted fields keep defaults; explicitly supplied collections replace them, including
// an empty ENV table. Guest paths are normalized without inspecting the guest filesystem.
//
// [Load] follows the configuration file's resolved location when interpreting relative host paths and expands a leading tilde.
// It does not require mount sources to exist; VM preflight checks them before allocating resources.
// Environment values are literal data, not host-variable or shell expressions.
//
// # Validation and diagnostics
//
// [Validate] checks field values and relationships, including schema version, resource sizes, module-ID uniqueness,
// reserved guest paths, and mount overlap. It does not query Lima or verify that a module is installed in the registry.
//
// [Error] identifies a field without printing its input value. An underlying cause may be available through Unwrap;
// callers should not expose it as a replacement for the value-free public diagnostic.
//
// # Reading the implementation
//
// Start with models.go and [Default], then parser.go and schema.go for decoding, validate.go for policy, and load.go
// for filesystem resolution. render.go and reference.go derive examples and reference tables from those same models.
package config
