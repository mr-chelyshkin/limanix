package lima

import (
	"fmt"
	"path/filepath"

	"github.com/lima-vm/lima/v2/pkg/limatype/dirnames"
	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
	"github.com/lima-vm/lima/v2/pkg/osutil"
)

// validateInstanceSocket uses the same byte boundary and longest temporary SSH
// socket as Lima's instance.Create, before allocating host home or VM records.
func validateInstanceSocket(name string) error {
	if err := validateOwnedInstanceName(name); err != nil {
		return err
	}
	directory, err := dirnames.InstanceDir(name)
	if err != nil {
		return err
	}
	socket := filepath.Join(directory, filenames.LongestSock)
	if len(socket) >= osutil.UnixPathMax {
		return fmt.Errorf("VM name is too long for the Lima directory: SSH socket path needs %d bytes (maximum %d); use a shorter VM name or storage path", len(socket), osutil.UnixPathMax-1)
	}
	return nil
}
