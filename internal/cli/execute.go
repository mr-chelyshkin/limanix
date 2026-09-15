package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// exitError separates usage failures and guest exit status from operational errors.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}
func (e *exitError) Unwrap() error { return e.err }
func usageError(err error) error   { return &exitError{code: 2, err: err} }
func exactArgs(count int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(count)(cmd, args); err != nil {
			return usageError(err)
		}
		return nil
	}
}

// Execute runs the command once and returns an OS exit status. Long operations
// observe interruption through context; an interactive SSH shell receives SIGINT
// in its own foreground process rather than being killed by parent cancellation.
func Execute(ctx context.Context, args []string, streams IO, dependencies Dependencies) int {
	root := Command(streams, dependencies)
	root.SetArgs(args)
	found, _, findErr := root.Find(args)
	if findErr != nil {
		_, _ = fmt.Fprintf(streams.Err, "limanix: %v\n", findErr)
		return 2
	}
	logrus.SetOutput(streams.Err)
	if found.Name() == "hostagent" {
		// Argument and flag errors precede the hostagent's action. Lima's event
		// watcher requires JSON even when that action is never reached.
		logrus.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{})
	}
	var stop context.CancelFunc
	if found.Name() == "hostagent" {
		// The detached hostagent owns its signal channel and graceful shutdown.
		stop = func() {}
	} else if found.Name() == "shell" {
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
		ctx, stop = signal.NotifyContext(ctx, syscall.SIGTERM)
	} else {
		ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	}
	defer stop()
	_, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}
	if found.Name() == "hostagent" {
		logrus.WithError(err).Error("host agent exited")
		return 1
	}
	var exit *exitError
	if errors.As(err, &exit) {
		if exit.err != nil {
			_, _ = fmt.Fprintf(streams.Err, "limanix: %v\n", exit.err)
		}
		return exit.code
	}
	if errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(streams.Err, "limanix: interrupted.")
		return 130
	}
	_, _ = fmt.Fprintf(streams.Err, "limanix: %v\n", err)
	return 1
}
