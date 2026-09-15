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

// artifact pairs a gzip checksum with the build metadata read from its executable.
type artifact struct {
	Name       string        `json:"name"`
	Arch       string        `json:"arch"`
	SHA256     string        `json:"sha256"`
	BuildFlags []string      `json:"build_flags"`
	Build      buildMetadata `json:"build"`
}

// manifest describes the published archives. It is written after both archives.
type manifest struct {
	SchemaVersion int        `json:"schema_version"`
	Artifacts     []artifact `json:"artifacts"`
}

func archiveDigest(archive []byte) string {
	sum := sha256.Sum256(archive)
	return hex.EncodeToString(sum[:])
}

func readManifest(directory string) (manifest, error) {
	var saved manifest
	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return saved, fmt.Errorf("read manifest: %w", err)
	}
	// Check the schema first to report unsupported formats explicitly.
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return saved, fmt.Errorf("decode manifest: %w", err)
	}
	if header.SchemaVersion != manifestSchema {
		return saved, fmt.Errorf("manifest schema: have %d, want %d", header.SchemaVersion, manifestSchema)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&saved); err != nil {
		return saved, fmt.Errorf("decode manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return saved, errors.New("manifest must contain exactly one JSON document")
	}
	return saved, nil
}

// checkAssets compares the plan, manifest, archive checksum, and actual ELF metadata.
func checkAssets(directory string, plans []buildPlan) error {
	saved, err := readManifest(directory)
	if err != nil {
		return err
	}
	if len(saved.Artifacts) != len(plans) {
		return fmt.Errorf("archive count: have %d, want %d", len(saved.Artifacts), len(plans))
	}
	for i, plan := range plans {
		if err := checkArtifact(directory, saved.Artifacts[i], plan); err != nil {
			return fmt.Errorf("%s: %w", plan.target.archiveName(), err)
		}
	}
	return nil
}

func checkArtifact(directory string, saved artifact, plan buildPlan) error {
	if saved.Name != plan.target.archiveName() || saved.Arch != plan.target.goArch {
		return errors.New("manifest target does not match the requested architecture")
	}
	if !slices.Equal(saved.BuildFlags, plan.flags) {
		return errors.New("compiler flags changed")
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
		return errors.New("archive checksum does not match the manifest")
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

// publish replaces each completed archive atomically, then commits the manifest.
// A failed or interrupted publication is detected by the next checksum check.
func publish(directory string, artifacts []builtArtifact) error {
	if err := filesystem.CheckDirectory(directory); err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	saved := manifest{SchemaVersion: manifestSchema, Artifacts: make([]artifact, 0, len(artifacts))}
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
	if err := filesystem.WriteFileAtomic(filepath.Join(directory, "manifest.json"), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("publish manifest: %w", err)
	}
	return nil
}
