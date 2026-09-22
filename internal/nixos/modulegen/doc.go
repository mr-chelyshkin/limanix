// Package modulegen downloads the selected limanix-modules tag and prepares its embedded catalog.
//
// Generate reads the GitHub source archive over HTTPS, retains module trees and LICENSE,
// and records the source repository and selected release tag. The catalog package validates the result before
// one atomic file replacement publishes resources/modules.zip.
//
// The published archive doubles as the build cache. A valid archive from the expected
// repository with the requested tag is reused without a network request. Tags are not checksum pins; moving a tag does
// not invalidate an existing cache. No Nix evaluation or runtime module installation occurs here.
package modulegen
