// Package cli implements Limanix's command-line interface and generated command reference.
package cli

import (
	"context"
	"io"
	"log"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
	"github.com/mr-chelyshkin/limanix/internal/bundle"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/hostagent"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/modules"
	"github.com/mr-chelyshkin/limanix/internal/nixos"
	"github.com/mr-chelyshkin/limanix/internal/state"
	"github.com/mr-chelyshkin/limanix/internal/vm"
)

// IO allows commands to inherit the terminal or use explicit streams in tests.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Manager is the VM surface consumed by the CLI.
type Manager interface {
	Create(context.Context, string) (state.Instance, error)
	Update(context.Context, string) (state.Instance, error)
	FetchAll(context.Context) ([]vm.Info, error)
	Start(context.Context, domain.VMName) error
	Stop(context.Context, domain.VMName) error
	Delete(context.Context, domain.VMName, bool, bool) (string, error)
	Shell(context.Context, domain.VMName, []string) (int, error)
}

// Registry is the module surface consumed by the CLI.
type Registry interface {
	Available(context.Context) ([]modules.Info, error)
	Add(context.Context, domain.ModuleName, string) error
	Remove(context.Context, domain.ModuleName) error
}

// Dependencies creates host services lazily; help and documentation need none.
type Dependencies struct {
	Manager  func() (Manager, error)
	Registry func() (Registry, error)
}

func withDefaultDependencies(streams IO, dependencies Dependencies) Dependencies {
	if dependencies.Manager == nil {
		dependencies.Manager = func() (Manager, error) {
			host, err := lima.HostArchitecture()
			if err != nil {
				return nil, err
			}

			if err = lima.RequireNativeArchitecture(host); err != nil {
				return nil, err
			}

			store, err := state.NewStore("")
			if err != nil {
				return nil, err
			}

			var (
				agents  = bundle.New(filepath.Join(store.Root(), "runtime", "guestagents"))
				client  = lima.NewClient(agents.Path)
				manager = vm.New(store, client)
			)
			client.Stdin, client.Stdout, client.Stderr = streams.In, streams.Out, streams.Err
			manager.Warn = log.New(streams.Err, "limanix: warning: ", 0).Printf
			return manager, nil
		}
	}
	if dependencies.Registry == nil {
		dependencies.Registry = func() (Registry, error) {
			store, err := state.NewStore("")
			if err != nil {
				return nil, err
			}
			return modules.NewRegistry(store, nixos.BuiltinModules()), nil
		}
	}
	return dependencies
}

// Command returns the complete CLI tree. It performs no host or VM operations.
func Command(streams IO, dependencies Dependencies) *cobra.Command {
	dependencies = withDefaultDependencies(streams, dependencies)

	root := &cobra.Command{
		Use:           "limanix",
		Short:         "Development sandboxes with Lima and NixOS.",
		Version:       buildinfo.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          exactArgs(0),

		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetIn(streams.In)
	root.SetOut(streams.Out)
	root.SetErr(streams.Err)
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err) })
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().Bool("debug", false, "Enable internal Lima diagnostic logging.")
	_ = root.PersistentFlags().MarkHidden("debug")

	root.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		debug, _ := cmd.Flags().GetBool("debug")
		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		}
	}
	root.AddCommand(
		hostagent.Command(),
		firstConfigCommand(),
		configurationCommand("create", dependencies),
		configurationCommand("update", dependencies),
		listCommand(dependencies),
		powerCommand("start", dependencies),
		powerCommand("stop", dependencies),
		deleteCommand(dependencies),
		shellCommand(dependencies),
		modulesCommand(dependencies),
	)
	return root
}
