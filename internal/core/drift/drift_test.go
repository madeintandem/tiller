package drift_test

// Engine tests run against real scratch git repos (t.TempDir() plus git
// init), the same style internal/vcs's own tests use: active_basis and
// answered_question both compare working-tree git blob hashes, so a stub
// git would only be testing the stub. The fixtures below drive real trees
// through the exported mutate verbs (AddEpic, AddFeature, AddTask,
// Transition, AddQuestion, AnswerQuestion, ReviewAssumption) rather than
// hand-writing YAML, the same idiom internal/core/mutate's own tests use;
// the one exception is the dangling-question-id case, which mutates the
// already-loaded in-memory tree directly, matching
// TestUnreviewedAssumptionsSkipsADanglingQuestionReference's own idiom in
// mutate_test, since a hand-edited assumption_basis is exactly what that
// case is.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// --- git + tree helpers ---

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	testgit.Init(t, dir)
}

func gitCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", msg)
}

// scratchManifest is a minimal manifest carrying one owner and all five
// drift checks, enough for the fixtures below (spec/approve/start touch
// nothing gate-related, so no gates: block is needed).
const scratchManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
git:
  branch_pattern: "tiller/${node.key}"
  integration_branch: main
drift:
  checks: [stale_spec, active_basis, answered_question, missing_surface, orphan_eval]
`

// scratchProjectGit lays a fresh manifest in a real git repo with one
// commit, the anchor approve/start/answer all need (HEAD cannot resolve
// before a first commit exists). It carries no tree yet; callers add their
// own epics, features, and tasks through the exported mutate verbs.
func scratchProjectGit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tdir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tdir, "tiller.yml"), []byte(scratchManifest), 0o644))
	gitInit(t, dir)
	gitCommitAll(t, dir, "init")
	// testgit.Init creates main, which the manifest above declares as
	// git.integration_branch, whatever a machine's init.defaultBranch says.
	return dir
}

// setManifestChecks rewrites root's drift.checks in place, preserving every
// other manifest field, so a test can exercise Run's validation and Disabled
// computation against a specific list without re-templating the whole file.
func setManifestChecks(t *testing.T, root string, checks []string) {
	t.Helper()
	m, err := store.LoadManifest(root)
	require.NoError(t, err)
	m.Drift.Checks = checks
	data, err := yaml.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), data, 0o644))
}

func loadTree(t *testing.T, root string) *store.Tree {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	return tree
}

// driftFixedClock is a deterministic events.Clock so a fixture's recorded
// timestamps are stable, matching mutate_test's own fixedClock.
type driftFixedClock struct{ t time.Time }

func (c driftFixedClock) Now() time.Time { return c.t }

var fixedInstant = time.Date(2026, 6, 27, 14, 30, 0, 0, time.UTC)

// seqDeps mints sequential ids across every prefix from one shared counter
// (E-s01, F-s02, T-s03, ...), so several epics/features/tasks/questions in
// one fixture tree never collide the way a constant minter would (matching
// mutate_test's own syncDeps for the identical reason). The token is padded
// to exactly six lowercase base36 characters (id.Valid's own shape,
// "^(E|F|T|Q|A|ADR|EV|AT)-[a-z0-9]{6}$"): a question is addressed by id
// only, so AnswerQuestion's resolver rejects anything shorter as a key
// lookup instead of an id lookup, which a keyless noun can never satisfy.
func seqDeps() mutate.Deps {
	n := 0
	return mutate.Deps{
		Contributor: "jc",
		Clock:       driftFixedClock{t: fixedInstant},
		Mint: func(prefix string) string {
			n++
			return fmt.Sprintf("%s-s%05d", prefix, n)
		},
	}
}

// amendSeqDeps is seqDeps in a separate token space (A-a00001, EV-a00002, ...),
// for a fixture that keeps driving a tree some earlier seqDeps already wrote
// events into. Event files are append-only and named by id, so two counters
// starting from zero over one repository plan the same path twice and the store
// refuses the second, which is the store being right.
func amendSeqDeps() mutate.Deps {
	n := 0
	return mutate.Deps{
		Contributor: "jc",
		Clock:       driftFixedClock{t: fixedInstant},
		Mint: func(prefix string) string {
			n++
			return fmt.Sprintf("%s-a%05d", prefix, n)
		},
	}
}

// driveFeature loads a fresh tree, resolves featureID in it, and applies one
// bare transition (no extra payload). Fixtures that need AssumeQuestion or a
// Reason call mutate.Transition directly instead.
func driveFeature(t *testing.T, root string, deps mutate.Deps, featureID, verb string) {
	t.Helper()
	tree := loadTree(t, root)
	f := tree.Features[featureID]
	require.NotNil(t, f, "feature %s must exist", featureID)
	_, err := mutate.Transition(tree, deps, f, mutate.TransitionInput{Verb: verb})
	require.NoError(t, err)
}

// featurePath is the repo-relative-to-absolute path of one file under a
// feature's own folder.
func featurePath(root, epicKey, featureKey, file string) string {
	return filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey, file)
}

// writeFeatureProse writes a non-stub feature.md and a one-criterion
// feature.eval.md, the stage-2 shape `spec feature` requires beyond the task
// explosion (mutate_test's own writeFeatureProse, generalized over the
// epic/feature keys and stamped with the feature's real minted id rather
// than a hardcoded one).
func writeFeatureProse(t *testing.T, root, epicKey, featureKey, featureID string) {
	t.Helper()
	dir := filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# "+featureKey+"\n\nReal specification prose for "+featureKey+".\n"), 0o644))
	eval := fmt.Sprintf("---\nnode: %s\ncriteria:\n  - id: C1\n    statement: it works\n    polarity: introduce\n    binding: { type: test, ref: spec/%s_spec.rb }\n---\n\n# Acceptance\n",
		featureID, featureKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(eval), 0o644))
}

// writeTaskSpec writes a non-stub task.md, the shape the spec gate's task
// explosion check requires.
func writeTaskSpec(t *testing.T, root, epicKey, featureKey, taskKey string) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir(epicKey, featureKey, taskKey), store.TaskProseFile)
	require.NoError(t, os.WriteFile(path, []byte("# "+taskKey+"\n\n## Approach\n\nReal implementation notes.\n"), 0o644))
}

// taskProsePath is the absolute path of one task's task.md, for a fixture
// that edits a task's OWN spec after approval (distinct from featurePath's
// edit of the parent feature's own file).
func taskProsePath(root, epicKey, featureKey, taskKey string) string {
	return filepath.Join(root, store.TaskRelDir(epicKey, featureKey, taskKey), store.TaskProseFile)
}

// clearFeatureCommitAnchor zeroes one feature's accepted_against.commit by
// hand, simulating a pre-anchor record (one approved before Task 2 added
// Commit to BasisRefs): the spec/eval pins stay real, so the comparison
// still runs and still finds drift, but there is no commit to date it by.
func clearFeatureCommitAnchor(t *testing.T, root, epicKey, featureKey string) {
	t.Helper()
	path := featurePath(root, epicKey, featureKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.AcceptedAgainst.Commit = store.RevisionRef{}
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// setFeatureStatus rewrites one feature.yml's status field alone, the cheap
// fixture path for a status this package's tests need but no real verb
// reaches (a done feature reached without driving the whole merge-and-sync
// machinery): read into the typed struct, mutate one field, marshal back,
// so every other field (accepted_against included) round-trips untouched.
func setFeatureStatus(t *testing.T, root, epicKey, featureKey, status string) {
	t.Helper()
	path := featurePath(root, epicKey, featureKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Status = status
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func setTaskStatus(t *testing.T, root, epicKey, featureKey, taskKey, status string) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir(epicKey, featureKey, taskKey), store.TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Task
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Status = status
	if status == "done" {
		doc.Started = true
	}
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func appendToFile(t *testing.T, path, text string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, []byte(text)...), 0o644))
}

// overwriteAndCommit replaces rel's whole content under root and commits the
// change: the "modified, not vanished" half of the vanish rule stale_spec's
// surface leg and missing_surface both turn on, content moving under a done
// node's surface is normal evolution, never a candidate on its own
// (artifact-schema.md, Drift control hooks). The commit matters here (unlike
// a plain edit): the surface leg's non-vanish comparison reads the tree hash
// at HEAD, not the working tree, since only a committed object has one
// (artifact-schema.md, Drift control hooks: "only recorded object hashes...
// are read at HEAD").
func overwriteAndCommit(t *testing.T, root, rel, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(content+"\n"), 0o644))
	gitCommitAll(t, root, "overwrite "+rel)
}

// removeAndCommit deletes rel under root and commits the removal: the vanish
// both stale_spec's surface leg and missing_surface turn a candidate on.
func removeAndCommit(t *testing.T, root, rel string) {
	t.Helper()
	require.NoError(t, os.Remove(filepath.Join(root, rel)))
	gitCommitAll(t, root, "remove "+rel)
}

// setEpicStatus rewrites one epic.yml's status field alone, the epic-grain
// twin of setFeatureStatus, for a status this package's tests need but no
// real verb reaches directly in one step (an archived epic, here, without
// separately driving an archive verb through a real tree).
func setEpicStatus(t *testing.T, root, epicKey, status string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Epic
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Status = status
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// setFeatureSupersededBy hand-writes one feature's superseded_by field. No
// verb sets this field yet: internal/views/nodepage.go reads it alone today,
// and the amend command that would write it is out of this plan's scope
// (see docs/spec/cli-surface.md). A
// superseded feature is therefore a hand-authored fixture on purpose here,
// the same idiom clearFeatureCommitAnchor already uses for the identical kind
// of gap one field over.
func setFeatureSupersededBy(t *testing.T, root, epicKey, featureKey, byID string) {
	t.Helper()
	path := featurePath(root, epicKey, featureKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.SupersededBy = &byID
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// driveTask loads a fresh tree, resolves taskID in it, and applies one bare
// transition (no extra payload), the task-grain twin of driveFeature.
func driveTask(t *testing.T, root string, deps mutate.Deps, taskID, verb string) {
	t.Helper()
	if verb == "start" {
		_, err := mutate.ClaimTask(loadTree(t, root), deps, taskID, "")
		require.NoError(t, err)
	}
	tree := loadTree(t, root)
	tk := tree.Tasks[taskID]
	require.NotNil(t, tk, "task %s must exist", taskID)
	_, err := mutate.Transition(tree, deps, tk, mutate.TransitionInput{Verb: verb})
	require.NoError(t, err)
}

// driveFeatureToDoneGit drives one feature, with one task and an optional
// brownfield surface, all the way through a real done crossing: spec,
// approve, start, task check/submit on its own branch, then a human no-ff
// merge into main and sync, which writes verified_against.
//
// surfacePaths, when non-empty, is recorded on the feature via SetSurface
// before the tree is first committed. The caller is responsible for the
// paths existing on disk beforehand: recordVerified's surface fingerprint
// needs them committed at the crossing, since TreeHash and TreeInventory both
// read HEAD, never the working tree.
func driveFeatureToDoneGit(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey, taskKey string, surfacePaths []string) (featureID, taskID string) {
	t.Helper()

	fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: epicKey + "/" + featureKey, Title: featureKey, ChangeType: "create", Source: "design/x.md",
	})
	require.NoError(t, err)
	featureID = fRes.ID
	writeFeatureProse(t, root, epicKey, featureKey, featureID)

	tRes, err := mutate.AddTask(loadTree(t, root), deps, mutate.TaskInput{
		Key: epicKey + "/" + featureKey + "/" + taskKey, Title: taskKey,
	})
	require.NoError(t, err)
	taskID = tRes.ID
	writeTaskSpec(t, root, epicKey, featureKey, taskKey)

	driveFeature(t, root, deps, featureID, "spec")
	driveFeature(t, root, deps, featureID, "approve")

	if len(surfacePaths) > 0 {
		tree := loadTree(t, root)
		_, err := mutate.SetSurface(tree, deps, tree.Features[featureID], surfacePaths)
		require.NoError(t, err)
	}

	carryApprovedFeatureToDoneGit(t, root, deps, epicKey, featureKey, taskKey, featureID, taskID)

	return featureID, taskID
}

// scratchDoneFeatureWithSurface lays one epic ("form-engine") and one feature
// ("feat-done", task "task-a"), driven to done through the real arc above,
// with a 2-file surface inventory under app/calc: the fixture
// TestStaleSpecVanishFiresModifyDoesNot and its siblings need, so that
// verified_against.surfaces carries a real recorded inventory to compare the
// vanish rule against. The lone feature also rolls its one epic up to done in
// the same sync, so a caller after epic-grain coverage can read it straight
// off the loaded tree rather than driving a second fixture.
// approveEpic drives one epic draft -> ready, the epic-grain twin of
// driveFeature's bare-verb shape. The epic rollup only ever fires from
// "ready" (syncEpics's own switch), so this has to run before any feature
// beneath the epic can carry it to done in the same sync.
func approveEpic(t *testing.T, root string, deps mutate.Deps, epicID string) {
	t.Helper()
	tree := loadTree(t, root)
	e := tree.Epics[epicID]
	require.NotNil(t, e, "epic %s must exist", epicID)
	_, err := mutate.Transition(tree, deps, e, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
}

func scratchDoneFeatureWithSurface(t *testing.T) (root, featureID string) {
	t.Helper()
	root = scratchProjectGit(t)
	deps := seqDeps()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "app", "calc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app/calc/project.rb"), []byte("class Project\nend\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app/calc/zone.rb"), []byte("class Zone\nend\n"), 0o644))

	epicRes, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)
	approveEpic(t, root, deps, epicRes.ID)

	featureID, _ = driveFeatureToDoneGit(t, root, deps, "form-engine", "feat-done", "task-a", []string{"app/calc"})
	return root, featureID
}

// treeHashFailsRev wraps a real vcs.Revisions and fails TreeHash for one
// specific path, leaving every other observation (including
// WorktreeBlobHash, which the spec/eval legs use) fully real via the
// embedded interface. It exists so a test can isolate a hard git failure to
// the surface leg alone, without the OS-permission fragility (and the
// root-bypasses-permissions caveat every chmod-based repro in this suite
// already carries) a git-internal read failure would need otherwise.
type treeHashFailsRev struct {
	vcs.Revisions
	path string
}

func (r treeHashFailsRev) TreeHash(path string) (string, error) {
	if path == r.path {
		return "", fmt.Errorf("simulated tree hash read failure under %s (test double)", path)
	}
	return r.Revisions.TreeHash(path)
}

func candidatesFor(rep drift.Report, marker string) []drift.Candidate {
	var out []drift.Candidate
	for _, c := range rep.Candidates {
		if c.Marker == marker {
			out = append(out, c)
		}
	}
	return out
}

func uncoveredFor(rep drift.Report, marker string) []drift.Uncovered {
	var out []drift.Uncovered
	for _, u := range rep.Uncovered {
		if u.Marker == marker {
			out = append(out, u)
		}
	}
	return out
}

// --- checks-list contract ---

func TestRunHonorsTheChecksList(t *testing.T) {
	root := scratchProjectGit(t) // helper this test file owns: init+join+a small tree in a real git repo
	setManifestChecks(t, root, []string{"active_basis"})

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]string{"stale_spec", "answered_question", "missing_surface", "orphan_eval", "stale_design", "shifted_design"},
		rep.Disabled, "a check absent from drift.checks is named, never silently skipped")
}

func TestRunRefusesAnUnknownCheckName(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{"active_basis", "typo_marker"})
	_, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.Error(t, err)

	var ue interface{ Reason() string }
	require.ErrorAs(t, err, &ue)
	require.Equal(t, "unknown_drift_check", ue.Reason())

	var coder store.ExitCoder
	require.ErrorAs(t, err, &coder)
	require.Equal(t, 2, coder.ExitCode(), "an unknown check is misuse, exit 2, not a business refusal")
}

// An empty manifest list enables nothing, including newly introduced markers.
func TestRunWithEmptyChecksDisablesAllMarkers(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{})

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]string{"stale_spec", "active_basis", "answered_question", "missing_surface", "orphan_eval", "stale_design", "shifted_design"},
		rep.Disabled)
	require.Empty(t, rep.Candidates)
	require.Empty(t, rep.Uncovered)
	require.Equal(t, 0, rep.Compared, "an empty checks list sweeps nothing, so nothing is compared")
}

// --- active_basis ---

// layActiveFeature approves the feature plan and starts its task, activating
// the parent. Both accepted bases remain eligible for the drift sweep.
func layActiveFeature(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey, taskKey string) (featureID, taskID string) {
	t.Helper()
	fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: epicKey + "/" + featureKey, Title: featureKey, ChangeType: "create", Source: "design/x.md",
	})
	require.NoError(t, err)
	featureID = fRes.ID
	writeFeatureProse(t, root, epicKey, featureKey, featureID)

	tRes, err := mutate.AddTask(loadTree(t, root), deps, mutate.TaskInput{
		Key: epicKey + "/" + featureKey + "/" + taskKey, Title: taskKey,
	})
	require.NoError(t, err)
	taskID = tRes.ID
	writeTaskSpec(t, root, epicKey, featureKey, taskKey)

	driveFeature(t, root, deps, featureID, "spec")
	driveFeature(t, root, deps, featureID, "approve")
	driveTask(t, root, deps, taskID, "start")
	return featureID, taskID
}

// TestActiveBasisSweep is written out fully.
// An approved feature whose feature.md is edited after approval is a
// candidate; the sweep dedupes the parent-feature report a task guard
// repeats; a DONE feature with an edited spec is NOT an active_basis
// candidate, because accepted_against is never cleared and the status
// filter in the sweep is what keeps stale_spec's business out of this
// marker (artifact-schema.md, the active_basis bullet).
func TestActiveBasisSweep(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	// Feature A: active, edited after approval. Its task rides at "ready",
	// which is what exercises the dedupe below.
	featA, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")
	appendToFile(t, featurePath(root, "form-engine", "feat-active", "feature.md"), "\nEdited after approval.\n")

	// Feature B: reaches active the identical way, then both it and its task
	// are hand-set to done (a real sync rollup is not what this half of the
	// test is about) before feature.md is edited too. Without the sweep's
	// own status filter this would report identically to feature A, since
	// accepted_against is never cleared at done.
	_, _ = layActiveFeature(t, root, deps, "form-engine", "feat-done", "task-b")
	setFeatureStatus(t, root, "form-engine", "feat-done", "done")
	setTaskStatus(t, root, "form-engine", "feat-done", "task-b", "done")
	appendToFile(t, featurePath(root, "form-engine", "feat-done", "feature.md"), "\nEdited after done.\n")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	// feat-active and its task-a are the only two nodes the status filter
	// admits (feat-done and task-b are both "done", excluded outright), so
	// Compared counts exactly them (Compared counts nodes whose comparisons
	// completed, feat-done's pair never even attempted).
	require.Equal(t, 2, rep.Compared)

	cs := candidatesFor(rep, drift.MarkerActiveBasis)
	require.Len(t, cs, 1,
		"feat-done must not surface (the status filter keeps stale_spec's business out), and feat-active's "+
			"task-grain hit must dedupe against its own feature-grain hit rather than doubling it")

	c := cs[0]
	require.Equal(t, store.NounFeature, c.Noun)
	require.Equal(t, featA, c.ID)
	require.Equal(t, "form-engine/feat-active", c.Key)
	require.Contains(t, c.Files, "feature.md")
	require.NotNil(t, c.Since, "the accepted_against commit anchor is the since-when")
}

// TestActiveBasisSweepReportsAKeylessFeatureAsUncoveredNotAPanic pins
// A store.Load-accepted tree whose
// feature carries no key at all (a feature.yml that lost its key: line)
// reaches BasisDrift's featureRelDir dereference of *f.Key; before the fix
// that panicked, and it must not. It is Uncovered with reason
// basis_unreadable, the same "cannot compare" answer any other unreadable
// basis gets, never silence and never a crash.
func TestActiveBasisSweepReportsAKeylessFeatureAsUncoveredNotAPanic(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

	tree := loadTree(t, root)
	tree.Features[featID].Key = nil

	var rep drift.Report
	require.NotPanics(t, func() {
		rep, err = drift.Run(tree, vcs.New(root))
	})
	require.NoError(t, err)

	var found bool
	for _, u := range uncoveredFor(rep, drift.MarkerActiveBasis) {
		if u.ID == featID {
			found = true
			require.Equal(t, "basis_unreadable", u.Reason)
			require.NotEmpty(t, u.Detail, "the row names the specific thing, not just a bare reason code")
		}
	}
	require.True(t, found, "the keyless feature is reported uncovered, not silently skipped")
	require.Empty(t, candidatesFor(rep, drift.MarkerActiveBasis), "there is nothing to name as drifted; it could not be compared at all")
}

// TestActiveBasisNamesTaskOwnDriftBesideParentDrift pins that BasisDrift's own
// task branch checks the parent feature FIRST and short-circuits there, so when
// both a feature's spec and its task's own spec have drifted, the parent-walk
// alone never names the task, and the agent's cluster is incomplete until a
// human clears the feature first. The sweep runs the task-own half
// (mutate.TaskOwnBasisDrift) separately, so the task's own drift survives
// beside whatever the parent-walk already found.
func TestActiveBasisNamesTaskOwnDriftBesideParentDrift(t *testing.T) {
	t.Run("task-only drift still names a task candidate", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		require.NoError(t, err)
		_, taskID := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

		appendToFile(t, taskProsePath(root, "form-engine", "feat-active", "task-a"), "\nEdited after approval.\n")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		cs := candidatesFor(rep, drift.MarkerActiveBasis)
		require.Len(t, cs, 1, "the feature's own pin is clean; only the task's own edit should surface")
		require.Equal(t, store.NounTask, cs[0].Noun)
		require.Equal(t, taskID, cs[0].ID)
		require.Contains(t, cs[0].Files, "task.md")
		require.NotNil(t, cs[0].Since)
	})

	t.Run("both feature and task drift survive as two distinct candidates", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		require.NoError(t, err)
		featID, taskID := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

		appendToFile(t, featurePath(root, "form-engine", "feat-active", "feature.md"), "\nEdited after approval.\n")
		appendToFile(t, taskProsePath(root, "form-engine", "feat-active", "task-a"), "\nEdited after approval too.\n")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		cs := candidatesFor(rep, drift.MarkerActiveBasis)
		require.Len(t, cs, 2, "BasisDrift's own short-circuit on the parent must not swallow the task's own drift")

		byNoun := map[string]drift.Candidate{}
		for _, c := range cs {
			byNoun[c.Noun] = c
		}

		featC, ok := byNoun[store.NounFeature]
		require.True(t, ok, "the feature's own drift is still reported")
		require.Equal(t, featID, featC.ID)
		require.Contains(t, featC.Files, "feature.md")
		require.NotNil(t, featC.Since)

		taskC, ok := byNoun[store.NounTask]
		require.True(t, ok, "the task's own drift is no longer masked by the parent-walk short-circuit")
		require.Equal(t, taskID, taskC.ID)
		require.Contains(t, taskC.Files, "task.md")
		require.NotNil(t, taskC.Since)
	})
}

// TestActiveBasisUnreadableIsUncovered pins the basis_unreadable arm:
// a pinned file git cannot read even
// though it still exists on disk (permission-denied rather than deleted) is
// neither a candidate, since checkBasis has no drifted/missing file list to
// report, nor silence: it is Uncovered with reason basis_unreadable.
func TestActiveBasisUnreadableIsUncovered(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits, so this repro needs a non-root process")
	}

	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

	path := featurePath(root, "form-engine", "feat-active", "feature.md")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	var found bool
	for _, u := range uncoveredFor(rep, drift.MarkerActiveBasis) {
		if u.ID == featID {
			found = true
			require.Equal(t, "basis_unreadable", u.Reason)
			require.NotEmpty(t, u.Detail)
		}
	}
	require.True(t, found, "a permission-denied pin is uncovered, not silently clean")
	require.Empty(t, candidatesFor(rep, drift.MarkerActiveBasis), "an unreadable basis names no drifted file, so it is never a candidate")
}

// TestActiveBasisCandidateWithNoCommitAnchorHasNilSince pins the pre-anchor
// case: a feature approved before Task 2
// added Commit to AcceptedAgainst (simulated here by zeroing it by hand)
// still gets compared and still surfaces its drift; Since is nil because
// there is nothing to date it by, not because the sweep failed to check it.
func TestActiveBasisCandidateWithNoCommitAnchorHasNilSince(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

	clearFeatureCommitAnchor(t, root, "form-engine", "feat-active")
	appendToFile(t, featurePath(root, "form-engine", "feat-active", "feature.md"), "\nEdited after approval.\n")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	cs := candidatesFor(rep, drift.MarkerActiveBasis)
	require.Len(t, cs, 1, "a pre-anchor record still gets compared and still surfaces its drift")
	require.Equal(t, featID, cs[0].ID)
	require.Nil(t, cs[0].Since, "a zero commit anchor is nil, not evidence the sweep failed to check the node")
}

// TestActiveBasisDetailDistinguishesDeletedFromChanged pins that
// a pinned file checkBasis recorded in Missing
// was deleted, not merely edited, and the remedy differs (--refresh-basis
// re-hashes the working tree, which it cannot do for a file that is not
// there), so the Detail sentence says "deleted" rather than the generic
// "changed" a merely-edited file gets, and when both kinds appear on the
// same candidate the deleted one leads the sentence while the merely-edited
// one rides beside it as Context.
func TestActiveBasisDetailDistinguishesDeletedFromChanged(t *testing.T) {
	t.Run("a vanished pin reads as deleted, not the generic changed", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		require.NoError(t, err)
		featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

		require.NoError(t, os.Remove(featurePath(root, "form-engine", "feat-active", "feature.md")))

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		cs := candidatesFor(rep, drift.MarkerActiveBasis)
		require.Len(t, cs, 1)
		require.Equal(t, featID, cs[0].ID)
		require.Contains(t, cs[0].Files, "feature.md")
		require.Contains(t, cs[0].Detail, "deleted")
		require.NotContains(t, cs[0].Detail, "changed since", "a deletion is not a change, and the remedy differs")
	})

	t.Run("a deleted pin beside a merely-edited one names both, deleted leading", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		require.NoError(t, err)
		featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

		require.NoError(t, os.Remove(featurePath(root, "form-engine", "feat-active", "feature.md")))
		appendToFile(t, featurePath(root, "form-engine", "feat-active", "feature.eval.md"), "\nEdited too.\n")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		cs := candidatesFor(rep, drift.MarkerActiveBasis)
		require.Len(t, cs, 1)
		require.Equal(t, featID, cs[0].ID)
		require.ElementsMatch(t, []string{"feature.md", "feature.eval.md"}, cs[0].Files)
		require.Contains(t, cs[0].Detail, "deleted")
		require.Contains(t, cs[0].Detail, "feature.md")
		require.NotEmpty(t, cs[0].Context, "the merely-edited file rides as context beside the deleted one")
		require.Contains(t, cs[0].Context[0], "feature.eval.md")
	})
}

// TestActiveBasisCoverageCountsNodesNotComparisons pins that Coverage must
// count NODES, the same unit Compared counts in, not the number of comparison
// CALLS a task's dual check happens to make, and the two buckets must stay
// disjoint (one node never lands in both "checked" and "unresolvable" in the
// same sweep). Three feature+task pairs: A is entirely clean, B has the
// feature's own spec drifted (a real candidate, deduped against the task's own
// parent-walk hit), and C has the feature's file made unreadable (chmod), which
// makes BOTH the feature and its task unresolvable, since a task's parent-walk
// reads the identical unreadable file before ever reaching its own pin.
func TestActiveBasisCoverageCountsNodesNotComparisons(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits, so pair C's repro needs a non-root process")
	}

	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)

	_, _ = layActiveFeature(t, root, deps, "form-engine", "feat-clean", "task-clean")

	_, _ = layActiveFeature(t, root, deps, "form-engine", "feat-drifted", "task-drifted")
	appendToFile(t, featurePath(root, "form-engine", "feat-drifted", "feature.md"), "\nEdited after approval.\n")

	_, _ = layActiveFeature(t, root, deps, "form-engine", "feat-unreadable", "task-unreadable")
	unreadablePath := featurePath(root, "form-engine", "feat-unreadable", "feature.md")
	require.NoError(t, os.Chmod(unreadablePath, 0o000))
	t.Cleanup(func() { _ = os.Chmod(unreadablePath, 0o644) })

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	// Pairs A and B complete fully: 2 nodes each, 4 total, all "checked".
	// Pair C completes for neither of its 2 nodes: both "unresolvable".
	require.Equal(t, 4, rep.Coverage[drift.MarkerActiveBasis]["checked"])
	require.Equal(t, 2, rep.Coverage[drift.MarkerActiveBasis]["unresolvable"])
	require.Equal(t, rep.Compared, rep.Coverage[drift.MarkerActiveBasis]["checked"],
		"Compared and the checked bucket must count the identical set of nodes")

	require.Len(t, uncoveredFor(rep, drift.MarkerActiveBasis), 2,
		"pair C's feature and task each get their own uncovered row, one row per node")
}

// TestActiveBasisTaskWithDriftUnderAKeylessParentIsUncoveredNotClean pins
// A task with a REAL drifted pin
// whose parent feature carries no key must not read as clean just because
// the parent cannot be resolved. Before the fix, TaskOwnBasisDrift's own
// "return nil" guard for an unresolvable parent silently swallowed the
// task's real drift (matching BasisDrift's own task branch and
// checkTaskBasis's combined guard, both of which read the identical gap as
// nil for their OWN callers); now it is Uncovered, never a candidate, and
// never counted in Compared, even though the parent-walk half alone would
// still (correctly, for ITS OWN purpose) answer clean.
func TestActiveBasisTaskWithDriftUnderAKeylessParentIsUncoveredNotClean(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	featID, taskID := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

	appendToFile(t, taskProsePath(root, "form-engine", "feat-active", "task-a"), "\nEdited after approval.\n")

	tree := loadTree(t, root)
	tree.Features[featID].Key = nil

	rep, err := drift.Run(tree, vcs.New(root))
	require.NoError(t, err)

	var found bool
	for _, u := range uncoveredFor(rep, drift.MarkerActiveBasis) {
		if u.ID == taskID {
			found = true
			require.Equal(t, "basis_unreadable", u.Reason)
			require.NotEmpty(t, u.Detail)
		}
	}
	require.True(t, found, "the task's real drift must not be swallowed by its unresolvable parent")

	for _, c := range candidatesFor(rep, drift.MarkerActiveBasis) {
		require.NotEqual(t, taskID, c.ID, "a node that could not be fully checked is never a candidate")
	}
	require.Equal(t, 0, rep.Compared, "neither the keyless feature nor its task was fully checked")
}

// TestActiveBasisKeylessTaskIsUncovered pins the other half of task-7-
// a task that itself carries no key (its
// own task.yml lost its key: line) cannot have its own task.md located
// either (store.TaskRelDir needs it), and must read as uncovered rather
// than silently clean.
func TestActiveBasisKeylessTaskIsUncovered(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, taskID := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

	tree := loadTree(t, root)
	tree.Tasks[taskID].Key = nil

	rep, err := drift.Run(tree, vcs.New(root))
	require.NoError(t, err)

	var found bool
	for _, u := range uncoveredFor(rep, drift.MarkerActiveBasis) {
		if u.ID == taskID {
			found = true
			require.Equal(t, "basis_unreadable", u.Reason)
		}
	}
	require.True(t, found, "a keyless task cannot be located, so it is uncovered rather than silently clean")

	for _, c := range candidatesFor(rep, drift.MarkerActiveBasis) {
		require.NotEqual(t, taskID, c.ID)
	}
}

// --- answered_question ---

// layFeatureWithAssumedQuestion records a feature assumption after plan
// approval, then answers it without reviewing that answer into the feature.
func layFeatureWithAssumedQuestion(t *testing.T, root string, deps mutate.Deps, featureKey string) (featureID, questionID string) {
	t.Helper()
	fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/" + featureKey, Title: featureKey, ChangeType: "create", Source: "design/x.md",
	})
	require.NoError(t, err)
	featureID = fRes.ID
	writeFeatureProse(t, root, "form-engine", featureKey, featureID)

	taskKey := featureKey + "-task"
	_, err = mutate.AddTask(loadTree(t, root), deps, mutate.TaskInput{
		Key: "form-engine/" + featureKey + "/" + taskKey, Title: taskKey,
	})
	require.NoError(t, err)
	writeTaskSpec(t, root, "form-engine", featureKey, taskKey)

	driveFeature(t, root, deps, featureID, "spec")
	driveFeature(t, root, deps, featureID, "approve")

	qRes, err := mutate.AddQuestion(loadTree(t, root), deps, mutate.QuestionInput{
		Title:     "Canonical schema version",
		Statement: "Which schema version is canonical for " + featureKey + "?",
	})
	require.NoError(t, err)
	questionID = qRes.ID

	tree := loadTree(t, root)
	f := tree.Features[featureID]
	_, err = mutate.Transition(tree, deps, f, mutate.TransitionInput{
		Verb: "approve", AssumeQuestion: questionID, Reason: "proceeding under a recorded assumption",
	})
	require.NoError(t, err)

	_, err = mutate.AnswerQuestion(loadTree(t, root), deps, vcs.New(root), mutate.AnswerInput{
		Node: questionID, Answer: "The 2025 revision; 2022 is read-only reference.",
	})
	require.NoError(t, err)

	return featureID, questionID
}

// TestAnsweredQuestionSweep is written out
// fully. A non-terminal feature with an answered, unreviewed assumption is a
// candidate carrying the question id, short title, and full statement;
// reviewing it in clears it; a DANGLING question id in assumption_basis is
// Uncovered with reason unknown_question, not a crash and not silence.
func TestAnsweredQuestionSweep(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerAnsweredQuestion})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	featAssumed, qAssumed := layFeatureWithAssumedQuestion(t, root, deps, "feat-assumed")
	featReviewed, qReviewed := layFeatureWithAssumedQuestion(t, root, deps, "feat-reviewed")
	_, _ = layFeatureWithAssumedQuestion(t, root, deps, "feat-done")

	// Reviewing the answer in clears feat-reviewed's entry.
	tree := loadTree(t, root)
	_, err = mutate.ReviewAssumption(tree, deps, tree.Features[featReviewed], qReviewed)
	require.NoError(t, err)

	// A terminal feature carrying the identical unreviewed shape must not
	// surface: the done case is deliberately out (its fix is an amendment, a
	// human judgment), not a status this marker's sweep ever reaches.
	setFeatureStatus(t, root, "form-engine", "feat-done", "done")

	tree = loadTree(t, root)
	wantStatement := tree.Questions[qAssumed].Statement
	wantTitle := tree.Questions[qAssumed].Title
	// This fixture answers on a dirty tree by construction (nothing above
	// ever commits), the NORMAL case: AnswerRevision is a
	// sha256 worktree fingerprint here, not a git_commit, so cs[0].Since
	// reads nil below rather than carrying that fingerprint dressed as a
	// commit anchor. TestAnsweredQuestionSinceOnlyGitCommitKind covers the
	// git_commit kind explicitly, on a tree committed clean before answering.
	require.Equal(t, store.KindSHA256, tree.Questions[qAssumed].AnswerRevision.Kind)

	// TWO dangling question ids (hand-edited assumption_basis entries naming
	// questions the ledger no longer carries): the predicate skips a dangling
	// reference silently (Task 5's contract), so this sweep has to notice it
	// separately, which is exactly what is under test here. Two on the same
	// feature must not collapse into one indistinguishable row.
	tree.Features[featAssumed].AssumptionBasis["Q-ghost-one"] = store.Assumption{Reason: "hand-edited"}
	tree.Features[featAssumed].AssumptionBasis["Q-ghost-two"] = store.Assumption{Reason: "hand-edited"}

	rep, err := drift.Run(tree, vcs.New(root))
	require.NoError(t, err)
	// Coverage buckets per FEATURE, matching Compared's own unit:
	// feat-reviewed is the only
	// feature with no dangling entry, so it alone is "checked" and counted;
	// feat-assumed carries dangling entries, so it is "unresolvable" and
	// NOT counted, even though it also produced a real candidate on its
	// other (real) entry; feat-done is terminal and never visited at all.
	require.Equal(t, 1, rep.Compared)
	require.Equal(t, 1, rep.Coverage[drift.MarkerAnsweredQuestion]["checked"])
	require.Equal(t, 1, rep.Coverage[drift.MarkerAnsweredQuestion]["unresolvable"],
		"feat-assumed carries TWO dangling entries but is ONE unresolvable feature, not two")

	cs := candidatesFor(rep, drift.MarkerAnsweredQuestion)
	require.Len(t, cs, 1,
		"the reviewed feature clears and the done feature is out of scope; only feat-assumed's unreviewed entry surfaces")
	require.Equal(t, store.NounFeature, cs[0].Noun)
	require.Equal(t, featAssumed, cs[0].ID)
	require.Equal(t, "form-engine/feat-assumed", cs[0].Key)
	require.Equal(t,
		fmt.Sprintf("%s %s (%s) was answered and has not been reviewed in", qAssumed, wantTitle, wantStatement),
		cs[0].Detail,
		"the candidate leads with id plus title and retains the full statement as review evidence")
	// Since is nil here, not the sha256 fingerprint AnswerRevision actually
	// holds: a fingerprint is not a commit a human or an agent can diff
	// from, so questionSince emits Since only for a git_commit-kind answer
	// revision (Since can be a fingerprint), the same "nothing
	// honest to date it by" reading a pre-anchor zero record already gets.
	require.Nil(t, cs[0].Since)

	us := uncoveredFor(rep, drift.MarkerAnsweredQuestion)
	require.Len(t, us, 2, "each dangling id is its own row, never silently dropped and never a crash")
	seenDetails := map[string]bool{}
	var sawGhostOne, sawGhostTwo bool
	for _, u := range us {
		require.Equal(t, featAssumed, u.ID)
		require.Equal(t, "unknown_question", u.Reason)
		require.NotEmpty(t, u.Detail)
		seenDetails[u.Detail] = true
		if strings.Contains(u.Detail, "Q-ghost-one") {
			sawGhostOne = true
		}
		if strings.Contains(u.Detail, "Q-ghost-two") {
			sawGhostTwo = true
		}
	}
	require.Len(t, seenDetails, 2, "two dangling ids on one feature must not read as one indistinguishable row")
	require.True(t, sawGhostOne, "the row names which id is dangling")
	require.True(t, sawGhostTwo, "the row names which id is dangling")
}

// TestAnsweredQuestionSinceOnlyGitCommitKind pins the finding that
// answered_question's Since could be a sha256 worktree fingerprint dressed up
// as a commit anchor: Question.AnswerRevision is written by mutate's own
// gateAgainst (check.go), which returns CurrentCommit() on a clean tree and a
// WorktreeFingerprint() otherwise, and answering a question mid-flight (the
// normal case; TestAnsweredQuestionSweep's own fixture never commits before
// answering) is dirty by construction. A fingerprint is not a commit a human
// or an agent can diff from (mend-drift/SKILL.md's own since sentence), so
// questionSince now emits Since only when AnswerRevision.Kind is git_commit;
// every other kind, including the zero pre-anchor value, reads as nil, the
// same "nothing to date it by" reading basisSince and staleSpecSince already
// give one marker over.
func TestAnsweredQuestionSinceOnlyGitCommitKind(t *testing.T) {
	t.Run("a clean tree at answer time yields a real, diffable commit Since", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerAnsweredQuestion})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
		require.NoError(t, err)

		fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
			Key: "form-engine/feat-clean", Title: "feat-clean", ChangeType: "create", Source: "design/x.md",
		})
		require.NoError(t, err)
		featureID := fRes.ID
		writeFeatureProse(t, root, "form-engine", "feat-clean", featureID)

		_, err = mutate.AddTask(loadTree(t, root), deps, mutate.TaskInput{
			Key: "form-engine/feat-clean/feat-clean-task", Title: "feat-clean-task",
		})
		require.NoError(t, err)
		writeTaskSpec(t, root, "form-engine", "feat-clean", "feat-clean-task")

		driveFeature(t, root, deps, featureID, "spec")
		driveFeature(t, root, deps, featureID, "approve")

		qRes, err := mutate.AddQuestion(loadTree(t, root), deps, mutate.QuestionInput{
			Title:     "Canonical schema version",
			Statement: "Which schema version is canonical for feat-clean?",
		})
		require.NoError(t, err)
		questionID := qRes.ID

		tree := loadTree(t, root)
		f := tree.Features[featureID]
		_, err = mutate.Transition(tree, deps, f, mutate.TransitionInput{
			Verb: "approve", AssumeQuestion: questionID, Reason: "proceeding under a recorded assumption",
		})
		require.NoError(t, err)

		// Commit everything so the tree is CLEAN at the moment the question is
		// answered: gateAgainst (mutate/check.go) reads a clean tree as HEAD
		// itself, a real git_commit anchor, never a worktree fingerprint.
		gitCommitAll(t, root, "lay tree, clean before answering")

		_, err = mutate.AnswerQuestion(loadTree(t, root), deps, vcs.New(root), mutate.AnswerInput{
			Node: questionID, Answer: "The 2025 revision; 2022 is read-only reference.",
		})
		require.NoError(t, err)

		tree = loadTree(t, root)
		require.Equal(t, store.KindGitCommit, tree.Questions[questionID].AnswerRevision.Kind,
			"the fixture's own precondition: answered on a clean tree")

		rep, err := drift.Run(tree, vcs.New(root))
		require.NoError(t, err)

		cs := candidatesFor(rep, drift.MarkerAnsweredQuestion)
		require.Len(t, cs, 1)
		require.NotNil(t, cs[0].Since, "a git_commit-kind answer revision is a real, diffable anchor")
		// require.Equal over a *RevisionRef is reflect.DeepEqual over its
		// embedded time.Time, the exact trap RevisionRef.SameAs exists to
		// avoid: a ledger round trip can
		// change ObservedAt's *Location pointer without changing the instant
		// it names, which DeepEqual would read as a difference and SameAs
		// correctly would not.
		require.True(t, tree.Questions[questionID].AnswerRevision.SameAs(*cs[0].Since))
	})

	t.Run("a dirty tree at answer time, the normal case, yields nil Since rather than a fingerprint", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerAnsweredQuestion})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
		require.NoError(t, err)

		featureID, questionID := layFeatureWithAssumedQuestion(t, root, deps, "feat-assumed")

		tree := loadTree(t, root)
		require.Equal(t, store.KindSHA256, tree.Questions[questionID].AnswerRevision.Kind,
			"the fixture's own precondition: every mutate verb above wrote uncommitted files, so the "+
				"tree was dirty at answer time, the NORMAL case, not an edge case")

		rep, err := drift.Run(tree, vcs.New(root))
		require.NoError(t, err)

		cs := candidatesFor(rep, drift.MarkerAnsweredQuestion)
		require.Len(t, cs, 1)
		require.Equal(t, featureID, cs[0].ID)
		require.Nil(t, cs[0].Since,
			"a sha256 worktree fingerprint is not a commit a human or an agent can diff from; nil is honest, a fingerprint dressed as Since was not")
	})
}

// --- stale_spec ---

// TestStaleSpecVanishFiresModifyDoesNot is written out fully. A done feature's
// recorded surface losing one file is the candidate the vanish rule exists for;
// the same file merely edited first is not, since on any active codebase that
// would be true of every done node within days (artifact-schema.md, Drift
// control hooks).
func TestStaleSpecVanishFiresModifyDoesNot(t *testing.T) {
	root, featID := scratchDoneFeatureWithSurface(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

	overwriteAndCommit(t, root, "app/calc/zone.rb", "class Zone\n  # changed body\nend")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec),
		"a modified surface is normal evolution; firing here would flag every done node within days")
	require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec))
	// The feature and its lone epic (rolled up done behind it) both compare
	// clean: two nodes, both found nothing candidate-worthy.
	require.Equal(t, 2, rep.Compared, "the comparison ran and found nothing candidate-worthy")

	// Delete the same file: the candidate the vanish rule exists for.
	removeAndCommit(t, root, "app/calc/zone.rb")

	rep, err = drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	cs := candidatesFor(rep, drift.MarkerStaleSpec)
	require.Len(t, cs, 1)
	require.Equal(t, store.NounFeature, cs[0].Noun)
	require.Equal(t, featID, cs[0].ID)
	require.Equal(t, "form-engine/feat-done", cs[0].Key)
	require.Contains(t, cs[0].Files, "app/calc/zone.rb")
	require.Contains(t, cs[0].Detail, "deleted")
	require.NotNil(t, cs[0].Since, "the completion record's commit anchor says since when")
}

// TestStaleSpecSpecLegAndExemptionsAndUncovered is written out fully as three
// subtests.
func TestStaleSpecSpecLegAndExemptionsAndUncovered(t *testing.T) {
	t.Run("editing feature.md under a done feature is a spec-leg candidate", func(t *testing.T) {
		root, featID := scratchDoneFeatureWithSurface(t)
		setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

		appendToFile(t, featurePath(root, "form-engine", "feat-done", "feature.md"), "\nEdited after done.\n")
		// The surface also moves under it, without vanishing: it rides as
		// Context beside the spec-leg candidate, never a candidate of its
		// own (the same vanish rule TestStaleSpecVanishFiresModifyDoesNot
		// pins on its own).
		overwriteAndCommit(t, root, "app/calc/project.rb", "class Project\n  # touched\nend")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		cs := candidatesFor(rep, drift.MarkerStaleSpec)
		require.Len(t, cs, 1)
		require.Equal(t, featID, cs[0].ID)
		// Files carries the full repo-relative path, the same namespace the
		// surface leg's own entries use;
		// Detail keeps the shorter basename for the prose sentence.
		require.Contains(t, cs[0].Files, ".tiller/epics/form-engine/features/feat-done/feature.md")
		require.Contains(t, cs[0].Detail, "feature.md")
		require.Contains(t, cs[0].Detail, "changed")
		require.NotEmpty(t, cs[0].Context,
			"the surface's own tree-hash movement rides as context beside the spec-leg candidate")
		require.Contains(t, cs[0].Context[0], "app/calc")
		require.NotNil(t, cs[0].Since)
	})

	t.Run("a superseded feature is exempt and counted, never compared", func(t *testing.T) {
		root, _ := scratchDoneFeatureWithSurface(t)
		setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

		// Edited too, so the exemption is provably a skip rather than
		// indistinguishable from "nothing to find" (no verb writes
		// superseded_by yet; setFeatureSupersededBy's own doc comment says
		// why this is hand-authored).
		appendToFile(t, featurePath(root, "form-engine", "feat-done", "feature.md"), "\nEdited after done.\n")
		setFeatureSupersededBy(t, root, "form-engine", "feat-done", "F-ghost01")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec),
			"superseded is exempt regardless of what also changed underneath it")
		require.Equal(t, 1, rep.Exempted[drift.MarkerStaleSpec])
		// The feature itself is exempt and never compared; its lone epic is
		// not superseded, so it alone accounts for the one clean comparison.
		require.Equal(t, 1, rep.Compared, "the exempt feature is never compared, only its unaffected epic is")
		require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec))
	})

	// The exemption's real writer, end to end. Every other superseded-feature
	// fixture in this file hand-writes superseded_by, because until the amendment
	// surface landed nothing could write it (setFeatureSupersededBy's own doc
	// comment records that gap). This drives the actual gate instead, so the
	// exemption is proved against the field the apply really produces rather than
	// against a hand-authored approximation of it.
	t.Run("a feature superseded by an applied amendment is exempt", func(t *testing.T) {
		root, featID := scratchDoneFeatureWithSurface(t)
		setManifestChecks(t, root, []string{drift.MarkerStaleSpec})
		// A second seqDeps would restart its counter and re-mint the event ids
		// the fixture's own already wrote, which the store refuses as a planned
		// write over an existing file. The token space is separated instead.
		deps := amendSeqDeps()

		// Edited after done, so the exemption is provably a skip rather than
		// indistinguishable from "nothing to find".
		appendToFile(t, featurePath(root, "form-engine", "feat-done", "feature.md"), "\nEdited after done.\n")

		aRes, err := mutate.AddAmendment(loadTree(t, root), deps, mutate.AmendmentInput{
			Key: "table-backed-forms", Title: "Replace the done feature"})
		require.NoError(t, err)
		amendment := loadTree(t, root).Amendments[aRes.ID]
		require.NotNil(t, amendment)

		// The replacement rides the amendment too, so the whole lineage is one
		// reviewed act and the staged key resolves at apply.
		const replacementKey = "form-engine/table-backed-forms"
		_, err = mutate.StageOperation(loadTree(t, root), deps, amendment, mutate.StageInput{
			Op: store.AmendmentOpAddFeature, Key: replacementKey,
			Title: "Table backed forms", ChangeType: "replace"})
		require.NoError(t, err)
		amendment = loadTree(t, root).Amendments[aRes.ID]
		_, err = mutate.StageOperation(loadTree(t, root), deps, amendment, mutate.StageInput{
			Op: store.AmendmentOpSupersede, Target: featID, By: replacementKey,
			Preconditions: map[string]string{"target_status": "done"}})
		require.NoError(t, err)
		amendment = loadTree(t, root).Amendments[aRes.ID]

		applied, err := mutate.ApproveAmendment(loadTree(t, root), deps, amendment)
		require.NoError(t, err)
		require.Len(t, applied.Applied, 2)

		superseded := loadTree(t, root).Features[featID]
		require.NotNil(t, superseded.SupersededBy, "the apply wrote the backward leg this sweep reads")
		require.Equal(t, "done", superseded.Status, "and left the done status alone")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec),
			"a feature an amendment superseded is drifted by construction and exempt")
		require.Equal(t, 1, rep.Exempted[drift.MarkerStaleSpec])
		require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec))
	})

	t.Run("a done node with no completion record is uncovered, never clean", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerStaleSpec})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		require.NoError(t, err)
		fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
			Key: "form-engine/hand-done", Title: "Hand done", ChangeType: "create", Source: "design/x.md",
		})
		require.NoError(t, err)
		featID := fRes.ID
		writeFeatureProse(t, root, "form-engine", "hand-done", featID)

		// No real arc reaches done without sync writing a completion record,
		// so a done node carrying none at all is hand-authored here on
		// purpose: it is the exact case the no_completion_record reason
		// exists for.
		setFeatureStatus(t, root, "form-engine", "hand-done", "done")

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec))
		require.Equal(t, 0, rep.Compared)
		us := uncoveredFor(rep, drift.MarkerStaleSpec)
		require.Len(t, us, 1)
		require.Equal(t, featID, us[0].ID)
		require.Equal(t, "no_completion_record", us[0].Reason)
		require.NotEmpty(t, us[0].Detail)
	})
}

// TestStaleSpecEpicGrainCandidateAndArchivedExemption pins the epic-grain
// twin of the feature tests above: reportStaleSpecEpic shares staleSpecFileLeg
// and staleSpecSurfacesLeg with the feature path, but has its own relDir and
// no eval leg, and archived is its own exemption rather than superseded's.
func TestStaleSpecEpicGrainCandidateAndArchivedExemption(t *testing.T) {
	root, _ := scratchDoneFeatureWithSurface(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

	tree := loadTree(t, root)
	require.Len(t, tree.Epics, 1, "one epic, rolled up done behind its one feature")
	var epicID string
	for id, e := range tree.Epics {
		epicID = id
		require.Equal(t, "done", e.Status, "the epic rolls up to done in the same sync as its lone feature")
	}

	appendToFile(t, filepath.Join(root, ".tiller", "epics", "form-engine", "epic.md"), "\nEdited after done.\n")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	cs := candidatesFor(rep, drift.MarkerStaleSpec)
	require.Len(t, cs, 1, "the epic's own spec leg fires exactly like the feature grain's")
	require.Equal(t, store.NounEpic, cs[0].Noun)
	require.Equal(t, epicID, cs[0].ID)
	// Files carries the full repo-relative path; Detail keeps the shorter
	// basename.
	require.Contains(t, cs[0].Files, ".tiller/epics/form-engine/epic.md")
	require.Contains(t, cs[0].Detail, "epic.md")
	require.NotNil(t, cs[0].Since)

	// Archiving clears it: exempt and counted, not silently absent.
	setEpicStatus(t, root, "form-engine", "archived")
	rep, err = drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec),
		"an archived epic is exempt, and the untouched feature beside it stays clean")
	require.Equal(t, 1, rep.Exempted[drift.MarkerStaleSpec])
}

// TestStaleSpecArchivedFromDraftEpicIsNeverExempt pins the finding
// that sweepStaleSpec's epic arm bumped Exempted[stale_spec] for ANY archived
// epic, including one that skipped done entirely, violating the "count only
// when otherwise in scope" rule sweepMissingSurface's and sweepOrphanEval's
// own cancelled arms already apply one section over. The lifecycle table
// (internal/core/lifecycle/table.go, store.NounEpic's own transitions) carries
// exactly one edge into archived, {From: "done", Verb: "archive"}; there is no
// draft/ready -> archived row, so a real verb sequence can never park an epic
// here straight from draft, and this fixture's hand-edit (setEpicStatus, this
// file's own established idiom for a status no real verb reaches in one step)
// is the ONLY way this state exists at all. Such an epic carries no
// verified_against, since no real sync crossing ever ran for it, which is
// exactly the signal the fix gates the exemption count on.
func TestStaleSpecArchivedFromDraftEpicIsNeverExempt(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "widget", Title: "Widget"})
	require.NoError(t, err)
	setEpicStatus(t, root, "widget", "archived")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	require.Equal(t, 0, rep.Exempted[drift.MarkerStaleSpec],
		"an epic archived without ever having been done carries no completion record, so it never otherwise qualified for this sweep's done set")
	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec))
	require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec))
	require.Equal(t, 0, rep.Compared)
}

// --- missing_surface ---

// TestMissingSurfaceFiresOnlyOnVanish is written out fully. A ready
// replace-feature whose recorded surface path is deleted from the working tree
// is a candidate; while the path exists, no candidate; a cancelled feature with
// a vanished path is exempt and counted.
func TestMissingSurfaceFiresOnlyOnVanish(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerMissingSurface})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)

	// A replace feature's specify gate refuses one with no recorded surface
	// (validateSpec, transition.go), so the path has to exist and the
	// surface has to be linked before spec ever runs.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "app", "legacy"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app/legacy/zone.rb"), []byte("class Zone\nend\n"), 0o644))
	gitCommitAll(t, root, "legacy code")

	fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/replace-zone", Title: "Replace zone", ChangeType: "replace", Source: "design/x.md",
	})
	require.NoError(t, err)
	featID := fRes.ID
	writeFeatureProse(t, root, "form-engine", "replace-zone", featID)

	// The recorded surface path is the file itself, not its parent
	// directory: missing_surface stats the exact recorded path, with no
	// inventory concept of its own (that is stale_spec's surface leg, over
	// verified_against's separately recorded Inventory), so a directory
	// would still stat clean after the one file inside it is gone.
	tree := loadTree(t, root)
	_, err = mutate.SetSurface(tree, deps, tree.Features[featID], []string{"app/legacy/zone.rb"})
	require.NoError(t, err)

	_, err = mutate.AddTask(loadTree(t, root), deps, mutate.TaskInput{
		Key: "form-engine/replace-zone/do-it", Title: "Do it",
	})
	require.NoError(t, err)
	writeTaskSpec(t, root, "form-engine", "replace-zone", "do-it")

	driveFeature(t, root, deps, featID, "spec")
	driveFeature(t, root, deps, featID, "approve")

	// While the path exists, no candidate.
	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Empty(t, candidatesFor(rep, drift.MarkerMissingSurface))
	require.Equal(t, 1, rep.Compared)

	// Delete the recorded surface path (a plain working-tree removal, no
	// commit needed: missing_surface stats the working tree, not HEAD).
	require.NoError(t, os.Remove(filepath.Join(root, "app/legacy/zone.rb")))

	rep, err = drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	cs := candidatesFor(rep, drift.MarkerMissingSurface)
	require.Len(t, cs, 1)
	require.Equal(t, store.NounFeature, cs[0].Noun)
	require.Equal(t, featID, cs[0].ID)
	require.Contains(t, cs[0].Files, "app/legacy/zone.rb")

	// A cancelled feature with a vanished path is exempt and counted, not a
	// second candidate.
	setFeatureStatus(t, root, "form-engine", "replace-zone", "cancelled")
	rep, err = drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Empty(t, candidatesFor(rep, drift.MarkerMissingSurface))
	require.Equal(t, 1, rep.Exempted[drift.MarkerMissingSurface])
	require.Equal(t, 0, rep.Compared, "a cancelled node is exempt, never compared")
}

// TestMissingSurfaceCancelledExemptionGatedOnCarryingASurface pins
// A cancelled node that never carried a
// surface is silently out of scope, symmetric with a non-cancelled
// surfaceless node, rather than inflating the footer for a set it was never
// in.
func TestMissingSurfaceCancelledExemptionGatedOnCarryingASurface(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerMissingSurface})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/cancelled-bare", Title: "Cancelled, no surface ever recorded", ChangeType: "create", Source: "design/x.md",
	})
	require.NoError(t, err)
	setFeatureStatus(t, root, "form-engine", "cancelled-bare", "cancelled")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Equal(t, 0, rep.Exempted[drift.MarkerMissingSurface],
		"a cancelled node that never carried a surface is out of scope, not exempt: it was never in the marker's set to begin with")
	require.Equal(t, 0, rep.Compared)
	require.Empty(t, candidatesFor(rep, drift.MarkerMissingSurface))
}

// TestStaleSpecWholeSurfaceDirectoryVanishIsOneCandidateNotUncovered pins the
// vanish rule's defining boundary:
// stat-before-TreeHash is what keeps a fully vanished surface directory a
// plain candidate naming every gone file, rather than a git failure reading
// a tree that no longer resolves at HEAD. Inverting that order (TreeHash
// first, Uncovered on its failure) passes every other committed test, since
// TestStaleSpecVanishFiresModifyDoesNot only ever removes one of two
// inventory files, leaving the directory itself, and so the tree, resolvable
// either way; this test removes the whole directory, which only the correct
// ordering survives.
func TestStaleSpecWholeSurfaceDirectoryVanishIsOneCandidateNotUncovered(t *testing.T) {
	root, featID := scratchDoneFeatureWithSurface(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

	require.NoError(t, os.RemoveAll(filepath.Join(root, "app", "calc")))
	gitCommitAll(t, root, "remove the whole surface directory")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec),
		"every inventory file vanishing together is still a plain candidate, never a git failure")
	cs := candidatesFor(rep, drift.MarkerStaleSpec)
	require.Len(t, cs, 1)
	require.Equal(t, featID, cs[0].ID)
	require.ElementsMatch(t, []string{"app/calc/project.rb", "app/calc/zone.rb"}, cs[0].Files)
}

// TestStaleSpecSurfaceTreeHashFailureIsUncoveredWithItsOwnReason pins
// The surface leg's own hard
// failures must report reason surface_unreadable, not spec_unreadable
// (Task 10 groups by Reason, and the two are different questions to route to
// a human). The failure is injected through treeHashFailsRev rather than a
// chmod repro, since the recorded path here still exists and is fully
// present on disk; only the git-internal tree-hash read is made to fail.
func TestStaleSpecSurfaceTreeHashFailureIsUncoveredWithItsOwnReason(t *testing.T) {
	root, featID := scratchDoneFeatureWithSurface(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

	rev := treeHashFailsRev{Revisions: vcs.New(root), path: "app/calc"}
	rep, err := drift.Run(loadTree(t, root), rev)
	require.NoError(t, err)

	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec),
		"nothing vanished and nothing could be read, so there is no finding to report")
	us := uncoveredFor(rep, drift.MarkerStaleSpec)
	var found bool
	for _, u := range us {
		if u.ID == featID {
			found = true
			require.Equal(t, "surface_unreadable", u.Reason)
			require.NotEmpty(t, u.Detail)
			require.Contains(t, u.Detail, "app/calc")
		}
	}
	require.True(t, found, "a git failure reading the surface's tree hash is uncovered, not silently clean")
	// The feature itself is never compared; its lone epic carries no surface
	// of its own, so treeHashFailsRev never intercepts anything for it, and
	// it alone accounts for the one clean comparison.
	require.Equal(t, 1, rep.Compared)
}

// TestStaleSpecGreenfieldDoneFeatureWithNoSurfaceIsComparedAndClean pins
// The default, greenfield-create
// path (no surface ever recorded) must compare clean, never uncovered.
// verifiedAgainstEmpty keys on every VerifiedAgainst leg reading empty, not
// Surfaces alone, so a real record whose only empty field is Surfaces (this
// one) is correctly read as a real, if partial, completion record rather
// than no record at all.
func TestStaleSpecGreenfieldDoneFeatureWithNoSurfaceIsComparedAndClean(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})
	deps := seqDeps()

	epicRes, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)
	approveEpic(t, root, deps, epicRes.ID)

	featID, _ := driveFeatureToDoneGit(t, root, deps, "form-engine", "feat-plain", "task-a", nil)

	tree := loadTree(t, root)
	require.NotNil(t, tree.Features[featID].VerifiedAgainst.Surfaces,
		"recordVerified always writes surfaces: [] even when there is nothing to record")
	require.Empty(t, tree.Features[featID].VerifiedAgainst.Surfaces)

	rep, err := drift.Run(tree, vcs.New(root))
	require.NoError(t, err)

	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec))
	require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec))
	// The feature and its lone epic (also rolled up done) both compare clean.
	require.Equal(t, 2, rep.Compared)
}

// TestStaleSpecSurfaceLegAccumulatesFoundAndFailedAcrossFingerprints pins
// A two-surface feature where one
// fingerprint fully vanished and the other is unreadable must report BOTH,
// the vanished one as an actionable Candidate and the unreadable one as
// Uncovered, rather than the failure silently discarding the finding. The
// node lands in both lists at once, the one deliberate exception to every
// other Uncovered arm in this package meaning "no Candidate at all".
func TestStaleSpecSurfaceLegAccumulatesFoundAndFailedAcrossFingerprints(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})
	deps := seqDeps()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "app", "calc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app/calc/zone.rb"), []byte("class Zone\nend\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "app", "other"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app/other/thing.rb"), []byte("class Thing\nend\n"), 0o644))

	epicRes, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)
	approveEpic(t, root, deps, epicRes.ID)

	featID, _ := driveFeatureToDoneGit(t, root, deps, "form-engine", "feat-two-surfaces", "task-a", []string{"app/calc", "app/other"})

	// Fully vanish one surface, committed: the actionable candidate.
	require.NoError(t, os.RemoveAll(filepath.Join(root, "app", "calc")))
	gitCommitAll(t, root, "remove app/calc entirely")

	// The other surface stays fully present; only its tree-hash read is made
	// to fail, isolating the failure to one fingerprint.
	rev := treeHashFailsRev{Revisions: vcs.New(root), path: "app/other"}

	rep, err := drift.Run(loadTree(t, root), rev)
	require.NoError(t, err)

	cs := candidatesFor(rep, drift.MarkerStaleSpec)
	require.Len(t, cs, 1, "the vanished surface still produces its candidate despite the other surface's failure")
	require.Equal(t, featID, cs[0].ID)
	require.ElementsMatch(t, []string{"app/calc/zone.rb"}, cs[0].Files)

	us := uncoveredFor(rep, drift.MarkerStaleSpec)
	var found bool
	for _, u := range us {
		if u.ID == featID {
			found = true
			require.Equal(t, "surface_unreadable", u.Reason)
			require.Contains(t, u.Detail, "app/other")
		}
	}
	require.True(t, found,
		"the same node appears in both lists: the vanish is actionable now, the other surface still needs a human look")
	// The feature itself is never compared (its surface leg failed); its
	// lone epic carries no surface of its own, so treeHashFailsRev never
	// intercepts anything for it, and it alone accounts for the one clean
	// comparison.
	require.Equal(t, 1, rep.Compared,
		"any surface-leg failure keeps the node out of Compared, even though it also produced a candidate")
}

// --- inert markers ---
//
// TestInertMarkersContributeNothingYet lived here through Tasks 7 and 8: cheap
// insurance that a known marker name Run
// validates against, but which has no sweep function wired into its dispatch
// yet, contributes nothing silently when enabled, rather than crashing or
// fabricating output. stale_spec and missing_surface left the set that test
// iterated at Task 8; Task 9 gives orphan_eval, the fifth and last, a real
// sweep, so the set is now permanently empty: every name Run's checks-list
// validates against has a real case in its dispatch, and there is no marker
// left this binary can enable but not run.
//
// The test is removed here rather than retargeted to loop over an empty
// slice: a loop with nothing left to iterate asserts nothing beyond what
// TestRunHonorsTheChecksList and TestRunWithEmptyChecksDisablesAllFive
// already cover for Disabled, and dead-bodied test data is worse insurance
// than no test at all. The day a sixth marker joins the schema, its own task
// should add a test exactly like this one beside its own sweep (the same way
// this comment, not a resurrected loop, is what marks where that insurance
// used to live).

// --- orphan_eval ---

// layFeatureWithEvalToApproved adds a feature and its one task under epicKey
// (already added), writes a real feature.md plus a CALLER-SUPPLIED
// feature.eval.md, and drives spec -> approve, stopping at "ready": approved
// but not started. That is the polarity clock's own approved-not-done
// midpoint (AcceptedAgainst non-zero, VerifiedAgainst still zero), which
// orphan_eval's own fixtures need to inspect on its own before any git
// branch exists, with contracts shaped around specific bindings and
// polarities rather than writeFeatureProse's generic one-criterion default.
// The eval file's own `node:` field is never cross-checked against the
// feature's real minted id anywhere in the load or spec-validation path
// (grep confirms transition.go never reads ef.Node), so a fixed placeholder
// in the caller's evalContent is fine.
func layFeatureWithEvalToApproved(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey, taskKey, evalContent string) (featureID, taskID string) {
	t.Helper()
	fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: epicKey + "/" + featureKey, Title: featureKey, ChangeType: "create", Source: "design/x.md",
	})
	require.NoError(t, err)
	featureID = fRes.ID

	dir := filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# "+featureKey+"\n\nReal specification prose for "+featureKey+".\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(evalContent), 0o644))

	tRes, err := mutate.AddTask(loadTree(t, root), deps, mutate.TaskInput{
		Key: epicKey + "/" + featureKey + "/" + taskKey, Title: taskKey,
	})
	require.NoError(t, err)
	taskID = tRes.ID
	writeTaskSpec(t, root, epicKey, featureKey, taskKey)

	driveFeature(t, root, deps, featureID, "spec")
	driveFeature(t, root, deps, featureID, "approve")
	return featureID, taskID
}

// carryApprovedFeatureToDoneGit continues a feature already approved
// (layFeatureWithEvalToApproved's own endpoint) through start and a task's
// own branch, check, and submit, followed by a no-ff merge into main and
// sync. The merge starts the introduce polarity clock when sync writes
// verified_against.
func carryApprovedFeatureToDoneGit(t *testing.T, root string, deps mutate.Deps, epicKey, featureKey, taskKey, featureID, taskID string) {
	t.Helper()

	driveTask(t, root, deps, taskID, "approve")
	driveTask(t, root, deps, taskID, "start")
	branch := loadTree(t, root).Tasks[taskID].Branch
	require.NotEmpty(t, branch)

	gitCommitAll(t, root, "lay tree")

	gitRun(t, root, "checkout", "-b", branch, "main")
	require.NoError(t, os.WriteFile(filepath.Join(root, "work-"+taskKey+".txt"), []byte("work\n"), 0o644))
	gitCommitAll(t, root, "task work")

	tree := loadTree(t, root)
	_, err := mutate.Check(tree, deps, vcs.New(root), tree.Tasks[taskID], mutate.CheckInput{})
	require.NoError(t, err)

	tree = loadTree(t, root)
	pr := 41
	_, err = mutate.Transition(tree, deps, tree.Tasks[taskID], mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.NoError(t, err)

	// Record review state on main before the human merges this task's work.
	gitRun(t, root, "checkout", "main")
	gitCommitAll(t, root, "record task review")
	gitRun(t, root, "merge", "--no-ff", "-m", "merge "+taskKey, branch)

	_, err = mutate.Sync(loadTree(t, root), deps, vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
}

// TestOrphanEvalPolarityClock is written out
// fully. An APPROVED (not done) feature's preserve criterion naming a
// deleted baseline artifact is a candidate: preserve names what exists
// before the work, so it is checked from accepted_against onward. The same
// feature's introduce criterion naming a not-yet-written test is NOT a
// candidate while the feature is only approved: introduce is checked only
// once verified_against exists, because a contract naming a test before the
// test is written is the workflow working, not drift (artifact-schema.md,
// the orphan_eval bullet). Driving the feature to done starts the introduce
// clock; while the test file is still present, there is still no candidate
// for it (the clock starting must not by itself manufacture one); deleting
// it afterward is what finally makes it one, merged into the SAME one
// candidate per node the preserve leg already produced, anchored to the
// earlier of the two clocks.
func TestOrphanEvalPolarityClock(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	const evalContent = `---
node: F-placeholder
criteria:
  - id: C-preserve
    statement: existing zone calculations are unchanged
    polarity: preserve
    binding: { type: command, run: "true" }
    baseline:
      surface: app/calc
      artifact: fixtures/calc/zone-baseline.json
  - id: C-introduce
    statement: the new discount code applies correctly
    polarity: introduce
    binding: { type: test, ref: spec/calc/discount_spec.rb }
---

# Acceptance
`

	require.NoError(t, os.MkdirAll(filepath.Join(root, "fixtures", "calc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "fixtures/calc/zone-baseline.json"), []byte("{}\n"), 0o644))

	featID, taskID := layFeatureWithEvalToApproved(t, root, deps, "form-engine", "feat-orphan", "task-a", evalContent)

	// Delete the preserve criterion's baseline artifact: it names what
	// existed before the work, so it is in scope from approval onward. The
	// introduce criterion's test file is simply never created yet, matching
	// the brief's own "not-yet-written test" framing exactly.
	require.NoError(t, os.Remove(filepath.Join(root, "fixtures/calc/zone-baseline.json")))

	tree := loadTree(t, root)
	acceptedCommit := tree.Features[featID].AcceptedAgainst.Commit
	require.NotEmpty(t, acceptedCommit.Value, "approve must have pinned a commit anchor")

	rep, err := drift.Run(tree, vcs.New(root))
	require.NoError(t, err)

	cs := candidatesFor(rep, drift.MarkerOrphanEval)
	require.Len(t, cs, 1, "only the preserve leg's baseline miss is in scope; the introduce leg's clock has not started")
	require.Equal(t, featID, cs[0].ID)
	require.Contains(t, cs[0].Files, "fixtures/calc/zone-baseline.json")
	require.Contains(t, cs[0].Detail, "C-preserve")
	require.NotContains(t, cs[0].Detail, "C-introduce", "the introduce criterion is skipped, not merely clean")
	require.NotContains(t, cs[0].Files, "spec/calc/discount_spec.rb")
	require.NotNil(t, cs[0].Since)
	require.True(t, acceptedCommit.SameAs(*cs[0].Since), "a preserve-only finding anchors to accepted_against's commit")

	require.Equal(t, 1, rep.Coverage[drift.MarkerOrphanEval]["not_yet_in_scope"],
		"the introduce criterion is skipped, one coverage count, not silently absent")
	require.Equal(t, 1, rep.Compared, "the one in-scope feature was fully read despite the skip")

	// Write the introduce test file, representing the agent's actual task
	// work; the "lay tree" commit inside carryApprovedFeatureToDoneGit sweeps
	// it up along with everything else via `git add -A`. Then drive the
	// feature all the way to done, starting the introduce clock.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "spec", "calc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "spec/calc/discount_spec.rb"), []byte("# spec\n"), 0o644))

	carryApprovedFeatureToDoneGit(t, root, deps, "form-engine", "feat-orphan", "task-a", featID, taskID)

	tree = loadTree(t, root)
	verifiedCommit := tree.Features[featID].VerifiedAgainst.Commit
	require.NotEmpty(t, verifiedCommit.Value, "sync must have pinned a completion commit anchor")
	require.False(t, acceptedCommit.SameAs(verifiedCommit), "approval and the done crossing must be different commits in this fixture")

	// Still clean for the introduce leg while its test file is present: the
	// clock starting must not by itself manufacture a candidate.
	repClean, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	csClean := candidatesFor(repClean, drift.MarkerOrphanEval)
	require.Len(t, csClean, 1, "the preserve leg's own candidate persists; the introduce leg's ref exists, so it contributes nothing new")
	require.NotContains(t, csClean[0].Detail, "C-introduce")

	// Now delete the introduce criterion's test file: the clock has started,
	// so a missing ref is finally a candidate.
	require.NoError(t, os.Remove(filepath.Join(root, "spec/calc/discount_spec.rb")))

	repDone, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	csDone := candidatesFor(repDone, drift.MarkerOrphanEval)
	require.Len(t, csDone, 1, "both misses merge into the SAME one candidate per node")
	require.Equal(t, featID, csDone[0].ID)
	require.ElementsMatch(t, []string{"fixtures/calc/zone-baseline.json", "spec/calc/discount_spec.rb"}, csDone[0].Files)
	require.Contains(t, csDone[0].Detail, "C-preserve")
	require.Contains(t, csDone[0].Detail, "C-introduce")
	require.NotNil(t, csDone[0].Since)
	require.True(t, acceptedCommit.SameAs(*csDone[0].Since),
		"a candidate mixing both clocks anchors to accepted_against, the earlier of the two, even once verified_against also exists")
}

// TestOrphanEvalCoverageIsStated is written out
// fully as two subtests. The stated coverage numbers are corrected from the
// brief's own draft, which put out_of_scope at 1: that is arithmetically
// impossible once a command binding AND a manual binding are both
// out_of_scope in the same contract.
func TestOrphanEvalCoverageIsStated(t *testing.T) {
	t.Run("one criterion of each kind yields the exact stated coverage", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
		require.NoError(t, err)

		require.NoError(t, os.MkdirAll(filepath.Join(root, "spec", "calc"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "spec/calc/existing_spec.rb"), []byte("# spec\n"), 0o644))

		const evalContent = `---
node: F-placeholder
criteria:
  - id: C-command
    statement: a command binding, out of scope for existence checks
    polarity: preserve
    binding: { type: command, run: "true" }
  - id: C-path
    statement: a path-shaped test ref that exists on disk
    polarity: preserve
    binding: { type: test, ref: spec/calc/existing_spec.rb }
  - id: C-bare
    statement: a bare test name, not a path
    polarity: preserve
    binding: { type: test, ref: CalcZoneTest }
  - id: C-manual
    statement: a human confirms the report reads naturally
    polarity: preserve
    binding: { type: manual }
---

# Acceptance
`
		_, _ = layFeatureWithEvalToApproved(t, root, deps, "form-engine", "feat-coverage", "task-a", evalContent)

		rep, err := drift.Run(loadTree(t, root), vcs.New(root))
		require.NoError(t, err)

		require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval), "every checkable ref exists or is out of scope; nothing missing")
		require.Empty(t, uncoveredFor(rep, drift.MarkerOrphanEval))
		require.Equal(t, 1, rep.Compared)

		cov := rep.Coverage[drift.MarkerOrphanEval]
		// No baseline is present on any criterion in this contract, so it
		// contributes nothing beyond the four bindings themselves: "plus
		// whatever the baseline contributes" is zero here, by
		// construction.
		require.Equal(t, 1, cov["checked"], "the path-shaped ref, present on disk")
		require.Equal(t, 1, cov["unresolvable"], "the bare test name, an argument to the tests runner, not a path")
		require.Equal(t, 2, cov["out_of_scope"], "the command binding and the manual binding")
		require.Equal(t, 0, cov["not_yet_in_scope"], "every criterion here is preserve on an approved feature; its clock has started")
	})

	t.Run("a malformed eval file is Uncovered, not a crash", func(t *testing.T) {
		root := scratchProjectGit(t)
		setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
		deps := seqDeps()

		_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
		require.NoError(t, err)

		featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-malformed", "task-a")

		// Overwrite the valid contract layActiveFeature already wrote (via
		// writeFeatureProse) with something that is not even frontmatter.
		// HasEval is set from directory layout at Load time, the file's mere
		// presence, independent of its content, so this feature stays in
		// orphan_eval's sweep set while carrying nothing the sweep can parse.
		require.NoError(t, os.WriteFile(
			featurePath(root, "form-engine", "feat-malformed", "feature.eval.md"),
			[]byte("not even yaml frontmatter\n"), 0o644))

		var rep drift.Report
		require.NotPanics(t, func() {
			rep, err = drift.Run(loadTree(t, root), vcs.New(root))
		})
		require.NoError(t, err)

		require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval))
		require.Equal(t, 0, rep.Compared)

		us := uncoveredFor(rep, drift.MarkerOrphanEval)
		require.Len(t, us, 1)
		require.Equal(t, featID, us[0].ID)
		require.Equal(t, "eval_unreadable", us[0].Reason)
		require.NotEmpty(t, us[0].Detail)
	})
}

// TestOrphanEvalRefStatFailureIsUncoveredNotAMissingCandidate pins the hard-
// failure arm orphanEvalCheckPath carries, beyond either test the brief
// sketched: a path-shaped ref under a directory with no search permission is
// neither "missing" (which would misreport a permission fault as a real
// candidate, false evidence of drift) nor silence. It is Uncovered with its
// own reason, ref_unreadable, deliberately distinct from eval_unreadable (the
// whole-file load/validate failure orphan_eval's other Uncovered arm
// carries), and a sibling criterion's own genuine miss on the SAME node
// still survives beside it rather than being swallowed.
func TestOrphanEvalRefStatFailureIsUncoveredNotAMissingCandidate(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits, so this repro needs a non-root process")
	}

	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	lockedDir := filepath.Join(root, "locked")
	require.NoError(t, os.MkdirAll(lockedDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lockedDir, "spec.rb"), []byte("# spec\n"), 0o644))

	const evalContent = `---
node: F-placeholder
criteria:
  - id: C-locked
    statement: a ref under a directory this process cannot search
    polarity: preserve
    binding: { type: test, ref: locked/spec.rb }
  - id: C-missing
    statement: a genuinely missing ref on the same node
    polarity: preserve
    binding: { type: test, ref: spec/calc/gone_spec.rb }
---

# Acceptance
`
	featID, _ := layFeatureWithEvalToApproved(t, root, deps, "form-engine", "feat-locked", "task-a", evalContent)

	require.NoError(t, os.Chmod(lockedDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) })

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	us := uncoveredFor(rep, drift.MarkerOrphanEval)
	require.Len(t, us, 1)
	require.Equal(t, featID, us[0].ID)
	require.Equal(t, "ref_unreadable", us[0].Reason)
	require.Contains(t, us[0].Detail, "locked/spec.rb")

	cs := candidatesFor(rep, drift.MarkerOrphanEval)
	require.Len(t, cs, 1, "the sibling criterion's genuine miss survives the other criterion's hard failure")
	require.Contains(t, cs[0].Files, "spec/calc/gone_spec.rb")
	require.NotContains(t, cs[0].Files, "locked/spec.rb", "a permission fault is not a candidate; guessing it is missing would be a false positive")

	require.Equal(t, 0, rep.Compared, "a hard failure on any one ref keeps the whole node out of Compared")
}

// TestOrphanEvalCancelledFeatureIsExemptWhenInScope pins that the schema's
// exemption paragraph binds every sweep (artifact-schema.md, Drift control
// hooks: "a node is exempt from a sweep only when a human recorded a decision
// that makes the drift expected"), and Task 8's markers honor it, but
// orphan_eval's own brief never asked, so sweepOrphanEval had no status filter
// at all. Without the fix, a cancelled feature is swept exactly like a live one
// and produces an actionable- looking candidate for a node a human already took
// off the table, one that can never self-clear (cancelled is terminal). This is
// the reviewer's own end-to-end reproduction, with the REAL cancel transition
// (legal from ready), as the fixture recipe.
func TestOrphanEvalCancelledFeatureIsExemptWhenInScope(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "fixtures", "calc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "fixtures/calc/zone-baseline.json"), []byte("{}\n"), 0o644))

	const evalContent = `---
node: F-placeholder
criteria:
  - id: C-preserve
    statement: existing zone calculations are unchanged
    polarity: preserve
    binding: { type: command, run: "true" }
    baseline:
      surface: app/calc
      artifact: fixtures/calc/zone-baseline.json
---

# Acceptance
`
	featID, _ := layFeatureWithEvalToApproved(t, root, deps, "form-engine", "feat-cancelled", "task-a", evalContent)

	tree := loadTree(t, root)
	_, err = mutate.Transition(tree, deps, tree.Features[featID], mutate.TransitionInput{Verb: "cancel", Reason: "subsumed by a redesign"})
	require.NoError(t, err)

	// Delete the baseline artifact AFTER cancellation: without the fix, this
	// alone would still read as an actionable candidate for a node a human
	// already took off the table.
	require.NoError(t, os.Remove(filepath.Join(root, "fixtures/calc/zone-baseline.json")))

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval),
		"a cancelled feature is exempt, never a candidate, however drifted its eval file now looks")
	require.Equal(t, 1, rep.Exempted[drift.MarkerOrphanEval])
	require.Equal(t, 0, rep.Compared, "the exempt feature is never compared")
	require.Empty(t, uncoveredFor(rep, drift.MarkerOrphanEval))
}

// TestOrphanEvalCancelledFeatureExemptionGatedOnScope pins the gating half of
// Finding 1, matching Task 8's own Minor-4 precedent
// (TestMissingSurfaceCancelledExemptionGatedOnCarryingASurface, one section
// up): a cancelled feature that was never in this marker's scope to begin
// with (no eval file at all here) must not inflate Exempted, symmetrically
// with the silent out-of-scope every non-cancelled, eval-less feature
// already gets.
func TestOrphanEvalCancelledFeatureExemptionGatedOnScope(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/feat-bare", Title: "feat-bare", ChangeType: "create", Source: "design/x.md",
	})
	require.NoError(t, err)
	featID := fRes.ID
	// No eval file is ever written for this feature: it stays in "draft",
	// HasEval stays false at Load, and cancel (legal straight from draft) is
	// the only transition it ever makes.

	tree := loadTree(t, root)
	_, err = mutate.Transition(tree, deps, tree.Features[featID], mutate.TransitionInput{Verb: "cancel", Reason: "never specced"})
	require.NoError(t, err)

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	require.Equal(t, 0, rep.Exempted[drift.MarkerOrphanEval],
		"a cancelled feature that never carried an eval file was never in this marker's scope to begin with")
	require.Equal(t, 0, rep.Compared)
	require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval))
}

// TestOrphanEvalKeylessFeatureIsUncoveredNotAPanic pins the keyless case,
// copy-adapted from active_basis's own
// TestActiveBasisSweepReportsAKeylessFeatureAsUncoveredNotAPanic: gen.EvalPath
// does not itself guard a *store.Feature against a nil key. path.Join's own
// Clean silently absorbs the resulting empty key segment (path.Join ignores
// empty elements), so the derived path is not malformed at all, just wrong: it
// resolves one directory level too shallow, ".../features/feature.eval.md"
// rather than ".../features/<key>/feature.eval.md", since the local feature-key
// segment that should have named the last directory is simply missing from it.
// reportOrphanEval guards f.Key == nil explicitly before ever calling EvalPath.
// Without that guard the sweep would still degrade safely (the shallow path
// simply fails to load, landing in Uncovered{eval_unreadable} via a plain
// ENOENT), but with a misleading Detail rather than an honest one; this pins
// the guard's own honest message.
func TestOrphanEvalKeylessFeatureIsUncoveredNotAPanic(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	const evalContent = `---
node: F-placeholder
criteria:
  - id: C1
    statement: a bare check
    polarity: preserve
    binding: { type: command, run: "true" }
---

# Acceptance
`
	featID, _ := layFeatureWithEvalToApproved(t, root, deps, "form-engine", "feat-keyless", "task-a", evalContent)

	tree := loadTree(t, root)
	tree.Features[featID].Key = nil

	var rep drift.Report
	require.NotPanics(t, func() {
		rep, err = drift.Run(tree, vcs.New(root))
	})
	require.NoError(t, err)

	var found bool
	for _, u := range uncoveredFor(rep, drift.MarkerOrphanEval) {
		if u.ID == featID {
			found = true
			require.Equal(t, "eval_unreadable", u.Reason)
			require.NotEmpty(t, u.Detail, "the row names the specific thing, not just a bare reason code")
			require.Contains(t, u.Detail, "key", "the guard's own honest message, not a misleading ENOENT string off the wrong, too-shallow path")
		}
	}
	require.True(t, found, "the keyless feature is reported uncovered, not silently skipped")
	require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval), "there is nothing to name as drifted; it could not be compared at all")
	require.Equal(t, 0, rep.Compared)
}

// TestOrphanEvalCancelledFeatureWithUnstartedClockIsNeverExempt pins fix
// round 2's finding: cancel is legal for a feature directly from "specced"
// (lifecycle table.go), and validateSpec requires a real, well-formed eval
// file before "specced" is ever reached, while approve is what starts the
// preserve clock, only after. So "eval file present, clock never started,
// cancelled" is buildable through real verbs alone, with no approve ever
// run: add feature, write a real feature.md and feature.eval.md, spec,
// cancel. This is a DIFFERENT fixture from
// TestOrphanEvalCancelledFeatureExemptionGatedOnScope (which has no eval
// file at all, so HasEval is false there): here HasEval is true, so a
// plausible wrong fix that exempts any cancelled feature carrying an eval
// file, without checking whether either clock actually started, would
// wrongly exempt this node too. A node whose clock never started was never
// in this marker's scope to begin with (the footer-honesty rule the
// Exempted gate exists for),
// so Exempted must read zero here, not one.
func TestOrphanEvalCancelledFeatureWithUnstartedClockIsNeverExempt(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/feat-specced-cancel", Title: "feat-specced-cancel", ChangeType: "create", Source: "design/x.md",
	})
	require.NoError(t, err)
	featID := fRes.ID
	writeFeatureProse(t, root, "form-engine", "feat-specced-cancel", featID)

	_, err = mutate.AddTask(loadTree(t, root), deps, mutate.TaskInput{
		Key: "form-engine/feat-specced-cancel/task-a", Title: "task-a",
	})
	require.NoError(t, err)
	writeTaskSpec(t, root, "form-engine", "feat-specced-cancel", "task-a")

	driveFeature(t, root, deps, featID, "spec")

	tree := loadTree(t, root)
	f := tree.Features[featID]
	require.Equal(t, "specced", f.Status)
	require.True(t, f.HasEval, "spec requires a real, well-formed eval file; writeFeatureProse wrote one")
	require.Empty(t, f.AcceptedAgainst.Commit.Value, "approve never ran; the preserve clock never started")
	require.Empty(t, f.VerifiedAgainst.Commit.Value, "done never happened; the introduce clock never started")

	_, err = mutate.Transition(tree, deps, f, mutate.TransitionInput{Verb: "cancel", Reason: "cut before approval"})
	require.NoError(t, err)

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	require.Empty(t, candidatesFor(rep, drift.MarkerOrphanEval))
	require.Empty(t, uncoveredFor(rep, drift.MarkerOrphanEval))
	require.Equal(t, 0, rep.Compared)
	require.Equal(t, 0, rep.Exempted[drift.MarkerOrphanEval],
		"a node whose clock never started was never in this marker's scope; exempting on cancelled-plus-HasEval alone would wrongly count it")
}

// TestOrphanEvalMissingRefBucketsAsChecked pins the finding that
// orphan_eval never buckets a missing criterion at all: orphanEvalCheckPath's
// own doc comment claimed finishOrphanEval decided checked vs candidate for
// it, but finishOrphanEval never bumped any Coverage bucket for a missing
// ref, so a two-criterion contract where one criterion's ref vanished and the
// other's clock had not started read "1 criteria not yet in scope" and
// accounted for only one of its two criteria. The missing arm now bumps
// checked, uniform with every other marker in this file, where a candidate-
// producing node (here, criterion) still counts as checked, not skipped.
func TestOrphanEvalMissingRefBucketsAsChecked(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerOrphanEval})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)

	const evalContent = `---
node: F-placeholder
criteria:
  - id: C-vanished
    statement: a path-shaped ref that no longer exists
    polarity: preserve
    binding: { type: test, ref: spec/calc/vanished_spec.rb }
  - id: C-future
    statement: a criterion whose clock has not started
    polarity: introduce
    binding: { type: test, ref: spec/calc/future_spec.rb }
---

# Acceptance
`
	featID, _ := layFeatureWithEvalToApproved(t, root, deps, "form-engine", "feat-coverage", "task-a", evalContent)

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	cs := candidatesFor(rep, drift.MarkerOrphanEval)
	require.Len(t, cs, 1)
	require.Equal(t, featID, cs[0].ID)
	require.Contains(t, cs[0].Files, "spec/calc/vanished_spec.rb")

	cov := rep.Coverage[drift.MarkerOrphanEval]
	require.Equal(t, 1, cov["checked"], "the vanished ref still counts as checked: the stat ran and completed, it just found the ref gone")
	require.Equal(t, 1, cov["not_yet_in_scope"], "the introduce criterion's clock has not started")
	require.Equal(t, 2, cov["checked"]+cov["not_yet_in_scope"]+cov["unresolvable"]+cov["out_of_scope"],
		"the buckets must sum to both criteria the contract carries, not silently drop the one that produced a candidate")
	require.Equal(t, 1, rep.Compared)
}

// --- Compared: dedupes across markers, never marker-visits (Run's own aggregation) ---

// TestComparedDedupesNodesAcrossMarkers pins the finding
// that Compared counted marker-visits, not nodes: every marker's sweep bumped
// it independently, so a node several enabled markers each fully compare in
// one Run (a ready feature, say, swept by active_basis, answered_question,
// missing_surface, AND orphan_eval all at once) inflated the total by one for
// every marker that reached it (a real 5-node scratch tree printed "Compared
// 9 node(s)"). Every marker's own test elsewhere in this file runs a single-
// marker manifest, where marker-visits and nodes coincide by construction, so
// none of them can see this; this is the one multi-marker fixture that can.
//
// Four distinct, human-countable nodes: a done epic and its done feature
// (stale_spec sweeps both; orphan_eval ALSO sweeps the feature, since its own
// introduce criterion's clock starts at the done crossing, so feat-done is
// itself visited twice), and a second, active feature plus its ready task
// under a different epic (the active feature independently swept by
// active_basis, answered_question, missing_surface, and orphan_eval, whose
// own introduce criterion is skipped as not_yet_in_scope but still counts the
// feature as compared, matching TestOrphanEvalPolarityClock's own point one
// section up). Marker-visits pre-fix: stale_spec 2 (epic, feat-done) +
// orphan_eval 2 (feat-done, feat-active) + active_basis 2 (feat-active,
// task-active) + answered_question 1 (feat-active) + missing_surface 1
// (feat-active) = 8. The fix dedupes by node id, so the identical run now
// reads 4, exactly the number of DISTINCT nodes any enabled marker actually
// compared.
func TestComparedDedupesNodesAcrossMarkers(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{
		drift.MarkerStaleSpec, drift.MarkerActiveBasis, drift.MarkerAnsweredQuestion,
		drift.MarkerMissingSurface, drift.MarkerOrphanEval,
	})
	deps := seqDeps()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "app", "calc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app/calc/zone.rb"), []byte("class Zone\nend\n"), 0o644))
	// writeFeatureProse (called inside driveFeatureToDoneGit) writes an eval
	// file whose one introduce criterion names spec/feat-done_spec.rb; that
	// criterion's clock starts the moment the feature reaches done, so
	// orphan_eval sweeps this feature too, and the ref has to actually exist
	// for the fixture to stay clean (this test's whole point is the COUNT,
	// not a candidate).
	require.NoError(t, os.MkdirAll(filepath.Join(root, "spec"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "spec/feat-done_spec.rb"), []byte("# spec\n"), 0o644))

	epicRes, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)
	approveEpic(t, root, deps, epicRes.ID)
	_, _ = driveFeatureToDoneGit(t, root, deps, "form-engine", "feat-done", "task-done", []string{"app/calc"})

	// A second epic carrying one active feature and its ready task: the
	// feature alone is independently in scope for active_basis,
	// answered_question, missing_surface (once it carries a surface), and
	// orphan_eval (it carries an eval file whose clock has started via
	// approve); the task is in scope for active_basis alone.
	_, err = mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "widget", Title: "Widget"})
	require.NoError(t, err)
	featActive, _ := layActiveFeature(t, root, deps, "widget", "feat-active", "task-active")

	require.NoError(t, os.WriteFile(filepath.Join(root, "widget.rb"), []byte("class Widget\nend\n"), 0o644))
	tree := loadTree(t, root)
	_, err = mutate.SetSurface(tree, deps, tree.Features[featActive], []string{"widget.rb"})
	require.NoError(t, err)

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)

	require.Empty(t, rep.Candidates, "every node in this fixture is clean; the point under test is the COUNT, not any candidate")
	require.Empty(t, rep.Uncovered)

	require.Equal(t, 2, rep.Coverage[drift.MarkerStaleSpec]["checked"], "the done epic and its done feature")
	require.Equal(t, 2, rep.Coverage[drift.MarkerActiveBasis]["checked"], "the active feature and its ready task")
	require.Equal(t, 1, rep.Coverage[drift.MarkerAnsweredQuestion]["checked"], "the active feature alone")
	require.Equal(t, 1, rep.Coverage[drift.MarkerMissingSurface]["checked"], "the active feature alone")
	require.Equal(t, 1, rep.Coverage[drift.MarkerOrphanEval]["checked"], "the done feature's own introduce criterion, its clock started at the done crossing")
	require.Equal(t, 1, rep.Coverage[drift.MarkerOrphanEval]["not_yet_in_scope"], "the active feature's own introduce criterion, clock not started")

	require.Equal(t, 4, rep.Compared,
		"4 distinct nodes: the done epic, the done feature, the active feature, and its ready task, "+
			"not 8, the sum of every marker's own bump the pre-fix code produced")
}

// --- ForNode ----
//
// ForNode shares every report/finish helper Run's own sweep calls rather than
// a second implementation of any marker's comparison, so these tests assert
// candidate/uncovered SHAPE at one node, not a second proof of what each
// marker's comparison decides; that proof already lives in the sweep tests
// above.

// TestForNodeYieldsCandidatesForADriftedNode pins the same active_basis
// fixture TestActiveBasisSweep already drives (an approved feature edited
// after approval), through ForNode instead of Run: a single node's own
// candidates must agree with what a whole-tree sweep would have found for
// it.
func TestForNodeYieldsCandidatesForADriftedNode(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")
	appendToFile(t, featurePath(root, "form-engine", "feat-active", "feature.md"), "\nEdited after approval.\n")

	tree := loadTree(t, root)
	cs, us, err := drift.ForNode(tree, vcs.New(root), tree.Features[featID])
	require.NoError(t, err)
	require.Empty(t, us)
	require.Len(t, cs, 1)
	require.Equal(t, drift.MarkerActiveBasis, cs[0].Marker)
	require.Equal(t, store.NounFeature, cs[0].Noun)
	require.Equal(t, featID, cs[0].ID)
	require.Contains(t, cs[0].Files, "feature.md")
}

// TestForNodeOnACleanNodeYieldsNothing pins the empty case beside the
// drifted one: the identical approved feature with no edit at all yields
// neither a candidate nor an uncovered row, the same "clean answer asserts
// coverage" promise Run's own report carries, at the single-node grain.
func TestForNodeOnACleanNodeYieldsNothing(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerActiveBasis})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")

	tree := loadTree(t, root)
	cs, us, err := drift.ForNode(tree, vcs.New(root), tree.Features[featID])
	require.NoError(t, err)
	require.Empty(t, cs)
	require.Empty(t, us)
}

// TestForNodeExemptNodeYieldsNothingForThatMarker pins the exemption case: a
// done, superseded feature is exempt from stale_spec before its
// verified_against is ever opened (staleSpecFeatureExempt, drift.go's own
// sweepStaleSpec comment), so ForNode must answer nothing for it too, even
// though the SAME shape (done, empty verified_against) would ordinarily be
// Uncovered{no_completion_record} for a NON-exempt done feature
// (reportStaleSpecFeature). The contrast is what makes this test able to
// fail: dropping the exemption check would turn this into a spurious
// Uncovered row rather than silence.
func TestForNodeExemptNodeYieldsNothingForThatMarker(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	fRes, err := mutate.AddFeature(loadTree(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/feat-superseded", Title: "feat-superseded", ChangeType: "create", Source: "design/x.md",
	})
	require.NoError(t, err)
	featID := fRes.ID

	setFeatureStatus(t, root, "form-engine", "feat-superseded", "done")
	setFeatureSupersededBy(t, root, "form-engine", "feat-superseded", "F-successor")

	tree := loadTree(t, root)
	cs, us, err := drift.ForNode(tree, vcs.New(root), tree.Features[featID])
	require.NoError(t, err)
	require.Empty(t, cs, "a superseded feature is exempt, never a candidate")
	require.Empty(t, us, "exemption is checked before verified_against is ever opened, so this is not even uncovered")
}

// TestForNodeAnsweredQuestionReachesATaskThroughItsParentFeature pins
// that a TASK page can carry an
// answered_question badge too, reading its PARENT feature's unreviewed
// assumption through the identical walk-up UnreviewedAssumptions and
// guardSubmitAssumptions both make for a submitting task
// (assumptionFeatureFor, mirrored inline in answeredQuestionForNode since it
// is the only line in this package that needs it). Without this, the
// refusing rule's task-and-submittable-window clause (views.driftRefusing,
// Task 12) would be unreachable: no ForNode call would ever produce an
// answered_question candidate for a task at all.
func TestForNodeAnsweredQuestionReachesATaskThroughItsParentFeature(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerAnsweredQuestion})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)
	featID, _ := layFeatureWithAssumedQuestion(t, root, deps, "feat-assumed")

	tree := loadTree(t, root)
	var taskID string
	for id, tk := range tree.Tasks {
		if tk.FeatureID == featID {
			taskID = id
		}
	}
	require.NotEmpty(t, taskID, "the fixture lays exactly one task under the feature")

	cs, us, err := drift.ForNode(tree, vcs.New(root), tree.Tasks[taskID])
	require.NoError(t, err)
	require.Empty(t, us)
	require.Len(t, cs, 1)
	require.Equal(t, drift.MarkerAnsweredQuestion, cs[0].Marker)
	// The candidate still names the FEATURE: assumption_basis lives there
	// alone, so the row is really about it even though it was reached from
	// the task's own page, the same not-always-the-acted-on-node shape
	// ActiveBasisError already carries for the identical parent walk-up.
	require.Equal(t, store.NounFeature, cs[0].Noun)
	require.Equal(t, featID, cs[0].ID)
}

// TestForNodeHonorsTheChecksList pins the same misuse-grade validation Run
// itself refuses on (TestRunRefusesAnUnknownCheckName, this file's own
// checks-list section): ForNode calls the identical enabledSet, so an
// unknown drift.checks entry refuses a single-node call exactly the same
// way, never silently answering an empty badge list for a manifest that is
// actually broken.
func TestForNodeHonorsTheChecksList(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{"not_a_real_marker"})

	tree := loadTree(t, root)
	// enabledSet refuses before ForNode ever looks at n, the same order
	// Run's own validation runs in, so a nil node is safe to pass here: the
	// point under test is the manifest check, not any particular node.
	_, _, err := drift.ForNode(tree, vcs.New(root), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown check")
}

// TestForNodeSkipsADisabledMarker pins fix round 1's Minor 1: the previous
// suite proved ForNode's checks-list validation refuses an unknown NAME
// (TestForNodeHonorsTheChecksList above), but nothing pinned the plain
// enable/disable gate (the `if !enabled[marker] { continue }` line inside
// ForNode's own dispatch loop) directly: a reviewer could delete that guard
// entirely and the whole drift package would stay green. Here active_basis
// genuinely drifted (the identical fixture TestForNodeYieldsCandidatesForADriftedNode
// uses) but drift.checks names only stale_spec, so ForNode must still answer
// nothing for it.
func TestForNodeSkipsADisabledMarker(t *testing.T) {
	root := scratchProjectGit(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})
	deps := seqDeps()

	_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	featID, _ := layActiveFeature(t, root, deps, "form-engine", "feat-active", "task-a")
	appendToFile(t, featurePath(root, "form-engine", "feat-active", "feature.md"), "\nEdited after approval.\n")

	tree := loadTree(t, root)
	cs, us, err := drift.ForNode(tree, vcs.New(root), tree.Features[featID])
	require.NoError(t, err)
	require.Empty(t, cs, "active_basis genuinely drifted, but drift.checks does not enable it")
	require.Empty(t, us)
}

// TestForNodeAnsweredQuestionExcludesATerminalTask pins fix round 1's Minor
// 2: answeredQuestionForNode filtered only on the PARENT FEATURE's status,
// so a task whose own status is already done or cancelled still carried a
// badge whenever its parent feature was non-terminal - asymmetric with
// activeBasisForNode, which gates on the swept node's OWN status
// (activeBasisTaskStatuses) beside the parent-walk it makes. A task that has
// already finished or been cancelled cannot act on this badge from its own
// page (there is no submit left to refuse, ever), so its own terminal
// status has to gate here too, not just the feature's.
func TestForNodeAnsweredQuestionExcludesATerminalTask(t *testing.T) {
	for _, status := range []string{"done", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			root := scratchProjectGit(t)
			setManifestChecks(t, root, []string{drift.MarkerAnsweredQuestion})
			deps := seqDeps()

			_, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
			require.NoError(t, err)
			featID, _ := layFeatureWithAssumedQuestion(t, root, deps, "feat-assumed")
			// The feature itself stays "active" (layFeatureWithAssumedQuestion's
			// own arc never moves it further), so only the TASK's own status
			// distinguishes this fixture from TestForNodeAnsweredQuestionReachesATaskThroughItsParentFeature.
			setTaskStatus(t, root, "form-engine", "feat-assumed", "feat-assumed-task", status)

			tree := loadTree(t, root)
			var taskID string
			for id, tk := range tree.Tasks {
				if tk.FeatureID == featID {
					taskID = id
				}
			}
			require.NotEmpty(t, taskID, "the fixture lays exactly one task under the feature")

			cs, us, err := drift.ForNode(tree, vcs.New(root), tree.Tasks[taskID])
			require.NoError(t, err)
			require.Emptyf(t, cs, "a %s task yields nothing, even though its still-active parent feature carries a live unreviewed assumption", status)
			require.Empty(t, us)
		})
	}
}

// scratchDoneFeatureWithFileSurface mirrors scratchDoneFeatureWithSurface at
// file grain: one epic, one feature driven to done through the real arc, its
// verified_against carrying a single git_blob fingerprint over
// app/calc/project.rb (sync records a file surface as a blob since the Lantern
// finding, 2026-08-07).
func scratchDoneFeatureWithFileSurface(t *testing.T) (root, featureID string) {
	t.Helper()
	root = scratchProjectGit(t)
	deps := seqDeps()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "app", "calc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app/calc/project.rb"), []byte("class Project\nend\n"), 0o644))

	epicRes, err := mutate.AddEpic(loadTree(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/x.md"})
	require.NoError(t, err)
	approveEpic(t, root, deps, epicRes.ID)

	featureID, _ = driveFeatureToDoneGit(t, root, deps, "form-engine", "feat-done", "task-a", []string{"app/calc/project.rb"})
	return root, featureID
}

// TestStaleSpecFileSurfaceVanishFiresEditDoesNot extends the vanish rule to
// blob fingerprints: a recorded file merely edited is everyday evolution and
// no candidate, the same file deleted is the vanish candidate, named. The
// sweep re-reads a git_blob fingerprint with BlobHash; TreeHash on it would
// land the node in Uncovered as surface_unreadable instead.
func TestStaleSpecFileSurfaceVanishFiresEditDoesNot(t *testing.T) {
	root, featID := scratchDoneFeatureWithFileSurface(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

	// Fixture sanity: the crossing recorded a blob, or this test is not
	// exercising the arm it claims to.
	f := loadTree(t, root).Features[featID]
	require.Len(t, f.VerifiedAgainst.Surfaces, 1)
	require.Equal(t, store.KindGitBlob, f.VerifiedAgainst.Surfaces[0].Kind)

	overwriteAndCommit(t, root, "app/calc/project.rb", "class Project\n  # changed body\nend")

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec),
		"an edited surface file is normal evolution, not a candidate")
	require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec),
		"a blob fingerprint must be re-read as a blob, not fail a tree read")

	removeAndCommit(t, root, "app/calc/project.rb")

	rep, err = drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	cs := candidatesFor(rep, drift.MarkerStaleSpec)
	require.Len(t, cs, 1)
	require.Equal(t, featID, cs[0].ID)
	require.Contains(t, cs[0].Files, "app/calc/project.rb")
}

// TestStaleSpecFileSurfaceUnchangedComparesClean pins the quiet path: a done
// feature whose file surface has not moved lands in Compared with no
// candidate and nothing uncovered.
func TestStaleSpecFileSurfaceUnchangedComparesClean(t *testing.T) {
	root, _ := scratchDoneFeatureWithFileSurface(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

	rep, err := drift.Run(loadTree(t, root), vcs.New(root))
	require.NoError(t, err)
	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec))
	require.Empty(t, uncoveredFor(rep, drift.MarkerStaleSpec))
	require.Equal(t, 2, rep.Compared, "the feature and its rolled-up epic both compare clean")
}

// blobHashFailsRev wraps a real vcs.Revisions and fails BlobHash for one
// specific path, leaving every other observation (including
// WorktreeBlobHash, which the spec/eval legs use) fully real via the
// embedded interface. It is treeHashFailsRev's twin for the blob arm, for
// the same reason: isolating a hard git failure to the surface leg alone,
// without the OS-permission fragility a git-internal read failure would
// need otherwise.
type blobHashFailsRev struct {
	vcs.Revisions
	path string
}

func (r blobHashFailsRev) BlobHash(path string) (string, error) {
	if path == r.path {
		return "", fmt.Errorf("simulated blob hash read failure at %s (test double)", path)
	}
	return r.Revisions.BlobHash(path)
}

// TestStaleSpecSurfaceBlobHashFailureIsUncoveredWithItsOwnReason is
// TestStaleSpecSurfaceTreeHashFailureIsUncoveredWithItsOwnReason's twin for
// the blob arm: it pins the exact wording staleSpecSurfacesLeg produces when
// BlobHash fails ("could not read the current blob hash of %s: %s"), since
// nothing else in this suite asserts that string and a rename or typo in it
// would otherwise pass every existing test silently. The failure is
// injected through blobHashFailsRev rather than a chmod repro, since the
// recorded path here still exists and is fully present on disk; only the
// git-internal blob-hash read is made to fail.
func TestStaleSpecSurfaceBlobHashFailureIsUncoveredWithItsOwnReason(t *testing.T) {
	root, featID := scratchDoneFeatureWithFileSurface(t)
	setManifestChecks(t, root, []string{drift.MarkerStaleSpec})

	rev := blobHashFailsRev{Revisions: vcs.New(root), path: "app/calc/project.rb"}
	rep, err := drift.Run(loadTree(t, root), rev)
	require.NoError(t, err)

	require.Empty(t, candidatesFor(rep, drift.MarkerStaleSpec),
		"nothing vanished and nothing could be read, so there is no finding to report")
	us := uncoveredFor(rep, drift.MarkerStaleSpec)
	var found bool
	for _, u := range us {
		if u.ID == featID {
			found = true
			require.Equal(t, "surface_unreadable", u.Reason)
			require.Contains(t, u.Detail, "could not read the current blob hash of app/calc/project.rb")
		}
	}
	require.True(t, found, "a git failure reading the surface's blob hash is uncovered, not silently clean")
}
