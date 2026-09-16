// bundle-socketvmnet prepares the macOS network helper embedded in Limanix.
//
// It downloads the pinned socket_vmnet release for both host architectures and
// verifies archive digests and executable contents. The original archives retain
// the upstream license. Runtime setup installs the selected helper after explicit
// administrator approval; this command never installs privileged host files.
//
// Upstream: https://github.com/lima-vm/socket_vmnet.
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

	"github.com/mr-chelyshkin/limanix/internal/bundle/vmnetgen"
)

func run(ctx context.Context, args []string, diagnostics io.Writer) int {
	flags := flag.NewFlagSet("bundle-socketvmnet", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	root := flags.String("root", ".", "Repository root containing internal/bundle/resources.")

	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}

	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(diagnostics, "bundle-socketvmnet: unexpected positional arguments")
		return 2
	}

	if err := vmnetgen.Generate(ctx, *root, diagnostics); err != nil {
		_, _ = fmt.Fprintln(diagnostics, "bundle-socketvmnet:", err)
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
