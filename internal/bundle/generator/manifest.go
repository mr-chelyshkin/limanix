package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

const manifestSchema = 2

type artifact struct {
	Name       string        `json:"name"`
	Arch       string        `json:"arch"`
	SHA256     string        `json:"sha256"`
	BuildFlags []string      `json:"build_flags"`
	Build      buildMetadata `json:"build"`
}

type manifest struct {
	SchemaVersion int        `json:"schema_version"`
	Artifacts     []artifact `json:"artifacts"`
}

func archiveDigest(archive []byte) string {
	sum := sha256.Sum256(archive)
	return hex.EncodeToString(sum[:])
}

func readManifest(directory string) (manifest, error) {
	var (
		trailing any
		saved    manifest
		header   struct {
			SchemaVersion int `json:"schema_version"`
		}
	)

	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return saved, fmt.Errorf("read manifest: %w", err)
	}

	if err = json.Unmarshal(data, &header); err != nil {
		return saved, fmt.Errorf("decode manifest: %w", err)
	}

	if header.SchemaVersion != manifestSchema {
		return saved, fmt.Errorf("manifest schema: have %d, want %d", header.SchemaVersion, manifestSchema)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err = decoder.Decode(&saved); err != nil {
		return saved, fmt.Errorf("decode manifest: %w", err)
	}

	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return saved, ErrManifestDocument
	}

	return saved, nil
}

func checkAssets(directory string, plans []buildPlan) error {
	saved, err := readManifest(directory)
	if err != nil {
		return err
	}

	if len(saved.Artifacts) != len(plans) {
		return fmt.Errorf("archive count: have %d, want %d", len(saved.Artifacts), len(plans))
	}

	for i, plan := range plans {
		if err = checkArtifact(directory, saved.Artifacts[i], plan); err != nil {
			return fmt.Errorf("%s: %w", plan.target.archiveName(), err)
		}
	}

	return nil
}

func checkArtifact(directory string, saved artifact, plan buildPlan) error {
	if saved.Name != plan.target.archiveName() || saved.Arch != plan.target.goArch {
		return ErrManifestTarget
	}

	if !slices.Equal(saved.BuildFlags, plan.flags) {
		return ErrCompilerFlags
	}

	if err := plan.build.reusable(); err != nil {
		return err
	}

	if err := saved.Build.compare(plan.build); err != nil {
		return err
	}

	archive, err := os.ReadFile(filepath.Join(directory, saved.Name))
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}

	if archiveDigest(archive) != saved.SHA256 {
		return ErrArchiveChecksum
	}

	binary, err := unpackArchive(archive)
	if err != nil {
		return fmt.Errorf("decompress archive: %w", err)
	}

	actual, err := inspectBinary(binary, plan.target)
	if err != nil {
		return err
	}

	return actual.compare(saved.Build)
}

func publish(directory string, artifacts []builtArtifact) error {
	if err := filesystem.CheckDirectory(directory); err != nil {
		return err
	}

	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}

	saved := manifest{
		SchemaVersion: manifestSchema,
		Artifacts:     make([]artifact, 0, len(artifacts)),
	}

	for _, built := range artifacts {
		path := filepath.Join(directory, built.record.Name)
		if err := filesystem.WriteFileAtomic(path, built.archive, 0o644); err != nil {
			return fmt.Errorf("publish %s: %w", built.record.Name, err)
		}

		saved.Artifacts = append(saved.Artifacts, built.record)
	}

	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}

	if err = filesystem.WriteFileAtomic(filepath.Join(directory, "manifest.json"), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("publish manifest: %w", err)
	}

	return nil
}
