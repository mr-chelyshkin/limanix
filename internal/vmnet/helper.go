package vmnet

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const helperProbeTimeout = 5 * time.Second

func checkHelper(ctx context.Context, path string) error {
	probe, cancel := context.WithTimeout(ctx, helperProbeTimeout)
	defer cancel()

	output, err := exec.CommandContext(probe, path, "--version").CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if probe.Err() != nil {
		return fmt.Errorf("%w: %s: %w", ErrHelperUnavailable, path, probe.Err())
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %w: %s", ErrHelperUnavailable, path, err, strings.TrimSpace(string(output)))
	}

	return nil
}
