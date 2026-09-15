// Command agentgen builds embedded Lima guest binaries from the pinned module.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

const agentPackage = "github.com/lima-vm/lima/v2/cmd/lima-guestagent"

const maxBinarySize = 128 << 20

type target struct {
	goArch   string
	limaArch string
	machine  elf.Machine
}

var targets = []target{
	{goArch: "arm64", limaArch: "aarch64", machine: elf.EM_AARCH64},
	{goArch: "amd64", limaArch: "x86_64", machine: elf.EM_X86_64},
}

type artifact struct {
	Name   string `json:"name"`
	Arch   string `json:"arch"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	SchemaVersion int        `json:"schema_version"`
	Package       string     `json:"package"`
	LimaVersion   string     `json:"lima_version"`
	ModuleDigest  string     `json:"module_digest"`
	GoVersion     string     `json:"go_version"`
	BuildFlags    []string   `json:"build_flags"`
	Artifacts     []artifact `json:"artifacts"`
}

type options struct {
	root  string
	check bool
}

func parseOptions(args []string, diagnostics io.Writer) (options, error) {
	var result options
	flags := flag.NewFlagSet("agentgen", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.StringVar(&result.root, "root", ".", "Repository root containing go.mod.")
	flags.BoolVar(&result.check, "check", false, "Verify generated assets without modifying them.")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		err := errors.New("unexpected positional arguments")
		_, _ = fmt.Fprintln(diagnostics, err)
		flags.Usage()
		return options{}, err
	}
	return result, nil
}

func run(ctx context.Context, args []string, diagnostics io.Writer, generateAssets func(context.Context, string, bool) error) int {
	opts, err := parseOptions(args, diagnostics)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	if err := generateAssets(ctx, opts.root, opts.check); err != nil {
		_, _ = fmt.Fprintln(diagnostics, "agentgen:", err)
		return 1
	}
	return 0
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stderr, generate)
	cancel()
	os.Exit(code)
}

func moduleDigest(root string) (string, error) {
	hash := sha256.New()
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte(name + "\x00"))
		_, _ = hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func limaVersion(ctx context.Context, root string) (string, error) {
	command := exec.CommandContext(ctx, "go", "list", "-m", "-json", "github.com/lima-vm/lima/v2")
	command.Dir = root
	data, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve pinned Lima version: %w", err)
	}
	var module struct {
		Version string
		Replace *json.RawMessage
	}
	if err := json.Unmarshal(data, &module); err != nil {
		return "", err
	}
	if module.Version == "" || module.Replace != nil {
		return "", errors.New("guest-agent generation requires an unreplaced, versioned Lima module")
	}
	return module.Version, nil
}

func buildFlags(version string) []string {
	return []string{"-mod=readonly", "-trimpath", "-ldflags=-s -w -X github.com/lima-vm/lima/v2/pkg/version.Version=" + version}
}

func generate(ctx context.Context, root string, check bool) (result error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	version, err := limaVersion(ctx, absolute)
	if err != nil {
		return err
	}
	digest, err := moduleDigest(absolute)
	if err != nil {
		return err
	}
	expected := manifest{
		SchemaVersion: 1,
		Package:       agentPackage,
		LimaVersion:   version,
		ModuleDigest:  digest,
		GoVersion:     runtime.Version(),
		BuildFlags:    buildFlags(version),
		Artifacts:     []artifact{},
	}
	directory := filepath.Join(absolute, "internal", "guestagent", "resources")
	if check {
		return checkAssets(directory, expected)
	}
	if err := checkAssets(directory, expected); err == nil {
		return nil
	}
	if err := filesystem.CheckDirectory(directory); err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	buildDir, err := os.MkdirTemp(directory, ".agentgen-")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(buildDir); err != nil {
			result = errors.Join(result, fmt.Errorf("remove guest-agent build directory: %w", err))
		}
	}()
	archives := map[string][]byte{}
	for _, platform := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		binaryPath := filepath.Join(buildDir, "lima-guestagent-"+platform.goArch)
		arguments := append(append([]string{"build"}, expected.BuildFlags...), "-o", binaryPath, agentPackage)
		command := exec.CommandContext(ctx, "go", arguments...)
		command.Dir = absolute
		command.Env = targetEnvironment(platform.goArch)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		fmt.Printf("Building Lima %s guest agent for linux/%s.\n", version, platform.goArch)
		if err := command.Run(); err != nil {
			return fmt.Errorf("build linux/%s guest agent: %w", platform.goArch, err)
		}
		data, err := os.ReadFile(binaryPath)
		if err != nil {
			return err
		}
		if err := validateBinary(data, platform.machine); err != nil {
			return err
		}
		archive, err := gzipBinary(data)
		if err != nil {
			return err
		}
		name := "lima-guestagent.Linux-" + platform.limaArch + ".gz"
		checksum := sha256.Sum256(archive)
		expected.Artifacts = append(expected.Artifacts, artifact{Name: name, Arch: platform.goArch, SHA256: hex.EncodeToString(checksum[:])})
		archives[name] = archive
	}
	for _, artifactInfo := range expected.Artifacts {
		if err := filesystem.WriteFileAtomic(filepath.Join(directory, artifactInfo.Name), archives[artifactInfo.Name], 0o644); err != nil {
			return err
		}
	}
	encoded, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		return err
	}
	if err := filesystem.WriteFileAtomic(filepath.Join(directory, "manifest.json"), append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println("Generated embedded Linux guest agents and integrity manifest.")
	return nil
}

func targetEnvironment(arch string) []string {
	result := []string{}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if name != "GOOS" && name != "GOARCH" && name != "CGO_ENABLED" && name != "GOAMD64" && name != "GOARM64" {
			result = append(result, value)
		}
	}
	return append(result, "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOAMD64=v1", "GOARM64=v8.0")
}

func gzipBinary(binary []byte) ([]byte, error) {
	var output bytes.Buffer
	writer, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	writer.OS = 255
	if _, err := writer.Write(binary); err != nil {
		return nil, errors.Join(err, writer.Close())
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func validateBinary(binary []byte, expected elf.Machine) error {
	file, err := elf.NewFile(bytes.NewReader(binary))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if file.Class != elf.ELFCLASS64 || file.Data != elf.ELFDATA2LSB || file.Machine != expected || file.Type != elf.ET_EXEC && file.Type != elf.ET_DYN {
		return errors.New("generated guest agent has an invalid executable architecture")
	}
	return nil
}

func checkAssets(directory string, expected manifest) error {
	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return fmt.Errorf("guest-agent assets are missing; run task assets/generate: %w", err)
	}
	var saved manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&saved); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("guest-agent manifest must contain exactly one JSON document")
	}
	if saved.SchemaVersion != expected.SchemaVersion ||
		saved.Package != expected.Package ||
		saved.LimaVersion != expected.LimaVersion ||
		saved.ModuleDigest != expected.ModuleDigest ||
		saved.GoVersion != expected.GoVersion ||
		!slices.Equal(saved.BuildFlags, expected.BuildFlags) ||
		len(saved.Artifacts) != len(targets) {
		return errors.New("guest-agent assets do not match the pinned source or compiler; run task assets/generate")
	}
	for index, platform := range targets {
		artifactInfo := saved.Artifacts[index]
		name := "lima-guestagent.Linux-" + platform.limaArch + ".gz"
		if artifactInfo.Name != name || artifactInfo.Arch != platform.goArch {
			return errors.New("guest-agent manifest has an invalid target")
		}
		archive, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		checksum := sha256.Sum256(archive)
		if hex.EncodeToString(checksum[:]) != artifactInfo.SHA256 {
			return errors.New("guest-agent archive checksum does not match its manifest")
		}
		reader, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return err
		}
		binary, readErr := io.ReadAll(io.LimitReader(reader, maxBinarySize+1))
		if err := errors.Join(readErr, reader.Close()); err != nil {
			return err
		}
		if len(binary) > maxBinarySize {
			return errors.New("guest-agent executable exceeds the supported size")
		}
		if err := validateBinary(binary, platform.machine); err != nil {
			return err
		}
	}
	fmt.Println("Embedded guest agents match the pinned source and compiler.")
	return nil
}
