package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin the dependency write path at the CLI surface (issue #21):
// --depends-on at add and tiller link after, with targets resolved
// all-or-nothing and confirmations naming the graph by the reference
// convention.

// TestAddFeatureDependsOn pins the at-add writer end to end: keys resolve to
// ids in the envelope, and the human line names the recorded graph.
func TestAddFeatureDependsOn(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")

	out, _, code := execCLI(t, "add", "feature", "form-engine/rendering-modes",
		"--title", "Rendering modes", "--depends-on", "form-engine/schema-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Created feature form-engine/rendering-modes (F-")
	require.Contains(t, out, "; depends on form-engine/schema-engine (F-")

	out, _, code = execCLI(t, "show", "feature", "form-engine/rendering-modes", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"depends_on"`)
	require.Regexp(t, `"depends_on": \[\s*"F-[a-z0-9]{6}"`, out, "the target is stored by id")
}

// TestAddEpicDependsOnEchoes pins the epic half at the surface: the envelope
// and the human line both carry the just-recorded graph, on the create and on
// the idempotent re-add.
func TestAddEpicDependsOnEchoes(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")

	out, _, code := execCLI(t, "add", "epic", "payments", "--title", "Payments",
		"--depends-on", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Created epic payments (E-")
	require.Contains(t, out, "; depends on form-engine (E-")

	out, _, code = execCLI(t, "add", "epic", "payments", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"depends_on"`, "the re-add envelope reports the recorded graph")
	require.Contains(t, out, `"created": false`)
}

// TestAddDependsOnRefusals pins the misuse surface: a dangling target is the
// usual not-found misuse with nothing written, and a ledger-item target
// refuses by naming the axis rule.
func TestAddDependsOnRefusals(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")

	out, _, code := execCLI(t, "add", "feature", "form-engine/held-wrong",
		"--title", "x", "--depends-on", "F-zzzzzz", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "not_found"`)

	stdout, stderr, code := execCLI(t, "add", "feature", "form-engine/held-wrong",
		"--title", "x", "--depends-on", "B-aaaaaa", "--contributor", "jc")
	require.NotEqual(t, 0, code)
	require.Contains(t, stdout+stderr, "a dependency names an epic or a feature")

	out, _, code = execCLI(t, "ls", "features")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "held-wrong", "nothing recorded on a refused add")
}

// TestLinkRecordsAndUnions pins the post-hoc writer: link records the edge as
// a real mutation, repeats are no-change successes, self-dependencies refuse,
// and an unresolvable node is the usual not-found misuse.
func TestLinkRecordsAndUnions(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/rendering-modes", "--title", "Rendering modes", "--contributor", "jc")

	out, _, code := execCLI(t, "link", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "feature form-engine/rendering-modes (F-")
	require.Contains(t, out, ") now depends on form-engine/schema-engine (F-")

	out, _, code = execCLI(t, "link", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "already depends on form-engine/schema-engine (F-")
	require.True(t, strings.Contains(out, "(no change)."), "a repeat edge is a no-change success")

	stdout, stderr, code := execCLI(t, "link", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/rendering-modes", "--contributor", "jc")
	require.NotEqual(t, 0, code)
	require.Contains(t, stdout+stderr, "itself")

	// The task noun is accepted; this address simply names no task in the repo,
	// so it is the same not-found misuse any bad address gets.
	stdout, stderr, code = execCLI(t, "link", "task", "T-aaaaaa", "--depends-on", "form-engine", "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stdout+stderr, "no task found", "the refusal is about the address, not the noun")

	_, _, code = execCLI(t, "link", "question", "Q-aaaaaa", "--depends-on", "form-engine", "--contributor", "jc")
	require.Equal(t, 2, code, "link applies to the three nouns that carry depends_on")
}

// laySiblingTasks lays one epic, one feature, and two tasks under it, the
// smallest repo a sibling edge can be recorded in.
func laySiblingTasks(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "task", "form-engine/schema-engine/first", "--title", "First", "--contributor", "jc")
	mustRun(t, "add", "task", "form-engine/schema-engine/second", "--title", "Second", "--contributor", "jc")
}

// TestLinkTaskRecordsASiblingEdge pins the task arm of link at the surface, the
// user-facing half of the sibling ordering: the edge is a real mutation
// visible to the next read, a repeat is the no-change success, a
// self-dependency refuses, and a task under another feature refuses as a
// non-sibling rather than being recorded as an edge no surface would honour.
func TestLinkTaskRecordsASiblingEdge(t *testing.T) {
	laySiblingTasks(t)
	mustRun(t, "add", "feature", "form-engine/other", "--title", "Other", "--contributor", "jc")
	mustRun(t, "add", "task", "form-engine/other/outsider", "--title", "Outsider", "--contributor", "jc")

	// The edge runs FIRST depends on SECOND, deliberately the direction that
	// fights the keys: "first" sorts ahead of "second", so the recorded order
	// and key order disagree and the ls assertion below can tell them apart.
	// The other direction reads more naturally and pins nothing, since both
	// orders agree on it.
	out, _, code := execCLI(t, "link", "task", "form-engine/schema-engine/first",
		"--depends-on", "form-engine/schema-engine/second", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "task form-engine/schema-engine/first (T-")
	require.Contains(t, out, ") now depends on form-engine/schema-engine/second (T-")

	out, _, code = execCLI(t, "show", "task", "form-engine/schema-engine/first", "--json")
	require.Equal(t, 0, code)
	require.Regexp(t, `"depends_on": \[\s*"T-[a-z0-9]{6}"`, out, "the sibling is stored by id")

	// The recorded edge is the rendered order: first follows the second it
	// depends on, though "first" sorts ahead of "second" by key.
	out, _, code = execCLI(t, "ls", "task", "--feature", "form-engine/schema-engine")
	require.Equal(t, 0, code)
	require.Less(t, strings.Index(out, "schema-engine/second"), strings.Index(out, "schema-engine/first"),
		"ls renders the recorded sequence, against the keys")

	out, _, code = execCLI(t, "link", "task", "form-engine/schema-engine/first",
		"--depends-on", "form-engine/schema-engine/second", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "(no change).")

	stdout, stderr, code := execCLI(t, "link", "task", "form-engine/schema-engine/first",
		"--depends-on", "form-engine/schema-engine/first", "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stdout+stderr, "itself")

	stdout, stderr, code = execCLI(t, "link", "task", "form-engine/schema-engine/first",
		"--depends-on", "form-engine/other/outsider", "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stdout+stderr, "not a sibling task")
}

// TestSiblingDependsOnNamesTheAddressFormAtTheSurface pins the refusal a human
// is most likely to hit: naming a sibling by its local segment alone. It reads
// as unambiguous inside one feature, and the sibling exists, so the message has
// to be about the address form and has to show the forms that work, not report
// a missing task.
func TestSiblingDependsOnNamesTheAddressFormAtTheSurface(t *testing.T) {
	laySiblingTasks(t)

	stdout, stderr, code := execCLI(t, "link", "task", "form-engine/schema-engine/second",
		"--depends-on", "first", "--contributor", "jc")
	out := stdout + stderr
	require.Equal(t, 2, code)
	require.Contains(t, out, "not a task address")
	require.Contains(t, out, "T-xxxxxx")
	require.Contains(t, out, "<epic-key>/<feature-key>/<task-key>")
	require.NotContains(t, out, "no task found", "the sibling named exists")

	// add task refuses the same way, before any folder is written.
	stdout, stderr, code = execCLI(t, "add", "task", "form-engine/schema-engine/third",
		"--title", "Third", "--depends-on", "first", "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stdout+stderr, "not a task address")

	out, _, code = execCLI(t, "ls", "task")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "schema-engine/third", "nothing written on a refused add")
}

// TestAddTaskDependsOnConfirmationShape pins the confirmation copy across the
// four shapes humanLine produces, because the dependency clause had to move
// behind the task title and a sentence must not lose its period on the way.
func TestAddTaskDependsOnConfirmationShape(t *testing.T) {
	laySiblingTasks(t)

	// A task with a title and no edges ends on the quoted title, unchanged.
	out, _, code := execCLI(t, "add", "task", "form-engine/schema-engine/third",
		"--title", "Third", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, `: "Third"`)
	require.NotContains(t, out, `"Third".`, "a title-terminated line takes no period")

	// A task with a title AND edges keeps the title clause, then the dependency
	// clause, then a period.
	out, _, code = execCLI(t, "add", "task", "form-engine/schema-engine/fourth",
		"--title", "Fourth", "--depends-on", "form-engine/schema-engine/first", "--contributor", "jc")
	require.Equal(t, 0, code)
	firstIdx := strings.Index(out, "; depends on form-engine/schema-engine/first (T-")
	require.Greater(t, firstIdx, strings.Index(out, `: "Fourth"`),
		"the title closes the identity clause; the dependencies follow it")
	require.Regexp(t, `depends on form-engine/schema-engine/first \(T-[a-z0-9]{6}\)\.`, out,
		"the sentence ends with a period")

	// An epic or a feature carrying edges still ends with a period, the shape
	// it had before the task clause existed.
	out, _, code = execCLI(t, "add", "epic", "payments", "--title", "Payments",
		"--depends-on", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Regexp(t, `depends on form-engine \(E-[a-z0-9]{6}\)\.`, out)

	out, _, code = execCLI(t, "add", "feature", "form-engine/rendering",
		"--title", "Rendering", "--depends-on", "form-engine/schema-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Regexp(t, `depends on form-engine/schema-engine \(F-[a-z0-9]{6}\)\.`, out)
}

// TestLinkDependsOnJSONCarriesTheKey pins link's own --json envelope, not just
// show's: a successful --depends-on call carries "depends_on" with the
// recorded targets. depends_on and surface both went omitempty on linkView so
// a call touching only one axis does not fabricate an empty or null entry for
// the other; this pins that the common, everyday depends-on-only call still
// prints the key with real values rather than the omitempty hiding it too.
func TestLinkDependsOnJSONCarriesTheKey(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/schema-engine", "--title", "Schema engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/rendering-modes", "--title", "Rendering modes", "--contributor", "jc")

	out, _, code := execCLI(t, "link", "feature", "form-engine/rendering-modes",
		"--depends-on", "form-engine/schema-engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Regexp(t, `"depends_on": \[\s*"F-[a-z0-9]{6}"`, out, "a successful depends-on link carries the key with values")
	require.NotContains(t, out, `"surface"`, "a depends-on-only call has nothing to say about surface")
}

// TestDependencyChangeLockedIsBlockedForLinkAndUnlinkWithoutAmendmentRemedy
// pins the shared refusal as exit 1 with data, current state, and the edge
// rule in detail, never an amendment or the illegal post-start return command.
func TestDependencyChangeLockedIsBlockedForLinkAndUnlinkWithoutAmendmentRemedy(t *testing.T) {
	for _, command := range []string{"link", "unlink"} {
		t.Run(command+" json", func(t *testing.T) {
			layUnlinkCLI(t)
			setCLIFeatureDependencyState(t, "active", "tiller/form-engine/rendering-modes")
			target := "payments"
			if command == "unlink" {
				target = "form-engine/schema-engine"
			}

			out, stderr, code := execCLI(t, command, "feature", "form-engine/rendering-modes",
				"--depends-on", target, "--contributor", "jc", "--json")
			require.Equal(t, 1, code, stderr)
			require.Contains(t, out, `"ok": false`)
			require.Contains(t, out, `"reason": "dependency_change_locked"`)
			require.Contains(t, out, `"status": "active"`)
			require.Contains(t, out, `"detail":`)
			require.NotContains(t, out, "tiller add amendment")
			require.NotContains(t, out, `"remedy"`)
			require.NotContains(t, out, `"error"`)
			require.NotContains(t, out, `"return_remedy"`)
		})

		t.Run(command+" human", func(t *testing.T) {
			layUnlinkCLI(t)
			setCLIFeatureDependencyState(t, "active", "tiller/form-engine/rendering-modes")
			target := "payments"
			if command == "unlink" {
				target = "form-engine/schema-engine"
			}

			stdout, stderr, code := execCLI(t, command, "feature", "form-engine/rendering-modes",
				"--depends-on", target, "--contributor", "jc")
			require.Equal(t, 1, code)
			require.Contains(t, stdout+stderr, "Blocked:")
			require.Contains(t, stdout+stderr, "active")
			require.Contains(t, stdout+stderr, "cannot change in active")
			require.NotContains(t, stdout+stderr, "tiller add amendment")
			require.NotContains(t, stdout+stderr, "tiller return")
			require.NotContains(t, stdout+stderr, "Before work starts")
		})
	}
}
