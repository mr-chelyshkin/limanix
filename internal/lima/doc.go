// Package lima adapts Lima's native Go runtime to Limanix operations.
//
// The adapter translates validated configuration, restricts backend operations
// to Limanix names, and narrows upstream metadata into [Instance]. It calls Lima
// libraries directly; it does not execute a separately installed limactl binary.
//
// # Runtime boundary
//
//	vm.Manager → Client → Lima store / drivers / lifecycle
//	                 ├→ system SSH → management or development command
//	                 └→ current Limanix executable → hidden hostagent process
//	                                      ↑
//	                         packaged Linux guest-agent archive
//
// [NewClient] wires the native APIs and defaults command streams. The zero value
// of [Client] is not usable. A guest-agent provider may be omitted for operations
// that do not launch a guest; launching then reports [ErrMissingAgentProvider].
//
// [Client.Preflight] checks host requirements without starting a VM. QEMU delegates
// missing network setup to package vmnet, which requires explicit administrator
// approval. [RequireMacOS] enforces the common host minimum for VM preflight
// and network setup. [HostArchitecture] detects the hardware
// architecture, including Apple Silicon under Rosetta; translated VM operations
// are rejected by [RequireNativeArchitecture].
//
// # Configuration and execution
//
// [Render] emits JSON accepted as Lima YAML. Matching host/guest architectures
// use VZ, virtiofs, and VZ NAT; cross-architecture guests use QEMU, 9p, and Lima's
// shared network. Base images are pinned by URL and digest. Home, generation,
// and explicit user mounts are distinct; ENV values are not inserted into the
// Lima template.
//
// Start, stop and delete serialize their network transitions across Limanix
// processes sharing LIMA_HOME. The lock covers image preparation and startup:
// Lima does not yet count a starting guest as running during that interval.
// It is released before guest provisioning and does not coordinate external
// limactl processes or separate Lima homes.
// Separate homes can share system socket/PID paths, but each reconciler counts
// only its own VMs. A home without network users can stop another home's helper.
// Lima retains start-on-demand and stop-after-last-user behavior; this package
// does not replace it with a persistent system service.
//
// [Client.Run] opens a management SSH command and preserves cancellation and
// bounded stderr diagnostics in [CommandError]. [Client.Shell] inherits terminal
// streams and returns the remote exit status. Command arguments are not included
// in CommandError, although guest-produced stderr may itself contain private data.
//
// # Reading the implementation
//
// Start with client.go and models.go. lifecycle.go, launch.go, and instances.go
// wrap upstream lifecycle and inspection; template.go and edit.go handle inputs;
// host.go and architecture.go check capabilities. ssh_session.go builds command
// arguments, ssh.go executes them, and process.go owns cancellation cleanup.
// native.go isolates upstream entry points for adapter tests; network_lock.go
// owns cross-process serialization of network lifecycle changes.
package lima
