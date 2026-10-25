package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// These tests pin command gate reuse on a node's own record (design
// 2026-10-21-gate-reuse): a pass recorded over the same code identity under the
// same resolved command stands in for a fresh run, and anything else runs.
// They drive real git (vcs.New), because the code identity is what decides.

// countingRun is a gate command that appends one line to a counter file
// outside the repo, so the counter never changes the code identity it is
// measuring. The echo sits last on purpose: the fixture's eval criterion runs
// the tests runner with its test ref appended, which lands as an extra echo
// argument, so that run writes a longer line runs does not count.
func countingRun(dir, name string) string {
	return "echo run >> " + filepath.Join(dir, name+".count")
}

// runs counts the gate runs a countingRun counter recorded (a missing file is
// zero runs).
func runs(t *testing.T, dir, name string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name+".count"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line == "run" {
			n++
		}
	}
	return n
}

// laterDeps is testDeps an hour on, so a second check's reused_at is
// distinguishable from the first run's run_at.
func laterDeps() mutate.Deps {
	d := testDeps()
	d.Clock = fixedClock{t: time.Date(2026, 6, 27, 15, 30, 0, 0, time.UTC)}
	return d
}

// checkGatedTask runs one check of the fixture task against real git.
func checkGatedTask(t *testing.T, root, taskID string, deps mutate.Deps, in mutate.CheckInput) mutate.CheckResult {
	t.Helper()
	tree := load(t, root)
	res, err := mutate.Check(tree, deps, vcs.New(root), resolveNode(t, tree, store.NounTask, taskID), in)
	require.NoError(t, err)
	return res
}

// outcome returns the named gate outcome of a check result.
func outcome(t *testing.T, res mutate.CheckResult, name string) mutate.GateOutcome {
	t.Helper()
	for _, o := range res.Gates {
		if o.Name == name {
			return o
		}
	}
	require.Failf(t, "no outcome", "check reported no %q gate", name)
	return mutate.GateOutcome{}
}

// editManifest replaces one exact span of the fixture's tiller.yml.
func editManifest(t *testing.T, root, old, replacement string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(data), old), "manifest span %q", old)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644))
}

// layCountedTask lays the gated task fixture with counting build and tests
// commands, plus one tracked source file, all committed.
func layCountedTask(t *testing.T, build, tests string) (root, taskID string) {
	t.Helper()
	root, taskID = layGatedTask(t, build, tests)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644))
	gitCommitAllM(t, root, "lay tree")
	return root, taskID
}

func TestCheckReusesUnchangedCode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	first := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	require.True(t, first.Passed)
	firstGates := load(t, root).Tasks[taskID].Gates
	gitCommitAllM(t, root, "record check")

	second := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
	require.True(t, second.Passed)
	require.Equal(t, 1, runs(t, dir, "build"))
	require.Equal(t, 1, runs(t, dir, "tests"))

	stored := load(t, root).Tasks[taskID].Gates
	for _, name := range []string{"build", "tests"} {
		o := outcome(t, second, name)
		require.True(t, o.Reused, name)
		require.Empty(t, o.ReusedFrom, "a record reused from the node itself names no source")
		require.Equal(t, firstGates[name].RunAt, o.ReusedRunAt, name)

		g := stored[name]
		require.Equal(t, laterDeps().Clock.Now().UTC(), g.ReusedAt, name)
		require.Empty(t, g.ReusedFrom, name)
		require.Equal(t, firstGates[name].RunAt, g.RunAt, name)
		require.Equal(t, firstGates[name].Evidence, g.Evidence, name)
		require.Equal(t, firstGates[name].Code, g.Code, name)
		require.NotEmpty(t, g.Code, name)
		require.NotEmpty(t, g.Config, name)
	}
}

func TestCheckRerunsChangedCode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	firstGates := load(t, root).Tasks[taskID].Gates
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))

	second := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
	require.Equal(t, 2, runs(t, dir, "build"))
	require.Equal(t, 2, runs(t, dir, "tests"))

	stored := load(t, root).Tasks[taskID].Gates
	for _, name := range []string{"build", "tests"} {
		require.False(t, outcome(t, second, name).Reused, name)
		require.NotEqual(t, firstGates[name].Code, stored[name].Code, name)
		require.True(t, stored[name].ReusedAt.IsZero(), name)
	}
}

func TestCheckDoesNotReusePassOverConversionHiddenShellBytes(t *testing.T) {
	t.Parallel()
	for _, conversion := range []string{"autocrlf", "attributes", "stripcr", "unset", "unspecified"} {
		t.Run(conversion, func(t *testing.T) {
			t.Parallel()
			// The gate fails on any carriage return in probe.sh, through tr and
			// cmp rather than the shell parsing the file: dash (Linux /bin/sh)
			// accepts "exit 0\r" where bash refuses it.
			root, taskID := layCountedTask(t, `tr -d '\r' < probe.sh | cmp -s - probe.sh`, "true")
			gitRunM(t, root, "config", "core.autocrlf", "false")
			switch conversion {
			case "autocrlf":
				gitRunM(t, root, "config", "core.autocrlf", "true")
			case "attributes":
				require.NoError(t, os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("probe.sh text\n"), 0o644))
			default:
				require.NoError(t, os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("probe.sh filter="+conversion+"\n"), 0o644))
				gitRunM(t, root, "config", "filter."+conversion+".clean", `tr -d '\r'`)
			}
			probe := filepath.Join(root, "probe.sh")
			require.NoError(t, os.WriteFile(probe, []byte("exit 0\n"), 0o644))
			gitCommitAllM(t, root, "lay gate input")

			first := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
			require.True(t, first.Passed)
			require.Equal(t, store.GateStatusPass, outcome(t, first, "build").Status)
			gitCommitAllM(t, root, "record passing check")

			require.NoError(t, os.WriteFile(probe, []byte("exit 0\r\n"), 0o644))
			require.Empty(t, gitOutM(t, root, "diff", "--name-only", "HEAD", "--", "probe.sh"),
				"the failing shell bytes are invisible to Git's normalized diff")
			second := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
			require.False(t, second.Passed, "an ordinary check must execute the failing input")
			build := outcome(t, second, "build")
			require.False(t, build.Reused)
			require.Equal(t, store.GateStatusFail, build.Status)
			require.Equal(t, store.GateStatusFail, load(t, root).Tasks[taskID].Gates["build"].Status)
		})
	}
}

func TestCheckRerunsChangedCommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tests := countingRun(dir, "tests")
	root, taskID := layCountedTask(t, countingRun(dir, "build"), tests)

	checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	// A .tiller/ edit: the code identity is unchanged, only the tests config.
	editManifest(t, root, `run: "`+tests+`"`, `run: "true; `+tests+`"`)

	second := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
	require.Equal(t, 1, runs(t, dir, "build"))
	require.Equal(t, 2, runs(t, dir, "tests"))
	require.True(t, outcome(t, second, "build").Reused)
	require.False(t, outcome(t, second, "tests").Reused)
}

func TestCheckNeverReusesAFail(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// `!` fails the run while keeping the echo last (see countingRun).
	root, taskID := layCountedTask(t, countingRun(dir, "build"), "! "+countingRun(dir, "tests"))

	first := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	require.False(t, first.Passed)
	require.Equal(t, store.GateStatusFail, outcome(t, first, "tests").Status)

	second := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
	require.Equal(t, 2, runs(t, dir, "tests"), "a fail always reruns")
	require.False(t, outcome(t, second, "tests").Reused)
	require.Equal(t, 1, runs(t, dir, "build"), "the pass beside it is still reused")
}

func TestCheckHonorsReuseFalseAndRerun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tests := countingRun(dir, "tests")
	root, taskID := layCountedTask(t, countingRun(dir, "build"), tests)
	editManifest(t, root, `run: "`+tests+`", on: [feature, task] }`, `run: "`+tests+`", on: [feature, task], reuse: false }`)

	checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	second := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
	require.Equal(t, 1, runs(t, dir, "build"))
	require.Equal(t, 2, runs(t, dir, "tests"), "reuse: false always runs")
	require.True(t, outcome(t, second, "build").Reused)
	require.False(t, outcome(t, second, "tests").Reused)

	third := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{Rerun: true})
	require.Equal(t, 2, runs(t, dir, "build"), "--rerun runs a reusable gate")
	require.Equal(t, 3, runs(t, dir, "tests"))
	require.False(t, outcome(t, third, "build").Reused)
	require.False(t, outcome(t, third, "tests").Reused)
}

// TestCheckDoesNotReuseLegacyRecords pins that a pass written before this
// build, with no code identity or config hash, is never reusable.
func TestCheckDoesNotReuseLegacyRecords(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	path := filepath.Join(root, store.TaskRelDir("form-engine", "nrca-form-loading", "transcribe-the-schema"), store.TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Task
	require.NoError(t, yaml.Unmarshal(data, &doc))
	legacy := store.Gate{
		Required: true, Kind: store.GateKindCommand, Status: store.GateStatusPass,
		RunBy: "jc", RunAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Against: fixedRev,
	}
	doc.Gates = map[string]store.Gate{"build": legacy, "tests": legacy}
	data, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	gitCommitAllM(t, root, "legacy records")

	res := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	require.Equal(t, 1, runs(t, dir, "build"))
	require.Equal(t, 1, runs(t, dir, "tests"))
	require.False(t, outcome(t, res, "build").Reused)
	require.False(t, outcome(t, res, "tests").Reused)
}

// TestCheckWithStubIdentityAlwaysRuns pins that an empty code identity (the
// stub's, or no git) never matches, so every check runs.
func TestCheckWithStubIdentityAlwaysRuns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layGatedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	for range 2 {
		tree := load(t, root)
		res, err := mutate.Check(tree, testDeps(), stubRev(root), resolveNode(t, tree, store.NounTask, taskID), mutate.CheckInput{})
		require.NoError(t, err)
		require.False(t, outcome(t, res, "build").Reused)
	}
	require.Equal(t, 2, runs(t, dir, "build"))
	require.Equal(t, 2, runs(t, dir, "tests"))
}

// checkGatedFeature runs one check of the fixture feature against real git.
func checkGatedFeature(t *testing.T, root string, deps mutate.Deps) mutate.CheckResult {
	t.Helper()
	tree := load(t, root)
	res, err := mutate.Check(tree, deps, vcs.New(root), resolveNode(t, tree, store.NounFeature, "form-engine/nrca-form-loading"), mutate.CheckInput{})
	require.NoError(t, err)
	return res
}

// TestFeatureCheckReusesRidingTaskRun pins decision 4: a feature check whose
// code identity equals a riding task's checked code reuses that task's pass
// instead of rerunning it, and the reused record names the task it came from.
func TestFeatureCheckReusesRidingTaskRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	require.True(t, checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{}).Passed)
	taskGates := load(t, root).Tasks[taskID].Gates
	gitCommitAllM(t, root, "record task check")

	res := checkGatedFeature(t, root, laterDeps())
	require.True(t, res.Passed)
	require.Equal(t, 1, runs(t, dir, "build"))
	require.Equal(t, 1, runs(t, dir, "tests"))

	stored := load(t, root).Features["F-aaaaaa"].Gates
	for _, name := range []string{"build", "tests"} {
		o := outcome(t, res, name)
		require.True(t, o.Reused, name)
		require.Equal(t, gatedTaskID, o.ReusedFrom, name)
		require.Equal(t, taskGates[name].RunAt, o.ReusedRunAt, name)

		g := stored[name]
		require.Equal(t, gatedTaskID, g.ReusedFrom, name)
		require.Equal(t, taskGates[name].RunBy, g.RunBy, name)
		require.Equal(t, taskGates[name].RunAt, g.RunAt, name)
		require.Equal(t, laterDeps().Clock.Now().UTC(), g.ReusedAt, name)
	}
}

// TestFeatureCheckRunsAfterAggregateChange pins that a change to tracked code
// after the task's check retires the task's record for the feature: the
// aggregate's code is no longer the code the task checked.
func TestFeatureCheckRunsAfterAggregateChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	gitCommitAllM(t, root, "record task check")
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))

	res := checkGatedFeature(t, root, laterDeps())
	require.Equal(t, 2, runs(t, dir, "build"))
	require.Equal(t, 2, runs(t, dir, "tests"))
	for _, name := range []string{"build", "tests"} {
		require.False(t, outcome(t, res, name).Reused, name)
	}
}

// TestFeatureCheckDoesNotReuseNodeNamedCommand pins Review Focus 4: a command
// that names the checked node resolves to a different command per node, so its
// config hash differs between a task and its feature and the feature runs it.
func TestFeatureCheckDoesNotReuseNodeNamedCommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	counter := filepath.Join(dir, "nodes.count")
	root, taskID := layCountedTask(t, countingRun(dir, "build"), "echo ${node.id} >> "+counter)

	checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	gitCommitAllM(t, root, "record task check")

	res := checkGatedFeature(t, root, laterDeps())
	require.False(t, outcome(t, res, "tests").Reused)
	require.True(t, outcome(t, res, "build").Reused, "a command that does not name the node is still reused")

	data, err := os.ReadFile(counter)
	require.NoError(t, err)
	require.Equal(t, "T-aaaaaa\nF-aaaaaa\n", string(data))
}

// TestCheckGateRunsOnlyNamed pins decision 10: `check --gate tests` shells
// tests and never build. A build with no current record reports not run; one
// whose recorded pass is current reports reused.
func TestCheckGateRunsOnlyNamed(t *testing.T) {
	t.Parallel()
	only := mutate.CheckInput{Gates: []string{"tests"}}

	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))
	res := checkGatedTask(t, root, taskID, testDeps(), only)
	require.Equal(t, 0, runs(t, dir, "build"))
	require.Equal(t, 1, runs(t, dir, "tests"))
	build := outcome(t, res, "build")
	require.True(t, build.NotRun)
	require.False(t, build.Reused)
	require.Equal(t, store.GateStatusPending, build.Status)
	require.False(t, outcome(t, res, "tests").NotRun)
	require.False(t, res.Passed, "a gate that has not run is not green")
	_, recorded := load(t, root).Tasks[taskID].Gates["build"]
	require.False(t, recorded, "a gate outside --gate is never written")

	// A failing tests run leaves the task in progress beside a current build pass.
	dir = t.TempDir()
	root, taskID = layCountedTask(t, countingRun(dir, "build"), "! "+countingRun(dir, "tests"))
	checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	firstBuild := load(t, root).Tasks[taskID].Gates["build"]
	res = checkGatedTask(t, root, taskID, laterDeps(), only)
	require.Equal(t, 1, runs(t, dir, "build"))
	require.Equal(t, 2, runs(t, dir, "tests"))
	build = outcome(t, res, "build")
	require.True(t, build.Reused)
	require.False(t, build.NotRun)
	require.Equal(t, store.GateStatusPass, build.Status)
	require.Equal(t, firstBuild.RunAt, build.ReusedRunAt)
	stored := load(t, root).Tasks[taskID].Gates["build"]
	require.Equal(t, laterDeps().Clock.Now().UTC(), stored.ReusedAt, "a current held gate is stamped like any reuse")
	require.Equal(t, firstBuild.RunAt, stored.RunAt)
	require.Equal(t, firstBuild.Evidence, stored.Evidence)
	// The fixture's one criterion is optional, so the first check's evals
	// record is a current pass too, and the held evals gate is reused.
	require.True(t, outcome(t, res, "evals").Reused)
	require.Equal(t, []any{"build", "evals"}, checkEvents(t, root)[1].After["reused"])
}

// layHeldBuild lays the counted task with a tests gate that passes only once
// <dir>/ok exists, checks it once (build passes, tests fails, the task stays
// in progress), then creates ok, so a following `--gate tests` passes tests
// over unchanged code and only build's standing decides the advance.
func layHeldBuild(t *testing.T) (root, taskID, dir, build string) {
	t.Helper()
	dir = t.TempDir()
	build = countingRun(dir, "build")
	ok := filepath.Join(dir, "ok")
	root, taskID = layCountedTask(t, build, "test -f "+ok+" && "+countingRun(dir, "tests"))
	first := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	require.Equal(t, store.GateStatusFail, outcome(t, first, "tests").Status)
	require.NoError(t, os.WriteFile(ok, nil, 0o644))
	return root, taskID, dir, build
}

// staled is a gate record as a blocked --gate check leaves a held pass it
// could not reuse: the same record, evidence and provenance kept, with the
// gate and each of its criteria stale. It fits fixtures whose criteria are all
// code-bound; a human's verdict is kept (TestCheckGateHeldEvalsKeepsHumanVerdicts).
func staled(g store.Gate) store.Gate {
	g.Status = store.GateStatusStale
	if g.Criteria != nil {
		criteria := make(map[string]store.Criterion, len(g.Criteria))
		for id, c := range g.Criteria {
			c.Status = store.CriterionStatusStale
			criteria[id] = c
		}
		g.Criteria = criteria
	}
	return g
}

// TestCheckGateHeldGateNotCurrent pins the rulings on a held gate: it counts
// only when its pass is reusable under the reuse key (code identity plus
// config hash), with reuse allowed. A changed command, `reuse: false`, or
// --rerun each leave it not run and mark its old pass stale, and the node
// does not advance.
func TestCheckGateHeldGateNotCurrent(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		edit func(t *testing.T, root, build string)
		in   mutate.CheckInput
	}{
		"changed command": {
			edit: func(t *testing.T, root, build string) {
				editManifest(t, root, `run: "`+build+`"`, `run: "true; `+build+`"`)
			},
			in: mutate.CheckInput{Gates: []string{"tests"}},
		},
		"reuse false": {
			edit: func(t *testing.T, root, build string) {
				editManifest(t, root, `run: "`+build+`", on: [feature, task] }`, `run: "`+build+`", on: [feature, task], reuse: false }`)
			},
			in: mutate.CheckInput{Gates: []string{"tests"}},
		},
		"rerun": {
			edit: func(*testing.T, string, string) {},
			in:   mutate.CheckInput{Gates: []string{"tests"}, Rerun: true},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, taskID, dir, build := layHeldBuild(t)
			firstBuild := load(t, root).Tasks[taskID].Gates["build"]
			tc.edit(t, root, build)

			res := checkGatedTask(t, root, taskID, laterDeps(), tc.in)
			require.Equal(t, store.GateStatusPass, outcome(t, res, "tests").Status)
			o := outcome(t, res, "build")
			require.True(t, o.NotRun)
			require.False(t, o.Reused)
			require.Equal(t, store.GateStatusStale, o.Status)
			require.Equal(t, 1, runs(t, dir, "build"), "a held gate is never shelled")
			require.False(t, res.Passed)
			require.Equal(t, "in_progress", load(t, root).Tasks[taskID].Status)
			require.Equal(t, staled(firstBuild), load(t, root).Tasks[taskID].Gates["build"], "a held pass that is not current is staled, never shelled")
		})
	}

	// A test-binding criterion resolves its command from the tests runner, so
	// moving the runner moves its config hash: a held evals record whose
	// criterion passed under the old runner is not current, on unchanged code.
	t.Run("moved tests runner", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		tests := countingRun(dir, "tests")
		root, taskID := layCountedTask(t, countingRun(dir, "build"), tests)
		first := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{Gates: []string{"build", "evals"}})
		require.Equal(t, store.GateStatusPass, outcome(t, first, "evals").Status)
		firstEvals := load(t, root).Tasks[taskID].Gates["evals"]
		require.Equal(t, store.CriterionStatusPass, firstEvals.Criteria["C1"].Status)
		editManifest(t, root, `run: "`+tests+`"`, `run: "true; `+tests+`"`)

		res := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{Gates: []string{"tests"}})
		require.Equal(t, store.GateStatusPass, outcome(t, res, "tests").Status)
		require.True(t, outcome(t, res, "build").Reused, "build's command did not move")
		evals := outcome(t, res, "evals")
		require.True(t, evals.NotRun)
		require.False(t, evals.Reused)
		require.Equal(t, store.GateStatusStale, evals.Status)
		require.False(t, res.Passed)
		require.Equal(t, "in_progress", load(t, root).Tasks[taskID].Status)
		require.Equal(t, staled(firstEvals), load(t, root).Tasks[taskID].Gates["evals"], "a held evals pass that is not current is staled, never shelled")
	})

	// A code change retires a held evals pass the same way: its gate and
	// criteria go stale, and no criterion runs.
	t.Run("changed code", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))
		first := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
		require.True(t, first.Passed)
		firstEvals := load(t, root).Tasks[taskID].Gates["evals"]
		require.Equal(t, store.GateStatusPass, firstEvals.Status)
		require.NotEmpty(t, firstEvals.Criteria)
		require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))
		lines := readCount(t, dir, "tests")

		res := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{Gates: []string{"build"}})
		evals := outcome(t, res, "evals")
		require.True(t, evals.NotRun)
		require.Equal(t, store.GateStatusStale, evals.Status)
		require.False(t, res.Passed)
		require.Equal(t, staled(firstEvals), load(t, root).Tasks[taskID].Gates["evals"])
		require.Equal(t, lines, readCount(t, dir, "tests"), "a held evals gate runs no criterion")
	})
}

// layAttestedFeature lays the gated repo with a feature whose contract has a
// command criterion C1 (a counter run) and a required manual criterion C2,
// checks the feature once, has a human attest C2, and lays one tracked source
// file, so the evals gate holds a code-bound pass beside a human's verdict.
func layAttestedFeature(t *testing.T, dir string) (root, featureID string) {
	t.Helper()
	root = t.TempDir()
	layGatedRepo(t, root, "true", "true")
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/nrca-form-loading", Title: "NRCA form loading", ChangeType: "create"})
	require.NoError(t, err)
	fdir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "nrca-form-loading")
	require.NoError(t, os.WriteFile(filepath.Join(fdir, "feature.md"), []byte("# NRCA form loading\n\nLoad the form.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fdir, "feature.eval.md"), []byte("---\nnode: F-aaaaaa\ncriteria:\n"+
		"  - id: C1\n    statement: The form loads\n    polarity: introduce\n    binding: { type: command, run: \"echo run >> "+filepath.Join(dir, "c1.count")+"\" }\n    required: true\n"+
		"  - id: C2\n    statement: The form reads well\n    polarity: introduce\n    binding: { type: manual }\n    required: true\n"+
		"---\n\n# Acceptance\n"), 0o644))
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/nrca-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), testDeps(), "form-engine/nrca-form-loading/transcribe-the-schema", "")
	require.NoError(t, err)
	writeTaskSpec(t, root, "transcribe-the-schema", "## Approach\n\nTranscribe the JSON Schema into the form model.")
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/nrca-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/nrca-form-loading", mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644))
	gitCommitAllM(t, root, "lay feature")

	first := checkFeatureWith(t, root, testDeps(), mutate.CheckInput{})
	require.Equal(t, store.CriterionStatusNeedsAttestation, outcome(t, first, "evals").Criteria["C2"])
	tree := load(t, root)
	_, err = mutate.Attest(tree, testDeps(), resolveNode(t, tree, store.NounFeature, "form-engine/nrca-form-loading"), mutate.AttestInput{Criterion: "C2", Status: store.GateStatusPass})
	require.NoError(t, err)
	gitCommitAllM(t, root, "record acceptance")
	evals := load(t, root).Features["F-aaaaaa"].Gates["evals"]
	require.Equal(t, store.GateStatusPass, evals.Status)
	require.Equal(t, store.CriterionStatusPass, evals.Criteria["C2"].Status)
	return root, "F-aaaaaa"
}

// checkFeatureWith runs one check of the fixture feature with the given input.
func checkFeatureWith(t *testing.T, root string, deps mutate.Deps, in mutate.CheckInput) mutate.CheckResult {
	t.Helper()
	tree := load(t, root)
	res, err := mutate.Check(tree, deps, vcs.New(root), resolveNode(t, tree, store.NounFeature, "form-engine/nrca-form-loading"), in)
	require.NoError(t, err)
	return res
}

// TestCheckGateHeldEvalsKeepsHumanVerdicts pins that staling a held evals
// pass on changed code stales the gate and only the criteria a full check
// would rerun: the command criterion goes stale, while a human's attested
// manual pass stays pass, so the full check after it keeps that acceptance
// exactly as a full check on the same code would.
func TestCheckGateHeldEvalsKeepsHumanVerdicts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, featureID := layAttestedFeature(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))

	partial := checkFeatureWith(t, root, laterDeps(), mutate.CheckInput{Gates: []string{"build"}})
	require.False(t, partial.Passed)
	held := load(t, root).Features[featureID].Gates["evals"]
	require.Equal(t, store.GateStatusStale, held.Status, "submit, sync, and readout read the held gate as stale")
	require.Equal(t, store.CriterionStatusStale, held.Criteria["C1"].Status, "a code-bound criterion goes stale")
	require.Equal(t, store.CriterionStatusPass, held.Criteria["C2"].Status, "a human's verdict is not discarded")
	require.Equal(t, 1, runs(t, dir, "c1"), "a held evals gate runs no criterion")

	full := checkFeatureWith(t, root, laterDeps(), mutate.CheckInput{})
	require.Equal(t, 2, runs(t, dir, "c1"))
	evals := load(t, root).Features[featureID].Gates["evals"]
	require.Equal(t, store.CriterionStatusPass, evals.Criteria["C2"].Status, "the full check keeps the attested pass")
	require.Equal(t, store.GateStatusPass, outcome(t, full, "evals").Status)
}

// readCount returns a countingRun counter file's raw contents, criterion runs
// included (a missing file is empty).
func readCount(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name+".count"))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

// TestCheckGateDoesNotAdvanceWithStaleSiblings pins Review Focus 5: after an
// edit, `--gate tests` passes tests, but build's pass is over the old code,
// so the check is not green and the task stays in progress. A plain check
// then reruns build and advances it.
func TestCheckGateDoesNotAdvanceWithStaleSiblings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// tests passes only once main.go defines a function.
	root, taskID := layCountedTask(t, countingRun(dir, "build"), "grep -q func main.go && "+countingRun(dir, "tests"))

	first := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{})
	require.Equal(t, store.GateStatusFail, outcome(t, first, "tests").Status)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))

	partial := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{Gates: []string{"tests"}})
	require.Equal(t, store.GateStatusPass, outcome(t, partial, "tests").Status)
	build := outcome(t, partial, "build")
	require.True(t, build.NotRun)
	require.Equal(t, store.GateStatusStale, build.Status, "a held pass over older code is marked stale")
	require.False(t, partial.Passed)
	require.Equal(t, "in_progress", partial.Status)
	require.Equal(t, "in_progress", load(t, root).Tasks[taskID].Status)
	require.Equal(t, 1, runs(t, dir, "build"))

	full := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
	require.Equal(t, 2, runs(t, dir, "build"))
	require.True(t, outcome(t, full, "tests").Reused)
	require.True(t, full.Passed)
	require.Equal(t, "check", full.Status)
}

// TestCheckGateUnknownName pins that a --gate name outside the node's required
// gates is a usage refusal before any gate runs or anything is written.
func TestCheckGateUnknownName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layGatedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))
	before := snapshotDir(t, root)

	tree := load(t, root)
	_, err := mutate.Check(tree, testDeps(), stubRev(root), resolveNode(t, tree, store.NounTask, taskID), mutate.CheckInput{Gates: []string{"lint"}})
	require.Equal(t, "unknown_gate", reasonOf(t, err))
	require.EqualError(t, err, "lint is not a required gate on this task; applicable: build, evals, tests")
	require.Equal(t, 0, runs(t, dir, "build"))
	require.Equal(t, 0, runs(t, dir, "tests"))
	requireNoWrites(t, before, snapshotDir(t, root))
}

// checkEvents returns every check.task event in ledger order.
func checkEvents(t *testing.T, root string) []events.Event {
	t.Helper()
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var out []events.Event
	for _, ev := range evs {
		if ev.Type == "check.task" {
			out = append(out, ev)
		}
	}
	return out
}

// TestCheckEventRecordsReused pins that the check event names what this check
// reused, so a later analysis can count avoided runs; a check that reused
// nothing carries no reused key.
func TestCheckEventRecordsReused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, taskID := layCountedTask(t, countingRun(dir, "build"), countingRun(dir, "tests"))

	// Leaving evals out keeps the first check from advancing the task.
	first := checkGatedTask(t, root, taskID, testDeps(), mutate.CheckInput{Gates: []string{"build", "tests"}})
	require.False(t, first.Passed)
	second := checkGatedTask(t, root, taskID, laterDeps(), mutate.CheckInput{})
	require.True(t, second.Passed)

	evs := checkEvents(t, root)
	require.Len(t, evs, 2)
	_, has := evs[0].After["reused"]
	require.False(t, has, "a check that reused nothing records no reused key")
	require.Equal(t, []any{"build", "tests"}, evs[1].After["reused"])
}

// TestCheckWarnsShelfWithoutCommandGates pins decision 12: a bug runs the
// gates whose on: list names task, so a manifest that scopes build and tests
// to features alone leaves it ungated, and check says so.
func TestCheckWarnsShelfWithoutCommandGates(t *testing.T) {
	t.Parallel()
	root := layStartedBug(t, "true", "true")
	tree := load(t, root)
	res, err := mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	require.Empty(t, res.Warnings, "a gated bug carries no warning")

	root = layStartedBug(t, "true", "true")
	path := filepath.Join(root, ".tiller", "tiller.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "on: [feature, task]", "on: [feature]")), 0o644))

	tree = load(t, root)
	res, err = mutate.Check(tree, testDeps(), vcs.New(root), tree.Bugs["B-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err)
	require.Equal(t, []string{"no build or test gate applies to this bug: bugs, chores, and tweaks run the gates whose on: list names task in .tiller/tiller.yml, and none does"}, res.Warnings)
}
