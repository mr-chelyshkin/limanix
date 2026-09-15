//go:build !darwin || !cgo

package lima

func nativeVZAvailable() bool { return false }
