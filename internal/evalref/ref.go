// Package evalref classifies recorded acceptance references without executing
// bindings, inspecting their targets, or interpreting shell commands.
package evalref

import "strings"

// IsPathShaped is the schema's conservative test-reference rule: only a forward
// slash distinguishes a path-shaped declaration from a bare test name or ID.
// It does not assert that the ref is one literal file or that anything exists.
func IsPathShaped(ref string) bool {
	return ref != "" && strings.Contains(ref, "/")
}
