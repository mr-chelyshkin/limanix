package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

// Execute runs one CLI invocation and returns its exit status without exiting the process.
func Execute(ctx context.Context, args []string, streams IO, dependencies Dependencies) int {
	root := Command(streams, dependencies)
	root.SetArgs(args)

	found, _, err := root.Find(args)
	if err != nil {
		_, _ = fmt.Fprintf(streams.Err, "limanix: %v\n", err)
		return 2
	}

	command := found.Name()
	configureLogging(command, streams)

	ctx, stop := commandContext(ctx, command)
	defer stop()

	if _, err := root.ExecuteContextC(ctx); err != nil {
		if command == "hostagent" {
			logrus.WithError(err).Error("host agent exited")
			return 1
		}

		return reportError(streams.Err, err)
	}

	return 0
}

func configureLogging(command string, streams IO) {
	logrus.SetOutput(streams.Err)

	if command == "hostagent" {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{})
	}
}

// commandContext leaves host-agent shutdown to its runtime. Interactive SSH owns
// foreground SIGINT; the parent CLI handles only SIGTERM for that command.
func commandContext(ctx context.Context, command string) (context.Context, context.CancelFunc) {
	switch command {
	case "hostagent":
		return ctx, func() {}

	case "shell":
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)

		session, cancel := signal.NotifyContext(ctx, syscall.SIGTERM)
		return session, func() {
			cancel()
			signal.Stop(interrupts)
		}

	default:
		return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	}
}
