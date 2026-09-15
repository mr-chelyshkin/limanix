// Package hostagent runs Lima's native host agent inside the Limanix executable.
package hostagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	limaagent "github.com/lima-vm/lima/v2/pkg/hostagent"
	"github.com/lima-vm/lima/v2/pkg/hostagent/api/server"
	"github.com/lima-vm/lima/v2/pkg/limatype/dirnames"
	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/mr-chelyshkin/limanix/internal/lima"
	"github.com/mr-chelyshkin/limanix/internal/state"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

var identifierPattern = regexp.MustCompile(fmt.Sprintf(`^[a-f0-9]{%d}$`, state.IDLength))

// validateName limits the internal agent to exact generated Limanix instance names.
func validateName(name string) error {
	if !strings.HasPrefix(name, "limanix-") {
		return errors.New("host agent requires a Limanix instance name")
	}
	identity := strings.TrimPrefix(name, "limanix-")
	separator := strings.LastIndexByte(identity, '-')
	if separator < 0 || !identifierPattern.MatchString(identity[separator+1:]) {
		return errors.New("host agent requires a generated Limanix instance identifier")
	}
	if _, err := domain.NewVMName(identity[:separator]); err != nil {
		return err
	}
	return nil
}

// Command implements the hidden subprocess interface used by Lima StartWithPaths.
// The parent CLI must leave SIGINT/SIGTERM handling to this command.
func Command() *cobra.Command {
	command := &cobra.Command{Use: "hostagent INSTANCE", Short: "Run the internal Lima host agent.", Hidden: true, Args: cobra.ExactArgs(1)}
	command.Flags().StringP("pidfile", "p", "", "Exact instance host-agent PID file.")
	command.Flags().String("socket", "", "Exact instance host-agent Unix socket.")
	command.Flags().String("guestagent", "", "Local compressed Lima guest-agent executable.")
	command.Flags().String("nerdctl-archive", "", "Local containerd/nerdctl archive, if configured.")
	command.Flags().Bool("run-gui", false, "Run the VZ GUI on this OS thread.")
	command.Flags().Bool("progress", false, "Show cloud-init progress.")
	command.RunE = runHostAgent
	return command
}

func validatePaths(name, pidfile, socket string) error {
	if err := validateName(name); err != nil {
		return err
	}
	directory, err := dirnames.InstanceDir(name)
	if err != nil {
		return err
	}
	if err := filesystem.CheckDirectory(directory); err != nil {
		return err
	}
	if pidfile != filepath.Join(directory, filenames.HostAgentPID) || socket != filepath.Join(directory, filenames.HostAgentSock) {
		return errors.New("host-agent PID and socket paths must belong to the selected instance")
	}
	return nil
}

func validateOptionalResource(filename string) error {
	if filename == "" {
		return nil
	}
	if !filepath.IsAbs(filename) {
		return errors.New("agent resources must be absolute local file paths")
	}
	file, err := filesystem.OpenRegular(filename, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	return file.Close()
}

func runHostAgent(command *cobra.Command, args []string) (result error) {
	stdout := &syncWriter{output: command.OutOrStdout()}
	stderr := &syncWriter{output: command.ErrOrStderr()}
	logrus.SetOutput(stderr)
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.DebugLevel)
	ctx := command.Context()
	if err := ctx.Err(); err != nil {
		return err
	}
	host, err := lima.HostArchitecture()
	if err != nil {
		return err
	}
	if err := lima.RequireNativeArchitecture(host); err != nil {
		return err
	}
	pidfile, _ := command.Flags().GetString("pidfile")
	socket, _ := command.Flags().GetString("socket")
	if err := validatePaths(args[0], pidfile, socket); err != nil {
		return err
	}
	guestAgentArchive, _ := command.Flags().GetString("guestagent")
	if guestAgentArchive == "" {
		return errors.New("the embedded guest-agent archive must be specified")
	}
	nerdctlArchive, _ := command.Flags().GetString("nerdctl-archive")
	for _, filename := range []string{guestAgentArchive, nerdctlArchive} {
		if err := validateOptionalResource(filename); err != nil {
			return err
		}
	}
	lease, err := acquirePID(pidfile)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, lease.Close()) }()
	if runGUI, _ := command.Flags().GetBool("run-gui"); runGUI {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	// Lima's graceful shutdown selects its signal channel, then needs a live
	// context to stop the VM. External cancellation is translated into SIGTERM.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	stopCancel := context.AfterFunc(ctx, func() {
		select {
		case signals <- syscall.SIGTERM:
		default:
		}
	})
	defer stopCancel()
	progress, _ := command.Flags().GetBool("progress")
	options := []limaagent.Opt{limaagent.WithGuestAgentBinary(guestAgentArchive), limaagent.WithCloudInitProgress(progress)}
	if nerdctlArchive != "" {
		options = append(options, limaagent.WithNerdctlArchive(nerdctlArchive))
	}
	agent, err := limaagent.New(runCtx, args[0], stdout, signals, options...)
	if err != nil {
		return err
	}
	if err := removeSocket(socket); err != nil {
		return err
	}
	listener, err := listenSocket(runCtx, socket)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	server.AddRoutes(mux, &server.Backend{Agent: agent})
	apiServer := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	serveErrors := make(chan error, 1)
	go func() {
		err := apiServer.Serve(listener)
		serveErrors <- err
		if !errors.Is(err, http.ErrServerClosed) {
			select {
			case signals <- syscall.SIGTERM:
			default:
			}
		}
	}()
	defer func() {
		closeErr := apiServer.Close()
		serveErr := <-serveErrors
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		result = errors.Join(result, closeErr, serveErr, removeSocket(socket))
	}()
	logrus.Infof("hostagent socket created at %s", socket)
	return agent.Run(runCtx)
}

func listenSocket(ctx context.Context, socket string) (*net.UnixListener, error) {
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
		return errors.New("host-agent socket path is occupied by a non-socket file")
	}
	return os.Remove(filename)
}

type syncWriter struct {
	mutex  sync.Mutex
	output io.Writer
}

func (writer *syncWriter) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	count, err := writer.output.Write(data)
	if file, ok := writer.output.(*os.File); ok && err == nil {
		if info, statErr := file.Stat(); statErr == nil && info.Mode().IsRegular() {
			err = file.Sync()
		}
	}
	return count, err
}
