// Package e2e holds the full-spine smoke test: it builds the real tiller binary
// and execs it against a throwaway git repo, driving the whole happy-path loop
// from init to a node reaching done. Unlike internal/cli/e2e_done_test.go, which
// drives the feature/task grain in-process through NewRootCmd, this test goes
// through the compiled binary as a subprocess and additionally covers the agent
// surface (tiller guide). It is hermetic: its own temp repo, its own git
// identity, and no dependency on ~/dev or the Acme corpus.
package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const (
	epicKey      = "gizmo"
	featureLocal = "assemble"
	featureKey   = epicKey + "/" + featureLocal
	taskLocal    = "wire-the-assembler"
)

// result is the captured outcome of one binary invocation.
type result struct {
	stdout string
	stderr string
	code   int
}

// harness carries the per-test fixture: the built binary, the run repo, and the
// isolated HOME the git identity is pinned to.
type harness struct {
	t    *testing.T
	bin  string
	dir  string
	home string
}

// TestFullSpineSmoke drives the entire happy-path spine through the real binary on
// a tiny fixture project and asserts every grain reaches done in the right order,
// plus that the agent surface answers. Each station asserts exit 0 so a regression
// points at the exact verb that broke, and the stations that carry a model claim
// assert the tree, not just the code:
//
//   - ONE `approve feature` moves the feature and its initial task and pins a
//     basis on each (cli-surface.md, "Specifying a feature").
//   - Nothing above the task is done before the human's merge, and the crossings
//     sync records read task, then feature, then epic (cli-surface.md, "The done
//     crossing").
//   - Every done above a task carries `done_basis: rollup`; the task's own carries
//     the observation that earned it.
func TestFullSpineSmoke(t *testing.T) {
	h := &harness{
		t:    t,
		bin:  buildTiller(t),
		dir:  t.TempDir(),
		home: t.TempDir(),
	}

	// A fresh git repo with one commit, so check/sync have a HEAD to anchor the
	// gate's against revision and the done-crossing ancestry to.
	h.git("init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(h.dir, "README.md"), []byte("smoke\n"), 0o644))
	h.git("add", "-A")
	h.git("commit", "-m", "initial commit")

	// init the workspace, join to record identity and seed the roster, then add one
	// passing required command gate to the generated manifest so check has something
	// real to run and record.
	requireOK(t, h.tiller("init", "--name", "Smoke Project", "--key", "smoke", "--kind", "greenfield"), "init")
	requireOK(t, h.joinAsOwner("jc", "JC Grubbs"), "join")
	addPassingGate(t, filepath.Join(h.dir, ".tiller", "tiller.yml"))

	// Lay the draft tree and take the epic gate.
	requireOK(t, h.tiller("add", "epic", epicKey, "--title", "Gizmo", "--source", "design/gizmo.md", "--contributor", "jc"), "add epic")
	requireOK(t, h.tiller("add", "feature", featureKey, "--title", "Assemble the gizmo", "--change-type", "create", "--source", "design/gizmo.md", "--contributor", "jc"), "add feature")
	requireOK(t, h.tiller("approve", "epic", epicKey, "--contributor", "jc"), "approve epic")

	// Specify is one act with one output: the spec, the acceptance contract, and
	// the task explosion, all written before the gate validates the whole of it
	// (cli-surface.md, "Specifying a feature"). Every one of those files is
	// written ONCE here and never touched again, so the active-basis guard never
	// bites downstream. approve feature is then the plan gate: it moves the
	// feature to ready and flips this task with it.
	writeFeatureSpec(t, h.featureDir())
	addTask := h.tiller("add", "task", featureKey+"/"+taskLocal, "--title", "Wire the assembler", "--contributor", "jc", "--json")
	requireOK(t, addTask, "add task")
	tid := regexp.MustCompile(`T-[a-z0-9]{6}`).FindString(addTask.stdout)
	require.NotEmpty(t, tid, "minted task id in add output")
	writeTaskSpec(t, h.featureDir())

	requireOK(t, h.tiller("spec", "feature", featureKey, "--contributor", "jc"), "spec feature")

	// The plan gate: one command, two grains. It moves the feature specced->ready,
	// flips its initial task proposed->ready in the same act, and pins the
	// active-work contract on each. Reading the tree back is what makes this the
	// plan gate and not a feature-only approve.
	planGate := h.tiller("approve", "feature", featureKey, "--contributor", "jc")
	requireOK(t, planGate, "approve feature")
	require.Contains(t, planGate.stdout, "1 task(s) in the plan moved to ready with it")

	planned, err := store.Load(h.dir)
	require.NoError(t, err)
	require.Len(t, planned.Features, 1)
	for _, f := range planned.Features {
		require.Equal(t, "ready", f.Status, "the approved feature is ready to start")
		require.NotEmpty(t, f.AcceptedAgainst.Spec.Value, "the gate pinned the feature's spec")
		require.NotEmpty(t, f.AcceptedAgainst.Eval.Value, "and its acceptance contract")
	}
	require.Equal(t, "ready", planned.Tasks[tid].Status, "the initial task rode the feature's gate to ready")
	require.NotNil(t, planned.Tasks[tid].AcceptedAgainst, "and its own task.md was pinned in the same act")
	require.NotEmpty(t, planned.Tasks[tid].AcceptedAgainst.Spec.Value)

	// The approved feature reads back through the node oracle and the summary,
	// the two read surfaces the loop routes from.
	nodeStatus := h.tiller("status", "feature", featureKey, "--json")
	requireOK(t, nodeStatus, "status feature")
	require.Contains(t, nodeStatus.stdout, `"status": "ready"`, "the approved feature is ready to start")
	summary := h.tiller("status", "--json")
	requireOK(t, summary, "status")
	require.Contains(t, summary.stdout, `"features": 1`)

	// Claim only this task. Its first start activates the approved feature.
	requireOK(t, h.tiller("claim", "task", tid, "--contributor", "jc"), "claim task")

	startTask := h.tiller("start", "task", tid, "--contributor", "jc", "--json")
	requireOK(t, startTask, "start task")
	branch := jsonField(t, startTask.stdout, "branch")
	baseBranch := jsonField(t, startTask.stdout, "base_branch")
	require.Equal(t, "tiller/"+featureKey+"/"+taskLocal, branch)
	require.Equal(t, "main", baseBranch)
	started, err := store.Load(h.dir)
	require.NoError(t, err)
	require.Equal(t, branch, started.Tasks[tid].Branch)
	require.True(t, started.Tasks[tid].Started)
	for _, f := range started.Features {
		require.Equal(t, "active", f.Status, "the task start activates its parent")
		require.Empty(t, f.Branch, "task-grain work has no feature review branch")
	}

	// Start records the task branch; Git creates it from the returned base.
	h.git("add", "-A")
	h.git("commit", "-m", "lay tree and start task")
	h.git("checkout", "-b", branch, baseBranch)
	require.NoError(t, os.WriteFile(filepath.Join(h.dir, "work.txt"), []byte("work\n"), 0o644))
	h.git("add", "-A")
	h.git("commit", "-m", "task work")

	// check task: the required command gate runs green and the task advances to check.
	checkTask := h.tiller("check", "task", tid, "--contributor", "jc", "--json")
	requireOK(t, checkTask, "check task")
	require.Contains(t, checkTask.stdout, `"passed": true`)
	require.Contains(t, checkTask.stdout, `"status": "check"`)

	// submit records the PR and moves the task check->in_review.
	requireOK(t, h.tiller("submit", "task", tid, "--pr", "7", "--contributor", "jc"), "submit task")

	// check feature: records the feature-level gate, the precondition the task-grain
	// rollup needs before it can cross the parent feature to done.
	requireOK(t, h.tiller("check", "feature", featureKey, "--contributor", "jc"), "check feature")

	// Keep the recorded review visible on main before merging the task's work.
	h.git("checkout", "main")
	h.git("add", "-A")
	h.git("commit", "-m", "record task review and feature check")

	// Nothing above the task has crossed yet. This is the ordering half of the
	// done-crossing claim: without it the assertions after sync would hold just
	// as well in a build that crossed the feature or the epic at the plan gate or
	// at check, rather than on the merge sync is about to observe.
	beforeMerge, err := store.Load(h.dir)
	require.NoError(t, err)
	require.Equal(t, "in_review", beforeMerge.Tasks[tid].Status)
	for _, f := range beforeMerge.Features {
		require.Equal(t, "active", f.Status, "the feature does not roll up before the merge")
	}
	for _, e := range beforeMerge.Epics {
		require.Equal(t, "ready", e.Status, "and neither does the epic")
	}

	// Run sync HERE, on the unmerged tree, and require it to cross nothing.
	//
	// The state check above proves only that nothing has crossed at this moment;
	// it cannot reach the rollup predicates, because the arc's real sync runs
	// after the merge, by which point the children genuinely are done and a
	// predicate that never looked at them answers the same as one that did. This
	// call is the arm that reaches them: the feature's gates are recorded and its
	// one task is still in_review, so featureRollupReady's "every non-cancelled
	// task is done" clause is the only thing standing between this run and a
	// feature crossing, and epicRollupReady's "every feature is done" clause the
	// only thing behind that. Dropping either one crosses a node right here.
	//
	// The failure this guards is not hypothetical bookkeeping: a loosened
	// predicate means a human running `tiller sync` on the integration branch
	// crosses a feature whose work is still under review, and the console then
	// reports finished work that nobody merged.
	preMerge := h.tiller("sync", "--contributor", "jc", "--json")
	requireOK(t, preMerge, "sync before the merge")
	require.Empty(t, crossingsOf(t, preMerge.stdout),
		"with the task still in review and the branch unmerged, sync must cross nothing at any grain")

	// The human merges the reviewed task branch; Tiller only observes the result.
	h.git("merge", "--no-ff", "-m", "merge task", branch)

	// sync observes the merge by ancestry and records the done crossings.
	sync := h.tiller("sync", "--contributor", "jc", "--json")
	requireOK(t, sync, "sync")
	require.Contains(t, sync.stdout, `"ok": true`)
	require.Contains(t, sync.stdout, `"status": "done"`)

	// One merge, three crossings, and the report reads bottom up: the task the PR
	// represented, then its feature, then the epic behind it. Only the first rests
	// on an observation; the two above it are arithmetic over what the human
	// merged (cli-surface.md, "The done crossing").
	require.Equal(t,
		[]crossing{
			{Noun: "task", Basis: "ancestry"},
			{Noun: "feature", Basis: "rollup"},
			{Noun: "epic", Basis: "rollup"},
		},
		crossingsOf(t, sync.stdout),
		"the crossings read task, then feature, then epic, and only the task's was observed")

	// The same three facts read off the tree the run left on disk.
	tree, err := store.Load(h.dir)
	require.NoError(t, err)
	require.Equal(t, "done", tree.Tasks[tid].Status, "the merged task crosses to done")
	require.Equal(t, "ancestry", tree.Tasks[tid].DoneBasis)
	require.Len(t, tree.Features, 1)
	for _, f := range tree.Features {
		require.Equal(t, "done", f.Status, "the parent feature rolls up to done")
		require.Equal(t, "rollup", f.DoneBasis, "a feature's done is arithmetic, never an observation")
	}
	require.Len(t, tree.Epics, 1)
	for _, e := range tree.Epics {
		require.Equal(t, "done", e.Status, "and the epic rolls up behind its last feature")
		require.Equal(t, "rollup", e.DoneBasis)
	}

	// The agent surface answers: a named skill comes back in full, and bare guide
	// routes the current state to one of the known skills.
	guide := h.tiller("guide", "execute", "--json")
	requireOK(t, guide, "guide execute")
	require.Contains(t, guide.stdout, `"skill": "execute"`)
	require.Contains(t, guide.stdout, `"text"`)
	suggest := h.tiller("guide", "--json")
	requireOK(t, suggest, "guide suggest")
	require.Regexp(t, `"skill":\s*"(orient|decompose|specify|execute)"`, suggest.stdout)
}

// featureDir is the on-disk folder of the one fixture feature.
func (h *harness) featureDir() string {
	return filepath.Join(h.dir, ".tiller", "epics", epicKey, "features", featureLocal)
}

// tiller runs the built binary in the run repo with the isolated environment and
// returns its captured streams plus the exit code in the Tiller scheme.
func (h *harness) tiller(args ...string) result {
	h.t.Helper()
	return h.run(cleanEnv(h.home), args)
}

// tillerAs runs tiller as contributor id through TILLER_CONTRIBUTOR, the way
// a second developer's checkout resolves its reader.
func (h *harness) tillerAs(id string, args ...string) result {
	h.t.Helper()
	return h.run(append(cleanEnv(h.home), "TILLER_CONTRIBUTOR="+id), args)
}

func (h *harness) run(env, args []string) result {
	h.t.Helper()
	cmd := exec.Command(h.bin, args...)
	cmd.Dir = h.dir
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			h.t.Fatalf("run tiller %v: %v", args, err)
		}
	}
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

// joinAsOwner registers id on the roster via `add contributor` (a human act, done
// once, ahead of any join), then joins as id. It is the fixture step every e2e
// test needs now that join only selects among contributors already registered:
// registration is add contributor's job, never a side effect of join.
func (h *harness) joinAsOwner(id, name string) result {
	h.t.Helper()
	requireOK(h.t, h.tiller("add", "contributor", id, "--name", name), "add contributor")
	return h.tiller("join", "--as", id)
}

// git runs a git command in the run repo with the pinned identity and fails the
// test on error.
func (h *harness) git(args ...string) {
	h.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = h.dir
	cmd.Env = cleanEnv(h.home)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(h.t, err, "git %s: %s", strings.Join(args, " "), out)
}

// requireOK asserts a station exited 0, surfacing both streams on failure so the
// break is legible.
func requireOK(t *testing.T, res result, station string) {
	t.Helper()
	require.Equalf(t, 0, res.code, "station %q exited %d\nstdout:\n%s\nstderr:\n%s", station, res.code, res.stdout, res.stderr)
}

// buildTiller compiles cmd/tiller into a temp path and returns it. The module root
// is found by walking up from this source file, so the build is independent of the
// test's working directory.
func buildTiller(t *testing.T) string {
	t.Helper()
	root := moduleRoot(t)
	bin := filepath.Join(t.TempDir(), "tiller")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/tiller")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go build cmd/tiller: %s", out)
	return bin
}

// moduleRoot walks up from this test file to the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqualf(t, parent, dir, "go.mod not found above %s", file)
		dir = parent
	}
}

// cleanEnv returns the process environment with HOME, the git identity, and the
// git config search pinned to a throwaway home, so the run never reads the
// developer's global git config or owner default. GIT_CONFIG_SYSTEM=/dev/null and
// an empty GIT_CONFIG_GLOBAL isolate config; the author/committer vars give commits
// a stable identity without writing any config file.
func cleanEnv(home string) []string {
	drop := map[string]bool{
		"HOME": true, "TILLER_CONTRIBUTOR": true,
		"GIT_CONFIG_GLOBAL": true, "GIT_CONFIG_SYSTEM": true,
		"GIT_AUTHOR_NAME": true, "GIT_AUTHOR_EMAIL": true,
		"GIT_COMMITTER_NAME": true, "GIT_COMMITTER_EMAIL": true,
	}
	var env []string
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i < 0 || drop[kv[:i]] {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"HOME="+home,
		"GIT_CONFIG_GLOBAL="+filepath.Join(home, "gitconfig-absent"),
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Tiller Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Tiller Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
}

// addPassingGate adds a passing build command gate to the init-generated required
// gates, which already carry the active evals gate, bound to feature grain (the
// contract gates the feature; tasks gate on build and tests). It is the realest
// minimal command gate the spine needs to record and cross alongside the seeded
// evals gate.
func addPassingGate(t *testing.T, manifestPath string) {
	t.Helper()
	raw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	const anchor = "  required:\n"
	require.Contains(t, string(raw), anchor, "init manifest carries the required-gates map")
	gate := anchor + "    build: { kind: command, run: \"true\", on: [feature, task] }\n"
	out := strings.Replace(string(raw), anchor, gate, 1)
	require.NoError(t, os.WriteFile(manifestPath, []byte(out), 0o644))
}

// writeTaskSpec writes the task's implementation spec over the stub `tiller add
// task` generated, the third file `spec feature` validates before draft->specced
// (cli-surface.md, "Specifying a feature": the stage's output is the spec, the
// contract, and the explosion).
func writeTaskSpec(t *testing.T, featureDir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		filepath.Join(featureDir, store.TaskDirName, taskLocal, store.TaskProseFile),
		[]byte("# Wire the assembler\n\n## Approach\n\nWire the parts into the assembler and check the result.\n"), 0o644))
}

// writeFeatureSpec writes the feature's prose spec and its single-criterion
// acceptance contract, two of the three non-stub files `spec feature` validates
// before draft->specced.
func writeFeatureSpec(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Assemble the gizmo\n\nAssemble the gizmo from its parts and validate the result.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The gizmo assembles\n    polarity: introduce\n    binding: { type: test, ref: spec/gizmo_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
}

// crossing is one row of sync's crossings list, narrowed to the two fields the
// spine asserts on: which grain crossed and what its crossing rested on.
type crossing struct {
	Noun  string `json:"noun"`
	Basis string `json:"basis"`
}

// crossingsOf parses sync's --json envelope into that list, in the order the
// envelope carries. Order is the point: a substring check over the same payload
// would pass on a report that named the three crossings in any sequence, and the
// sequence is exactly what "the merge crosses the task, and only then does the
// feature roll up, and only then the epic" means on the wire.
func crossingsOf(t *testing.T, jsonText string) []crossing {
	t.Helper()
	var env struct {
		Data struct {
			Crossings []crossing `json:"crossings"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonText), &env), "sync --json must parse")
	return env.Data.Crossings
}

// jsonField extracts the first string value for a top-level-ish JSON key from the
// envelope text. It is a deliberately small scraper: the smoke test only needs one
// scalar (the recorded branch) out of an otherwise asserted-by-substring envelope.
func jsonField(t *testing.T, jsonText, key string) string {
	t.Helper()
	m := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":\s*"([^"]*)"`).FindStringSubmatch(jsonText)
	require.Lenf(t, m, 2, "field %q not found in %s", key, jsonText)
	return m[1]
}
