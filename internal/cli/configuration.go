package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

func firstConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "first-config [PATH]",
		Short: "Write the default limanix.toml.",
		Long:  "Write the default limanix.toml in an existing directory (default: current directory). Existing files are overwritten; configuration symlinks are rejected.",

		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return usageError(err)
			}

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			directory := "."

			if len(args) > 0 {
				directory = args[0]
			}

			parent, err := filesystem.RequireDirectory(directory)
			if err != nil {
				return err
			}

			document, err := config.RenderExample()
			if err != nil {
				return err
			}

			destination, err := filesystem.WriteTextAtomic(filepath.Join(parent, "limanix.toml"), string(document), 0)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s.\n", destination)
			return err
		},
	}
}

func createCommand(dependencies Dependencies) *cobra.Command {
	var path string

	command := &cobra.Command{
		Use:   "create",
		Short: "Create a development sandbox from a TOML configuration.",
		Args:  requiredConfig(&path),

		RunE: func(cmd *cobra.Command, _ []string) error {
			manager, err := dependencies.Manager()
			if err != nil {
				return err
			}

			instance, err := manager.Create(cmd.Context(), path)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Created %s. Managed home: %s\n",
				instance.Identity.Name,
				instance.Identity.Home,
			)

			return err
		},
	}

	command.Flags().StringVar(&path, "config", "", "Path to the VM configuration file (required).")
	return command
}

func updateCommand(dependencies Dependencies) *cobra.Command {
	var path string

	command := &cobra.Command{
		Use:   "update",
		Short: "Apply a configuration and restart an existing VM.",
		Args:  requiredConfig(&path),

		RunE: func(cmd *cobra.Command, _ []string) error {
			manager, err := dependencies.Manager()
			if err != nil {
				return err
			}

			instance, err := manager.Update(cmd.Context(), path)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated %s.\n", instance.Identity.Name)
			return err
		},
	}

	command.Flags().StringVar(&path, "config", "", "Path to the VM configuration file (required).")
	return command
}
