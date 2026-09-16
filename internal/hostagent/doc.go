// Package hostagent runs Lima's persistent host agent inside the Limanix executable.
//
// [Command] implements a hidden subprocess interface invoked by Lima. It is not the public VM CLI and is not
// the Linux agent running inside the guest. internal/lima launches this process with the packaged guest-agent archive.
//
// # Process lifetime
//
//	Lima StartWithPaths → limanix hostagent INSTANCE
//	                               ↓
//	                 validate owned paths and resources
//	                               ↓
//	                   acquire and hold PID-file lease
//	                               ↓
//	                 construct Lima agent → bind API socket
//	                               ↓
//	                          run until shutdown
//	                               ↓
//	                      close socket → release PID
//
// The PID file is locked for the process lifetime. The API uses a Unix socket inside the instance directory.
// Path validation rejects foreign names and unexpected PID/socket locations; the directory must have private 0700
// permissions, and the socket is changed to 0600 before use.
//
// SIGINT, SIGTERM, or parent-context cancellation request orderly shutdown. Lima receives a context that remains live
// during that cleanup. stdout/stderr writers serialize concurrent writes; the CLI configures JSON diagnostics for
// this subprocess.
//
// Socket and PID cleanup verify the path they are about to remove. A replacement regular file is not treated as the
// old socket, and PID cleanup checks identity rather than blindly removing the current path.
//
// Read command.go and options.go for the subprocess contract, run.go for lifetime, pid.go for the lease, server.go
// for HTTP/socket ownership, and writer.go for stream synchronization.
package hostagent
