package cli

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/mr-chelyshkin/limanix/internal/buildinfo"
	"github.com/mr-chelyshkin/limanix/internal/hostagent"
)

// Command returns the complete CLI tree. It performs no host or VM operations.
func Command(streams IO, dependencies Dependencies) *cobra.Command {
	dependencies = withDefaultDependencies(streams, dependencies)

	root := &cobra.Command{
		Use:           "limanix",
		Short:         "Development sandboxes with Lima and NixOS.",
		Version:       buildinfo.Version,
		Args:          exactArgs(0),
		SilenceErrors: true,
		SilenceUsage:  true,

		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetIn(streams.In)
	root.SetOut(streams.Out)
	root.SetErr(streams.Err)
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError(err)
	})
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
		networkInstallerCommand(),
		networkCommand(),
		firstConfigCommand(),
		createCommand(dependencies),
		updateCommand(dependencies),
		listCommand(dependencies),
		startCommand(dependencies),
		stopCommand(dependencies),
		deleteCommand(dependencies),
		shellCommand(dependencies),
		modulesCommand(dependencies),
	)
	return root
}
