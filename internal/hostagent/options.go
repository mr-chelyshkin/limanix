package hostagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	limaagent "github.com/lima-vm/lima/v2/pkg/hostagent"
	"github.com/lima-vm/lima/v2/pkg/limatype/dirnames"
	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

type options struct {
	name              string
	socket            string
	pidfile           string
	nerdctlArchive    string
	guestAgentArchive string
	runGUI            bool
	progress          bool
}

func readOptions(command *cobra.Command, name string) options {
	var (
		flags  = command.Flags()
		result = options{name: name}
	)

	result.pidfile, _ = flags.GetString("pidfile")
	result.socket, _ = flags.GetString("socket")
	result.guestAgentArchive, _ = flags.GetString("guestagent")
	result.nerdctlArchive, _ = flags.GetString("nerdctl-archive")
	result.runGUI, _ = flags.GetBool("run-gui")
	result.progress, _ = flags.GetBool("progress")

	return result
}

func (opts options) validate() error {
	if err := validatePaths(opts.name, opts.pidfile, opts.socket); err != nil {
		return err
	}

	if opts.guestAgentArchive == "" {
		return ErrMissingGuestAgent
	}

	for _, filename := range []string{opts.guestAgentArchive, opts.nerdctlArchive} {
		if err := validateOptionalResource(filename); err != nil {
			return err
		}
	}

	return nil
}

func (opts options) agentOptions() []limaagent.Opt {
	result := []limaagent.Opt{
		limaagent.WithGuestAgentBinary(opts.guestAgentArchive),
		limaagent.WithCloudInitProgress(opts.progress),
	}

	if opts.nerdctlArchive != "" {
		result = append(result, limaagent.WithNerdctlArchive(opts.nerdctlArchive))
	}

	return result
}

func validateName(name string) error {
	if !strings.HasPrefix(name, "limanix-") {
		return ErrForeignInstance
	}

	var (
		identity  = strings.TrimPrefix(name, "limanix-")
		separator = strings.LastIndexByte(identity, '-')
	)

	if separator < 0 || !domain.ValidIdentifier(identity[separator+1:]) {
		return ErrInvalidIdentifier
	}

	_, err := domain.NewVMName(identity[:separator])
	return err
}

func validatePaths(name, pidfile, socket string) error {
	if err := validateName(name); err != nil {
		return err
	}

	directory, err := dirnames.InstanceDir(name)
	if err != nil {
		return err
	}

	if err = requirePrivateDirectory(directory); err != nil {
		return err
	}

	if pidfile != filepath.Join(directory, filenames.HostAgentPID) {
		return ErrForeignPaths
	}

	if socket != filepath.Join(directory, filenames.HostAgentSock) {
		return ErrForeignPaths
	}

	return nil
}

func requirePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("%w: %s", ErrDirectoryPermissions, path)
	}

	return nil
}

func validateOptionalResource(filename string) error {
	if filename == "" {
		return nil
	}

	if !filepath.IsAbs(filename) {
		return ErrRelativeResource
	}

	file, err := filesystem.OpenRegular(filename, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}

	return file.Close()
}
