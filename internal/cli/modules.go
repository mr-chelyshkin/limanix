package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

func modulesCommand(dependencies Dependencies) *cobra.Command {
	catalog := &cobra.Command{
		Use:   "modules",
		Short: "Manage bundled and third-party NixOS modules.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return usageError(errors.New("a modules subcommand is required"))
		},
	}
	catalog.AddCommand(moduleListCommand(dependencies), moduleAddCommand(dependencies), moduleRemoveCommand(dependencies))
	return catalog
}

func moduleListCommand(dependencies Dependencies) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List available module identifiers.",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			registry, err := dependencies.Registry()
			if err != nil {
				return err
			}
			entries, err := registry.Available(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), entries)
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			for _, entry := range entries {
				detail := entry.Description
				if entry.Error != nil {
					detail = "error: " + *entry.Error
				}
				if _, err := fmt.Fprintf(writer, "%s\t%s\n", entry.Name, detail); err != nil {
					return err
				}
			}
			return writer.Flush()
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "Print machine-readable JSON.")
	return command
}

func moduleAddCommand(dependencies Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "add NAME DIRECTORY",
		Short: "Import a directory containing default.nix.",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := domain.NewModuleName(args[0])
			if err != nil {
				return err
			}
			registry, err := dependencies.Registry()
			if err != nil {
				return err
			}
			if err := registry.Add(cmd.Context(), name, args[1]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Imported third-party:%s.\n", name)
			return err
		},
	}
}

func moduleRemoveCommand(dependencies Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove an imported module from the catalog.",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := domain.NewModuleName(args[0])
			if err != nil {
				return err
			}
			registry, err := dependencies.Registry()
			if err != nil {
				return err
			}
			if err := registry.Remove(cmd.Context(), name); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed third-party:%s from the catalog.\n", name)
			return err
		},
	}
}
