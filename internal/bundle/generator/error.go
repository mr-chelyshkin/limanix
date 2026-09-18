package generator

import "errors"

var (
	ErrInvalidToolchain  = errors.New("go compiler returned an invalid GOROOT or GOVERSION")
	ErrUnversionedLima   = errors.New("generation requires an unreplaced, versioned Lima module")
	ErrSourceMismatch    = errors.New("guest-agent package graph does not match the resolved Lima module")
	ErrDynamicExecutable = errors.New("guest agent requires a dynamic loader (PT_INTERP); a static executable is required")
	ErrManifestDocument  = errors.New("manifest must contain exactly one JSON document")
	ErrManifestTarget    = errors.New("manifest target does not match the requested architecture")
	ErrCompilerFlags     = errors.New("compiler flags changed")
	ErrArchiveChecksum   = errors.New("archive checksum does not match the manifest")
)
