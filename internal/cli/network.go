package cli

import (
	"context"

	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/vmnet"
	"github.com/spf13/cobra"
)

func networkCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "network",
		Short: "Prepare Lima's macOS networking for QEMU guests.",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	command.AddCommand(&cobra.Command{
		Use:   "setup",
		Short: "Install the bundled socket_vmnet helper and Lima sudoers with administrator approval.",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNativeNetworkHost(cmd.Context()); err != nil {
				return err
			}

			return vmnet.New(cmd.InOrStdin(), cmd.ErrOrStderr()).Ensure(cmd.Context())
		},
	})

	return command
}

func networkInstallerCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "vmnet-install",
		Short:  "Run the internal privileged network installer.",
		Hidden: true,
		Args:   exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireNativeNetworkHost(cmd.Context()); err != nil {
				return err
			}

			return vmnet.Install(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr())
		},
	}
}

func requireNativeNetworkHost(ctx context.Context) error {
	if err := lima.RequireMacOS(ctx); err != nil {
		return err
	}

	architecture, err := lima.HostArchitecture()
	if err != nil {
		return err
	}

	return lima.RequireNativeArchitecture(architecture)
}
