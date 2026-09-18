package nixos

import (
	"encoding/json"
	"fmt"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// Image identifies the base disk that Lima downloads and verifies before first boot.
type Image struct {
	Location string
	SHA256   string
}

// baseFlakeLock reads only the original input references from Nix's lock format.
type baseFlakeLock struct {
	Nodes map[string]struct {
		Original struct {
			Ref string `json:"ref"`
		} `json:"original"`
	} `json:"nodes"`
}

// BaseImage selects the disk matching the pinned nixos-lima input and guest architecture.
//
// Updating this dependency is not a Taskfile version bump: change the input in resources/base/flake.nix, regenerate its flake.lock and update both digests here.
// Review resources/base/platform.nix against the new image's partition layout, bootloader and NixOS compatibility, then verify first boot, update and disk growth.
func BaseImage(architecture domain.Architecture) (Image, error) {
	arch, err := architecture.LimaArch()
	if err != nil {
		return Image{}, err
	}

	release, err := baseImageRelease()
	if err != nil {
		return Image{}, err
	}

	checksums := map[domain.Architecture]string{
		domain.ARM64: "ebdf8363bcb51542892963790c08ddfbffe45443ab19c5e70275c4f0c0aa6f11",
		domain.AMD64: "967da3baf4ea410e728c751ca9e0a617299b6809297e4e50e773cae4ce79197d",
	}

	return Image{
		Location: fmt.Sprintf("https://github.com/nixos-lima/nixos-lima/releases/download/%s/nixos-lima-%s-%s.qcow2", release, release, arch),
		SHA256:   checksums[architecture],
	}, nil
}

func baseImageRelease() (string, error) {
	data, err := resources.ReadFile("resources/base/flake.lock")
	if err != nil {
		return "", err
	}

	var lock baseFlakeLock
	if err = json.Unmarshal(data, &lock); err != nil {
		return "", fmt.Errorf("read base image release: %w", err)
	}

	release := lock.Nodes["nixos-lima"].Original.Ref
	if release == "" {
		return "", ErrImageRelease
	}

	return release, nil
}
