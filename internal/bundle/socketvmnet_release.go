package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// SocketVMNetVersion is linked from Taskfile's socket_vmnet.version, together
// with the archive pins below. It is build metadata, not a runtime setting.
var SocketVMNetVersion string

// socketVMNetArchives is linked from socket_vmnet.archives as JSON. Keeping the
// version and both pins in Taskfile gives packaging and runtime identical inputs.
var socketVMNetArchives string

// Version labels become URL and filename components; arbitrary paths are not accepted.
var socketVMNetVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

// socketVMNetArchive pins the exact compressed bytes for one host architecture.
type socketVMNetArchive struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// SocketVMNetTargets validates the linked release metadata and derives archive names.
// Upstream calls the Intel archive x86_64 and the Apple Silicon archive arm64;
// these are packaging identifiers, independent of Lima's guest architecture names.
func SocketVMNetTargets() ([]SocketVMNetTarget, error) {
	if !socketVMNetVersionPattern.MatchString(SocketVMNetVersion) {
		return nil, fmt.Errorf("%w: version %q", ErrVMNetRelease, SocketVMNetVersion)
	}

	archives, err := readSocketVMNetArchives()
	if err != nil {
		return nil, err
	}
	if len(archives) != 2 {
		return nil, fmt.Errorf("%w: expected arm64 and amd64 archive pins", ErrVMNetRelease)
	}

	targets := make([]SocketVMNetTarget, 0, len(archives))
	for _, host := range []struct {
		architecture domain.Architecture
		archiveArch  string
	}{
		{domain.ARM64, "arm64"},
		{domain.AMD64, "x86_64"},
	} {
		pin := archives[host.architecture]
		if err := pin.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", host.architecture, err)
		}

		targets = append(targets, SocketVMNetTarget{
			Architecture: host.architecture,
			Filename:     fmt.Sprintf("socket_vmnet-%s-%s.tar.gz", SocketVMNetVersion, host.archiveArch),
			SHA256:       pin.SHA256,
			Size:         pin.Size,
		})
	}

	return targets, nil
}

func readSocketVMNetArchives() (map[domain.Architecture]socketVMNetArchive, error) {
	var archives map[domain.Architecture]socketVMNetArchive
	decoder := json.NewDecoder(strings.NewReader(socketVMNetArchives))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&archives); err != nil {
		return nil, fmt.Errorf("%w: archives: %w", ErrVMNetRelease, err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: expected one archive-pins document", ErrVMNetRelease)
	}

	return archives, nil
}

func (archive socketVMNetArchive) validate() error {
	digest, err := hex.DecodeString(archive.SHA256)
	if err != nil || len(digest) != sha256.Size || archive.SHA256 != strings.ToLower(archive.SHA256) {
		return fmt.Errorf("%w: expected a lowercase SHA-256 digest", ErrVMNetRelease)
	}
	if archive.Size <= 0 || archive.Size == math.MaxInt64 {
		return fmt.Errorf("%w: invalid archive size %d", ErrVMNetRelease, archive.Size)
	}

	return nil
}
