// Package vmnetgen prepares the pinned socket_vmnet release for embedding.
//
// Generate verifies existing archives or downloads replacements from the upstream
// release. Archive digests, sizes, Mach-O architecture, deployment target and
// library dependencies are checked before publishing. Packaging works on Linux
// and macOS; it never runs the helper, requests root privileges, or installs host
// networking.
package vmnetgen
