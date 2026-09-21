// bundle-modules prepares the standard NixOS modules embedded in Limanix.
//
// It downloads the limanix-modules tag selected by Taskfile. The nixos package embeds the archive in Limanix,
// making its modules available for VM configuration when needed.
//
// The --version flag selects the exact Git tag; --root selects the repository.
// Existing valid output for that tag is reused without a download.
//
// Upstream: https://github.com/mr-chelyshkin/limanix-modules.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/mr-chelyshkin/limanix/internal/nixos/modulegen"
)

func run(ctx context.Context, args []string, diagnostics io.Writer) int {
	var (
		flags = flag.NewFlagSet("bundle-modules", flag.ContinueOnError)

		version = flags.String("version", "", "Required limanix-modules Git release tag.")
		root    = flags.String("root", ".", "Repository root containing internal/nixos/resources.")
	)
	flags.SetOutput(diagnostics)

	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	if flags.NArg() != 0 || *version == "" {
		_, _ = fmt.Fprintln(diagnostics, "bundle-modules: --version is required; positional arguments are not accepted")
		return 2
	}

	if err = modulegen.Generate(ctx, *root, *version, diagnostics); err != nil {
		_, _ = fmt.Fprintln(diagnostics, "bundle-modules:", err)
		return 1
	}

	return 0
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stderr)
	cancel()
	os.Exit(code)
}
