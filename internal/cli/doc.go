// Package cli implements Limanix's command-line interface and generated command reference.
//
// Handlers decode arguments, request application services, and render results. VM lifecycle decisions belong to internal/vm;
// module imports belong to internal/modules. The command tree is also the source of the CLI reference.
//
// # Invocation
//
//	cmd/limanix → Execute → Command → handler → Manager or Registry
//	                ↑                   ↓
//	             exit code ← diagnostics and formatted result
//
// [Command] constructs the Cobra tree without querying Lima or opening state.
// [Dependencies] supplies lazy service factories; omitted factories are filled by internal/app.
// [IO] keeps terminal streams explicit.
// [Reference] walks the same command definitions without executing their handlers.
//
// # Process boundary
//
// [Execute] returns an exit code rather than calling os.Exit.
// Success returns 0, usage errors return 2, ordinary failures return 1, and canceled foreground operations return 130.
// Shell commands retain the remote process exit status.
//
// Normal commands handle SIGINT and SIGTERM through cancellation. Interactive SSH handles foreground SIGINT itself.
// The hidden hostagent command owns its shutdown lifecycle and uses JSON diagnostics, unlike ordinary CLI output.
//
// # Reading the implementation
//
// Read cli.go and dependencies.go for command assembly, execute.go and error.go for process behavior, then configuration.go,
// power.go, vms.go, and modules.go for handlers. output.go and reference.go contain presentation, not lifecycle or persistence rules.
package cli
