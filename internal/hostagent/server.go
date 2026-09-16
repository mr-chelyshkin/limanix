package hostagent

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	limaagent "github.com/lima-vm/lima/v2/pkg/hostagent"
	"github.com/lima-vm/lima/v2/pkg/hostagent/api/server"
)

// apiServer owns the listener and its path until Serve has returned.
type apiServer struct {
	server *http.Server
	socket string
	result <-chan error
}

func startAPIServer(ctx context.Context, socket string, agent *limaagent.HostAgent, signals chan<- os.Signal) (*apiServer, error) {
	if err := removeSocket(socket); err != nil {
		return nil, err
	}

	listener, err := listenSocket(ctx, socket)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	server.AddRoutes(mux, &server.Backend{Agent: agent})

	var (
		httpServer = &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		result = make(chan error, 1)
	)

	go func() {
		err := httpServer.Serve(listener)
		result <- err

		if !errors.Is(err, http.ErrServerClosed) {
			requestShutdown(signals)
		}
	}()

	return &apiServer{
		server: httpServer,
		socket: socket,
		result: result,
	}, nil
}

// Close stops serving, waits for Serve to finish, and removes the checked socket.
func (api *apiServer) Close() error {
	closeErr := api.server.Close()
	serveErr := <-api.result
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}

	return errors.Join(closeErr, serveErr, removeSocket(api.socket))
}

func listenSocket(ctx context.Context, socket string) (*net.UnixListener, error) {
	// The directory prevents access before the socket receives its own 0600 mode.
	// Do not change the process-wide umask while other goroutines may create files.
	if err := requirePrivateDirectory(filepath.Dir(socket)); err != nil {
		return nil, err
	}

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "unix", socket)
	if err != nil {
		return nil, err
	}

	unixListener := listener.(*net.UnixListener)
	// Closing the listener must leave path validation to controlled cleanup.
	unixListener.SetUnlinkOnClose(false)
	if err := os.Chmod(socket, 0o600); err != nil {
		return nil, errors.Join(err, unixListener.Close(), removeSocket(socket))
	}

	return unixListener, nil
}

func removeSocket(filename string) error {
	info, err := os.Lstat(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSocket == 0 {
		return ErrOccupiedSocket
	}

	return os.Remove(filename)
}
