package guest

import (
	"context"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Shell enters the regular user's home and preserves the command's exit status.
// An empty command opens that user's login shell.
func (guest *Guest) Shell(ctx context.Context, name string, user domain.Username, command []string) (int, error) {
	return guest.client.Shell(ctx, name, userCommand(user, command))
}

// userCommand changes to HOME and execs literal positional arguments.
// A fixed explicit Bash script avoids the argument-quoting changes made by sudo --login.
func userCommand(user domain.Username, command []string) []string {
	var (
		bash = "/run/current-system/sw/bin/bash"
		args = []string{
			"sudo",
			"--set-home",
			"--user", string(user),
			"--", bash,
		}
	)

	if len(command) > 0 {
		args = append(args, "--login")
	} else {
		command = []string{bash, "--login"}
	}

	args = append(args, "-c", `cd -- "$HOME" && exec "$@"`, "limanix-command")
	return append(args, command...)
}
