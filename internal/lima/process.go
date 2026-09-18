package lima

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"syscall"
	"time"
)

func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = 5 * time.Second

	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}

		return err
	}
}

func killProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}

	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}

	return err
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

// Write accepts the full input while retaining only the configured diagnostic tail.
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
