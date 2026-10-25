package mutate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	featureMDPath = ".tiller/epics/form-engine/features/nrca-form-loading/feature.md"
	featureEvalMD = ".tiller/epics/form-engine/features/nrca-form-loading/feature.eval.md"
	epicMDPath    = ".tiller/epics/form-engine/epic.md"
)

// commitPaths stages exactly paths and commits them, leaving every other
// working-tree change (the uncommitted Tiller state sync acts on) in place.
func commitPaths(t *testing.T, root, msg string, paths ...string) {
	t.Helper()
	gitRunM(t, root, append([]string{"add", "--"}, paths...)...)
	gitRunM(t, root, "commit", "-m", msg)
}

func writeRel(t *testing.T, root, rel, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644))
}

// layScratchCheckoutAfterMerge is the defect 03 shape: main carries the merged
// task branch and committed surfaces, and the human runs sync from a scratch
// branch cut off main whose own commits change the specs and surfaces. The
// build gate is red so crossings also record policy violations. It returns the
// repository root and main's full tip.
func layScratchCheckoutAfterMerge(t *testing.T) (string, string) {
	t.Helper()
	root := layActiveFeatureGit(t, "false", "true")
	submitTask(t, root)
	declareIntegrationBranch(t, root)
	writeRel(t, root, "src/a.swift", "struct A {}\n")
	writeRel(t, root, "chrome.swift", "struct Chrome {}\n")
	commitPaths(t, root, "surfaces on main", "src", "chrome.swift")
	setFeatureSurfacePath(t, root, "form-engine", "nrca-form-loading", "src", "chrome.swift")
	setEpicSurfacePath(t, root, "form-engine", "src")
	mainTip := gitOutM(t, root, "rev-parse", "main")

	gitRunM(t, root, "checkout", "-b", "scratch")
	writeRel(t, root, featureMDPath, "# Scratch-only spec\n")
	writeRel(t, root, featureEvalMD, "---\nnode: F-aaaaaa\ncriteria: []\n---\n\n# Scratch-only eval\n")
	writeRel(t, root, epicMDPath, "# Scratch-only epic\n")
	writeRel(t, root, "src/a.swift", "struct ScratchA {}\n")
	writeRel(t, root, "src/extra.swift", "struct Extra {}\n")
	writeRel(t, root, "chrome.swift", "struct ScratchChrome {}\n")
	commitPaths(t, root, "scratch-only changes", featureMDPath, featureEvalMD, epicMDPath, "src", "chrome.swift")
	return root, mainTip
}

// integrationRecord is what every completion fingerprint must equal: the
// values read from one integration commit.
type integrationRecord struct {
	commit, spec, eval, epicSpec, srcTree, chrome string
	srcInventory                                  []string
}

func recordAt(t *testing.T, root, commit string) integrationRecord {
	t.Helper()
	return integrationRecord{
		commit:       "git:" + gitOutM(t, root, "rev-parse", "--short", commit),
		spec:         "git:" + gitOutM(t, root, "rev-parse", commit+":"+featureMDPath),
		eval:         "git:" + gitOutM(t, root, "rev-parse", commit+":"+featureEvalMD),
		epicSpec:     "git:" + gitOutM(t, root, "rev-parse", commit+":"+epicMDPath),
		srcTree:      "git:" + gitOutM(t, root, "rev-parse", commit+":src"),
		chrome:       "git:" + gitOutM(t, root, "rev-parse", commit+":chrome.swift"),
		srcInventory: []string{"src/a.swift"},
	}
}

func requireIntegrationRecord(t *testing.T, root string, res mutate.SyncResult, want integrationRecord) {
	t.Helper()
	require.ElementsMatch(t, []string{"T-aaaaaa", "F-aaaaaa", "E-aaaaaa"}, crossedIDs(res))
	for _, crossing := range res.Crossings {
		require.Equal(t, want.commit, crossing.Observed, "%s %s observed revision", crossing.Noun, crossing.ID)
	}
	tree := load(t, root)
	task := tree.Tasks["T-aaaaaa"]
	require.NotEmpty(t, task.PolicyViolations)
	for _, pv := range task.PolicyViolations {
		require.Equal(t, want.commit, pv.Observed.Value, "task policy violation observed revision")
	}

	f := tree.Features["F-aaaaaa"]
	require.Equal(t, "done", f.Status)
	require.Equal(t, want.commit, f.VerifiedAgainst.Commit.Value)
	require.Equal(t, want.spec, f.VerifiedAgainst.Spec.Value)
	require.Equal(t, featureMDPath, filepath.ToSlash(f.VerifiedAgainst.Spec.Path))
	require.Equal(t, want.eval, f.VerifiedAgainst.Eval.Value)
	require.Len(t, f.VerifiedAgainst.Surfaces, 2)
	require.Equal(t, want.srcTree, f.VerifiedAgainst.Surfaces[0].Value)
	require.Equal(t, want.srcInventory, f.VerifiedAgainst.Surfaces[0].Inventory)
	require.Equal(t, want.chrome, f.VerifiedAgainst.Surfaces[1].Value)
	for _, pv := range f.PolicyViolations {
		require.Equal(t, want.commit, pv.Observed.Value, "feature policy violation observed revision")
	}

	e := tree.Epics["E-aaaaaa"]
	require.Equal(t, "done", e.Status)
	require.Equal(t, want.commit, e.VerifiedAgainst.Commit.Value)
	require.Equal(t, want.epicSpec, e.VerifiedAgainst.Spec.Value)
	require.Len(t, e.VerifiedAgainst.Surfaces, 1)
	require.Equal(t, want.srcTree, e.VerifiedAgainst.Surfaces[0].Value)
	require.Equal(t, want.srcInventory, e.VerifiedAgainst.Surfaces[0].Inventory)
}

// Defect 03: sync run from a scratch checkout records main's commit, spec and
// eval blobs, surface trees and blobs, and inventories, never scratch-only
// changes. Its state writes stay in the invoking checkout, and it moves no ref.
func TestSyncOnAScratchCheckoutRecordsTheIntegrationRevision(t *testing.T) {
	t.Parallel()
	root, mainTip := layScratchCheckoutAfterMerge(t)
	want := recordAt(t, root, mainTip)
	refsBefore := gitOutM(t, root, "for-each-ref")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Equal(t, "main", res.Into, "the selected integration name is kept for diagnostics")

	requireIntegrationRecord(t, root, res, want)
	require.Equal(t, "scratch", gitOutM(t, root, "symbolic-ref", "--short", "HEAD"), "sync switches no branch")
	require.Equal(t, refsBefore, gitOutM(t, root, "for-each-ref"), "sync moves no ref")
	require.Equal(t, "# Scratch-only spec\n", readRel(t, root, featureMDPath), "the checkout's own files are untouched")
}

func readRel(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err)
	return string(data)
}

// movingIntegration moves the integration ref once, right after sync's first
// ancestry or first-parent read, so a sync that reread the branch would mix two
// revisions into one observation.
type movingIntegration struct {
	*vcs.Git
	move func()
}

func (m *movingIntegration) moveOnce() {
	if m.move != nil {
		move := m.move
		m.move = nil
		move()
	}
}

func (m *movingIntegration) IsMergedInto(branch, into string) (bool, error) {
	merged, err := m.Git.IsMergedInto(branch, into)
	m.moveOnce()
	return merged, err
}

func (m *movingIntegration) FirstParentContains(ref, commit string) (bool, error) {
	contained, err := m.Git.FirstParentContains(ref, commit)
	m.moveOnce()
	return contained, err
}

// prepareIntegrationAdvance commits a descendant of branch in a detached
// scratch worktree, changing the specs and surfaces, and returns a function
// that moves branch onto it.
func prepareIntegrationAdvance(t *testing.T, root, branch string) func() {
	t.Helper()
	worktree := filepath.Join(t.TempDir(), "advance")
	gitRunM(t, root, "worktree", "add", "--detach", worktree, branch)
	for rel, content := range map[string]string{
		featureMDPath: "# Advanced spec\n", epicMDPath: "# Advanced epic\n",
		"src/a.swift": "struct AdvancedA {}\n", "chrome.swift": "struct AdvancedChrome {}\n",
	} {
		writeRel(t, worktree, rel, content)
	}
	gitRunM(t, worktree, "add", "-A")
	gitRunM(t, worktree, "commit", "-m", "integration advanced during sync")
	next := gitOutM(t, worktree, "rev-parse", "HEAD")
	return func() { gitRunM(t, root, "update-ref", "refs/heads/"+branch, next) }
}

// A moving integration ref cannot mix revisions within one observation: every
// record names the commit the branch resolved to when sync began.
func TestSyncPinsOneIntegrationCommitWhileTheRefMoves(t *testing.T) {
	t.Parallel()
	root, mainTip := layScratchCheckoutAfterMerge(t)
	want := recordAt(t, root, mainTip)
	rev := &movingIntegration{Git: vcs.New(root), move: prepareIntegrationAdvance(t, root, "main")}

	res, err := mutate.Sync(load(t, root), syncDeps(), rev, mutate.SyncInput{})
	require.NoError(t, err)
	require.Nil(t, rev.move, "the integration ref moved during the run")
	require.NotEqual(t, mainTip, gitOutM(t, root, "rev-parse", "main"))
	requireIntegrationRecord(t, root, res, want)
}

// A path the integration commit lacks is an explicit skip, never read from the
// checkout's HEAD instead.
func TestSyncSkipsAPathTheIntegrationCommitLacks(t *testing.T) {
	t.Parallel()
	root, _ := layScratchCheckoutAfterMerge(t)
	writeRel(t, root, "scratch-only.swift", "struct ScratchOnly {}\n")
	commitPaths(t, root, "scratch-only surface", "scratch-only.swift")
	setFeatureSurfacePath(t, root, "form-engine", "nrca-form-loading", "scratch-only.swift")

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)
	require.Len(t, res.Skipped, 1)
	require.Equal(t, store.NounFeature, res.Skipped[0].Noun)
	require.Equal(t, "scratch-only.swift", res.Skipped[0].Path)
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)
}

// Recovered feature and epic sync from a scratch checkout, with integration
// advancing between reads, records the integration commit sync resolved and
// keeps the recovery's historical head, result, basis, and policy snapshots.
func TestRecoveredSyncFromAScratchCheckoutPinsTheIntegrationCommit(t *testing.T) {
	t.Parallel()
	for _, grain := range []string{store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			root, record := recoveredSyncFixture(t, grain, "merge")
			mainTip := gitOutM(t, root, "rev-parse", "main")
			wantCommit := "git:" + gitOutM(t, root, "rev-parse", "--short", mainTip)
			wantSpec := "git:" + gitOutM(t, root, "rev-parse", mainTip+":"+featureMDPath)
			gitRunM(t, root, "checkout", "-b", "scratch")
			writeRel(t, root, featureMDPath, "# Scratch-only spec\n")
			commitPaths(t, root, "scratch-only spec", featureMDPath)
			rev := &movingIntegration{Git: vcs.New(root), move: prepareIntegrationAdvance(t, root, "main")}

			res, err := mutate.Sync(load(t, root), syncDeps(), rev, mutate.SyncInput{})
			require.NoError(t, err)
			require.Nil(t, rev.move, "the integration ref moved during the run")
			require.Equal(t, []string{"T-aaaaaa", "T-aaaaab", "F-aaaaaa", "E-aaaaaa"}, crossedIDs(res))
			for _, crossing := range res.Crossings {
				require.Equal(t, wantCommit, crossing.Observed)
			}
			tree := load(t, root)
			f := tree.Features["F-aaaaaa"]
			require.Equal(t, wantCommit, f.VerifiedAgainst.Commit.Value)
			require.Equal(t, wantSpec, f.VerifiedAgainst.Spec.Value)
			retained := f.RecoveredReview
			if grain == store.ReviewGrainEpic {
				retained = tree.Epics["E-aaaaaa"].RecoveredReview
			}
			require.NotNil(t, retained)
			require.Equal(t, record.Head, retained.Head)
			require.Equal(t, record.MergeCommit, retained.MergeCommit)
			require.Equal(t, record.Basis, retained.Basis)
			require.Equal(t, record.PolicySnapshots, retained.PolicySnapshots)
			require.Equal(t, "scratch", gitOutM(t, root, "symbolic-ref", "--short", "HEAD"))
		})
	}
}

// An integration name that resolves to no commit refuses before any write,
// rather than observing or recording against the checkout's HEAD.
func TestSyncRefusesAnIntegrationThatResolvesToNoCommit(t *testing.T) {
	t.Parallel()
	root, _ := layScratchCheckoutAfterMerge(t)
	before := snapshotDir(t, root)

	_, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{Into: "release"})
	var missing *mutate.IntegrationMissingError
	require.ErrorAs(t, err, &missing)
	require.Equal(t, "release", missing.Into)
	requireNoWrites(t, before, snapshotDir(t, root))
}

// setFeatureAcceptedEval rewrites feature.yml's accepted_against.eval value, the
// shape a working-tree re-approval leaves when the refreshed contract is never
// committed to integration.
func setFeatureAcceptedEval(t *testing.T, root, value string) {
	t.Helper()
	p := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "nrca-form-loading", "feature.yml")
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.AcceptedAgainst.Eval.Value = value
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, out, 0o644))
}

// Final review item 1: the completion record reads the eval from the
// integration commit while acceptance reads the working tree. When the two
// disagree at a feature crossing, sync still crosses (it is advisory) and names
// the feature with both blobs, so a done record pinning a contract no gate
// judged is never silent.
func TestSyncWarnsWhenTheIntegrationEvalIsNotTheAcceptedOne(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)
	writeRel(t, root, featureEvalMD, "---\nnode: F-aaaaaa\ncriteria: []\n---\n\n# Refreshed, uncommitted eval\n")
	accepted := "git:" + gitOutM(t, root, "hash-object", featureEvalMD)
	setFeatureAcceptedEval(t, root, accepted)
	integrationEval := "git:" + gitOutM(t, root, "rev-parse", "main:"+featureEvalMD)
	require.NotEqual(t, accepted, integrationEval)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1, "the advisory mismatch never blocks the crossing")
	require.Equal(t, "done", fc[0].Status)
	require.NotNil(t, fc[0].EvalContractMismatch)
	require.Equal(t, accepted, fc[0].EvalContractMismatch.Accepted)
	require.Equal(t, integrationEval, fc[0].EvalContractMismatch.Verified)
	require.Equal(t, integrationEval, load(t, root).Features["F-aaaaaa"].VerifiedAgainst.Eval.Value)
}

func TestSyncDoesNotWarnWhenTheIntegrationEvalIsTheAcceptedOne(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGit(t, "true", "true")
	submitTask(t, root)

	res, err := mutate.Sync(load(t, root), syncDeps(), vcs.New(root), mutate.SyncInput{})
	require.NoError(t, err)

	fc := crossingsOf(res, store.NounFeature)
	require.Len(t, fc, 1)
	require.Nil(t, fc[0].EvalContractMismatch)
}
