package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mr-chelyshkin/limanix/internal/config"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/state"
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

func configurationCommand(operation string, dependencies Dependencies) *cobra.Command {
	var path string
	command := &cobra.Command{
		Use: operation,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := exactArgs(0)(cmd, args); err != nil {
				return err
			}
			if path == "" {
				return usageError(errors.New("required flag --config was not provided"))
			}
			return nil
		},
	}
	if operation == "create" {
		command.Short = "Create a development sandbox from a TOML configuration."
	} else {
		command.Short = "Apply a configuration and restart an existing VM."
	}
	command.Flags().StringVar(&path, "config", "", "Path to the VM configuration file (required).")
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		manager, err := dependencies.Manager()
		if err != nil {
			return err
		}
		var instance state.Instance
		if operation == "create" {
			instance, err = manager.Create(cmd.Context(), path)
		} else {
			instance, err = manager.Update(cmd.Context(), path)
		}
		if err != nil {
			return err
		}
		if operation == "create" {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created %s. Managed home: %s\n", instance.Identity.Name, instance.Identity.Home)
		} else {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated %s.\n", instance.Identity.Name)
		}
		return err
	}
	return command
}
