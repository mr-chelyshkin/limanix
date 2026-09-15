package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// Reference renders the CLI from its actual command tree and flag definitions.
func Reference() string {
	root := Command(IO{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard}, Dependencies{})
	root.InitDefaultHelpCmd()
	root.InitDefaultHelpFlag()

	var (
		result strings.Builder
		render func(*cobra.Command)
	)

	render = func(cmd *cobra.Command) {
		if cmd.Hidden || cmd.Name() == "help" {
			return
		}
		cmd.InitDefaultHelpFlag()
		
		fmt.Fprintf(&result, "## `%s`\n\n%s\n\n```text\n%s\n```\n\n", cmd.CommandPath(), cmd.Short, strings.TrimSpace(cmd.UsageString()))
		for _, child := range cmd.Commands() {
			render(child)
		}
	}
	render(root)
	return result.String()
}
