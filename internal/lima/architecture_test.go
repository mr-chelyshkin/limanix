package lima

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"golang.org/x/sys/unix"
)

func TestDarwinHostArchitectureMatrix(t *testing.T) {
	for _, test := range []struct {
		name    string
		process string
		value   uint32
		failure error
		host    domain.Architecture
		queried bool
		native  bool
	}{
		{name: "native Apple Silicon", process: "arm64", host: domain.ARM64, native: true},
		{name: "native Intel", process: "amd64", host: domain.AMD64, queried: true, native: true},
		{name: "Intel without translation sysctl", process: "amd64", failure: unix.ENOENT, host: domain.AMD64, queried: true, native: true},
		{name: "Rosetta on Apple Silicon", process: "amd64", value: 1, host: domain.ARM64, queried: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			queried := false
			host, err := darwinHostArchitecture(test.process, func() (uint32, error) {
				queried = true
				return test.value, test.failure
			})
			if err != nil || host != test.host || queried != test.queried {
				t.Fatalf("host = %q, queried = %v, error = %v", host, queried, err)
			}
			err = requireNativeArchitecture(domain.Architecture(test.process), host)
			if (err == nil) != test.native {
				t.Fatalf("native process check: %v", err)
			}
			if !test.native && !strings.Contains(err.Error(), "limanix-arm64") {
				t.Fatalf("translated process did not identify the required binary: %v", err)
			}
		})
	}
}

func TestDarwinHostArchitecturePropagatesQueryFailures(t *testing.T) {
	for _, failure := range []error{unix.EPERM, unix.EIO} {
		_, err := darwinHostArchitecture("amd64", func() (uint32, error) { return 0, failure })
		if !errors.Is(err, failure) {
			t.Fatalf("query error was replaced with an architecture guess: %v", err)
		}
	}
	if _, err := darwinHostArchitecture("amd64", func() (uint32, error) { return 2, nil }); err == nil {
		t.Fatal("unexpected translation value accepted")
	}
	if _, err := darwinHostArchitecture("unknown", func() (uint32, error) {
		t.Fatal("unsupported process architecture queried translation state")
		return 0, nil
	}); err == nil {
		t.Fatal("unsupported process architecture accepted")
	}
}

func TestHostArchitectureLiveProcess(t *testing.T) {
	host, err := HostArchitecture()
	if err != nil {
		t.Fatal(err)
	}
	process, err := domain.NewArchitecture(runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	nativeErr := RequireNativeArchitecture(host)
	if runtime.GOOS == "darwin" && process != host {
		if nativeErr == nil || !strings.Contains(nativeErr.Error(), "limanix-arm64") {
			t.Fatalf("translated process was not rejected clearly: %v", nativeErr)
		}
	} else if nativeErr != nil {
		t.Fatal(nativeErr)
	}
	t.Logf("process=%s hardware=%s native_check=%v", process, host, nativeErr)
}
