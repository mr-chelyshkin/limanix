// Package managedhome manages exact host home allocations recorded in VM ownership.
//
// [Manager] creates and removes the directory derived from domain.Identity:
// home.root/<name>-<id>. This directory is mounted as the development user's
// home. It is separate from Limanix's state directory and from user-supplied
// mount sources.
//
// # Ownership boundary
//
//	domain.Identity → checked root and exact allocation → Manager.Create / Remove
//	       ↑
//	state identity.json or a preserved-home ownership record
//
// [Manager.Create] makes a private empty directory. An existing allocation is
// never adopted, even when empty. [Manager.Remove] treats an absent allocation
// as already removed and refuses a redirected or non-directory allocation.
// Guest-writable marker files are never used as ownership evidence.
//
// Symlinks inside an owned home are removed as links, not followed. Callers must
// supply trusted host-side ownership and hold the appropriate VM operation lock;
// this package does not load records or acquire that lock itself.
//
// The zero value of Manager is ready to use. internal/vm decides whether deletion
// removes the home or retains it. internal/state archives ownership metadata;
// preserving a home does not copy or move its contents.
package managedhome
