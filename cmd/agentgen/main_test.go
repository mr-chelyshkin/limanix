package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUsageAndHelpNeverBuildGuestAgents(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"help", []string{"--help"}, 0},
		{"unknown flag", []string{"--unknown"}, 2},
		{"missing root", []string{"--root"}, 2},
		{"invalid boolean", []string{"--check=invalid"}, 2},
		{"positional before check", []string{"typo", "--check"}, 2},
		{"positional after check", []string{"--check", "typo"}, 2},
		{"separator positional", []string{"--", "typo"}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			var diagnostics bytes.Buffer
			code := run(context.Background(), test.args, &diagnostics, func(context.Context, string, bool) error {
				t.Fatal("invalid invocation reached guest-agent generation")
				return nil
			})
			if code != test.code || diagnostics.Len() == 0 {
				t.Fatalf("exit = %d, diagnostics = %q", code, diagnostics.String())
			}
		})
	}
}

func TestRunPreservesOptionsAndExitStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, test := range []struct {
		name    string
		args    []string
		want    options
		failure error
	}{
		{name: "defaults", want: options{root: "."}},
		{name: "check", args: []string{"--check", "--root", "/project with spaces"}, want: options{root: "/project with spaces", check: true}},
		{name: "single dash", args: []string{"-root=project", "-check=true"}, want: options{root: "project", check: true}},
		{name: "explicit false", args: []string{"--check=false"}, want: options{root: "."}},
		{name: "generation failure", args: []string{"--root", "project", "--check"}, want: options{root: "project", check: true}, failure: errors.New("cannot build guest agent")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var diagnostics bytes.Buffer
			calls := 0
			code := run(ctx, test.args, &diagnostics, func(gotCtx context.Context, root string, check bool) error {
				calls++
				if gotCtx != ctx || root != test.want.root || check != test.want.check {
					t.Fatalf("generation arguments changed: root=%q, check=%v", root, check)
				}
				return test.failure
			})
			wantCode, wantDiagnostics := 0, ""
			if test.failure != nil {
				wantCode, wantDiagnostics = 1, "agentgen: cannot build guest agent\n"
			}
			if calls != 1 || code != wantCode || diagnostics.String() != wantDiagnostics {
				t.Fatalf("calls=%d, exit=%d, diagnostics=%q", calls, code, diagnostics.String())
			}
		})
	}
}

func TestCompressionHasDeterministicHeadersAndRoundTrips(t *testing.T) {
	binary := []byte("guest-agent build bytes")
	first, err := gzipBinary(binary)
	if err != nil {
		t.Fatal(err)
	}
	second, err := gzipBinary(binary)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("gzip output is nondeterministic")
	}
	reader, err := gzip.NewReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if !reader.ModTime.IsZero() || reader.Name != "" || reader.Comment != "" || reader.OS != 255 {
		t.Fatal("gzip has host or timestamp metadata")
	}
	actual, err := io.ReadAll(reader)
	if err := errors.Join(err, reader.Close()); err != nil || !bytes.Equal(actual, binary) {
		t.Fatalf("gzip round trip: %v", err)
	}
}

func TestModuleDigestChangesWithBothPinnedInputFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"go.mod", "go.sum"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := moduleDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("changed"), 0o600); err != nil {
			t.Fatal(err)
		}
		next, err := moduleDigest(root)
		if err != nil || next == first {
			t.Fatalf("digest ignored %s", name)
		}
		first = next
	}
}

func TestCheckMissingAssetsMakesNoFiles(t *testing.T) {
	root := t.TempDir()
	if err := checkAssets(root, manifest{}); err == nil {
		t.Fatal("missing assets accepted")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("asset check modified directory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := generate(ctx, root, false); err == nil {
		t.Fatal("canceled build succeeded")
	}
}

func TestCheckAssetsComparesIndividualBuildFlags(t *testing.T) {
	directory := t.TempDir()
	expected := manifest{
		SchemaVersion: 1,
		Package:       agentPackage,
		LimaVersion:   "v2.2.0",
		ModuleDigest:  "pinned-module-digest",
		GoVersion:     runtime.Version(),
		BuildFlags:    buildFlags("v2.2.0"),
	}
	for _, platform := range targets {
		header := make([]byte, 64)
		copy(header, "\x7fELF")
		header[elf.EI_CLASS], header[elf.EI_DATA], header[elf.EI_VERSION] = byte(elf.ELFCLASS64), byte(elf.ELFDATA2LSB), byte(elf.EV_CURRENT)
		binary.LittleEndian.PutUint16(header[16:], uint16(elf.ET_EXEC))
		binary.LittleEndian.PutUint16(header[18:], uint16(platform.machine))
		binary.LittleEndian.PutUint32(header[20:], uint32(elf.EV_CURRENT))
		binary.LittleEndian.PutUint16(header[52:], uint16(len(header)))
		archive, err := gzipBinary(header)
		if err != nil {
			t.Fatal(err)
		}
		name := "lima-guestagent.Linux-" + platform.limaArch + ".gz"
		if err := os.WriteFile(filepath.Join(directory, name), archive, 0o600); err != nil {
			t.Fatal(err)
		}
		checksum := sha256.Sum256(archive)
		expected.Artifacts = append(expected.Artifacts, artifact{Name: name, Arch: platform.goArch, SHA256: hex.EncodeToString(checksum[:])})
	}
	for _, joined := range []bool{false, true} {
		saved := expected
		if joined {
			saved.BuildFlags = []string{strings.Join(expected.BuildFlags, "\x00")}
		}
		encoded, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "manifest.json"), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := checkAssets(directory, expected); (err != nil) != joined {
			t.Fatalf("joined flags = %v: %v", joined, err)
		}
	}
}
