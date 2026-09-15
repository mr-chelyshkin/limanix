// Command docsgen generates references from the same models and CLI used at runtime.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
	"github.com/mr-chelyshkin/limanix/internal/cli"
	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

type options struct {
	root  string
	check bool
}

func parseOptions(args []string, diagnostics io.Writer) (options, error) {
	var result options
	flags := flag.NewFlagSet("docsgen", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.BoolVar(&result.check, "check", false, "Check the tracked example rather than replacing it.")
	flags.StringVar(&result.root, "root", ".", "Repository root containing docs/ and the example.")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		err := errors.New("unexpected positional arguments")
		_, _ = fmt.Fprintln(diagnostics, err)
		flags.Usage()
		return options{}, err
	}
	return result, nil
}

func run(args []string, diagnostics io.Writer, generateDocuments func(string, bool) error) int {
	opts, err := parseOptions(args, diagnostics)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	if err := generateDocuments(opts.root, opts.check); err != nil {
		_, _ = fmt.Fprintln(diagnostics, "docsgen:", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr, generate))
}

func generate(root string, check bool) error {
	example, err := config.RenderExample()
	if err != nil {
		return err
	}
	examplePath := filepath.Join(root, "limanix.example.toml")
	if check {
		saved, err := os.ReadFile(examplePath)
		if err != nil {
			return err
		}
		if !bytes.Equal(saved, example) {
			return errors.New("example differs from the model; run task docs/generate")
		}
	} else if err := filesystem.WriteFileAtomic(examplePath, example, 0o644); err != nil {
		return err
	}
	configuration, err := config.RenderReference()
	if err != nil {
		return err
	}
	cliReference := cli.Reference()
	generatedDir := filepath.Join(root, "docs", "_generated")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return err
	}
	metadata, err := json.Marshal(struct {
		Version string `json:"version"`
	}{Version: buildinfo.Version})
	if err != nil {
		return err
	}
	files := []struct {
		name    string
		content []byte
	}{
		{"configuration.md", []byte(configuration)},
		{"cli.md", []byte(cliReference)},
		{"metadata.json", append(metadata, '\n')},
	}
	for _, file := range files {
		if err := filesystem.WriteFileAtomic(filepath.Join(generatedDir, file.name), file.content, 0o644); err != nil {
			return err
		}
	}
	fmt.Println("Example and references match the Go configuration and CLI.")
	return nil
}
