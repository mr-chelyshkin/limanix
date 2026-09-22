package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIdentityBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		valid   []string
		invalid []string
		check   func(string) error
	}{
		{"VMName", []string{"2-rust-box", "box", strings.Repeat("a", 63)}, []string{"", "../box", "Box", "-box", "box-", "box.local", strings.Repeat("a", 64)}, func(v string) error { _, err := NewVMName(v); return err }},
		{"Username", []string{"dev", "_dev", "rust-dev_2", strings.Repeat("a", 32)}, []string{"", "root", "limanix-admin", "2dev", "Dev", "dev:1000", "dev\x00", strings.Repeat("a", 33)}, func(v string) error { _, err := NewUsername(v); return err }},
		{"ModuleName", []string{"my-tools", "git", "rust2"}, []string{"", "2tools", "a--b", "../tools", strings.Repeat("a", 64)}, func(v string) error { _, err := NewModuleName(v); return err }},
		{"ModuleID", []string{"lmx:git", "work:git", "third-party:my-tools", "lmx:go-1.24", "lmx:my-tools-3.13"}, []string{"git", "./tools.nix", ":git", "Lmx:git", "third-party:../tools", "third-party:", "third-party:third-party:git", "lmx:go-1..24", "lmx:go-1.24rc1", "lmx:go-../escape"}, func(v string) error { _, err := NewModuleID(v); return err }},
		{"EnvName", []string{"TOKEN", "_TOKEN", "token2"}, []string{"", "1TOKEN", "TOKEN-NAME", "TOKEN\nNAME"}, func(v string) error { _, err := NewEnvName(v); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, value := range tt.valid {
				if err := tt.check(value); err != nil {
					t.Errorf("valid value %q: %v", value, err)
				}
			}
			for _, value := range tt.invalid {
				if err := tt.check(value); err == nil {
					t.Errorf("accepted invalid value %q", value)
				}
			}
		})
	}
	module, err := NewModuleID("third-party:my-tools")
	if err != nil || module.Selector() != "my-tools" || !module.IsThirdParty() {
		t.Fatalf("module reference: %v, %v", module, err)
	}
	for _, selector := range []string{"go-1.24", "nodejs-26", "docker-28", "my-tools-3.13", strings.Repeat("a", 63) + "-26"} {
		versioned, err := NewModuleID("lmx:" + selector)
		if err != nil || versioned.Selector() != selector {
			t.Fatalf("versioned reference: %v, %v", versioned, err)
		}
	}
}

func TestGuestPathSyntaxAndNormalization(t *testing.T) {
	for input, expected := range map[string]GuestPath{"/home/./dev//": "/home/dev", "/workspace//project/./": "/workspace/project", "/etc/service": "/etc/service"} {
		actual, err := NewGuestPath(input)
		if err != nil || actual != expected {
			t.Errorf("%q: got %q, %v", input, actual, err)
		}
	}
	for _, input := range []string{"", "/", "/./", "relative", "//home/dev", "/home/../dev", "/home/dev space", "/home/dev\tname", "/home/dev\nname", "/home/dev\rname", "/home/dev\x00name"} {
		if _, err := NewGuestPath(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestEnvironmentLiteralValuesAndErrors(t *testing.T) {
	for _, input := range []string{"", "Привет 🦀\n\t\r", "literal $HOME ' \""} {
		actual, err := NewEnvValue(input)
		if err != nil || string(actual) != input {
			t.Errorf("literal changed: %q, %v", actual, err)
		}
	}
	for _, suffix := range []string{"\x00", "\ufeff", "\ufdd0", "\ufdef", "\ufffe", "\U0001ffff", string([]byte{0xff})} {
		_, err := NewEnvValue("never-display-this-value" + suffix)
		if err == nil || strings.Contains(err.Error(), "never-display-this-value") {
			t.Errorf("unsafe error: %v", err)
		}
	}
}

func TestSizesAreNumericWithStablePublicEncoding(t *testing.T) {
	size, err := ParseByteSize("8GiB")
	if err != nil || int64(size) != 8*GiB {
		t.Fatalf("parsed size: %d, %v", size, err)
	}
	encoded, err := json.Marshal(size)
	if err != nil || string(encoded) != `"8GiB"` {
		t.Fatalf("encoded size: %s, %v", encoded, err)
	}
	var restored ByteSize
	if err := json.Unmarshal(encoded, &restored); err != nil || restored != size {
		t.Fatalf("round trip: %d, %v", restored, err)
	}
	for _, input := range []string{"0GiB", "1.5GiB", "8GB", "01GiB", "8589934592GiB"} {
		if _, err := ParseByteSize(input); err == nil {
			t.Errorf("accepted invalid size %q", input)
		}
	}
	for _, input := range []int64{0, -1} {
		if _, err := NewByteSize(input); err == nil {
			t.Errorf("accepted byte count %d", input)
		}
	}
	if _, err := ByteSize(1).GiB(); err == nil {
		t.Error("encoded fractional GiB")
	}
	for _, input := range []string{`8`, `true`, `null`, `"0GiB"`} {
		if err := json.Unmarshal([]byte(input), &restored); err == nil {
			t.Errorf("accepted JSON %s", input)
		}
	}
	for architecture, expected := range map[Architecture]string{ARM64: "aarch64", AMD64: "x86_64"} {
		actual, err := architecture.LimaArch()
		if err != nil || actual != expected {
			t.Errorf("architecture: %q, %v", actual, err)
		}
	}
}
