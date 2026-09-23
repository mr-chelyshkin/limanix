package lima

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Client operates Lima's store, drivers and instance lifecycle in-process.
type Client struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	native     nativeAPI
	executable func() (string, error)
	agentPath  func(context.Context, domain.Architecture) (string, error)
	sshCommand func(context.Context, *limatype.Instance, []string, bool) (*exec.Cmd, error)
}

// NewClient connects the native Lima APIs to the packaged guest-agent provider.
func NewClient(agentPath func(context.Context, domain.Architecture) (string, error)) *Client {
	client := &Client{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		agentPath:  agentPath,
		native:     newNativeAPI(),
		executable: os.Executable,
	}

	client.sshCommand = client.newSSHCommand
	return client
}

func queryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}
