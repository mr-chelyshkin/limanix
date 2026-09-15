// Command limanix creates and manages Lima/NixOS development sandboxes.
package main

import (
	"context"
	"os"

	"github.com/mr-chelyshkin/limanix/internal/cli"
)

func main() {
	os.Exit(cli.Execute(context.Background(), os.Args[1:], cli.IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}, cli.Dependencies{}))
}
