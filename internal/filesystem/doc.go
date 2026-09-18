// Package filesystem provides checked host paths and atomic file replacement.
//
// This package supplies filesystem mechanics, not VM ownership policy. Callers decide which paths they own, what
// permissions to request, and whether an operation needs a higher-level lock.
//
// # Path contracts
//
// [ExpandHome] expands a leading tilde. [Resolve] resolves existing symlinks before interpreting later path components,
// including "..", and permits a missing tail. [RequireDirectory] resolves a path and requires it to exist.
//
// [CheckDirectory] has a different contract: it checks the final path with Lstat, rejects existing symlinks and
// non-directories, and accepts absence without creating anything. It does not verify owner or permission bits.
//
// [OpenRegular] refuses a final symlink or special file. These checks do not provide a sandbox against another process
// replacing parent directories between path operations.
//
// # Atomic replacement
//
//	inspect destination → create sibling temporary file
//	                                  ↓
//	                          chmod → write → sync
//	                                  ↓
//	                           rename into place
//	                                  ↓
//	                       sync containing directory
//
// [WriteFileAtomic] preserves old contents on failures before rename. A directory sync error is reported after
// replacement has already occurred; an error does not always mean the old file remains. Temporary-file cleanup
// errors are kept alongside the primary failure.
//
// A zero requested mode preserves existing permission bits, with 0600 used for new files or an existing zero mode.
// Replacing a regular read-only file requires a writable parent, not write access to the old contents.
// Final symlinks and special-file destinations are rejected.
//
// [WriteTextAtomic] adds UTF-8 validation and returns the resolved destination.
// [Error] retains path, operation, and underlying cause for errors.Is/As.
// Read path.go, file.go, and atomic.go for the corresponding contracts.
package filesystem
