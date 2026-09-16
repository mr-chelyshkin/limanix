// Package app assembles the host services used by Limanix commands.
//
// This is the composition root: it connects concrete implementations to the
// interfaces consumed by internal/vm and internal/cli. It does not decide VM
// lifecycle ordering or own a background process.
//
// # Service graph
//
//	cmd/limanix → cli.Execute → command handler
//	                                ↓ requests a service
//	                           app.Services
//	                           ├→ vm.Manager
//	                           │  ├→ state.Store + managedhome.Manager
//	                           │  ├→ modules.Registry + nixos generation inputs
//	                           │  └→ lima.Client + guest.Guest
//	                           └→ modules.Registry
//
// [New] retains command streams. State-path resolution is deferred until a
// service is requested and its result is shared by that Services instance.
// [Services.Manager] checks the host architecture and wires the guest-agent
// cache into Lima. [Services.Registry] does not initialize a Lima client.
// Help, version output, and generated CLI references need neither service.
//
// # Reading the implementation
//
// Start with services.go for wiring, then internal/cli for dispatch and
// internal/vm for create, update, delete, and recovery ordering.
//
// Follow internal/config and internal/domain for input and identity contracts;
// internal/state and internal/managedhome for ownership on disk; and
// internal/modules, internal/nixos, internal/lima, and internal/guest for the
// generation-to-guest path.
//
// The persistent subprocess lives in internal/hostagent. Build-time tooling is
// separate: internal/bundle/generator builds guest agents and
// internal/docs/generator prepares Hugo inputs.
//
// # Construction and failures
//
// Use [New]; the zero value of [Services] is not usable. Service methods return
// host-compatibility and path-resolution errors. Missing required dependencies
// passed to vm.New are programming errors, not recoverable host failures.
package app
