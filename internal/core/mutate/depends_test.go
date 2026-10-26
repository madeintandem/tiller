package mutate_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the dependency write path: depends_on was
// modeled and rendered but nothing could record it, so decompose buried the
// graph in epic prose. --depends-on at add and tiller link after are the two
// writers; targets resolve all-or-nothing to stored node ids.

// dependsRepo lays an owned repo with one epic and two features, the graph's
// raw material.
func dependsRepo(t *testing.T) string {
	t.Helper()
	root := writeOwnedRepo(t)
	tree := load(t, root)
	_, err := mutate.AddEpic(tree, testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)

	mint := func(suffix string) mutate.Deps {
		d := testDeps()
		d.Mint = func(prefix string) string { return prefix + "-" + suffix }
		return d
	}
	tree = load(t, root)
	_, err = mutate.AddFeature(tree, mint("bbbbbb"), mutate.FeatureInput{Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create"})
	require.NoError(t, err)
	tree = load(t, root)
	_, err = mutate.AddFeature(tree, mint("cccccc"), mutate.FeatureInput{Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	return root
}

// TestAddFeatureRecordsDependsOn pins the at-add writer: keys resolve to
// stored ids and the graph lands in feature.yml, visible to the next load.
func TestAddFeatureRecordsDependsOn(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	tree := load(t, root)

	d := testDeps()
	d.Mint = func(prefix string) string { return prefix + "-dddddd" }
	res, err := mutate.AddFeature(tree, d, mutate.FeatureInput{
		Key: "form-engine/rendering-modes", Title: "Rendering modes", ChangeType: "create",
		DependsOn: []string{"form-engine/schema-engine", "F-cccccc"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"F-bbbbbb", "F-cccccc"}, res.DependsOn, "keys resolve to stored ids")

	tree = load(t, root)
	require.Equal(t, []string{"F-bbbbbb", "F-cccccc"}, tree.Features["F-dddddd"].DependsOn)
}

// TestAddEpicRecordsAndEchoesDependsOn pins the epic half of the at-add
// writer: the graph is stored AND carried on the result, so the envelope and
// human line report what was just recorded (the create arm dropped it once).
func TestAddEpicRecordsAndEchoesDependsOn(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	tree := load(t, root)

	d := testDeps()
	d.Mint = func(prefix string) string { return prefix + "-ffffff" }
	res, err := mutate.AddEpic(tree, d, mutate.EpicInput{
		Key: "payments", Title: "Payments", DependsOn: []string{"form-engine"},
	})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, []string{"E-aaaaaa"}, res.DependsOn, "the created arm echoes the recorded graph")

	tree = load(t, root)
	require.Equal(t, []string{"E-aaaaaa"}, tree.Epics["E-ffffff"].DependsOn)

	// The idempotent re-add reports the recorded graph too.
	again, err := mutate.AddEpic(tree, testDeps(), mutate.EpicInput{Key: "payments"})
	require.NoError(t, err)
	require.False(t, again.Created)
	require.Equal(t, []string{"E-aaaaaa"}, again.DependsOn, "the no-op arm reports what is on disk")
}

// TestDependsOnDeduplicates pins the set semantics at the resolver: the same
// node named by key and by id (or a repeated flag) records one edge.
func TestDependsOnDeduplicates(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	tree := load(t, root)

	d := testDeps()
	d.Mint = func(prefix string) string { return prefix + "-dddddd" }
	res, err := mutate.AddFeature(tree, d, mutate.FeatureInput{
		Key: "form-engine/rendering-modes", Title: "Rendering modes", ChangeType: "create",
		DependsOn: []string{"form-engine/schema-engine", "F-bbbbbb", "F-bbbbbb"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"F-bbbbbb"}, res.DependsOn, "one edge, however many spellings")

	tree = load(t, root)
	require.Equal(t, []string{"F-bbbbbb"}, tree.Features["F-dddddd"].DependsOn)
}

// TestLinkRefusesAKeylessFeature pins the guard for a hand-damaged
// feature.yml: a feature loaded without its key line refuses with a plain
// error instead of panicking on the path join.
func TestLinkRefusesAKeylessFeature(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)

	// Strip the key line the way a bad hand edit or merge would.
	ymlPath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
	data, err := os.ReadFile(ymlPath)
	require.NoError(t, err)
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "key:") {
			kept = append(kept, line)
		}
	}
	require.NoError(t, os.WriteFile(ymlPath, []byte(strings.Join(kept, "\n")), 0o644))

	tree := load(t, root)
	_, err = mutate.Link(tree, testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "F-cccccc", DependsOn: []string{"F-bbbbbb"},
	})
	require.ErrorContains(t, err, "has no key", "a damaged yml refuses plainly rather than panicking")
}

// TestDependsOnRefusesNonDefinitionTargets pins the class rule: a dependency
// names an epic or a feature; ledger items refuse plainly, and a dangling
// address refuses with nothing written.
func TestDependsOnRefusesNonDefinitionTargets(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)

	for _, target := range []string{"T-aaaaaa", "Q-aaaaaa"} {
		tree := load(t, root)
		_, err := mutate.AddFeature(tree, testDeps(), mutate.FeatureInput{
			Key: "form-engine/held-wrong", Title: "x", ChangeType: "create",
			DependsOn: []string{target},
		})
		require.ErrorContains(t, err, "an epic or a feature", "target %s", target)
	}

	tree := load(t, root)
	_, err := mutate.AddFeature(tree, testDeps(), mutate.FeatureInput{
		Key: "form-engine/held-wrong", Title: "x", ChangeType: "create",
		DependsOn: []string{"F-zzzzzz"},
	})
	require.Error(t, err)
	tree = load(t, root)
	require.Len(t, tree.Features, 2, "nothing written on a refused add")
}

// TestLinkUnionsDependencies pins the post-hoc writer: link unions new targets
// into the node's depends_on as a real mutation, no-ops on a repeat, and
// refuses a self-dependency.
func TestLinkUnionsDependencies(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)

	tree := load(t, root)
	res, err := mutate.Link(tree, testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "form-engine/certification-form-loading",
		DependsOn: []string{"form-engine/schema-engine"},
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, []string{"F-bbbbbb"}, res.DependsOn)

	tree = load(t, root)
	require.Equal(t, []string{"F-bbbbbb"}, tree.Features["F-cccccc"].DependsOn, "the union persisted")

	// The same link again is a no-change success.
	again, err := mutate.Link(tree, testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "F-cccccc", DependsOn: []string{"F-bbbbbb"},
	})
	require.NoError(t, err)
	require.False(t, again.Changed)

	// A self-dependency refuses.
	_, err = mutate.Link(tree, testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "F-cccccc", DependsOn: []string{"F-cccccc"},
	})
	require.ErrorContains(t, err, "itself")
}

// TestLinkRecordsTheEdgesItAdded pins the link event's shape as the mirror
// of unlink's: depends_on before and after, plus the edges this one call
// added. The correction readout (`tiller readout <seam> --since <sha>`)
// renders "linked N edges" from that added list, so a link that unions two
// new targets onto one existing edge must record two, not three.
func TestLinkRecordsTheEdgesItAdded(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "form-engine/certification-form-loading",
		DependsOn: []string{"form-engine/schema-engine"},
	})
	require.NoError(t, err)
	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "form-engine/certification-form-loading",
		DependsOn: []string{"form-engine/schema-engine", "form-engine"},
	})
	require.NoError(t, err)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	linked := eventsOfType(evs, "link.feature")
	require.Len(t, linked, 2)
	// testDeps' fixed clock stamps both events at one instant, so the stream's
	// order is by id rather than by call; find the second call by its before.
	var second *events.Event
	for i := range linked {
		if len(linked[i].Before["depends_on"].([]any)) == 1 {
			second = &linked[i]
		}
	}
	require.NotNil(t, second, "the second link records the first link's edge as its before")
	require.Equal(t, []any{"F-bbbbbb"}, second.Before["depends_on"])
	require.Equal(t, []any{"F-bbbbbb", "E-aaaaaa"}, second.After["depends_on"])
	require.Equal(t, []any{"E-aaaaaa"}, second.After["added"], "only the edge this call added, not the whole union")
}

// siblingTaskRepo builds on dependsRepo by laying two tasks under
// schema-engine (T-first, T-second) and one under certification-form-loading
// (T-other), the cross-feature target a sibling edge must refuse.
func siblingTaskRepo(t *testing.T) string {
	t.Helper()
	root := dependsRepo(t)
	mint := func(suffix string) mutate.Deps {
		d := testDeps()
		d.Mint = func(prefix string) string { return prefix + "-" + suffix }
		return d
	}
	for _, tk := range []struct{ key, suffix string }{
		{"form-engine/schema-engine/first", "first0"},
		{"form-engine/schema-engine/second", "second"},
		{"form-engine/certification-form-loading/other", "other0"},
	} {
		_, err := mutate.AddTask(load(t, root), mint(tk.suffix), mutate.TaskInput{Key: tk.key, Title: tk.key})
		require.NoError(t, err)
	}
	return root
}

// TestAddTaskRecordsSiblingDependsOn pins the at-add writer at task grain: a
// sibling named by id and one named by its canonical key both resolve to
// stored ids, and the graph lands in task.yml where the next load reads it.
func TestAddTaskRecordsSiblingDependsOn(t *testing.T) {
	t.Parallel()
	root := siblingTaskRepo(t)

	d := testDeps()
	d.Mint = func(prefix string) string { return prefix + "-third0" }
	res, err := mutate.AddTask(load(t, root), d, mutate.TaskInput{
		Key: "form-engine/schema-engine/third", Title: "Third",
		DependsOn: []string{"T-first0", "form-engine/schema-engine/second"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"T-first0", "T-second"}, res.DependsOn, "keys resolve to stored ids")

	tree := load(t, root)
	require.Equal(t, []string{"T-first0", "T-second"}, tree.Tasks["T-third0"].DependsOn)
}

// TestAddTaskRefusesNonSiblingDependsOn pins the scope rule: a task depends on
// its siblings and nothing else (artifact-schema.md, Tasks), so a task under
// another feature, an epic, and a feature each refuse, and resolution is
// all-or-nothing, so a list mixing a good sibling with a bad target writes
// nothing at all.
func TestAddTaskRefusesNonSiblingDependsOn(t *testing.T) {
	t.Parallel()
	root := siblingTaskRepo(t)

	for _, target := range []string{"T-other0", "form-engine/certification-form-loading/other"} {
		_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
			Key: "form-engine/schema-engine/held-wrong", Title: "x", DependsOn: []string{target},
		})
		require.ErrorContains(t, err, "sibling", "target %s is under another feature", target)
	}
	for _, target := range []string{"E-aaaaaa", "F-bbbbbb", "form-engine", "form-engine/schema-engine"} {
		_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
			Key: "form-engine/schema-engine/held-wrong", Title: "x", DependsOn: []string{target},
		})
		require.Error(t, err, "target %s is not a task at all", target)
	}
	// All-or-nothing: the good sibling in front of the bad target buys nothing.
	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/schema-engine/held-wrong", Title: "x",
		DependsOn: []string{"T-first0", "T-other0"},
	})
	require.Error(t, err)

	tree := load(t, root)
	require.Len(t, tree.Tasks, 3, "nothing written on a refused add")
}

// TestLinkTasksToSiblings pins the post-hoc writer at task grain: link unions
// a sibling into a task's depends_on, no-ops on a repeat, refuses a
// self-dependency, and refuses a task under another feature.
func TestLinkTasksToSiblings(t *testing.T) {
	t.Parallel()
	root := siblingTaskRepo(t)

	res, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: "form-engine/schema-engine/second",
		DependsOn: []string{"form-engine/schema-engine/first"},
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, []string{"T-first0"}, res.DependsOn)

	tree := load(t, root)
	require.Equal(t, []string{"T-first0"}, tree.Tasks["T-second"].DependsOn, "the union persisted")

	again, err := mutate.Link(tree, testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: "T-second", DependsOn: []string{"T-first0"},
	})
	require.NoError(t, err)
	require.False(t, again.Changed)

	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: "T-second", DependsOn: []string{"T-second"},
	})
	require.ErrorContains(t, err, "itself")

	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: "T-second", DependsOn: []string{"T-other0"},
	})
	require.ErrorContains(t, err, "sibling")
}

// exitCodeOf reads the exit code an error carries, the classifier the CLI keys
// off, so a test can assert misuse (2) rather than just "some error".
func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	var coded interface{ ExitCode() int }
	require.ErrorAs(t, err, &coded)
	return coded.ExitCode()
}

// TestReAddNeverDiscardsDependsOn pins the re-add arm of all three add verbs
// against silently dropping the targets it was handed. A re-add is a no-op
// success, so the edge is never written here; but naming an edge the node does
// not carry refuses as misuse and points at link, and naming one it already
// carries stays the plain no-op success that makes re-running a command safe.
// An unresolvable target refuses either way, which is the case a re-add used to
// accept while a fresh add exited 2 on it.
func TestReAddNeverDiscardsDependsOn(t *testing.T) {
	t.Parallel()
	root := siblingTaskRepo(t)

	// Give each grain one recorded edge to re-state.
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: "F-cccccc", DependsOn: []string{"F-bbbbbb"}})
	require.NoError(t, err)
	_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: "T-second", DependsOn: []string{"T-first0"}})
	require.NoError(t, err)

	reAdd := func(t *testing.T, deps []string) (mutate.Result, error) {
		t.Helper()
		return mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{
			Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create",
			DependsOn: deps})
	}

	// Already recorded: the no-op success, reporting the stored graph.
	res, err := reAdd(t, []string{"F-bbbbbb"})
	require.NoError(t, err)
	require.False(t, res.Created)
	require.Equal(t, []string{"F-bbbbbb"}, res.DependsOn)

	// Not recorded: refused as misuse, naming link. The target is named in the
	// prose by the reference convention every other human surface uses, not as
	// the bare id the caller never typed, while the command that follows
	// carries the bare address, which is what can actually be run.
	_, err = reAdd(t, []string{"form-engine"})
	require.ErrorContains(t, err, "already exists and does not depend on")
	require.Regexp(t, `does not depend on form-engine \(E-[a-z0-9]{6}\)`, err.Error(),
		"the prose names the target as <key> (<id>)")
	require.ErrorContains(t, err, "tiller link feature form-engine/certification-form-loading --depends-on form-engine",
		"the suggested command is runnable, so it carries the address alone")
	require.Equal(t, 2, exitCodeOf(t, err), "a re-add naming an unrecorded edge is misuse")

	// Unresolvable: refused before the existence arm is even reached.
	_, err = reAdd(t, []string{"no-such-epic"})
	require.Error(t, err)

	// The task grain behaves identically.
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/schema-engine/second", Title: "Second",
		DependsOn: []string{"T-first0"}})
	require.NoError(t, err, "a re-add restating the recorded sibling edge is the no-op success")

	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/schema-engine/second", Title: "Second",
		DependsOn: []string{"form-engine/schema-engine/first", "T-other0"}})
	require.ErrorContains(t, err, "sibling", "an unresolvable target still refuses on a re-add")

	// And the epic grain.
	_, err = mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{
		Key: "form-engine", DependsOn: []string{"F-bbbbbb"}})
	require.ErrorContains(t, err, "already exists and does not depend on")

	// Nothing was written by any refusal.
	tree := load(t, root)
	require.Empty(t, tree.Epics["E-aaaaaa"].DependsOn)
	require.Equal(t, []string{"F-bbbbbb"}, tree.Features["F-cccccc"].DependsOn)
	require.Equal(t, []string{"T-first0"}, tree.Tasks["T-second"].DependsOn)
}

// TestSiblingDependsOnRefusesABareLocalKeyByName pins the message, not just the
// refusal. A sibling named by its local segment alone is a near miss: it reads
// as unambiguous inside one feature, and the sibling it names exists, so
// reporting that no such task was found blames the wrong thing. The address
// form is the mistake, and the refusal has to say so and show what is accepted.
func TestSiblingDependsOnRefusesABareLocalKeyByName(t *testing.T) {
	t.Parallel()
	root := siblingTaskRepo(t)

	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/schema-engine/third", Title: "Third", DependsOn: []string{"first"}})

	require.Error(t, err)
	require.NotContains(t, err.Error(), "no task found",
		"the sibling named exists; the address form is what is not accepted")
	require.ErrorContains(t, err, "not a task address")
	require.ErrorContains(t, err, "T-xxxxxx", "the refusal shows the id form")
	require.ErrorContains(t, err, "<epic-key>/<feature-key>/<task-key>", "and the full-key form")

	// The two accepted forms both resolve the very same sibling. Each probe
	// gets its own mint suffix: both land in the same tree, and the mint now
	// retries past a collision with what is already there (this task), so two
	// probes sharing testDeps()'s fixed "-aaaaaa" would make the second exhaust
	// against the first's own id rather than exercise the depends_on forms.
	for i, form := range []string{"T-first0", "form-engine/schema-engine/first"} {
		d := testDeps()
		d.Mint = func(prefix string) string { return prefix + "-probe" + strconv.Itoa(i) }
		res, aerr := mutate.AddTask(load(t, root), d, mutate.TaskInput{
			Key: "form-engine/schema-engine/probe-" + form[:2], Title: "Probe", DependsOn: []string{form}})
		require.NoError(t, aerr, "form %s", form)
		require.Equal(t, []string{"T-first0"}, res.DependsOn, "form %s", form)
	}
}

// TestLinkRefusesASiblingTargetWhenTheParentIsUnnamed pins the scope's two-fact
// split. A task whose parent feature the tree could not name has no siblings
// anyone can address, so every target refuses. What must NOT happen is the fall
// through to the epic-and-feature arm, where a sibling's own three-segment key
// would be read as a feature key and refused as a missing FEATURE, which
// describes neither the node nor the problem.
func TestLinkRefusesASiblingTargetWhenTheParentIsUnnamed(t *testing.T) {
	t.Parallel()
	root := siblingTaskRepo(t)
	tree := load(t, root)
	tree.Tasks["T-second"].FeatureID = ""

	_, err := mutate.Link(tree, testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: "T-second",
		DependsOn: []string{"form-engine/schema-engine/first"}})

	require.ErrorContains(t, err, "not a sibling task")
	require.NotContains(t, err.Error(), "no feature found",
		"an unparented task must not be read as an epic-or-feature recorder")
}

// TestLinkEpics pins the epic grain: epics link to epics the same way.
func TestLinkEpics(t *testing.T) {
	t.Parallel()
	root := dependsRepo(t)
	tree := load(t, root)
	d := testDeps()
	d.Mint = func(prefix string) string { return prefix + "-eeeeee" }
	_, err := mutate.AddEpic(tree, d, mutate.EpicInput{Key: "payments", Title: "Payments"})
	require.NoError(t, err)

	tree = load(t, root)
	res, err := mutate.Link(tree, testDeps(), mutate.LinkInput{
		Noun: store.NounEpic, Node: "payments", DependsOn: []string{"form-engine"},
	})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, []string{"E-aaaaaa"}, res.DependsOn)

	tree = load(t, root)
	require.Equal(t, []string{"E-aaaaaa"}, tree.Epics["E-eeeeee"].DependsOn)
}

// TestDependencyChangeStateMatrix pins the shared Link/Unlink safety boundary.
// The tree passed to each call is deliberately loaded before the status edit:
// both verbs must judge fresh state.yml bytes, not the resolver's cached node.
func TestDependencyChangeStateMatrix(t *testing.T) {
	t.Parallel()
	matrix := []struct {
		noun     string
		statuses []struct {
			status string
			open   bool
		}
	}{
		{store.NounTask, []struct {
			status string
			open   bool
		}{
			{"proposed", true}, {"ready", true}, {"in_progress", false}, {"check", false},
			{"in_review", false}, {"done", false}, {"cancelled", false},
		}},
		{store.NounFeature, []struct {
			status string
			open   bool
		}{
			{"draft", true}, {"specced", true}, {"ready", true}, {"active", false},
			{"done", false}, {"cancelled", false},
		}},
		{store.NounEpic, []struct {
			status string
			open   bool
		}{
			{"draft", true}, {"ready", true}, {"done", false}, {"cancelled", false}, {"archived", false},
		}},
	}
	operations := []struct {
		name    string
		changed bool
	}{
		{"link changed", true},
		{"unlink changed", true},
		{"link existing no-op", false},
		{"unlink absent no-op", false},
	}

	for _, grain := range matrix {
		for _, state := range grain.statuses {
			for _, operation := range operations {
				t.Run(grain.noun+"/"+state.status+"/"+operation.name, func(t *testing.T) {
					root, node, existing, absent := dependencyChangeFixture(t, grain.noun)
					stale := load(t, root)
					setDependencyNodeState(t, root, grain.noun, node, state.status, "")
					before := snapshotDir(t, root)

					changed, err := callDependencyChange(stale, operation.name, grain.noun, node, existing, absent)
					if !operation.changed {
						require.NoError(t, err)
						require.False(t, changed)
						requireNoWrites(t, before, snapshotDir(t, root))
						return
					}
					if state.open {
						require.NoError(t, err)
						require.True(t, changed)
						return
					}
					var locked *mutate.DependencyChangeLockedError
					require.ErrorAs(t, err, &locked)
					require.Equal(t, mutate.ReasonDependencyChangeLocked, locked.Reason)
					require.Equal(t, grain.noun, locked.Noun)
					require.Equal(t, state.status, locked.Status)
					requireNoWrites(t, before, snapshotDir(t, root))
				})
			}
		}
	}
}

func TestDependencyChangeRejectsFreshIdentityReplacementAtEveryGrain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		noun   string
		change func(*testing.T, string, store.Node)
	}{
		{"epic id", store.NounEpic, func(t *testing.T, root string, node store.Node) {
			doc := node.(*store.Epic)
			path := filepath.Join(root, ".tiller", "epics", *doc.Key, "epic.yml")
			var fresh store.Epic
			readDependencyYAML(t, path, &fresh)
			fresh.ID = "E-replce"
			writeDependencyYAML(t, path, fresh)
		}},
		{"epic key", store.NounEpic, func(t *testing.T, root string, node store.Node) {
			doc := node.(*store.Epic)
			path := filepath.Join(root, ".tiller", "epics", *doc.Key, "epic.yml")
			var fresh store.Epic
			readDependencyYAML(t, path, &fresh)
			key := "replacement"
			fresh.Key = &key
			writeDependencyYAML(t, path, fresh)
		}},
		{"feature id", store.NounFeature, func(t *testing.T, root string, node store.Node) {
			doc := node.(*store.Feature)
			path := filepath.Join(root, ".tiller", "epics", doc.EpicKey, "features", localKeyT(*doc.Key), "feature.yml")
			var fresh store.Feature
			readDependencyYAML(t, path, &fresh)
			fresh.ID = "F-replce"
			writeDependencyYAML(t, path, fresh)
		}},
		{"feature key", store.NounFeature, func(t *testing.T, root string, node store.Node) {
			doc := node.(*store.Feature)
			path := filepath.Join(root, ".tiller", "epics", doc.EpicKey, "features", localKeyT(*doc.Key), "feature.yml")
			var fresh store.Feature
			readDependencyYAML(t, path, &fresh)
			key := doc.EpicKey + "/replacement"
			fresh.Key = &key
			writeDependencyYAML(t, path, fresh)
		}},
		{"feature parent", store.NounFeature, func(t *testing.T, root string, node store.Node) {
			doc := node.(*store.Feature)
			path := filepath.Join(root, ".tiller", "epics", doc.EpicKey, "features", localKeyT(*doc.Key), "feature.yml")
			var fresh store.Feature
			readDependencyYAML(t, path, &fresh)
			key := "replacement-parent/" + localKeyT(*doc.Key)
			fresh.Key = &key
			writeDependencyYAML(t, path, fresh)
		}},
		{"task id", store.NounTask, func(t *testing.T, root string, node store.Node) {
			path := taskYMLPath(t, root, node.NodeID())
			var fresh store.Task
			readDependencyYAML(t, path, &fresh)
			fresh.ID = "T-replce"
			writeDependencyYAML(t, path, fresh)
		}},
		{"task key", store.NounTask, func(t *testing.T, root string, node store.Node) {
			doc := node.(*store.Task)
			path := taskYMLPath(t, root, doc.ID)
			var fresh store.Task
			readDependencyYAML(t, path, &fresh)
			parts := strings.Split(*doc.Key, "/")
			key := strings.Join([]string{parts[0], parts[1], "replacement"}, "/")
			fresh.Key = &key
			writeDependencyYAML(t, path, fresh)
		}},
		{"task parent", store.NounTask, func(t *testing.T, root string, node store.Node) {
			doc := node.(*store.Task)
			path := taskYMLPath(t, root, doc.ID)
			var fresh store.Task
			readDependencyYAML(t, path, &fresh)
			key := "replacement-parent/replacement-feature/" + localKeyT(*doc.Key)
			fresh.Key = &key
			writeDependencyYAML(t, path, fresh)
		}},
	}

	for _, tt := range tests {
		for _, operation := range []string{"link changed", "unlink changed"} {
			t.Run(tt.name+"/"+operation, func(t *testing.T) {
				root, ref, existing, absent := dependencyChangeFixture(t, tt.noun)
				stale := load(t, root)
				node := resolveNode(t, stale, tt.noun, ref)
				tt.change(t, root, node)
				before := snapshotDir(t, root)

				_, err := callDependencyChange(stale, operation, tt.noun, ref, existing, absent)
				var env *store.EnvError
				require.True(t, errors.As(err, &env))
				require.Equal(t, store.ReasonMutationStale, env.Reason)
				requireNoWrites(t, before, snapshotDir(t, root))
			})
		}
	}
}

func TestDependencyChangeRefusesRecordedFeatureAndDescendantEpicBranches(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"link changed", "unlink changed"} {
		t.Run("feature branch/"+operation, func(t *testing.T) {
			root, node, existing, absent := dependencyChangeFixture(t, store.NounFeature)
			stale := load(t, root)
			setDependencyNodeState(t, root, store.NounFeature, node, "draft", "tiller/form-engine/certification-form-loading")
			before := snapshotDir(t, root)

			_, err := callDependencyChange(stale, operation, store.NounFeature, node, existing, absent)
			var locked *mutate.DependencyChangeLockedError
			require.ErrorAs(t, err, &locked)
			require.Equal(t, mutate.ReasonDependencyChangeLocked, locked.Reason)
			requireNoWrites(t, before, snapshotDir(t, root))
		})

		t.Run("ready epic child branch/"+operation, func(t *testing.T) {
			root, node, existing, absent := dependencyChangeFixture(t, store.NounEpic)
			stale := load(t, root)
			setDependencyNodeState(t, root, store.NounEpic, node, "ready", "")
			setDependencyNodeState(t, root, store.NounFeature, "form-engine/schema-engine", "draft", "tiller/form-engine/schema-engine")
			before := snapshotDir(t, root)

			_, err := callDependencyChange(stale, operation, store.NounEpic, node, existing, absent)
			var locked *mutate.DependencyChangeLockedError
			require.ErrorAs(t, err, &locked)
			require.Equal(t, "ready", locked.Status)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}

	t.Run("branch-locked no-op retries remain legal", func(t *testing.T) {
		root, node, existing, absent := dependencyChangeFixture(t, store.NounFeature)
		stale := load(t, root)
		setDependencyNodeState(t, root, store.NounFeature, node, "ready", "tiller/form-engine/certification-form-loading")
		before := snapshotDir(t, root)

		changed, err := callDependencyChange(stale, "link existing no-op", store.NounFeature, node, existing, absent)
		require.NoError(t, err)
		require.False(t, changed)
		changed, err = callDependencyChange(stale, "unlink absent no-op", store.NounFeature, node, existing, absent)
		require.NoError(t, err)
		require.False(t, changed)
		requireNoWrites(t, before, snapshotDir(t, root))
	})
}

func TestDependencyChangeReadyEpicRefusesStartedDescendantButNoOpsRemainLegal(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"link changed", "unlink changed", "link existing no-op", "unlink absent no-op"} {
		t.Run(operation, func(t *testing.T) {
			root, node, existing, absent := dependencyChangeFixture(t, store.NounEpic)
			_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
				Key: "form-engine/schema-engine/started", Title: "Started",
			})
			require.NoError(t, err)
			stale := load(t, root)
			setDependencyNodeState(t, root, store.NounEpic, node, "ready", "")
			setDependencyNodeState(t, root, store.NounTask, "form-engine/schema-engine/started", "in_progress", "")
			before := snapshotDir(t, root)

			changed, err := callDependencyChange(stale, operation, store.NounEpic, node, existing, absent)
			if strings.Contains(operation, "no-op") {
				require.NoError(t, err)
				require.False(t, changed)
			} else {
				var locked *mutate.DependencyChangeLockedError
				require.ErrorAs(t, err, &locked)
				require.Equal(t, mutate.ReasonDependencyChangeLocked, locked.Reason)
			}
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestDependencyChangeAllowsPreWorkCancelledEpicDescendants(t *testing.T) {
	t.Parallel()
	for _, epicStatus := range []string{"draft", "ready"} {
		for _, descendant := range []string{"feature", "task"} {
			for _, operation := range []string{"link changed", "unlink changed"} {
				t.Run(epicStatus+"/"+descendant+"/"+operation, func(t *testing.T) {
					root, node, existing, absent := dependencyChangeFixture(t, store.NounEpic)
					if descendant == "task" {
						_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
							Key: "form-engine/schema-engine/cancelled-before-start", Title: "Cancelled before start",
						})
						require.NoError(t, err)
					}
					stale := load(t, root)
					setDependencyNodeState(t, root, store.NounEpic, node, epicStatus, "")
					if descendant == "feature" {
						setDependencyNodeState(t, root, store.NounFeature, "form-engine/schema-engine", "cancelled", "")
					} else {
						setDependencyNodeState(t, root, store.NounTask, "form-engine/schema-engine/cancelled-before-start", "cancelled", "")
					}

					changed, err := callDependencyChange(stale, operation, store.NounEpic, node, existing, absent)
					require.NoError(t, err)
					require.True(t, changed)
				})
			}
		}
	}
}

func TestDependencyChangeLocksCancelledEpicDescendantsUnderRecordedBranch(t *testing.T) {
	t.Parallel()
	for _, descendant := range []string{"feature", "task"} {
		for _, operation := range []string{"link changed", "unlink changed"} {
			t.Run(descendant+"/"+operation, func(t *testing.T) {
				root, node, existing, absent := dependencyChangeFixture(t, store.NounEpic)
				if descendant == "task" {
					_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
						Key: "form-engine/schema-engine/cancelled-after-start", Title: "Cancelled after start",
					})
					require.NoError(t, err)
				}
				stale := load(t, root)
				setDependencyNodeState(t, root, store.NounEpic, node, "ready", "")
				setDependencyNodeState(t, root, store.NounFeature, "form-engine/schema-engine", "cancelled",
					"tiller/form-engine/schema-engine")
				if descendant == "task" {
					setDependencyNodeState(t, root, store.NounFeature, "form-engine/schema-engine", "ready",
						"tiller/form-engine/schema-engine")
					setDependencyNodeState(t, root, store.NounTask, "form-engine/schema-engine/cancelled-after-start", "cancelled", "")
				}
				before := snapshotDir(t, root)

				_, err := callDependencyChange(stale, operation, store.NounEpic, node, existing, absent)
				var locked *mutate.DependencyChangeLockedError
				require.ErrorAs(t, err, &locked)
				require.Equal(t, mutate.ReasonDependencyChangeLocked, locked.Reason)
				requireNoWrites(t, before, snapshotDir(t, root))
			})
		}
	}

}

func TestDependencyChangeReadyEpicLocksStartedThenCancelledTaskWithoutFeatureBranch(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"link changed", "unlink changed", "link existing no-op", "unlink absent no-op"} {
		t.Run(operation, func(t *testing.T) {
			root := laySpeccableFeature(t, "code")
			d := testDeps()
			d.Mint = func(prefix string) string { return prefix + "-paymnt" }
			_, err := mutate.AddEpic(load(t, root), d, mutate.EpicInput{Key: "payments", Title: "Payments"})
			require.NoError(t, err)
			setDependencyNodeState(t, root, store.NounEpic, "payments", "done", "")
			_, err = mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
				Noun: store.NounEpic, Node: "form-engine", DependsOn: []string{"payments"},
			})
			require.NoError(t, err)
			driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
			driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
			driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
			driveTo(t, root, store.NounTask, "form-engine/certification-form-loading/transcribe-the-schema", mutate.TransitionInput{Verb: "start"})
			driveTo(t, root, store.NounTask, "form-engine/certification-form-loading/transcribe-the-schema", mutate.TransitionInput{
				Verb: "cancel", Reason: "work was started before the proposal was retired",
			})
			tree := load(t, root)
			require.Equal(t, "ready", tree.Epics["E-aaaaaa"].Status)
			require.Equal(t, "active", tree.Features["F-aaaaaa"].Status)
			require.Empty(t, tree.Features["F-aaaaaa"].Branch)
			require.Equal(t, "cancelled", tree.Tasks["T-aaaaaa"].Status)
			before := snapshotDir(t, root)

			changed, err := callDependencyChange(tree, operation, store.NounEpic, "form-engine", "payments", "form-engine/certification-form-loading")
			if strings.Contains(operation, "no-op") {
				require.NoError(t, err)
				require.False(t, changed)
			} else {
				var locked *mutate.DependencyChangeLockedError
				require.ErrorAs(t, err, &locked)
				require.Equal(t, mutate.ReasonDependencyChangeLocked, locked.Reason)
			}
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func dependencyChangeFixture(t *testing.T, noun string) (root, node, existing, absent string) {
	t.Helper()
	switch noun {
	case store.NounTask:
		root = siblingTaskRepo(t)
		addSiblingTaskWithDependencyMint(t, root, "third", "dddddd")
		node = "form-engine/schema-engine/first"
		existing = "form-engine/schema-engine/second"
		absent = "form-engine/schema-engine/third"
	case store.NounFeature:
		root = dependsRepo(t)
		node = "form-engine/certification-form-loading"
		existing = "form-engine/schema-engine"
		absent = "form-engine"
	case store.NounEpic:
		root = dependsRepo(t)
		addEpicWithDependencyMint(t, root, "payments", "dddddd")
		node = "form-engine"
		existing = "payments"
		absent = "form-engine/schema-engine"
	default:
		t.Fatalf("unsupported dependency fixture noun %q", noun)
	}
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: noun, Node: node, DependsOn: []string{existing},
	})
	require.NoError(t, err)
	return root, node, existing, absent
}

func callDependencyChange(tree *store.Tree, operation, noun, node, existing, absent string) (bool, error) {
	switch operation {
	case "link changed":
		res, err := mutate.Link(tree, testDeps(), mutate.LinkInput{Noun: noun, Node: node, DependsOn: []string{absent}})
		return res.Changed, err
	case "unlink changed":
		res, err := mutate.Unlink(tree, testDeps(), mutate.UnlinkInput{Noun: noun, Node: node, DependsOn: []string{existing}})
		return res.Changed, err
	case "link existing no-op":
		res, err := mutate.Link(tree, testDeps(), mutate.LinkInput{Noun: noun, Node: node, DependsOn: []string{existing}})
		return res.Changed, err
	case "unlink absent no-op":
		res, err := mutate.Unlink(tree, testDeps(), mutate.UnlinkInput{Noun: noun, Node: node, DependsOn: []string{absent}})
		return res.Changed, err
	default:
		return false, nil
	}
}

func linkStartedTaskToTarget(t *testing.T, targetStatus, target string) (string, error) {
	t.Helper()
	root, node, _, absent := dependencyChangeFixture(t, store.NounTask)
	setDependencyNodeState(t, root, store.NounTask, node, "in_progress", "")
	setDependencyNodeState(t, root, store.NounTask, absent, targetStatus, "")
	if target == "" {
		target = absent
	}
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounTask, Node: node, DependsOn: []string{target},
	})
	return root, err
}

func TestLinkTaskAddsEdgeToDoneTargetWhileInProgress(t *testing.T) {
	t.Parallel()
	root, err := linkStartedTaskToTarget(t, "done", "")
	require.NoError(t, err)
	tree := load(t, root)
	first := resolveNode(t, tree, store.NounTask, "form-engine/schema-engine/first").(*store.Task)
	third := resolveNode(t, tree, store.NounTask, "form-engine/schema-engine/third").(*store.Task)
	require.Contains(t, first.DependsOn, third.ID)
}

func TestLinkTaskAddsEdgeToDoneTargetByKey(t *testing.T) {
	t.Parallel()
	root, err := linkStartedTaskToTarget(t, "done", "form-engine/schema-engine/third")
	require.NoError(t, err)
	tree := load(t, root)
	first := resolveNode(t, tree, store.NounTask, "form-engine/schema-engine/first").(*store.Task)
	third := resolveNode(t, tree, store.NounTask, "form-engine/schema-engine/third").(*store.Task)
	require.Contains(t, first.DependsOn, third.ID)
}

func TestLinkTaskRefusesEdgeToInReviewTargetWhileInProgress(t *testing.T) {
	t.Parallel()
	_, err := linkStartedTaskToTarget(t, "in_review", "")
	var locked *mutate.DependencyChangeLockedError
	require.ErrorAs(t, err, &locked)
	require.Equal(t, mutate.ReasonDependencyChangeLocked, locked.Reason)
}

func TestUnlinkTaskStillLockedWhileInProgress(t *testing.T) {
	t.Parallel()
	root, node, existing, _ := dependencyChangeFixture(t, store.NounTask)
	setDependencyNodeState(t, root, store.NounTask, node, "in_progress", "")
	setDependencyNodeState(t, root, store.NounTask, existing, "done", "")
	_, err := mutate.Unlink(load(t, root), testDeps(), mutate.UnlinkInput{
		Noun: store.NounTask, Node: node, DependsOn: []string{existing},
	})
	var locked *mutate.DependencyChangeLockedError
	require.ErrorAs(t, err, &locked)
}

func TestLinkFeatureStillLockedWithDoneTarget(t *testing.T) {
	t.Parallel()
	root, node, _, absent := dependencyChangeFixture(t, store.NounFeature)
	setDependencyNodeState(t, root, store.NounFeature, node, "active", "tiller/form-engine/certification-form-loading")
	setDependencyNodeState(t, root, store.NounEpic, absent, "done", "")
	_, err := mutate.Link(load(t, root), testDeps(), mutate.LinkInput{
		Noun: store.NounFeature, Node: node, DependsOn: []string{absent},
	})
	var locked *mutate.DependencyChangeLockedError
	require.ErrorAs(t, err, &locked)
}
