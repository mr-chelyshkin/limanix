// Package app assembles the host services used by Limanix commands.
//
// This is the composition root: it connects concrete implementations to the interfaces consumed by internal/vm and internal/cli.
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
// - [New] retains command streams.
// State-path resolution is deferred until a service is requested and its result is shared by that Services instance.
// - [Services.Manager] checks the host architecture and wires the guest-agent cache into Lima.
// - [Services.Registry] does not initialize a Lima client. Help, version output, and generated CLI references need neither service.
package app
