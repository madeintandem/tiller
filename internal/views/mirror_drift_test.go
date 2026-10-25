package views_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const mirrorDriftManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Mirror Drift Test
  key: mirrordrift
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

// mirrorDriftFeatureKey is the one feature-grain feature every mirrorDrift
// fixture builds.
const mirrorDriftFeatureKey = "form-engine/nrca-form-loading"

// queueSetTaskStatus rewrites one task's status line directly,
// queueSetTaskInReview's task-grain twin for the one case this file needs:
// a task left at in_progress rather than moved to in_review.
func queueSetTaskStatus(t *testing.T, path, from, to string) {
	t.Helper()
	task := queueReadTask(t, path)
	require.Equal(t, from, task.Status)
	task.Status = to
	queueWriteTask(t, path, task)
}

// mirrorDriftFixture lays a one-epic-one-feature-one-task tree, feature-grain
// reviewed, with the task claimed for jc and staged at taskFinalStatus
// ("in_review" or "in_progress"): the precondition these callers compare.
// The manifest carries
// no required gates; a valid optional contract isolates task readiness without
// pretending unread acceptance facts establish submission readiness.
func mirrorDriftFixture(t *testing.T, taskFinalStatus string) (root string, deps mutate.Deps) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(mirrorDriftManifest), 0o644))
	deps = queueDeps(queueMint())

	_, err := mutate.AddEpic(queueLoad(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(queueLoad(t, root), deps, mutate.FeatureInput{
		Key: mirrorDriftFeatureKey, Title: "NRCA form loading", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddTask(queueLoad(t, root), deps, mutate.TaskInput{
		Key: mirrorDriftFeatureKey + "/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(queueLoad(t, root), deps, mirrorDriftFeatureKey+"/transcribe-the-schema", "")
	require.NoError(t, err)
	for _, feature := range queueLoad(t, root).Features {
		contract := "---\nnode: " + feature.ID + "\ncriteria:\n  - id: C1\n    statement: Optional human observation\n    binding: {type: manual}\n---\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "epics", "form-engine", "features", "nrca-form-loading", "feature.eval.md"), []byte(contract), 0o644))
	}

	// The epic is approved the way the arc would have it: a draft epic is
	// its own route wait ahead of every arm this fixture exercises (issue
	// #93), so leaving it draft would hide the finish line behind that gate.
	epicYML := filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml")
	queueSetStatusOnDisk(t, epicYML, "draft", "ready")
	featureYML := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "nrca-form-loading", "feature.yml")
	queueSetStatusOnDisk(t, featureYML, "draft", "active")

	_, err = mutate.Configure(queueLoad(t, root), deps, mutate.ConfigureInput{
		Noun: store.NounFeature, Key: mirrorDriftFeatureKey, ReviewGrain: "feature"})
	require.NoError(t, err)

	taskYML := queueTaskYML(root, "form-engine", "nrca-form-loading", "transcribe-the-schema")
	if taskFinalStatus == "in_review" {
		queueSetTaskInReview(t, taskYML)
	} else {
		queueSetTaskStatus(t, taskYML, "proposed", taskFinalStatus)
	}

	// Use observable Git refs, including the task's submission state tip.
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	git("config", "user.name", "Mirror fixture")
	git("config", "user.email", "mirror@example.test")
	aggregate := "tiller/reviews/features/" + mirrorDriftFeatureKey
	featureBytes, err := os.ReadFile(featureYML)
	require.NoError(t, err)
	var feature store.Feature
	require.NoError(t, yaml.Unmarshal(featureBytes, &feature))
	feature.Branch = aggregate
	featureBytes, err = yaml.Marshal(&feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(featureYML, featureBytes, 0o644))
	task := queueReadTask(t, taskYML)
	task.Branch = "tiller/" + mirrorDriftFeatureKey + "/transcribe-the-schema"
	queueWriteTask(t, taskYML, task)
	git("add", ".")
	git("commit", "-m", "task implementation")
	git("checkout", "-b", task.Branch)
	if taskFinalStatus == "in_review" {
		task.ReviewRef = &store.ReviewRef{Branch: task.Branch, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + git("rev-parse", "--short", "HEAD")}}
		queueWriteTask(t, taskYML, task)
		git("add", ".")
		git("commit", "-m", "record task submission")
	}
	git("checkout", "-b", aggregate)
	deps.Rev = vcs.New(root)
	return root, deps
}

func TestSubmitAndTheFinishLineAgreeAtTheSeam(t *testing.T) {
	// Phase 1: every precondition met (the task reached in_review, required
	// gates vacuously green). The finish line must already name the exact
	// submit about to be run, and the real submit must succeed.
	root, deps := mirrorDriftFixture(t, "in_review")
	tree := queueLoad(t, root)
	top := topMove(t, tree, "jc", readinput.Workflow(tree))
	require.NotNil(t, top.Workflow)
	require.Equal(t, "submit", top.Workflow.Action)

	res, err := mutate.SubmitNode(queueLoad(t, root), deps, mutate.SubmitNodeInput{
		Noun: store.NounFeature, Key: mirrorDriftFeatureKey, PR: intPtrMD(7)})
	require.NoError(t, err, "the finish line's own claim must be honored by the real submit")
	require.True(t, res.Changed)
	require.Equal(t, res.ID, top.Workflow.ReviewID)

	// Phase 2: flip the one precondition the two callers share, a task's own
	// status, back to in_progress instead of in_review (a fresh, otherwise
	// identical tree, since phase 1's own tree already carries a recorded PR
	// that would suppress the finish line for an unrelated reason). Both
	// readers must now agree the feature is NOT ready.
	root2, deps2 := mirrorDriftFixture(t, "in_progress")
	tree2 := queueLoad(t, root2)
	requireNoSubmitMove(t, nextFor(tree2, "jc", readinput.Workflow(tree2)))

	_, err = mutate.SubmitNode(queueLoad(t, root2), deps2, mutate.SubmitNodeInput{
		Noun: store.NounFeature, Key: mirrorDriftFeatureKey, PR: intPtrMD(7)})
	require.Error(t, err, "the real submit must refuse the same precondition the finish line now withholds")
	var taskBlk *mutate.TaskNotSubmittableError
	require.ErrorAs(t, err, &taskBlk, "submit's own refusal is the task-readiness one, the mirrored precondition")
}

func TestLegacyTaskPRBlocksBothSubmitAndTheCoarseFinishLine(t *testing.T) {
	root, deps := mirrorDriftFixture(t, "in_review")
	taskPath := queueTaskYML(root, "form-engine", "nrca-form-loading", "transcribe-the-schema")
	task := queueReadTask(t, taskPath)
	pr := 41
	task.PR = &pr
	task.ReviewRef = nil
	queueWriteTask(t, taskPath, task)

	tree := queueLoad(t, root)
	next := nextFor(tree, "jc", readinput.Workflow(tree))
	requireNoSubmitMove(t, next)
	top := next.Shortlist()[0]
	require.Equal(t, "merge_task", top.Kind, "the task's own PR waits on a merge")
	require.Equal(t, "PR #41 awaits your merge", top.Reason)

	_, err := mutate.SubmitNode(queueLoad(t, root), deps, mutate.SubmitNodeInput{
		Noun: store.NounFeature, Key: mirrorDriftFeatureKey, PR: intPtrMD(7)})
	var open *mutate.TaskReviewOpenError
	require.ErrorAs(t, err, &open)
	require.NotNil(t, open.PR)
	require.Equal(t, 41, *open.PR)
}

func TestSharedCloseoutRequiresCurrentTaskTipInAggregate(t *testing.T) {
	root, _ := mirrorDriftFixture(t, "in_review")
	tree := queueLoad(t, root)
	var task *store.Task
	for _, candidate := range tree.Tasks {
		task = candidate
	}
	require.NotNil(t, task)
	feature := tree.Features[task.FeatureID]
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = root
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	runGit("checkout", task.Branch)
	runGit("commit", "--allow-empty", "-m", "later submission-state commit")
	runGit("checkout", feature.Branch)

	in := readinput.Workflow(tree)
	top := topMove(t, tree, "unrelated-reviewer", in)
	require.NotNil(t, top.Workflow)
	require.Equal(t, "assembly", top.Workflow.Phase)
	queue := views.QueueOfInput(tree, views.QueueInput{Workflow: in})
	require.Len(t, queue.Entries, 1)
	require.Equal(t, feature.ID, queue.Entries[0].ID)
	require.Equal(t, "assembly", queue.Entries[0].Phase)
	require.Contains(t, queue.Entries[0].Prose, task.ID)

	runGit("merge", "--ff-only", task.Branch)
	in = readinput.Workflow(tree)
	top = topMove(t, tree, "unrelated-reviewer", in)
	require.Equal(t, "submission", top.Workflow.Phase)
	require.Equal(t, "agent", top.Workflow.Actor)
}

// requireNoSubmitMove fails when any ranked move's workflow advice is the
// feature submit.
func requireNoSubmitMove(t *testing.T, next views.Next) {
	t.Helper()
	for _, c := range next.Candidates {
		if c.Workflow != nil {
			require.NotEqual(t, "submit", c.Workflow.Action, "move %s on %v", c.Kind, c.Node)
		}
	}
}

// intPtrMD is this file's own *int helper (queue_test.go and
// submit_node_test.go each keep a private copy of the identical one-liner
// rather than exporting it across packages for a single test's use).
func intPtrMD(n int) *int { return &n }
