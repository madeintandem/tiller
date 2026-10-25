package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin Task 8: the recorded soft order riding the read surfaces,
// `show` and `status`. order_cmd_test.go pins the write (`tiller order`
// itself); these pin the two reads it feeds.

// TestShowEpicHumanRendersRecordedFeatureOrder pins the epic body: once
// `tiller order` has recorded a feature sequence, `show epic` prints one
// order: line naming it by ref, in the recorded sequence, and the JSON
// envelope carries the same list as bare ids.
func TestShowEpicHumanRendersRecordedFeatureOrder(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/rendering-modes", "--title", "Rendering modes", "--contributor", "jc")
	mustRun(t, "order", "feature", "form-engine/rendering-modes", "form-engine/schema-engine", "--contributor", "jc")

	out, _, code := execCLI(t, "show", "epic", "form-engine")
	require.Equal(t, 0, code)
	require.Contains(t, out, "order: form-engine/rendering-modes (F-")
	require.Contains(t, out, "form-engine/schema-engine (F-")

	out, _, code = execCLI(t, "show", "epic", "form-engine", "--json")
	require.Equal(t, 0, code)
	require.Regexp(t, `"order": \[\s*"F-[a-z0-9]{6}",\s*"F-[a-z0-9]{6}"\s*\]`, out)
}

// TestShowFeatureHumanRendersRecordedTaskOrder pins the feature body one
// grain down: a recorded task sequence prints the same order: line.
func TestShowFeatureHumanRendersRecordedTaskOrder(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "task", "form-engine/schema-engine/first", "--title", "First", "--contributor", "jc")
	mustRun(t, "add", "task", "form-engine/schema-engine/second", "--title", "Second", "--contributor", "jc")
	mustRun(t, "order", "task", "form-engine/schema-engine/second", "form-engine/schema-engine/first", "--contributor", "jc")

	out, _, code := execCLI(t, "show", "feature", "form-engine/schema-engine")
	require.Equal(t, 0, code)
	require.Contains(t, out, "order: form-engine/schema-engine/second (T-")
	require.Contains(t, out, "form-engine/schema-engine/first (T-")
}

// TestShowHumanOmitsOrderWithoutOne pins the additive, omitempty guarantee at
// the human render: a node that never ran `tiller order` prints no order
// line at all, byte-identical to before this task.
func TestShowHumanOmitsOrderWithoutOne(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")

	out, _, code := execCLI(t, "show", "epic", "form-engine")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "order:")
}

// TestStatusHumanRendersRecordedRootOrder pins the bare `status` summary: a
// recorded root epic order prints one Order: line naming it by ref, and the
// JSON envelope carries the ids while the human refs stay off the wire (the
// same posture active_epics/Active epics already keep).
func TestStatusHumanRendersRecordedRootOrder(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	mustRun(t, "order", "epic", "payments", "form-engine", "--contributor", "jc")

	out, _, code := execCLI(t, "status")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Order: payments (E-")
	require.Contains(t, out, "form-engine (E-")

	out, _, code = execCLI(t, "status", "--json")
	require.Equal(t, 0, code)
	require.Regexp(t, `"order": \[\s*"E-[a-z0-9]{6}",\s*"E-[a-z0-9]{6}"\s*\]`, out)
	require.NotContains(t, out, `"order_refs"`, "the human refs stay CLI-only, like active_epic_refs")
}

// TestStatusHumanOmitsOrderWithoutOne pins the same additive guarantee at the
// project summary: a fresh project with no recorded root order prints no
// Order line at all.
func TestStatusHumanOmitsOrderWithoutOne(t *testing.T) {
	initOwnedRepo(t)

	out, _, code := execCLI(t, "status")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "Order:")
}
