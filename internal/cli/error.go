package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
)

var (
	errMissingConfig        = errors.New("required flag --config was not provided")
	errMissingModuleCommand = errors.New("a modules subcommand is required")
)

// exitError carries either a usage failure or an unmodified guest exit status.
type exitError struct {
	code int
	err  error
}

// Error returns the underlying message, or an empty string for a status-only result.
func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}

	return e.err.Error()
}

// Unwrap exposes the underlying error to errors.Is and errors.As.
func (e *exitError) Unwrap() error {
	return e.err
}

func usageError(err error) error {
	return &exitError{
		code: 2,
		err:  err,
	}
}

func reportError(output io.Writer, err error) int {
	if exit, ok := errors.AsType[*exitError](err); ok {
		if exit.err != nil {
			_, _ = fmt.Fprintf(output, "limanix: %v\n", exit.err)
		}

		return exit.code
	}

	if errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(output, "limanix: interrupted.")
		return 130
	}

	_, _ = fmt.Fprintf(output, "limanix: %v\n", err)
	return 1
}
