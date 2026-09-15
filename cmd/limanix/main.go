// limanix creates and manages development sandboxes with Lima and NixOS.
//
// It uses TOML configuration to define VM resources, host mounts, and the guest environment.
// The CLI manages the VM lifecycle, built-in and imported NixOS modules, and guest shell sessions.
//
// Run limanix --help to list commands, or limanix COMMAND --help for command-specific usage.
package main

import (
	"context"
	"os"

	"github.com/mr-chelyshkin/limanix/internal/cli"
)

func main() {
	os.Exit(
		cli.Execute(
			context.Background(),
			os.Args[1:],
			cli.IO{
				In:  os.Stdin,
				Out: os.Stdout,
				Err: os.Stderr,
			},
			cli.Dependencies{},
		),
	)
}
