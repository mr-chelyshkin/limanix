package hostagent

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	limaagent "github.com/lima-vm/lima/v2/pkg/hostagent"
	"github.com/sirupsen/logrus"
)

func (opts options) run(ctx context.Context, stdout io.Writer) (failure error) {
	lease, err := acquirePID(opts.pidfile)
	if err != nil {
		return err
	}

	defer func() {
		failure = errors.Join(failure, lease.Close())
	}()

	if opts.runGUI {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	// Lima needs a live context after receiving its shutdown signal.
	// Translate parent cancellation into SIGTERM instead of canceling its cleanup.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()

	stopCancel := context.AfterFunc(ctx, func() {
		requestShutdown(signals)
	})
	defer stopCancel()

	agent, err := limaagent.New(runCtx, opts.name, stdout, signals, opts.agentOptions()...)
	if err != nil {
		return err
	}

	api, err := startAPIServer(runCtx, opts.socket, agent, signals)
	if err != nil {
		return err
	}

	defer func() {
		failure = errors.Join(failure, api.Close())
	}()

	logrus.Infof("hostagent socket created at %s", opts.socket)
	return agent.Run(runCtx)
}

func requestShutdown(signals chan<- os.Signal) {
	select {
	case signals <- syscall.SIGTERM:
	default:
	}
}
