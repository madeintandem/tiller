//go:build tools

// This file is never compiled into the binary. It pins Phase-1 dependencies
// that no non-test source imports yet, so that `go mod tidy` retains them in
// go.mod and parallel later tasks do not have to touch the module file. Drop
// an entry here once a real import for it lands (yaml.v3 arrives in the store
// package in a later task).
package tools

import (
	_ "gopkg.in/yaml.v3"
)
