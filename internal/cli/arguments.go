package cli

import "github.com/spf13/cobra"

func exactArgs(count int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(count)(cmd, args); err != nil {
			return usageError(err)
		}

		return nil
	}
}

func requiredConfig(path *string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := exactArgs(0)(cmd, args); err != nil {
			return err
		}

		if *path == "" {
			return usageError(errMissingConfig)
		}

		return nil
	}
}
