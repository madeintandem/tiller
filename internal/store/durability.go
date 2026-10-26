package store

import (
	"os"
	"strings"
	"testing"
)

// fsyncEnabled reports whether the store forces its writes to stable storage.
//
// A real binary always syncs: the temp file before the rename, the directory
// after it, and the mutation lock, so a crash mid-write leaves either the old
// file or the whole new one. Under `go test` the default flips to off, because
// a fixture that performs a hundred atomic writes paid a hundred fsyncs and the
// suite took twelve minutes on a busy disk; the write seams take an
// injected sync function, so the tests that cover the wiring still run it.
//
// TILLER_FSYNC=on or TILLER_FSYNC=off overrides the default in either
// direction: on, to run a test against the real durable path; off, for a
// harness that drives a real binary and wants the speed.
func fsyncEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TILLER_FSYNC"))) {
	case "on", "1", "true", "yes":
		return true
	case "off", "0", "false", "no":
		return false
	}
	return !testing.Testing()
}

// syncFile is f.Sync behind the durability switch.
func syncFile(f *os.File) error {
	if !fsyncEnabled() {
		return nil
	}
	return f.Sync()
}
