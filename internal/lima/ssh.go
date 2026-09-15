package lima

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/sshutil"
	"github.com/mattn/go-isatty"
)

// remoteScript preserves each argument across SSH's remote shell parsing.
// Entering the management user's login shell loads the guest's NixOS PATH.
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
	if client.sshCommand != nil {
		return client.sshCommand(ctx, inst, args, interactive)
	}
	return client.newSSHCommand(ctx, inst, args, interactive)
}

func (client *Client) newSSHCommand(ctx context.Context, inst *limatype.Instance, args []string, interactive bool) (*exec.Cmd, error) {
	if inst.Config == nil || inst.Config.User.Name == nil || inst.SSHLocalPort <= 0 || inst.SSHLocalPort > 65535 || inst.SSHAddress == "" || strings.HasPrefix(inst.SSHAddress, "-") {
		return nil, errors.New("invalid Lima SSH metadata")
	}
	sshExe, err := sshutil.NewSSHExe()
	if err != nil {
		return nil, fmt.Errorf("locate system SSH: %w", err)
	}
	ssh := inst.Config.SSH
	value := func(setting *bool) bool { return setting != nil && *setting }
	opts, err := sshutil.SSHOpts(ctx, sshExe, inst.Dir, *inst.Config.User.Name,
		value(ssh.LoadDotSSHPubKeys), value(ssh.ForwardAgent), value(ssh.ForwardX11), value(ssh.ForwardX11Trusted))
	if err != nil {
		return nil, err
	}
	opts = append(opts, "ConnectTimeout=10")
	arguments := append(slices.Clone(sshExe.Args), sshutil.SSHArgsFromOpts(opts)...)
	if interactive {
		if output, ok := client.Stdout.(*os.File); ok && isatty.IsTerminal(output.Fd()) {
			arguments = append(arguments, "-t")
		}
	} else {
		arguments = append(arguments, "-T", "-n", "-o", "BatchMode=yes")
	}
	arguments = append(arguments, "-o", "LogLevel=ERROR", "-p", strconv.Itoa(inst.SSHLocalPort), inst.SSHAddress, "--", remoteScript(args))
	return exec.CommandContext(ctx, sshExe.Exe, arguments...), nil
}

// Run executes a management command without a host shell or a build timeout.
func (client *Client) Run(ctx context.Context, name string, args []string, capture bool) (string, error) {
	if len(args) == 0 {
		return "", errors.New("a guest command is required")
	}
	command, err := client.session(ctx, name, args, false)
	if err != nil {
		return "", &Error{Operation: "SSH", Err: err}
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 5 * time.Second
	var stdout bytes.Buffer
	stderr := &tailBuffer{limit: 64 * 1024}
	if capture {
		command.Stdout, command.Stderr = &stdout, stderr
	} else {
		command.Stdout, command.Stderr = client.Stdout, io.MultiWriter(client.Stderr, stderr)
	}
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			// WaitDelay kills SSH itself; helpers can remain in its process group.
			var cleanupErr error
			if command.Process != nil {
				cleanupErr = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				if errors.Is(cleanupErr, syscall.ESRCH) {
					cleanupErr = nil
				}
			}
			return "", &Error{Operation: "SSH", Err: errors.Join(ctx.Err(), cleanupErr)}
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			message := fmt.Sprintf("exited with status %d", exitStatus(exit))
			if detail := strings.TrimSpace(string(stderr.data)); detail != "" {
				message += ": " + detail
			}
			return "", &Error{Operation: "SSH", Err: errors.New(message)}
		}
		return "", &Error{Operation: "SSH", Err: err}
	}
	return stdout.String(), nil
}

// Shell inherits the controlling terminal and preserves the guest's exit status.
// SIGINT belongs to foreground SSH; the CLI only cancels this context on SIGTERM.
func (client *Client) Shell(ctx context.Context, name string, args []string) (int, error) {
	command, err := client.session(ctx, name, args, true)
	if err != nil {
		return 0, &Error{Operation: "SSH", Err: err}
	}
	command.Stdin, command.Stdout, command.Stderr = client.Stdin, client.Stdout, client.Stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return 0, &Error{Operation: "SSH", Err: ctx.Err()}
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exitStatus(exit), nil
		}
		return 0, &Error{Operation: "SSH", Err: err}
	}
	return 0, nil
}

func exitStatus(exit *exec.ExitError) int {
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return exit.ExitCode()
}

type tailBuffer struct {
	data  []byte
	limit int
}

func (buffer *tailBuffer) Write(data []byte) (int, error) {
	count := len(data)
	if count >= buffer.limit {
		buffer.data = append(buffer.data[:0], data[count-buffer.limit:]...)
		return count, nil
	}
	if len(buffer.data)+count > buffer.limit {
		buffer.data = slices.Clone(buffer.data[len(buffer.data)+count-buffer.limit:])
	}
	buffer.data = append(buffer.data, data...)
	return count, nil
}
