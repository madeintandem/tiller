package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin the tiller order command at the CLI surface
// (docs/spec/cli-surface.md): a thin shell over
// mutate.SetOrder, echoing refs per the reference convention exactly as
// link.go's own confirmation does. All-or-nothing resolution, replace
// semantics, and every refusal live in mutate; these tests pin the shell
// around it, not the rules it delegates.

// TestOrderCmdEpicGrainRecordsAndEchoes pins the root grain end to end: the
// human line names the recorded sequence by ref, the JSON envelope carries
// the orderView shape with no parent (the root has none), and a rerun with
// the identical list is a no-change success.
func TestOrderCmdEpicGrainRecordsAndEchoes(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")

	out, _, code := execCLI(t, "order", "epic", "payments", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "payments (E-")
	require.Contains(t, out, "form-engine (E-")

	out, _, code = execCLI(t, "order", "epic", "payments", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"noun": "epic"`)
	require.NotContains(t, out, `"parent"`, "epic grain has no parent to name")
	require.Contains(t, out, `"changed": false`, "the identical list is a no-change rerun")
	require.Regexp(t, `"order": \[\s*"E-[a-z0-9]{6}",\s*"E-[a-z0-9]{6}"\s*\]`, out)
}

// TestOrderCmdRerunIdenticalListHumanLineNoChange pins humanOrderLine's
// no-change-with-a-nonempty-list branch in human (non-JSON) mode: a rerun of
// the identical list renders "already: <refs> (no change)." rather than the
// "now:" wording a real change gets.
func TestOrderCmdRerunIdenticalListHumanLineNoChange(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	mustRun(t, "order", "epic", "payments", "form-engine", "--contributor", "jc")

	out, _, code := execCLI(t, "order", "epic", "payments", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "epic order already: payments (E-")
	require.Contains(t, out, "form-engine (E-")
	require.Contains(t, out, "(no change).")
	require.NotContains(t, out, " now: ", "a no-change rerun must not read like a real change")
}

// TestOrderCmdClearNonEmptyListHumanLine pins humanOrderLine's cleared branch
// in human (non-JSON) mode: a --clear that actually erases a recorded list
// renders "<noun> order under <parent ref> cleared." rather than the
// already-empty wording a no-op clear gets.
func TestOrderCmdClearNonEmptyListHumanLine(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/rendering-modes", "--title", "Rendering modes", "--contributor", "jc")
	mustRun(t, "order", "feature", "form-engine/rendering-modes", "form-engine/schema-engine", "--contributor", "jc")

	out, _, code := execCLI(t, "order", "feature", "--clear", "--epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "feature order under form-engine (E-")
	require.Contains(t, out, "cleared.")
	require.NotContains(t, out, "already empty", "a real clear must not read like the no-op case")
}

// TestOrderCmdFeatureGrainNamesParentAndJSON pins the feature grain: the
// human line names the owning epic by ref, and the JSON envelope's "parent"
// carries the epic's id.
func TestOrderCmdFeatureGrainNamesParentAndJSON(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/rendering-modes", "--title", "Rendering modes", "--contributor", "jc")

	out, _, code := execCLI(t, "order", "feature", "form-engine/rendering-modes",
		"form-engine/schema-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "form-engine (E-")
	require.Contains(t, out, "rendering-modes (F-")
	require.Contains(t, out, "schema-engine (F-")

	out, _, code = execCLI(t, "order", "feature", "form-engine/rendering-modes",
		"form-engine/schema-engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"noun": "feature"`)
	require.Regexp(t, `"parent": "E-[a-z0-9]{6}"`, out)
	require.Contains(t, out, `"changed": false`)
}

// TestOrderCmdTaskGrainRecordsAndClears pins the task grain: the human line
// names the owning feature by ref, and --clear --feature erases the parent's
// list back to an empty, real change.
func TestOrderCmdTaskGrainRecordsAndClears(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "task", "form-engine/schema-engine/first", "--title", "First", "--contributor", "jc")
	mustRun(t, "add", "task", "form-engine/schema-engine/second", "--title", "Second", "--contributor", "jc")

	out, _, code := execCLI(t, "order", "task", "form-engine/schema-engine/second",
		"form-engine/schema-engine/first", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "schema-engine (F-")
	require.Contains(t, out, "second (T-")
	require.Contains(t, out, "first (T-")

	out, _, code = execCLI(t, "order", "task", "--clear", "--feature", "form-engine/schema-engine",
		"--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"changed": true`)
	require.Regexp(t, `"order": \[\s*\]`, out, "a cleared list still carries the key, as an empty array")
}

// TestOrderCmdClearEmptyEpicListIsNoChangeSuccess pins the brief's explicit
// case: clearing a root list that was never written is a no-change success,
// not a refusal.
func TestOrderCmdClearEmptyEpicListIsNoChangeSuccess(t *testing.T) {
	initOwnedRepo(t)

	out, _, code := execCLI(t, "order", "epic", "--clear", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "no change")

	out, _, code = execCLI(t, "order", "epic", "--clear", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"changed": false`)
	require.Regexp(t, `"order": \[\s*\]`, out)
}

// TestOrderCmdFeatureClearMissingParentRefuses pins the guard --clear needs
// at feature and task grain: with no siblings to infer a parent from, the
// parent flag is required.
func TestOrderCmdFeatureClearMissingParentRefuses(t *testing.T) {
	initOwnedRepo(t)

	out, _, code := execCLI(t, "order", "feature", "--clear", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "needs --epic to name whose list to clear")
}

// TestOrderCmdQuestionNounRefuses pins the noun gate at the CLI surface, the
// same way link.go restricts its own noun set: order applies to the three
// grains that carry a soft order, and a question is not one of them.
func TestOrderCmdQuestionNounRefuses(t *testing.T) {
	initOwnedRepo(t)

	out, _, code := execCLI(t, "order", "question", "Q-aaaaaa", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "unknown_noun"`)
}

// TestOrderCmdClearWithNodesRefusesAsErrorEnvelope pins the whole-branch
// review's finding: `tiller order feature some-feature --clear` used to
// record a one-element order instead of refusing, because SetOrder only
// examined Clear when no nodes were given. Naming a node and --clear
// together must refuse with the standard exit-2 envelope, the same as every
// other mutate-level refusal this command surfaces.
func TestOrderCmdClearWithNodesRefusesAsErrorEnvelope(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")

	out, _, code := execCLI(t, "order", "feature", "form-engine/schema-engine", "--clear", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"ok": false`)
	require.Contains(t, out, "order takes a node list or --clear, never both")
}

// TestOrderCmdMutateRefusalsSurfaceAsErrorEnvelope pins that every mutate-level
// refusal (a duplicate entry, an empty list without --clear, nodes split
// across two parents, an unresolvable ref) reaches the caller as the standard
// exit-2 error envelope, with mutate's own message intact.
func TestOrderCmdMutateRefusalsSurfaceAsErrorEnvelope(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	mustRun(t, "add", "feature", "payments/checkout", "--title", "Checkout", "--contributor", "jc")

	out, _, code := execCLI(t, "order", "epic", "form-engine", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"ok": false`)
	require.Contains(t, out, "is listed twice in the order")

	out, _, code = execCLI(t, "order", "epic", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "order needs at least one node, or --clear")

	out, _, code = execCLI(t, "order", "feature", "form-engine/schema-engine", "payments/checkout",
		"--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "must share one parent")

	out, _, code = execCLI(t, "order", "epic", "form-engine", "E-zzzzzz", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "not_found"`)
}

// TestOrderBugCommand pins the shelf grain at the CLI surface: the human
// line echoes the recorded sequence by ref, the recorded order steers `ls
// bugs`, and a shelf --clear needs no parent flag.
func TestOrderBugCommand(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "bug", "alpha", "--title", "A", "--contributor", "jc")
	mustRun(t, "add", "bug", "bravo", "--title", "B", "--contributor", "jc")
	out, stderr, code := execCLI(t, "order", "bug", "bravo", "alpha", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "bug order now: bugs/bravo (B-")
	out, stderr, code = execCLI(t, "ls", "bugs")
	require.Equal(t, 0, code, stderr)
	require.Less(t, strings.Index(out, "bugs/bravo"), strings.Index(out, "bugs/alpha"))
	_, stderr, code = execCLI(t, "order", "chore", "--clear", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
}
