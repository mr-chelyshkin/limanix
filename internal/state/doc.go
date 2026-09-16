// Package state persists VM ownership and operation progress and supplies host locks.
//
// Domain models describe a VM; [Store] owns their versioned JSON representation.
// Lima keeps its own backend store. A Limanix state directory is not a guest
// disk or the guest user's managed home.
//
// # Storage layout
//
//	<state root>/
//	├─ instances/<name>/
//	│  ├─ identity.json          immutable ownership
//	│  ├─ instance.json          status, generation ID, recovery message
//	│  └─ generations/<id>/      inputs prepared by internal/vm
//	├─ modules/<name>/           imported trees managed by internal/modules
//	├─ homes/<name>-<id>.json    ownership retained after VM deletion
//	├─ locks/instances/<name>.lock
//	├─ locks/registry.lock
//	└─ runtime/guestagents/      archives materialized by internal/bundle
//
// [DefaultRoot] honors LIMANIX_HOME; otherwise it selects the platform's state
// location. This is separate from configuration home.root, which selects where
// guest home contents live. [NewStore] resolves the root without creating it.
//
// # Persistence and ownership
//
// [Store.Save] creates identity once and rejects a conflicting replacement.
// identity.json and instance.json are written atomically as separate files,
// not as one transaction. [Store.LoadIdentity] reads ownership even when the
// mutable runtime record is damaged.
//
// [Store.Remove] removes VM records and generation inputs, never home contents.
// [Store.PreserveHome] archives ownership metadata, not a copy of the home.
// [Store.ForgetHome] removes only the matching archived identity.
//
// # Locking and interrupted operations
//
// Callers hold [Store.InstanceLock] across a mutating VM scenario. Contention
// fails immediately with [ErrLockBusy]. [Store.RegistryLock] supports shared
// readers and bounded waiting for module-registry changes. Locks are cooperative
// process locks; Save and Remove do not acquire a VM lock on the caller's behalf.
//
// [Store.FetchAll] uses a nonblocking shared lock to distinguish an active
// operation from an abandoned one. After acquiring it, ownership and progress
// are read again. An in-flight record with no active writer is presented as
// interrupted without rewriting JSON. Missing or unreadable identity is not
// replaced by a stale earlier read.
//
// Read state.go for paths, records.go and instance.go for persistence, lock.go
// for contention, list.go for recovery views, and homes.go for retained ownership.
package state
