package hostagent

import (
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Command implements the hidden subprocess interface used by Lima StartWithPaths.
func Command() *cobra.Command {
	command := &cobra.Command{
		Use:    "hostagent INSTANCE",
		Short:  "Run the internal Lima host agent.",
		Args:   cobra.ExactArgs(1),
		RunE:   runHostAgent,
		Hidden: true,
	}

	flags := command.Flags()
	flags.StringP("pidfile", "p", "", "Exact instance host-agent PID file.")
	flags.String("socket", "", "Exact instance host-agent Unix socket.")
	flags.String("guestagent", "", "Local compressed Lima guest-agent executable.")
	flags.String("nerdctl-archive", "", "Local containerd/nerdctl archive, if configured.")
	flags.Bool("run-gui", false, "Run the VZ GUI on this OS thread.")
	flags.Bool("progress", false, "Show cloud-init progress.")

	return command
}

func runHostAgent(command *cobra.Command, args []string) error {
	var (
		ctx    = command.Context()
		stdout = &syncWriter{output: command.OutOrStdout()}
		stderr = &syncWriter{output: command.ErrOrStderr()}
	)

	logrus.SetOutput(stderr)
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.DebugLevel)

	if err := ctx.Err(); err != nil {
		return err
	}

	host, err := lima.HostArchitecture()
	if err != nil {
		return err
	}

	if err = lima.RequireNativeArchitecture(host); err != nil {
		return err
	}

	options := readOptions(command, args[0])
	if err = options.validate(); err != nil {
		return err
	}

	return options.run(ctx, stdout)
}
