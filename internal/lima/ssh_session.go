package lima

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/sshutil"
	"github.com/mattn/go-isatty"
)

func remoteScript(args []string) string {
	script := `cd -- / && exec "$SHELL" -l`
	if len(args) == 0 {
		return script
	}

	quoted := make([]string, len(args))

	for index, arg := range args {
		quoted[index] = shellescape.Quote(arg)
	}

	return script + " -c " + shellescape.Quote(strings.Join(quoted, " "))
}

func (client *Client) session(ctx context.Context, name string, args []string, interactive bool) (*exec.Cmd, error) {
	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return nil, err
	}

	if inst.Status != limatype.StatusRunning {
		return nil, fmt.Errorf("VM %s is not running", name)
	}

	return client.sshCommand(ctx, inst, args, interactive)
}

func (client *Client) newSSHCommand(ctx context.Context, inst *limatype.Instance, args []string, interactive bool) (*exec.Cmd, error) {
	if err := validateSSHMetadata(inst); err != nil {
		return nil, err
	}

	sshExe, err := sshutil.NewSSHExe()
	if err != nil {
		return nil, fmt.Errorf("locate system SSH: %w", err)
	}

	ssh := inst.Config.SSH
	opts, err := sshutil.SSHOpts(
		ctx,
		sshExe,
		inst.Dir,
		*inst.Config.User.Name,
		enabled(ssh.LoadDotSSHPubKeys),
		enabled(ssh.ForwardAgent),
		enabled(ssh.ForwardX11),
		enabled(ssh.ForwardX11Trusted),
	)
	if err != nil {
		return nil, err
	}

	opts = append(opts, "ConnectTimeout=10")
	arguments := append(slices.Clone(sshExe.Args), sshutil.SSHArgsFromOpts(opts)...)

	switch {
	case !interactive:
		arguments = append(arguments, "-T", "-n", "-o", "BatchMode=yes")
	case terminalOutput(client.Stdout):
		arguments = append(arguments, "-t")
	}

	arguments = append(arguments,
		"-o", "LogLevel=ERROR",
		"-p", strconv.Itoa(inst.SSHLocalPort),
		inst.SSHAddress,
		"--", remoteScript(args),
	)

	return exec.CommandContext(ctx, sshExe.Exe, arguments...), nil
}

func validateSSHMetadata(inst *limatype.Instance) error {
	switch {
	case inst.Config == nil:
		return ErrInvalidSSHMetadata
	case inst.Config.User.Name == nil:
		return ErrInvalidSSHMetadata
	case inst.SSHLocalPort <= 0 || inst.SSHLocalPort > 65535:
		return ErrInvalidSSHMetadata
	case inst.SSHAddress == "" || strings.HasPrefix(inst.SSHAddress, "-"):
		return ErrInvalidSSHMetadata
	default:
		return nil
	}
}

func terminalOutput(output any) bool {
	file, ok := output.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}
