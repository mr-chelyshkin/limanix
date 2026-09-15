//go:build darwin && cgo

package lima

import _ "github.com/lima-vm/lima/v2/pkg/driver/vz"

func nativeVZAvailable() bool { return true }
