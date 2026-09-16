package cli

import (
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/spf13/cobra"
)

func startCommand(dependencies Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "start NAME",
		Short: "Start an existing VM.",
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

			if err := manager.Start(cmd.Context(), name); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Started %s.\n", name)
			return err
		},
	}
}

func stopCommand(dependencies Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "stop NAME",
		Short: "Stop a VM and preserve its disk and home.",
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

			if err := manager.Stop(cmd.Context(), name); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Stopped %s.\n", name)
			return err
		},
	}
}
