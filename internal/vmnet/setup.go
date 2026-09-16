package vmnet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/lima-vm/lima/v2/pkg/networks"
	"github.com/mattn/go-isatty"
	"github.com/mr-chelyshkin/limanix/internal/bundle"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Setup owns the interactive, unprivileged side of one network setup attempt.
type Setup struct {
	input       io.Reader
	diagnostics io.Writer
}

// New retains streams without inspecting the host or requesting privileges.
func New(input io.Reader, diagnostics io.Writer) *Setup {
	return &Setup{input: input, diagnostics: diagnostics}
}

// Ensure reuses a secure Lima setup or explicitly requests its installation.
// Noninteractive calls return ErrSetupRequired rather than starting sudo.
func (s *Setup) Ensure(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return ErrMacOS
	}
	if os.Geteuid() == 0 {
		return ErrRunAsUser
	}

	cfg, err := inspect(ctx)
	if !errors.Is(err, ErrSetupRequired) {
		return err
	}
	reason := err

	if err = validateInstallation(cfg); err != nil {
		return err
	}

	if _, err = bundle.SocketVMNet(domain.Architecture(runtime.GOARCH)); err != nil {
		return err
	}

	if err = s.confirm(ctx, cfg, reason); err != nil {
		return err
	}

	if err = s.install(ctx, cfg); err != nil {
		return err
	}

	_, err = inspect(ctx)
	return err
}

func (s *Setup) confirm(ctx context.Context, cfg networks.Config, reason error) error {
	input, ok := s.input.(*os.File)
	if !ok || !isatty.IsTerminal(input.Fd()) {
		return reason
	}

	_, err := fmt.Fprintf(s.diagnostics,
		"Limanix needs Lima's privileged network setup for QEMU.\n"+
			"  Reason:  %v\n"+
			"  Helper:  %s (bundled %s, installed only when missing)\n"+
			"  Sudoers: %s (Lima-generated rules for group %s)\n"+
			"Existing sudoers will be backed up before replacement.\n"+
			"Limanix and QEMU will continue running as your regular user.\n"+
			"Install with administrator approval? [y/N] ",
		reason, helperPath, bundle.SocketVMNetVersion, sudoersPath, cfg.Group)
	if err != nil {
		return err
	}

	answer, err := readAnswer(ctx, s.input)
	if err != nil {
		return fmt.Errorf("read setup confirmation: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	default:
		return ErrSetupDeclined
	}
}

// readAnswer lets cancellation finish the CLI while the terminal is still waiting.
// The buffered result channel does not retain a sender when the context wins.
func readAnswer(ctx context.Context, input io.Reader) (string, error) {
	type result struct {
		answer string
		err    error
	}

	completed := make(chan result, 1)
	go func() {
		answer, err := bufio.NewReader(input).ReadString('\n')
		completed <- result{answer: answer, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-completed:
		return result.answer, result.err
	}
}

func (s *Setup) install(ctx context.Context, cfg networks.Config) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	snapshot, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	// sudo reads the password from the terminal, never from this JSON stream.
	// No persistent sudo rule authorizes this command or the Limanix executable.
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "--user=root", "--group=wheel", "--", executable, "vmnet-install")
	command.Stdin = bytes.NewReader(snapshot)
	command.Stdout = s.diagnostics
	command.Stderr = s.diagnostics
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C"}

	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("install Lima networking: %w", err)
	}

	return nil
}
