package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

func listCommand(dependencies Dependencies) *cobra.Command {
	var asJSON bool

	command := &cobra.Command{
		Use:   "list",
		Short: "List VMs managed by Limanix.",
		Args:  exactArgs(0),

		RunE: func(cmd *cobra.Command, _ []string) error {
			manager, err := dependencies.Manager()
			if err != nil {
				return err
			}

			entries, err := manager.FetchAll(cmd.Context())
			if err != nil {
				return err
			}

			if asJSON {
				return writeJSON(cmd.OutOrStdout(), entries)
			}

			return writeInstances(cmd.OutOrStdout(), entries)
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "Print machine-readable JSON.")
	return command
}

func deleteCommand(dependencies Dependencies) *cobra.Command {
	var force, removeHome bool

	command := &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a VM; preserve its managed host home by default.",
		Args:  exactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := domain.NewVMName(args[0])
			if err != nil {
				return err
			}

			manager, err := dependencies.Manager()
			if err != nil {
				return err
			}

			home, err := manager.Delete(cmd.Context(), name, force, removeHome)
			if err != nil {
				return err
			}

			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s.\n", name); err != nil {
				return err
			}

			if !removeHome {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Preserved managed home: %s\n", home)
			}

			return err
		},
	}

	command.Flags().BoolVar(&force, "force", false, "Force Lima to stop and delete the VM.")
	command.Flags().BoolVar(&removeHome, "remove-home", false, "Also remove this VM's managed host home and its contents.")
	return command
}

func shellCommand(dependencies Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:                "shell NAME [-- COMMAND ...]",
		Short:              "Connect as the configured development user.",
		DisableFlagParsing: true,

		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
				return usageError(err)
			}

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "--help" || args[0] == "-h" {
				return cmd.Help()
			}

			name, err := domain.NewVMName(args[0])
			if err != nil {
				return err
			}

			command := args[1:]
			if len(command) > 0 && command[0] == "--" {
				command = command[1:]
			}

			manager, err := dependencies.Manager()
			if err != nil {
				return err
			}

			status, err := manager.Shell(cmd.Context(), name, command)
			if err != nil {
				return err
			}

			if status != 0 {
				return &exitError{code: status}
			}

			return nil
		},
	}
}
