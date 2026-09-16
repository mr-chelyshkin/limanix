package buildinfo

import "errors"

var (
	ErrMissingPlatform = errors.New("minimum macOS version is not configured; build with task ci/build")
	ErrMacOSVersion    = errors.New("invalid macOS version")
)
