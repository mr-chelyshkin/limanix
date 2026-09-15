// docsgen prepares generated content for the Limanix documentation site.
//
// It derives CLI and configuration references from the runtime command definitions and models, put it to docs/_generated.
//
// Run this command before building the documentation.
// The --root flag selects the repository directory.
//
// [Hugo]: https://gohugo.io/
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

	"github.com/mr-chelyshkin/limanix/internal/docs/generator"
)

// options holds the command's repository selection.
type options struct {
	root string
}

func parseOptions(args []string, diagnostics io.Writer) (options, error) {
	var (
		flags = flag.NewFlagSet("docsgen", flag.ContinueOnError)
		opts  options
	)

	flags.SetOutput(diagnostics)
	flags.StringVar(&opts.root, "root", ".", "Repository root containing docs/ and the example.")

	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		err := errors.New("unexpected positional arguments")
		_, _ = fmt.Fprintln(diagnostics, err)
		flags.Usage()
		return options{}, err
	}
	return opts, nil
}

func run(ctx context.Context, args []string, diagnostics io.Writer) int {
	opts, err := parseOptions(args, diagnostics)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}

	if err = generator.Generate(ctx, opts.root, diagnostics); err != nil {
		_, _ = fmt.Fprintln(diagnostics, "docsgen:", err)
		return 1
	}
	return 0
}

func main() {
	var (
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code        = run(ctx, os.Args[1:], os.Stderr)
	)

	cancel()
	os.Exit(code)
}
