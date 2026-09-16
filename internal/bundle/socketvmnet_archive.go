package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/macho"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// DecodeSocketVMNet checks pinned archive bytes before reading any tar contents.
func DecodeSocketVMNet(target SocketVMNetTarget, archive []byte) (payload SocketVMNetPayload, failure error) {
	if int64(len(archive)) != target.Size || fmt.Sprintf("%x", sha256.Sum256(archive)) != target.SHA256 {
		return SocketVMNetPayload{}, fmt.Errorf("%w: %s", ErrVMNetArchive, target.Filename)
	}

	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return SocketVMNetPayload{}, err
	}
	defer func() {
		failure = errors.Join(failure, reader.Close())
	}()

	payload, err = readSocketVMNet(tar.NewReader(reader))
	if err != nil {
		return SocketVMNetPayload{}, err
	}

	if err = validateSocketVMNet(target.Architecture, payload.Executable); err != nil {
		return SocketVMNetPayload{}, err
	}

	return payload, nil
}

// readSocketVMNet depends on upstream's /opt/socket_vmnet archive layout.
// A release changing these members needs an adapter change, not just new Taskfile pins.
func readSocketVMNet(reader *tar.Reader) (SocketVMNetPayload, error) {
	var payload SocketVMNetPayload

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return payload, err
		}

		var destination *[]byte
		switch strings.TrimPrefix(header.Name, "./") {
		case "opt/socket_vmnet/bin/socket_vmnet":
			destination = &payload.Executable
		case "opt/socket_vmnet/share/doc/socket_vmnet/LICENSE":
			destination = &payload.License
		default:
			continue
		}

		if header.Typeflag != tar.TypeReg || *destination != nil {
			return payload, fmt.Errorf("%w: invalid member %q", ErrVMNetArchive, header.Name)
		}

		*destination, err = io.ReadAll(reader)
		if err != nil {
			return payload, err
		}
	}

	if len(payload.Executable) == 0 || len(payload.License) == 0 {
		return payload, fmt.Errorf("%w: executable or license missing", ErrVMNetArchive)
	}

	return payload, nil
}

func validateSocketVMNet(architecture domain.Architecture, data []byte) (failure error) {
	file, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("socket_vmnet Mach-O: %w", err)
	}
	defer func() {
		failure = errors.Join(failure, file.Close())
	}()

	cpu := map[domain.Architecture]macho.Cpu{
		domain.ARM64: macho.CpuArm64,
		domain.AMD64: macho.CpuAmd64,
	}[architecture]

	if cpu == 0 || file.Cpu != cpu || file.Type != macho.TypeExec {
		return ErrVMNetArchitecture
	}

	if err = validateSocketVMNetDeployment(file); err != nil {
		return err
	}

	libraries, err := file.ImportedLibraries()
	if err != nil {
		return err
	}

	for _, library := range libraries {
		if !strings.HasPrefix(library, "/usr/lib/") && !strings.HasPrefix(library, "/System/Library/") {
			return fmt.Errorf("%w: non-system library %q", ErrVMNetArchive, library)
		}
	}

	return nil
}
