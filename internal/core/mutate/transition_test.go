package mutate_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// layEpicFeature lays a one-epic-one-feature draft tree and returns the repo
// root. It is the shared fixture the transition tests walk forward.
func layEpicFeature(t *testing.T) string {
	t.Helper()
	return layEpicFeatureCT(t, "create")
}

// layEpicFeatureCT is layEpicFeature with the change_type knob laySpeccableFeature
// needs below: the spec gate's surface requirement reads change_type, so its
// fixture has to vary the one field layEpicFeature hardcodes to "create".
// It is laid once per change_type and copied into each test.
func layEpicFeatureCT(t *testing.T, changeType string) string {
	t.Helper()
	return epicFeatureRepos.Copy(t, changeType, func(t testing.TB, root string) {
		layOwnedRepo(t, root)
		addEpicFeature(t, root, changeType)
	})
}

// epicFeatureRepos holds one laid layEpicFeatureCT fixture per change_type.
var epicFeatureRepos = testgit.Templates{Name: "epic and feature"}

// addEpicFeature adds the fixture's one epic and one feature to root.
func addEpicFeature(t testing.TB, root, changeType string) {
	t.Helper()
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/04-form-engine.md"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: changeType, Source: "design/04-form-engine.md"})
	require.NoError(t, err)
}

// writeFeatureProse writes a non-stub feature.md and a feature.eval.md carrying
// one criterion: the prose pair alone, for the fixtures that lay their own tasks.
func writeFeatureProse(t testing.TB, root string) {
	t.Helper()
	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Certification form loading\n\nLoad the CERT-LTI-02-A form from its JSON Schema and render it.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The form loads\n    polarity: introduce\n    binding: { type: test, ref: spec/form_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
}

// writeStage2Output writes the whole of the second stage's output onto the
// fixture feature: the specification, the acceptance contract, and one task
// carrying a written task.md. `spec feature` validates all three, because the
// specify stage is one act with one output (cli-surface.md, "Specifying a
// feature"), so a fixture writing only the prose pair is refused at the gate.
//
// The task it lays is the one the fixtures downstream go on to name, so their
// own AddTask of that key is the idempotent no-op the add verbs promise.
func writeStage2Output(t testing.TB, root string) {
	t.Helper()
	writeFeatureProse(t, root)
	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), testDeps(), "form-engine/certification-form-loading/transcribe-the-schema", "")
	require.NoError(t, err)
	writeTaskSpec(t, root, "transcribe-the-schema", "## Approach\n\nTranscribe the JSON Schema into the form model.")
}

// laySpeccableFeature lays the shared one-epic-one-feature fixture with the
// given change_type and the whole of stage 2 written (spec, contract, and one
// task carrying a real implementation spec): the shape `spec feature` is ready
// to validate, left in draft so the caller drives, or is refused, the
// transition itself. It is layEpicFeatureCT plus writeStage2Output, the same
// two fixtures TestSpecFeatureRequiresATaskWithARealImplementationSpec and
// TestSpecFeatureContractFaultsStayMisuse already compose by hand. It is laid
// once per change_type and copied into each test.
func laySpeccableFeature(t *testing.T, changeType string) string {
	t.Helper()
	return speccableFeatureRepos.Copy(t, changeType, func(t testing.TB, root string) {
		layOwnedRepo(t, root)
		addEpicFeature(t, root, changeType)
		writeStage2Output(t, root)
	})
}

// speccableFeatureRepos holds one laid laySpeccableFeature fixture per
// change_type.
var speccableFeatureRepos = testgit.Templates{Name: "speccable feature"}

func resolveNode(t testing.TB, tree *store.Tree, noun, raw string) store.Node {
	t.Helper()
	n, err := resolveOne(tree, noun, raw)
	require.NoError(t, err)
	return n
}

func TestTransitionEpicApproveDraftToReady(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "form-engine")

	res, err := mutate.Transition(tree, testDeps(), epic, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, "ready", res.Status)
	require.Equal(t, "E-aaaaaa", res.ID)

	reloaded := load(t, root)
	require.Equal(t, "ready", reloaded.Epics["E-aaaaaa"].Status)

	// Source provenance survives the rewrite.
	raw, err := os.ReadFile(filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "source: design/04-form-engine.md")

	// An approve event is recorded (the add wrote one per node already).
	evs, err := events.List(reloaded)
	require.NoError(t, err)
	var found bool
	for _, e := range evs {
		if e.Type == "approve.epic" && containsStr(e.Targets, "E-aaaaaa") {
			found = true
		}
	}
	require.True(t, found, "an approve.epic event should be recorded")
}

func TestTransitionSpecValidatesFiles(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")

	// A freshly added feature has a stub feature.md and no feature.eval.md.
	_, err := mutate.Transition(tree, testDeps(), feat, mutate.TransitionInput{Verb: "spec"})
	require.Error(t, err)
	var inc *mutate.SpecIncompleteError
	require.ErrorAs(t, err, &inc)
	require.Contains(t, inc.Incomplete, "feature.md")
	require.Contains(t, inc.Incomplete, "feature.eval.md")
	require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status)

	// With a real spec and eval the assertion validates draft -> specced.
	writeStage2Output(t, root)
	tree = load(t, root)
	feat = resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	res, err := mutate.Transition(tree, testDeps(), feat, mutate.TransitionInput{Verb: "spec"})
	require.NoError(t, err)
	require.Equal(t, "specced", res.Status)
	require.Equal(t, "specced", load(t, root).Features["F-aaaaaa"].Status)
}

// TestTransitionSpecRejectsMalformedEval is the acceptance-contract teeth: a
// feature.md that is real prose but a feature.eval.md whose single criterion is
// just prose (no binding) is refused as an invalid contract (exit 2 misuse), not
// silently accepted, and the message names the offending criterion.
func TestTransitionSpecRejectsMalformedEval(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Certification form loading\n\nReal spec prose, no longer a stub.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The form should feel nice\n---\n\n# Acceptance\n"), 0o644))

	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	_, err := mutate.Transition(tree, testDeps(), feat, mutate.TransitionInput{Verb: "spec"})
	require.Error(t, err)

	var ce *store.EvalContractError
	require.ErrorAs(t, err, &ce, "a malformed contract is a typed EvalContractError")
	require.Equal(t, 2, ce.ExitCode())
	require.Contains(t, err.Error(), "C1")
	require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status, "a refused spec must not move the feature")
}

func TestTransitionIllegalReturnsTypedError(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")

	_, err := mutate.Transition(tree, testDeps(), feat, mutate.TransitionInput{Verb: "approve"})
	require.Error(t, err)
	var ill *lifecycle.IllegalTransitionError
	require.ErrorAs(t, err, &ill)
	require.Equal(t, "draft", ill.From)
	require.Equal(t, "approve", ill.Verb)
	require.ElementsMatch(t, []string{"cancel", "spec"}, ill.Legal)
	require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status)
}

func TestTransitionStartBlockedByOpenQuestion(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	// Walk the feature to ready.
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	// Record an open question that blocks the feature.
	writeOpenQuestion(t, root, "Q-zzzzzz", "F-aaaaaa")

	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, "T-aaaaaa")
	_, err := mutate.Transition(tree, testDeps(), task, mutate.TransitionInput{Verb: "start"})
	require.Error(t, err)
	var blk *mutate.BlockedByQuestionsError
	require.ErrorAs(t, err, &blk)
	require.Len(t, blk.Blockers, 1)
	require.Equal(t, "Q-zzzzzz", blk.Blockers[0].Question)
	require.Equal(t, "ready", load(t, root).Features["F-aaaaaa"].Status, "a blocked start must not transition")

	// A feature assumption is record-only and does not clear the task's hold.
	tree = load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	res, err := mutate.Transition(tree, testDeps(), feat, mutate.TransitionInput{Verb: "approve", AssumeQuestion: "Q-zzzzzz", Reason: "proceeding under the documented assumption"})
	require.NoError(t, err)
	require.Equal(t, "ready", res.Status)
	err = tryTransition(t, root, testDeps(), store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	require.ErrorAs(t, err, &blk)
	_, err = mutate.AnswerQuestion(load(t, root), testDeps(), stubRev(root), mutate.AnswerInput{
		Node: "Q-zzzzzz", Answer: "Use the reviewed schema."})
	require.NoError(t, err)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})

	reloaded := load(t, root)
	f := reloaded.Features["F-aaaaaa"]
	require.Equal(t, "active", f.Status)
	require.Contains(t, f.AssumptionBasis, "Q-zzzzzz")
	require.Equal(t, "proceeding under the documented assumption", f.AssumptionBasis["Q-zzzzzz"].Reason)
}

func TestTaskAssumeRefusalNamesCancel(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	err := tryTransition(t, root, testDeps(), store.NounTask, "T-aaaaaa",
		mutate.TransitionInput{Verb: "start", AssumeQuestion: "Q-zzzzzz", Reason: "assume it"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `answer the question before executing the task, or cancel one that no longer needs a decision with tiller cancel question <Q-id> --reason "..."`)
}

func TestFeatureQuestionBlockersIgnoreResolvedAndCancelledQuestions(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"resolved", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			root := layEpicFeature(t)
			writeQuestionWithStatus(t, root, "Q-zzzzzz", "F-aaaaaa", status)
			tree := load(t, root)
			feat := tree.Features["F-aaaaaa"]

			require.Empty(t, gen.FeatureQuestionBlockers(tree, feat))
		})
	}
}

func TestTransitionTaskApproveThenStart(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)

	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	require.Equal(t, "ready", load(t, root).Tasks["T-aaaaaa"].Status)

	res := driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, "in_progress", res.Status)
	require.Equal(t, "in_progress", load(t, root).Tasks["T-aaaaaa"].Status)
}

func TestTransitionRefreshBasisRecordsAcceptedAgainst(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	// The basis is a git_blob taken over committed content, so the feature files
	// must be under version control before the basis is recorded.
	gitInitM(t, root)
	gitCommitAllM(t, root, "lay tree")
	gitRunM(t, root, "branch", "-M", "main")

	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	res, err := mutate.Transition(tree, testDeps(), feat, mutate.TransitionInput{Verb: "approve", RefreshBasis: true})
	require.NoError(t, err)
	require.Equal(t, "ready", res.Status, "refresh-basis does not change lifecycle state")

	f := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, store.KindGitBlob, f.AcceptedAgainst.Spec.Kind)
	require.True(t, strings.HasPrefix(f.AcceptedAgainst.Spec.Value, "git:"))
	require.Equal(t, filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md"), f.AcceptedAgainst.Spec.Path)
	require.Equal(t, store.KindGitBlob, f.AcceptedAgainst.Eval.Kind)
}

func TestRefreshBasisRefusesTerminalFeatureAndTask(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		noun     string
		status   string
		stateRel string
		proseRel string
		node     func(*store.Tree) store.Node
		basis    func(*store.Tree) store.BasisRefs
	}{
		{
			name:     "done feature",
			noun:     store.NounFeature,
			status:   "done",
			stateRel: filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml"),
			proseRel: filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md"),
			node: func(tree *store.Tree) store.Node {
				return tree.Features["F-aaaaaa"]
			},
			basis: func(tree *store.Tree) store.BasisRefs {
				return tree.Features["F-aaaaaa"].AcceptedAgainst
			},
		},
		{
			name:     "cancelled feature",
			noun:     store.NounFeature,
			status:   "cancelled",
			stateRel: filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml"),
			proseRel: filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md"),
			node: func(tree *store.Tree) store.Node {
				return tree.Features["F-aaaaaa"]
			},
			basis: func(tree *store.Tree) store.BasisRefs {
				return tree.Features["F-aaaaaa"].AcceptedAgainst
			},
		},
		{
			name:     "done task",
			noun:     store.NounTask,
			status:   "done",
			stateRel: filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "tasks", "transcribe-the-schema", "task.yml"),
			proseRel: filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "tasks", "transcribe-the-schema", "task.md"),
			node: func(tree *store.Tree) store.Node {
				return tree.Tasks["T-aaaaaa"]
			},
			basis: func(tree *store.Tree) store.BasisRefs {
				return *tree.Tasks["T-aaaaaa"].AcceptedAgainst
			},
		},
		{
			name:     "cancelled task",
			noun:     store.NounTask,
			status:   "cancelled",
			stateRel: filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "tasks", "transcribe-the-schema", "task.yml"),
			proseRel: filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading", "tasks", "transcribe-the-schema", "task.md"),
			node: func(tree *store.Tree) store.Node {
				return tree.Tasks["T-aaaaaa"]
			},
			basis: func(tree *store.Tree) store.BasisRefs {
				return *tree.Tasks["T-aaaaaa"].AcceptedAgainst
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := layApprovedFeatureGit(t)
			beforeBasis := tc.basis(load(t, root))
			require.NotEmpty(t, beforeBasis.Spec.Value)

			require.NoError(t, os.WriteFile(filepath.Join(root, tc.proseRel),
				[]byte("# Changed prose\n\nThis edit must never re-pin terminal work.\n"), 0o644))

			statePath := filepath.Join(root, tc.stateRel)
			state, err := os.ReadFile(statePath)
			require.NoError(t, err)
			terminalState := strings.Replace(string(state), "status: ready", "status: "+tc.status, 1)
			require.NotEqual(t, string(state), terminalState, "the terminal status edit must land")
			if tc.noun == store.NounTask && tc.status == "done" {
				terminalState = strings.Replace(terminalState, "started: false", "started: true", 1)
			}
			require.NoError(t, os.WriteFile(statePath, []byte(terminalState), 0o644))

			before := snapshotDir(t, root)
			tree := load(t, root)
			_, err = mutate.Transition(tree, gitDeps(root), tc.node(tree),
				mutate.TransitionInput{Verb: "approve", RefreshBasis: true})

			var illegal *lifecycle.IllegalTransitionError
			require.ErrorAs(t, err, &illegal)
			require.Equal(t, tc.status, illegal.From)
			require.Equal(t, "approve", illegal.Verb)
			require.Empty(t, illegal.Legal)
			require.Equal(t, tc.noun, illegal.Noun)

			afterTree := load(t, root)
			require.Equal(t, beforeBasis, tc.basis(afterTree), "terminal accepted basis must remain unchanged")
			require.Equal(t, tc.status, tc.node(afterTree).NodeStatus())
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestRefreshBasisExpiresAnAttestationWhenTheEvalContractChanges(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGitWithEval(t, "true", "true", requiredManualEvalMD)
	feat := load(t, root).Features["F-aaaaaa"]
	_, err := mutate.Eval(load(t, root), testDeps(), vcs.New(root), feat, mutate.EvalInput{})
	require.NoError(t, err)
	_, err = mutate.Attest(load(t, root), testDeps(), load(t, root).Features["F-aaaaaa"], mutate.AttestInput{
		Criterion: "C1", Status: store.CriterionStatusPass,
	})
	require.NoError(t, err)

	evalPath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.eval.md")
	data, err := os.ReadFile(evalPath)
	require.NoError(t, err)
	changed := strings.Replace(string(data), "A reviewer confirms the rendered layout matches the source", "A reviewer confirms the changed layout matches the source", 1)
	require.NotEqual(t, string(data), changed)
	require.NoError(t, os.WriteFile(evalPath, []byte(changed), 0o644))

	tree := load(t, root)
	_, err = mutate.Transition(tree, gitDeps(root), tree.Features["F-aaaaaa"], mutate.TransitionInput{
		Verb: "approve", RefreshBasis: true,
	})
	require.NoError(t, err)

	refreshed := load(t, root).Features["F-aaaaaa"].Gates["evals"]
	require.Equal(t, store.GateStatusStale, refreshed.Status)
	require.Equal(t, store.CriterionStatusStale, refreshed.Criteria["C1"].Status)

	feat = load(t, root).Features["F-aaaaaa"]
	_, err = mutate.Check(load(t, root), testDeps(), vcs.New(root), feat, mutate.CheckInput{})
	require.NoError(t, err)

	checked := load(t, root).Features["F-aaaaaa"].Gates["evals"]
	require.Equal(t, store.CriterionStatusNeedsAttestation, checked.Criteria["C1"].Status)
	require.Equal(t, store.GateStatusPending, checked.Status)
}

func TestRefreshBasisPreservesAnAttestationWhenTheEvalContractIsUnchanged(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGitWithEval(t, "true", "true", requiredManualEvalMD)
	feat := load(t, root).Features["F-aaaaaa"]
	_, err := mutate.Eval(load(t, root), testDeps(), vcs.New(root), feat, mutate.EvalInput{})
	require.NoError(t, err)
	_, err = mutate.Attest(load(t, root), testDeps(), load(t, root).Features["F-aaaaaa"], mutate.AttestInput{
		Criterion: "C1", Status: store.CriterionStatusPass,
	})
	require.NoError(t, err)
	before := load(t, root).Features["F-aaaaaa"].Gates["evals"]

	tree := load(t, root)
	_, err = mutate.Transition(tree, gitDeps(root), tree.Features["F-aaaaaa"], mutate.TransitionInput{
		Verb: "approve", RefreshBasis: true,
	})
	require.NoError(t, err)

	refreshed := load(t, root).Features["F-aaaaaa"].Gates["evals"]
	require.Equal(t, store.GateStatusPass, refreshed.Status)
	require.Equal(t, store.CriterionStatusPass, refreshed.Criteria["C1"].Status)
	require.Equal(t, before.Criteria["C1"].Evidence, refreshed.Criteria["C1"].Evidence)
}

func TestRefreshBasisRejectsMalformedEvalContract(t *testing.T) {
	t.Parallel()
	root := layActiveFeatureGitWithEval(t, "true", "true", requiredManualEvalMD)
	evalPath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.eval.md")
	require.NoError(t, os.WriteFile(evalPath, []byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: Missing a binding\n---\n"), 0o644))
	before := load(t, root).Features["F-aaaaaa"].AcceptedAgainst

	tree := load(t, root)
	_, err := mutate.Transition(tree, gitDeps(root), tree.Features["F-aaaaaa"], mutate.TransitionInput{
		Verb: "approve", RefreshBasis: true,
	})

	var contractErr *store.EvalContractError
	require.ErrorAs(t, err, &contractErr)
	require.Equal(t, before, load(t, root).Features["F-aaaaaa"].AcceptedAgainst)
}

// TestTransitionIdempotentNoOp asserts re-approving an already-ready epic is a
// safe no-op that writes no event. The epic's other end has no verb to retry at
// all now: done is a rollup sync computes and archive is the human's own
// deliberate close, so approve is where the mutation contract's idempotency rule
// is exercised at this grain (cli-surface.md, Output and exit codes).
func TestTransitionIdempotentNoOp(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	require.Equal(t, "ready", load(t, root).Epics["E-aaaaaa"].Status)

	before, err := events.List(load(t, root))
	require.NoError(t, err)

	tree := load(t, root)
	epic := resolveNode(t, tree, store.NounEpic, "form-engine")
	res, err := mutate.Transition(tree, testDeps(), epic, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	require.False(t, res.Changed, "re-approving a ready epic is a no-op")
	require.Equal(t, "ready", res.Status)

	after, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Equal(t, len(before), len(after), "an idempotent no-op writes no event")
}

// TestTransitionEpicHasNoVerbToDone pins the model change: no verb crosses an
// epic to done. accept is gone outright, and every other verb the table knows is
// refused from ready with the typed illegal-transition answer naming what IS
// legal, which is cancel alone (artifact-schema.md, Epics States: "The done
// crossing is not a gate at all"). sync records the crossing instead, over the
// features (sync_test.go's epic rollup).
func TestTransitionEpicHasNoVerbToDone(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})

	for _, verb := range []string{"accept", "sync", "start", "submit"} {
		tree := load(t, root)
		epic := resolveNode(t, tree, store.NounEpic, "form-engine")
		_, err := mutate.Transition(tree, testDeps(), epic, mutate.TransitionInput{Verb: verb})
		require.Errorf(t, err, "%s must not cross a ready epic", verb)
		var ill *lifecycle.IllegalTransitionError
		require.ErrorAs(t, err, &ill)
		require.Equal(t, []string{"cancel"}, ill.Legal,
			"a ready epic's only verb is cancel; done arrives by rollup")
		require.Equal(t, "ready", load(t, root).Epics["E-aaaaaa"].Status,
			"a refused verb must not mutate state")
	}
}

// --- the merged specify stage and the plan gate ---

// TestSpecFeatureRequiresATaskWithARealImplementationSpec pins the merged second
// stage: `spec feature` validates the WHOLE stage-2 output, the spec, the
// contract, and the task explosion, so a feature whose tasks are missing or
// still stubs is the not-written-yet blocked answer rather than a pass
// (cli-surface.md, "Specifying a feature").
func TestSpecFeatureRequiresATaskWithARealImplementationSpec(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeFeatureProse(t, root)

	// Spec and contract written, zero tasks: the explosion is not written yet.
	err := trySpec(t, root)
	var inc *mutate.SpecIncompleteError
	require.ErrorAs(t, err, &inc, "a feature with no tasks is blocked, not specced")
	require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status)

	// A task whose task.md is still the generated stub is no more written.
	_, addErr := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/certification-form-loading/only-task", Title: "Only task"})
	require.NoError(t, addErr)
	err = trySpec(t, root)
	require.ErrorAs(t, err, &inc, "a stub task.md is still not written yet")
	require.Contains(t, strings.Join(inc.Incomplete, " "), "only-task",
		"the refusal names the task whose implementation spec is missing")
	require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status)

	// With the implementation spec written, the whole stage validates.
	writeTaskSpec(t, root, "only-task", "## Approach\n\nParse the schema, emit the form.")
	res := driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	require.Equal(t, "specced", res.Status)
}

// TestSpecFeatureContractFaultsStayMisuse pins the split cli-surface.md draws
// over the stage-2 output across the space of contract faults rather than one
// of them: a contract that is written but malformed is the exit-2 misuse class
// naming the fault, never the exit-1 "not written yet" answer, and adding the
// task explosion does not reclassify it.
func TestSpecFeatureContractFaultsStayMisuse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		criteria string
		names    string
	}{
		{"no criteria", "criteria: []", "criteria"},
		{"criterion is just prose", "criteria:\n  - id: C1\n    statement: The form should feel nice", "C1"},
		{"unknown binding", "criteria:\n  - id: C2\n    statement: The form loads\n    binding: { type: vibes }", "C2"},
		{"command with no run", "criteria:\n  - id: C3\n    statement: The form loads\n    binding: { type: command }", "C3"},
	} {
		// Each fault is checked with the explosion written and without it. The
		// second arm is what pins the precedence validateSpec documents: a
		// contract fault is the agent's own bug and outranks the missing
		// explosion, which is the ordinary state of work in progress.
		for _, tasks := range []string{"with the explosion written", "with no tasks yet"} {
			t.Run(tc.name+", "+tasks, func(t *testing.T) {
				root := layEpicFeature(t)
				dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
				require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
					[]byte("# Certification form loading\n\nReal spec prose, no longer a stub.\n"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
					[]byte("---\nnode: F-aaaaaa\n"+tc.criteria+"\n---\n\n# Acceptance\n"), 0o644))
				if tasks == "with the explosion written" {
					_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
						Key: "form-engine/certification-form-loading/only-task", Title: "Only task"})
					require.NoError(t, err)
					writeTaskSpec(t, root, "only-task", "## Approach\n\nParse the schema.")
				}

				err := trySpec(t, root)
				var ce *store.EvalContractError
				require.ErrorAs(t, err, &ce, "a written but malformed contract is misuse, not blocked")
				require.Equal(t, 2, ce.ExitCode())
				require.Contains(t, err.Error(), tc.names)
				require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status,
					"a refused spec must not move the feature")
			})
		}
	}
}

// TestSpecFeatureIgnoresCancelledTasks pins both directions of the spec gate's
// question about a cancelled task, which is work a human ended: it cannot carry
// the feature to specced, and it is never named as the file to write. The blocked
// answer is the one place the tool tells an agent what to do next, so naming a
// cancelled task's task.md there would send it to write an implementation spec
// for work nobody is going to do.
func TestSpecFeatureIgnoresCancelledTasks(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	writeFeatureProse(t, root)

	// Two cancelled tasks, one carrying a fully written spec and one still a
	// stub, so both halves have something to be wrong about.
	added, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "form-engine/certification-form-loading/dropped", Title: "Dropped"})
	require.NoError(t, err)
	writeTaskSpec(t, root, "dropped", "## Approach\n\nA fully written spec for work that was then cancelled.")
	stubbed, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "form-engine/certification-form-loading/also-dropped", Title: "Also dropped"})
	require.NoError(t, err)
	for _, id := range []string{added.ID, stubbed.ID} {
		driveWith(t, root, deps, store.NounTask, id, mutate.TransitionInput{Verb: "cancel", Reason: "subsumed"})
		require.Equal(t, "cancelled", load(t, root).Tasks[id].Status)
	}

	// Half one: a written spec on a cancelled task does not satisfy the bar.
	err = trySpec(t, root)
	var inc *mutate.SpecIncompleteError
	require.ErrorAs(t, err, &inc, "a cancelled task cannot carry a feature to specced")
	require.Equal(t, "draft", load(t, root).Features[featureIDIn(t, root)].Status)

	// Half two: neither cancelled task is named as the file to write. With no live
	// task at all the answer is the folder, since there is no live file to name.
	require.Equal(t, []string{store.TaskDirName + "/"}, inc.Incomplete,
		"the blocked answer never names a cancelled task's task.md")

	// A live task beside it is what the answer is about, and what satisfies it.
	live, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "form-engine/certification-form-loading/kept", Title: "Kept"})
	require.NoError(t, err)
	err = trySpec(t, root)
	require.ErrorAs(t, err, &inc)
	require.Equal(t, []string{filepath.Join(store.TaskDirName, "kept", store.TaskProseFile)}, inc.Incomplete,
		"only the live task's spec is asked for")

	writeTaskSpec(t, root, "kept", "## Approach\n\nThe work that is actually happening.")
	res := driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	require.Equal(t, "specced", res.Status)

	// And the plan gate moves the live task alone: the cancelled one is terminal.
	plan := driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	require.Len(t, plan.Tasks, 1, "the gate moves the live task, not the cancelled one")
	require.Equal(t, live.ID, plan.Tasks[0].ID)
	require.Equal(t, "cancelled", load(t, root).Tasks[added.ID].Status, "the cancelled task is left where the human put it")
	require.Nil(t, load(t, root).Tasks[added.ID].AcceptedAgainst, "and nothing is pinned on it")
}

// TestSpecRefusesReplaceWithNoSurface pins the fourth stage-2 check: a replace
// or migrate feature names something that already exists by definition, so a
// plan that cannot say what it replaces has not been specified, and spec
// refuses it as misuse (exit 2) rather than a blocked answer, naming the
// writer that fixes it (cli-surface.md, the spec row).
func TestSpecRefusesReplaceWithNoSurface(t *testing.T) {
	t.Parallel()
	for _, ct := range []string{"replace", "migrate"} {
		t.Run(ct, func(t *testing.T) {
			root := laySpeccableFeature(t, ct)
			tree := load(t, root)

			_, err := mutate.Transition(tree, testDeps(), tree.Features["F-aaaaaa"],
				mutate.TransitionInput{Verb: "spec"})

			var ue interface{ Reason() string }
			require.ErrorAs(t, err, &ue)
			require.Equal(t, "surface_required", ue.Reason())
			require.Contains(t, err.Error(), "link feature",
				"the refusal names the writer that fixes it")
			require.Contains(t, err.Error(), "--surface",
				"the refusal names the flag that fixes it")
			require.Contains(t, err.Error(), "form-engine/certification-form-loading",
				"the refusal names the feature by its key, ready to paste into the command")
			require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status,
				"a refused spec must not move the feature")
		})
	}
}

// TestSpecAllowsReplaceAndMigrateWithSurface is the other half of
// TestSpecRefusesReplaceWithNoSurface: once a surface path is on record (the
// writer the refusal names, tiller link --surface, wired in Task 3), the gate
// that refused for want of one has nothing left to refuse on, and spec
// validates the rest of the stage-2 output exactly as it does for a create.
func TestSpecAllowsReplaceAndMigrateWithSurface(t *testing.T) {
	t.Parallel()
	for _, ct := range []string{"replace", "migrate"} {
		t.Run(ct, func(t *testing.T) {
			root := laySpeccableFeature(t, ct)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "src/legacy"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "src/legacy/thing.go"), []byte("package legacy\n"), 0o644))
			tree := load(t, root)
			_, err := mutate.SetSurface(tree, testDeps(), tree.Features["F-aaaaaa"], []string{"src/legacy/thing.go"})
			require.NoError(t, err)

			tree = load(t, root)
			res, err := mutate.Transition(tree, testDeps(), tree.Features["F-aaaaaa"],
				mutate.TransitionInput{Verb: "spec"})
			require.NoError(t, err)
			require.Equal(t, "specced", res.Status)
		})
	}
}

// TestSpecAllowsModifyAndCreateWithNoSurface is the exempt half of the same
// gate: modify shades into "touch a bit of this" and create touches nothing
// existing, so neither is required to record a surface (artifact-schema.md,
// the brownfield hinge) and spec validates the rest of the stage-2 output with
// none on record.
func TestSpecAllowsModifyAndCreateWithNoSurface(t *testing.T) {
	t.Parallel()
	for _, ct := range []string{"modify", "create"} {
		t.Run(ct, func(t *testing.T) {
			root := laySpeccableFeature(t, ct)
			tree := load(t, root)

			res, err := mutate.Transition(tree, testDeps(), tree.Features["F-aaaaaa"],
				mutate.TransitionInput{Verb: "spec"})
			require.NoError(t, err)
			require.Equal(t, "specced", res.Status)
		})
	}
}

// TestSpecEarlierChecksOutrankMissingSurface pins the ordering validateSpec's
// own doc comment asserts in prose: the surface check runs LAST, so a replace
// or migrate feature short on an earlier check as well as its surface gets the
// earlier refusal, never surface_required. It is the fourth check's own
// instance of the same precedence claim TestSpecFeatureContractFaultsStayMisuse
// pins one check lower (a contract fault outranks a missing explosion). Without
// a test like this only the order the clauses happen to be written in
// guarantees the precedence, and every other test in the file would still pass
// if a future edit moved the surface check first.
func TestSpecEarlierChecksOutrankMissingSurface(t *testing.T) {
	t.Parallel()
	for _, ct := range []string{"replace", "migrate"} {
		t.Run(ct+", stub spec outranks missing surface", func(t *testing.T) {
			// layEpicFeatureCT leaves feature.md as the generated stub, writes
			// no feature.eval.md, and records no surface, so this fixture is
			// short on check 1 AND check 4 at once: a pass here only proves
			// something if the not-written-yet refusal wins.
			root := layEpicFeatureCT(t, ct)

			err := trySpec(t, root)
			var inc *mutate.SpecIncompleteError
			require.ErrorAs(t, err, &inc, "the not-written-yet refusal must win over the surface requirement")
			require.Contains(t, inc.Incomplete, "feature.md")
			require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status,
				"a refused spec must not move the feature")
		})

		t.Run(ct+", malformed contract outranks missing surface", func(t *testing.T) {
			root := layEpicFeatureCT(t, ct)
			dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
				[]byte("# Certification form loading\n\nReal spec prose, no longer a stub.\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
				[]byte("---\nnode: F-aaaaaa\ncriteria: []\n---\n\n# Acceptance\n"), 0o644))
			_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
				Key: "form-engine/certification-form-loading/only-task", Title: "Only task"})
			require.NoError(t, err)
			writeTaskSpec(t, root, "only-task", "## Approach\n\nParse the schema.")
			// Checks 1 and 3 are satisfied (real prose, a written task spec)
			// and no surface is recorded, isolating this case to check 2 (the
			// malformed contract) against check 4 (the missing surface) alone,
			// rather than leaving check 3 also short as an uncontrolled variable.

			err = trySpec(t, root)
			var ce *store.EvalContractError
			require.ErrorAs(t, err, &ce, "the malformed-contract refusal must win over the surface requirement")
			require.Equal(t, "draft", load(t, root).Features["F-aaaaaa"].Status,
				"a refused spec must not move the feature")
		})
	}
}

// TestApproveFeatureApprovesThePlanAndPinsEveryBasis is the plan gate: one human
// decision moves the feature specced->ready, flips every initial proposed task to
// ready in the same act, and records the active-work contract at both grains, the
// feature's spec and eval and each task's own task.md (cli-surface.md,
// "Specifying a feature"; artifact-schema.md, Tasks: States).
func TestApproveFeatureApprovesThePlanAndPinsEveryBasis(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa", "zulu")

	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	res, err := mutate.Transition(tree, deps, feat, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)
	require.Equal(t, "ready", res.Status)
	require.Len(t, res.Tasks, 2, "the result names every task the one gate moved")

	after := load(t, root)
	f := after.Features[feat.NodeID()]
	require.Equal(t, "ready", f.Status)
	require.Equal(t, store.KindGitBlob, f.AcceptedAgainst.Spec.Kind)
	require.Equal(t, blobOf(t, root, filepath.Join(featureRelDirT, "feature.md")), f.AcceptedAgainst.Spec.Value)
	require.Equal(t, blobOf(t, root, filepath.Join(featureRelDirT, "feature.eval.md")), f.AcceptedAgainst.Eval.Value)

	// Each task is pinned against its OWN task.md, which is what a per-task
	// value would satisfy only by accident: the two files differ, so the two
	// pins must differ, and each must equal the blob of its own file.
	pins := map[string]string{}
	for _, tk := range after.FeatureTasks[f.ID] {
		require.Equal(t, "ready", tk.Status, "initial tasks flip in the same act")
		require.NotNil(t, tk.AcceptedAgainst, "task.md is pinned exactly as the feature spec is")
		rel := filepath.Join(store.TaskRelDir("form-engine", "certification-form-loading", localKeyT(*tk.Key)), store.TaskProseFile)
		require.Equal(t, store.KindGitBlob, tk.AcceptedAgainst.Spec.Kind)
		require.Equal(t, rel, tk.AcceptedAgainst.Spec.Path)
		require.Equal(t, blobOf(t, root, rel), tk.AcceptedAgainst.Spec.Value)
		pins[tk.ID] = tk.AcceptedAgainst.Spec.Value
	}
	require.Len(t, distinct(pins), 2, "two different task.md files pin two different blobs")
}

// TestApproveFeaturePlanGateReadsStatusNotProse pins what the plan gate does
// with a task whose task.md is still the generated stub. The spec gate's bar is
// one written task.md, so a stub sibling reaches the gate; the gate selects on
// status alone (ProposedTasks) and never reads prose, so it flips that task to
// ready and pins accepted_against over the boilerplate like any other. No
// per-task prose gate stands behind it, and this is the test that says so, since
// nothing else in the tree would notice if one appeared or vanished.
func TestApproveFeaturePlanGateReadsStatusNotProse(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	writeFeatureProse(t, root)

	// alfa's spec is written; zulu is left exactly as `tiller add task` generated
	// it, so the stub under test is the real one rather than a retyped copy.
	for _, key := range []string{"alfa", "zulu"} {
		_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
			Key: "form-engine/certification-form-loading/" + key, Title: "Task " + key})
		require.NoError(t, err)
	}
	writeTaskSpec(t, root, "alfa", "## Approach\n\nThe one written spec, which is the bar.")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})

	res := driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	require.Len(t, res.Tasks, 2, "the gate moves both, the stub included")

	for _, tk := range load(t, root).Tasks {
		require.Equal(t, "ready", tk.Status)
		require.NotNil(t, tk.AcceptedAgainst, "the stub is pinned like any other task")
	}
}

// TestApproveFeatureRecordsOneEventPerTransition pins the journaled multi-file
// mutation's audit half: the plan gate moves N+1 nodes and records exactly one
// event per logical transition, each naming its own node (cli-surface.md,
// Working-tree posture).
func TestApproveFeatureRecordsOneEventPerTransition(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa", "zulu")

	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	_, err := mutate.Transition(tree, deps, feat, mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)

	after := load(t, root)
	evs, err := events.List(after)
	require.NoError(t, err)

	var featureTargets, taskTargets []string
	for _, e := range evs {
		switch e.Type {
		case "approve.feature":
			featureTargets = append(featureTargets, e.Targets...)
		case "approve.task":
			taskTargets = append(taskTargets, e.Targets...)
		}
	}
	require.Equal(t, []string{feat.NodeID()}, featureTargets, "one event for the feature")
	require.Len(t, taskTargets, 2, "one event per task the gate moved")
	require.ElementsMatch(t, taskIDs(after, feat.NodeID()), taskTargets)
}

// TestApproveFeatureLeavesNothingHalfApplied is the atomicity requirement, checked
// by making a write fail rather than by asserting it holds. A half-applied plan
// gate is a feature at ready whose tasks are still proposed, or tasks pinned
// against a basis the feature never recorded, and cli-surface.md's Working-tree
// posture is what forbids it.
func TestApproveFeatureLeavesNothingHalfApplied(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa", "zulu")

	// Fail at the event destination after every node write has landed, the
	// widest partial-apply window, while leaving the source directories
	// writable so rollback can restore every preimage.
	eventDir := filepath.Join(root, ".tiller", "events", "2026", "06", "27")
	require.NoError(t, os.MkdirAll(eventDir, 0o755))
	require.NoError(t, os.Chmod(eventDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(eventDir, 0o755) })

	tree := load(t, root)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading")
	_, err := mutate.Transition(tree, deps, feat, mutate.TransitionInput{Verb: "approve"})
	require.Error(t, err, "a task file that cannot be written fails the whole gate")

	after := load(t, root)
	f := after.Features[feat.NodeID()]
	require.Equal(t, "specced", f.Status, "the feature is left where it was")
	require.Empty(t, f.AcceptedAgainst.Spec.Value, "with no basis pinned")
	for _, tk := range after.FeatureTasks[f.ID] {
		require.Equal(t, "proposed", tk.Status, "no task moved, including the one already written")
		require.Nil(t, tk.AcceptedAgainst)
	}
	evs, err := events.List(after)
	require.NoError(t, err)
	for _, e := range evs {
		require.NotEqual(t, "approve.feature", e.Type, "a refused gate records no transition")
		require.NotEqual(t, "approve.task", e.Type)
	}
	require.NoFileExists(t, filepath.Join(root, ".tiller", ".cache", "journal.yml"))
}

// TestApproveTaskPinsItsOwnBasisAndRefreshRepins is the mid-flight arm: a task
// added after the plan was approved lands proposed, is approved individually, and
// pins its own task.md the same way; --refresh-basis then re-records the current
// revision without touching the lifecycle state (cli-surface.md, "Specifying a
// feature").
func TestApproveTaskPinsItsOwnBasisAndRefreshRepins(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	added, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "form-engine/certification-form-loading/mid-flight", Title: "Mid flight"})
	require.NoError(t, err)
	require.Equal(t, "proposed", added.Status, "a task added mid-flight lands proposed")
	writeTaskSpec(t, root, "mid-flight", "## Approach\n\nAdd the missing validator.")

	rel := filepath.Join(store.TaskRelDir("form-engine", "certification-form-loading", "mid-flight"), store.TaskProseFile)
	driveWith(t, root, deps, store.NounTask, added.ID, mutate.TransitionInput{Verb: "approve"})
	tk := load(t, root).Tasks[added.ID]
	require.Equal(t, "ready", tk.Status)
	require.NotNil(t, tk.AcceptedAgainst)
	firstPin := tk.AcceptedAgainst.Spec.Value
	require.Equal(t, blobOf(t, root, rel), firstPin)

	// The human reviews an edit to the implementation spec and refreshes.
	writeTaskSpec(t, root, "mid-flight", "## Approach\n\nAdd the validator AND the fixture.")
	res := driveWith(t, root, deps, store.NounTask, added.ID,
		mutate.TransitionInput{Verb: "approve", RefreshBasis: true})
	require.Equal(t, "ready", res.Status, "refresh-basis does not change lifecycle state")

	tk = load(t, root).Tasks[added.ID]
	require.Equal(t, "ready", tk.Status)
	require.NotEqual(t, firstPin, tk.AcceptedAgainst.Spec.Value, "the edit is re-recorded")
	require.Equal(t, blobOf(t, root, rel), tk.AcceptedAgainst.Spec.Value)
}

// gitDeps is testDeps with an explicit real git window, so a test exercising
// the commit anchor documents its git dependency at the call site (task-2-brief,
// "Commit anchors on both basis records").
func gitDeps(root string) mutate.Deps {
	d := testDeps()
	d.Rev = vcs.New(root)
	return d
}

// layEpicFeatureGit is layEpicFeature staged through spec, in a real git repo
// with the specced tree committed: the shape TestApprovePinsTheCommitAnchor
// approves from, since pinning the commit anchor needs a real HEAD to read
// (artifact-schema.md, Accepted basis).
func layEpicFeatureGit(t *testing.T) string {
	t.Helper()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})

	gitInitM(t, root)
	gitCommitAllM(t, root, "lay tree")
	return root
}

// layApprovedFeatureGit is layEpicFeatureGit carried one step further: the
// feature approved, so accepted_against (and its commit anchor) is on record
// before a test advances HEAD again.
func layApprovedFeatureGit(t *testing.T) string {
	t.Helper()
	root := layEpicFeatureGit(t)
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	return root
}

// TestApprovePinsTheCommitAnchor pins the new leg of accepted_against: approve
// records HEAD as a git_commit RevisionRef alongside the spec/eval blobs, on
// the feature and on every task the same act carries to ready
// (artifact-schema.md, Accepted basis: "The record also carries a commit
// RevisionRef pinning HEAD at the moment of approval"). The task's own
// schema example comments its anchor as "same anchor the feature record
// carries" (artifact-schema.md, Tasks), so this checks equality, not just
// that both are non-empty: recordBasis and recordTaskBasis each shell their
// own CurrentCommit call, so the shared value is otherwise only an emergent
// fact of HEAD not moving mid-act, never checked structurally.
func TestApprovePinsTheCommitAnchor(t *testing.T) {
	t.Parallel()
	root := layEpicFeatureGit(t)
	tree := load(t, root)

	_, err := mutate.Transition(tree, gitDeps(root), tree.Features["F-aaaaaa"],
		mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)

	got := load(t, root).Features["F-aaaaaa"].AcceptedAgainst
	require.Equal(t, store.KindGitCommit, got.Commit.Kind,
		"approve pins HEAD as the code baseline the plan was approved against")
	require.NotEmpty(t, got.Commit.Value)
	// The initial tasks approved in the same act carry the same anchor.
	for id, tk := range load(t, root).Tasks {
		if tk.AcceptedAgainst != nil {
			require.NotEmptyf(t, tk.AcceptedAgainst.Commit.Value, "task %s missing its anchor", id)
			require.Equalf(t, got.Commit.Value, tk.AcceptedAgainst.Commit.Value,
				"task %s must carry the same anchor the feature record does", id)
		}
	}
}

// TestNewCommitsAloneDoNotTripActiveBasis is a known exception to "every test
// must be able to fail before implementation": BasisDrift compares spec and
// eval content only, so the mutation this pins against (the commit anchor
// quietly joining that comparison) is one the new code could introduce, not
// one the unmodified tree already commits. It passes against base and stays
// passing after, and that is the point: the anchor is a recording, not a
// comparand, so commits landing after approval must never read as drift.
// Without this pin, adding the anchor would silently turn every commit into an
// active_basis refusal.
func TestNewCommitsAloneDoNotTripActiveBasis(t *testing.T) {
	t.Parallel()
	root := layApprovedFeatureGit(t)
	commitFile(t, root, "unrelated.txt", "new work landed")

	tree := load(t, root)
	err := mutate.BasisDrift(tree, tree.Features["F-aaaaaa"], vcs.New(root))
	require.NoError(t, err, "a moved HEAD with untouched spec files is not basis drift")
}

// TestApproveFailsLoudlyWhenHeadCannotResolve pins the error-propagation half
// of the commit anchor (task-2-brief.md, "Error handling"): CurrentCommit
// failing means approve fails loudly with the propagated environment error,
// not a record with a zero anchor. A repo whose .git has been removed cannot
// resolve HEAD, the same failure a repo that was never a git repo at all
// raises. The feature must be left exactly where it was: still specced, with
// no accepted_against written, since recordBasis runs before any write in
// approveFeaturePlan (transition.go) and a failure there returns before
// lifecycle.Apply, the marshal, or commitNodeWrites ever run.
func TestApproveFailsLoudlyWhenHeadCannotResolve(t *testing.T) {
	t.Parallel()
	root := layEpicFeatureGit(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))

	tree := load(t, root)
	_, err := mutate.Transition(tree, gitDeps(root), tree.Features["F-aaaaaa"],
		mutate.TransitionInput{Verb: "approve"})

	var ee *store.EnvError
	require.ErrorAs(t, err, &ee, "HEAD cannot resolve without a .git directory")
	require.Equal(t, vcs.ReasonGitUnavailable, ee.Reason)
	require.Equal(t, 3, ee.ExitCode(), "an unresolvable HEAD is the environment class, not a blocked answer or misuse")

	after := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "specced", after.Status, "a refused approve must not move the feature")
	require.Zero(t, after.AcceptedAgainst, "recordBasis runs before any write, so a HEAD failure leaves no partial basis")
}

// TestRefreshBasisFailsLoudlyWhenHeadCannotResolve isolates recordBasis's own
// error propagation from recordTaskBasis's. The plan-gate approve above
// exercises both in one call (approveFeaturePlan calls recordBasis, then loops
// recordTaskBasis over the initial tasks), so a regression that swallowed the
// error in ONE of the two alone would still be caught by the other
// independently hitting the same missing HEAD, and the test above would not
// tell them apart. approve --refresh-basis on an already-ready feature calls
// recordBasis alone (transitionFeature's RefreshBasis branch never touches a
// task), so this is the one path that pins recordBasis's propagation with
// nothing else able to mask a regression in it specifically.
func TestRefreshBasisFailsLoudlyWhenHeadCannotResolve(t *testing.T) {
	t.Parallel()
	root := layApprovedFeatureGit(t)
	before := load(t, root).Features["F-aaaaaa"].AcceptedAgainst
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))

	tree := load(t, root)
	_, err := mutate.Transition(tree, gitDeps(root), tree.Features["F-aaaaaa"],
		mutate.TransitionInput{Verb: "approve", RefreshBasis: true})

	var ee *store.EnvError
	require.ErrorAs(t, err, &ee, "HEAD cannot resolve without a .git directory")
	require.Equal(t, vcs.ReasonGitUnavailable, ee.Reason)
	require.Equal(t, 3, ee.ExitCode(), "an unresolvable HEAD is the environment class, not a blocked answer or misuse")

	after := load(t, root).Features["F-aaaaaa"]
	require.Equal(t, "ready", after.Status, "a refused refresh-basis must not move the feature")
	require.Equal(t, before, after.AcceptedAgainst,
		"refresh-basis re-records in place, so a refused one must leave the prior basis exactly as it was")
}

// TestTaskRefreshBasisFailsLoudlyWhenHeadCannotResolve is the task-grain
// mirror of TestRefreshBasisFailsLoudlyWhenHeadCannotResolve, and closes the
// isolation the two feature-grain tests above leave open. approveFeaturePlan
// checks recordBasis's error before the per-task loop starts (transition.go,
// approveFeaturePlan), so on a fully HEAD-less repo the feature's own
// CurrentCommit call always fails first and a doctored recordTaskBasis is
// never reached through the plan gate: both tests above isolate recordBasis
// twice and recordTaskBasis never. task approve --refresh-basis
// (transitionTask's RefreshBasis branch) calls recordTaskBasis alone, with no
// recordBasis call in the same act at all, so it is the one path that pins
// recordTaskBasis's own propagation with nothing else able to mask a
// regression in it specifically.
func TestTaskRefreshBasisFailsLoudlyWhenHeadCannotResolve(t *testing.T) {
	t.Parallel()
	root := layApprovedFeatureGit(t)
	before := load(t, root).Tasks["T-aaaaaa"].AcceptedAgainst
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))

	tree := load(t, root)
	_, err := mutate.Transition(tree, gitDeps(root), tree.Tasks["T-aaaaaa"],
		mutate.TransitionInput{Verb: "approve", RefreshBasis: true})

	var ee *store.EnvError
	require.ErrorAs(t, err, &ee, "HEAD cannot resolve without a .git directory")
	require.Equal(t, vcs.ReasonGitUnavailable, ee.Reason)
	require.Equal(t, 3, ee.ExitCode(), "an unresolvable HEAD is the environment class, not a blocked answer or misuse")

	after := load(t, root).Tasks["T-aaaaaa"]
	require.Equal(t, "ready", after.Status, "a refused refresh-basis must not move the task")
	require.Equal(t, before, after.AcceptedAgainst,
		"refresh-basis re-records in place, so a refused one must leave the prior basis exactly as it was")
}

// TestTaskBasisGuardRefusesStartCheckAndSubmit checks the guard over the whole
// verb space cli-surface.md names rather than the one verb it is easiest to
// reach: an approved task.md edited afterwards refuses start, check, AND submit
// with the active_basis blocked answer naming the task, and none of the three
// moves the node.
func TestTaskBasisGuardRefusesStartCheckAndSubmit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		verb  string
		drive []string
		state string
		run   func(t *testing.T, root string, deps mutate.Deps, taskID string) error
	}{
		{
			verb:  "start",
			drive: nil,
			state: "ready",
			run: func(t *testing.T, root string, deps mutate.Deps, taskID string) error {
				return tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})
			},
		},
		{
			verb:  "check",
			drive: []string{"start"},
			state: "in_progress",
			run: func(t *testing.T, root string, deps mutate.Deps, taskID string) error {
				tree := load(t, root)
				n := resolveNode(t, tree, store.NounTask, taskID)
				_, err := mutate.Check(tree, deps, vcs.New(root), n, mutate.CheckInput{})
				return err
			},
		},
		{
			verb:  "submit",
			drive: []string{"start", "check"},
			state: "check",
			run: func(t *testing.T, root string, deps mutate.Deps, taskID string) error {
				pr := 41
				return tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "submit", PR: &pr})
			},
		},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			deps := seqDeps()
			root := laySpeccedPlan(t, deps, "alfa")
			driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

			taskID := onlyTaskID(t, load(t, root))
			for _, v := range tc.drive {
				driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: v})
			}

			// The human edits the implementation spec after approving it.
			writeTaskSpec(t, root, "alfa", "## Approach\n\nA different approach entirely.")

			err := tc.run(t, root, deps, taskID)
			var ab *mutate.ActiveBasisError
			require.ErrorAs(t, err, &ab, "%s must refuse an edited task.md", tc.verb)
			require.Equal(t, store.NounTask, ab.Noun, "the blocked answer names the task, whose basis moved")
			require.Equal(t, taskID, ab.NodeID)
			require.Equal(t, []string{store.TaskProseFile}, ab.Drifted)
			require.Equal(t, tc.state, load(t, root).Tasks[taskID].Status,
				"a refused %s must not move the task", tc.verb)
		})
	}
}

// TestBasisGuardTreatsADeletedPinAsDrift pins the classification of the other
// way a pinned file stops matching: deleting it rather than editing it. It is
// the same active_basis blocked answer (exit 1) the edit gets, not a tool
// failure, because an agent branching on the exit code has to be sent at the
// basis rather than at its own invocation. Checked at both grains, since a task
// pin and a feature pin fail the hash the same way.
func TestBasisGuardTreatsADeletedPinAsDrift(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, rel, drifted, noun string }{
		{"the task's own spec", filepath.Join(store.TaskRelDir("form-engine", "certification-form-loading", "alfa")), store.TaskProseFile, store.NounTask},
		{"the feature's spec", featureRelDirT, "feature.md", store.NounFeature},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := seqDeps()
			root := laySpeccedPlan(t, deps, "alfa")
			driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

			taskID := onlyTaskID(t, load(t, root))

			require.NoError(t, os.Remove(filepath.Join(root, tc.rel, tc.drifted)))

			err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})
			var ab *mutate.ActiveBasisError
			require.ErrorAs(t, err, &ab, "a pinned file that is gone is drift, not a tool failure")
			require.Equal(t, tc.noun, ab.Noun)
			require.Equal(t, []string{tc.drifted}, ab.Drifted)
			require.Equal(t, "ready", load(t, root).Tasks[taskID].Status, "a refused start must not move the task")
		})
	}
}

// TestLegalityIsSettledBeforeTheGuards pins the ordering: a verb that was never
// legal from this state answers illegal_transition, even when the node also has
// a drifted basis or an open question a guard would otherwise refuse on. An
// agent told to refresh a basis when the real problem is that the move does not
// exist from here is sent somewhere it cannot recover from.
func TestLegalityIsSettledBeforeTheGuards(t *testing.T) {
	t.Parallel()
	t.Run("task, drifted basis and an illegal verb", func(t *testing.T) {
		deps := seqDeps()
		root := laySpeccedPlan(t, deps, "alfa")
		driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

		taskID := onlyTaskID(t, load(t, root))
		writeTaskSpec(t, root, "alfa", "## Approach\n\nEdited after approval.")

		// submit is not legal from ready, and the task's basis has also drifted.
		pr := 41
		err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "submit", PR: &pr})
		var ill *lifecycle.IllegalTransitionError
		require.ErrorAs(t, err, &ill, "the move was never legal, so that is the answer")
		require.Equal(t, "ready", ill.From)
		require.Equal(t, "submit", ill.Verb)
		var ab *mutate.ActiveBasisError
		require.False(t, errors.As(err, &ab), "the drift is real but it is not what stopped this call")
	})

	t.Run("feature, open question and an illegal verb", func(t *testing.T) {
		root := layEpicFeature(t)
		writeOpenQuestion(t, root, "Q-zzzzzz", "F-aaaaaa")

		// Approval is illegal before specification, even with a question hold.
		err := tryTransition(t, root, testDeps(), store.NounFeature, "form-engine/certification-form-loading",
			mutate.TransitionInput{Verb: "approve"})
		var ill *lifecycle.IllegalTransitionError
		require.ErrorAs(t, err, &ill)
		require.Equal(t, "draft", ill.From)
		var blk *mutate.BlockedByQuestionsError
		require.False(t, errors.As(err, &blk), "the blocker is real but it is not what stopped this call")
	})
}

// TestTaskSubmitRefusesOnFeatureBasisDrift is the other half of the guard
// cli-surface.md specifies before submit: it compares the parent feature's spec
// and contract too, so an edited feature.md stops a task from reaching review.
func TestTaskSubmitRefusesOnFeatureBasisDrift(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	taskID := onlyTaskID(t, load(t, root))
	driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})
	driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "check"})

	require.NoError(t, os.WriteFile(filepath.Join(root, featureRelDirT, "feature.md"),
		[]byte("# Certification form loading\n\nThe spec changed after the human approved it.\n"), 0o644))

	pr := 41
	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "submit", PR: &pr})
	var ab *mutate.ActiveBasisError
	require.ErrorAs(t, err, &ab, "submit compares the feature's spec, not only the task's")
	require.Equal(t, store.NounFeature, ab.Noun, "the drifted node is the feature, so the refusal names it")
	require.Equal(t, []string{"feature.md"}, ab.Drifted)
	require.Equal(t, "check", load(t, root).Tasks[taskID].Status, "a refused submit must not move the task")
}

// TestSubmitRefusesUnreviewedAnsweredAssumption pins the teeth cli-surface.md
// names in the guard paragraph: "submit alone adds the assumption review ...
// an assumption_basis entry whose question has been answered and not yet
// reviewed in refuses the task." layInProgressTaskUnderAssumedFeature
// (assumption_test.go) carries exactly the shape UnreviewedAssumptions
// reports (Task 5's own predicate tests), so the refusal must name that one
// question by id and the feature that carries it, never the submitting task.
func TestSubmitRefusesUnreviewedAnsweredAssumption(t *testing.T) {
	t.Parallel()
	root := layInProgressTaskUnderAssumedFeature(t)
	tree := load(t, root)

	pr := 7
	_, err := mutate.Transition(tree, gitDeps(root), tree.Tasks["T-aaaaaa"],
		mutate.TransitionInput{Verb: "submit", PR: &pr})

	var aq *mutate.AnsweredQuestionError
	require.ErrorAs(t, err, &aq)
	require.Equal(t, 1, len(aq.Questions))
	require.Equal(t, "Q-aaaaaa", aq.Questions[0].QuestionID)
	require.Equal(t, store.NounFeature, aq.Noun, "the carrying node is the parent feature, never the submitting task")
	require.Equal(t, "F-aaaaaa", aq.NodeID)
	require.Equal(t, "check", load(t, root).Tasks["T-aaaaaa"].Status, "a refused submit must not move the task")
}

// TestStartAndCheckIgnoreUnreviewedAssumptions is the other half of the guard
// paragraph: "start and check deliberately do not [add the assumption
// review], so the escape hatch keeps its purpose." It drives start then check
// itself, over the pre-start fixture layFeatureWithAssumedQuestion leaves
// behind (rather than reusing layInProgressTaskUnderAssumedFeature, which has
// already driven both), so a regression that wired the guard into either one
// surfaces here as a real error, not as an already-satisfied precondition.
// require.NoError is the whole assertion: it subsumes "did not raise
// AnsweredQuestionError specifically" without needing a second, weaker check
// alongside it, and check's advance to "check" status proves the assumption
// is not silently holding the task back under some other name.
func TestStartAndCheckIgnoreUnreviewedAssumptions(t *testing.T) {
	t.Parallel()
	root := layFeatureWithAssumedQuestion(t)
	tree := load(t, root)

	_, err := mutate.Transition(tree, gitDeps(root), tree.Tasks["T-aaaaaa"], mutate.TransitionInput{Verb: "start"})
	require.NoError(t, err, "start must not refuse over an unreviewed answered assumption")

	tree = load(t, root)
	res, err := mutate.Check(tree, gitDeps(root), vcs.New(root), tree.Tasks["T-aaaaaa"], mutate.CheckInput{})
	require.NoError(t, err, "check must not refuse over an unreviewed answered assumption")
	require.Equal(t, "check", res.Status, "check still advances the task; nothing is silently holding it back")
}

// TestSubmitPassesOnceReviewedIn proves the remedy the refusal names actually
// clears it: ReviewAssumption stamps the question's answer in, and the
// identical submit that was just refused now succeeds.
func TestSubmitPassesOnceReviewedIn(t *testing.T) {
	t.Parallel()
	root := layInProgressTaskUnderAssumedFeature(t)
	tree := load(t, root)
	gitRunM(t, root, "checkout", "-b", tree.Tasks["T-aaaaaa"].Branch)

	_, err := mutate.ReviewAssumption(tree, gitDeps(root), tree.Features["F-aaaaaa"], "Q-aaaaaa")
	require.NoError(t, err)

	tree = load(t, root)
	pr := 7
	res, err := mutate.Transition(tree, gitDeps(root), tree.Tasks["T-aaaaaa"],
		mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.NoError(t, err, "the remedy the refusal names must actually clear it")
	require.Equal(t, "in_review", res.Status)
}

// TestSubmitActiveBasisOutranksUnreviewedAssumption pins task-6-brief.md's
// resolution 2: the assumption guard runs AFTER guardTask's existing basis
// check, so when a task's own basis has ALSO drifted, the human gets the
// active_basis refusal first and corrects one thing at a time, the same
// idiom TestLegalityIsSettledBeforeTheGuards already uses above for the
// verb-versus-guard ordering.
func TestSubmitActiveBasisOutranksUnreviewedAssumption(t *testing.T) {
	t.Parallel()
	root := layInProgressTaskUnderAssumedFeature(t)
	// The human edits the task's own implementation spec after it was
	// approved, drifting the task's basis on top of the feature's already
	// unreviewed assumption.
	writeTaskSpec(t, root, "transcribe-the-schema", "## Approach\n\nEdited after approval.")

	tree := load(t, root)
	pr := 7
	_, err := mutate.Transition(tree, gitDeps(root), tree.Tasks["T-aaaaaa"],
		mutate.TransitionInput{Verb: "submit", PR: &pr})

	var ab *mutate.ActiveBasisError
	require.ErrorAs(t, err, &ab, "basis drift wins when both conditions are present")
	var aq *mutate.AnsweredQuestionError
	require.False(t, errors.As(err, &aq), "one refusal at a time: the assumption check must not also have run")
}

// --- helpers ---

// featureRelDirT is the fixture feature's repo-relative folder, the one every
// test in this file lays its epic and feature under.
var featureRelDirT = filepath.Join(".tiller", "epics", "form-engine", "features", "certification-form-loading")

// seqDeps gives every node and event in the multi-node plan gate a stable,
// distinct id.
func seqDeps() mutate.Deps {
	d := testDeps()
	seq := 0
	d.Mint = func(prefix string) string {
		seq++
		return fmt.Sprintf("%s-%06d", prefix, seq)
	}
	return d
}

// laySpeccedPlan lays the shared one-epic-one-feature fixture with a written
// spec, contract, and one task per key carrying a written task.md, then drives
// the feature to specced: the state the plan gate is aimed at.
func laySpeccedPlan(t *testing.T, deps mutate.Deps, taskKeys ...string) string {
	t.Helper()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	writeFeatureProse(t, root)
	for _, key := range taskKeys {
		_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
			Key: "form-engine/certification-form-loading/" + key, Title: "Task " + key})
		require.NoError(t, err)
		_, err = mutate.ClaimTask(load(t, root), deps, "form-engine/certification-form-loading/"+key, "")
		require.NoError(t, err)
		writeTaskSpec(t, root, key, "## Approach\n\nThe implementation spec for "+key+".")
	}
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	return root
}

// layDraftPlan is laySpeccedPlan minus the spec drive: the epic and feature
// exist, tasks are cut with real task.md prose, and the feature has never
// crossed spec. This is the tree the specify stage stands in mid-act, and the
// tree issue #33's incident arc started from.
func layDraftPlan(t *testing.T, deps mutate.Deps, taskKeys ...string) string {
	t.Helper()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	for _, key := range taskKeys {
		_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
			Key: "form-engine/certification-form-loading/" + key, Title: "Task " + key})
		require.NoError(t, err)
		_, err = mutate.ClaimTask(load(t, root), deps, "form-engine/certification-form-loading/"+key, "")
		require.NoError(t, err)
		writeTaskSpec(t, root, key, "## Approach\n\nThe implementation spec for "+key+".")
	}
	return root
}

// setTaskStatus hand-lays a task status onto task.yml, the shape a pre-guard
// tree can already hold on disk (issue #33's field trees do); the guard must
// bite on what is there, not only on what today's verbs can produce.
func setTaskStatus(t *testing.T, root, taskKey, from, to string) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir("form-engine", "certification-form-loading", taskKey), store.TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var task store.Task
	require.NoError(t, yaml.Unmarshal(data, &task))
	require.Equal(t, from, task.Status)
	task.Status = to
	if to == "in_progress" || to == "check" || to == "in_review" || to == "done" {
		task.Started = true
	}
	out, err := yaml.Marshal(task)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// TestApproveTaskRefusesUnderDraftFeature pins the first hole of issue #33's
// incident arc: approve task moved proposed->ready under a feature whose plan
// was never written, and the confirmation routed straight into execute.
func TestApproveTaskRefusesUnderDraftFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	taskID := onlyTaskID(t, load(t, root))

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "approve"})

	var pg *mutate.ParentPlanUngatedError
	require.ErrorAs(t, err, &pg)
	require.Equal(t, "draft", pg.FeatureState)
	require.Equal(t, taskID, pg.NodeID)
	require.Equal(t, "proposed", load(t, root).Tasks[taskID].Status, "a refused approve must not move the task")
}

// TestApproveTaskRefusesUnderSpeccedFeature: the plan exists but the human has
// not taken the plan gate; an individual task approve before approve feature
// would bypass the feature-grain review the initial explosion rides.
func TestApproveTaskRefusesUnderSpeccedFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	taskID := onlyTaskID(t, load(t, root))

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "approve"})

	var pg *mutate.ParentPlanUngatedError
	require.ErrorAs(t, err, &pg)
	require.Equal(t, "specced", pg.FeatureState)
}

// TestStartTaskRefusesUnderDraftFeature covers the legacy shape: a ready task
// already sitting under a draft feature (a pre-guard tree). start is where the
// arc turns into real code, so it is the refusal that matters most.
func TestStartTaskRefusesUnderDraftFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	setTaskStatus(t, root, "alfa", "proposed", "ready")
	taskID := onlyTaskID(t, load(t, root))

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})

	var pg *mutate.ParentPlanUngatedError
	require.ErrorAs(t, err, &pg)
	require.Equal(t, "ready", load(t, root).Tasks[taskID].Status, "a refused start must not move the task")
}

// TestSubmitTaskRefusesUnderUngatedFeature: work already in flight under an
// ungated parent may not reach review either; the guard answers the same way
// at every doing verb rather than only at the door.
func TestSubmitTaskRefusesUnderUngatedFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	setTaskStatus(t, root, "alfa", "proposed", "check")
	taskID := onlyTaskID(t, load(t, root))

	pr := 33
	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "submit", PR: &pr})

	var pg *mutate.ParentPlanUngatedError
	require.ErrorAs(t, err, &pg)
	require.Equal(t, "check", load(t, root).Tasks[taskID].Status, "a refused submit must not move the task")
}

// TestApproveTaskUnderReadyFeatureStillWorks pins the negative space: a task
// added after the plan gate approves individually, exactly as before.
func TestApproveTaskUnderReadyFeatureStillWorks(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{
		Key: "form-engine/certification-form-loading/bravo", Title: "Task bravo"})
	require.NoError(t, err)
	writeTaskSpec(t, root, "bravo", "## Approach\n\nA late addition.")
	tree := load(t, root)
	var lateID string
	for id, tk := range tree.Tasks {
		if tk.Status == "proposed" {
			lateID = id
		}
	}
	require.NotEmpty(t, lateID)

	require.NoError(t, tryTransition(t, root, deps, store.NounTask, lateID, mutate.TransitionInput{Verb: "approve"}))
	require.Equal(t, "ready", load(t, root).Tasks[lateID].Status)
}

// TestApproveRefreshBasisBypassesParentPlanGate pins the escape hatch the
// refresh-basis early return promises now that the approve arm carries its own
// guardParentState call: refresh-basis must answer and return before that
// guard ever runs, or a human re-pinning a task's spec under a feature
// returned to specced for rework would refuse alongside every other doing
// verb, with no way left to clear the drift the guard is refusing on. The
// fixture's feature is specced (not yet approved), one of the two states the
// guard refuses under, so this is exactly the refactor this test protects
// against: hoisting the approve guard above the refresh-basis return.
func TestApproveRefreshBasisBypassesParentPlanGate(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	taskID := onlyTaskID(t, load(t, root))

	err := tryTransition(t, root, deps, store.NounTask, taskID,
		mutate.TransitionInput{Verb: "approve", RefreshBasis: true})

	require.NoError(t, err)
	require.Equal(t, "proposed", load(t, root).Tasks[taskID].Status, "refresh-basis re-pins; it never moves the task")
}

// TestCheckTaskRefusesUnderDraftFeature gives check its own direct net beside
// the shared wiring the other three doing verbs get through transitionTask:
// checkTask (check.go) calls guardTask on its own path, never through
// Transition, so this pins that check answers ParentPlanUngatedError too
// rather than assuming the shared call site inside guardTask covers every
// caller by construction. The task is hand-set to in_progress (setTaskStatus)
// so check is legal from where it stands and the refusal on test is the
// parent-plan one, not the engine's illegal-transition answer.
func TestCheckTaskRefusesUnderDraftFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	setTaskStatus(t, root, "alfa", "proposed", "in_progress")
	taskID := onlyTaskID(t, load(t, root))

	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)
	_, err := mutate.Check(tree, deps, stubRev(root), task, mutate.CheckInput{})

	var pg *mutate.ParentPlanUngatedError
	require.ErrorAs(t, err, &pg)
	require.Equal(t, "in_progress", load(t, root).Tasks[taskID].Status, "a refused check must not move the task")
}

// TestApproveTaskRefusesUnderCancelledFeature is issue #35's front door: a
// feature cancelled from draft leaves its proposed tasks with no plan gate to
// ever cross, and approve used to pass because the parent was neither draft
// nor specced.
func TestApproveTaskRefusesUnderCancelledFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "cancel", Reason: "scope cut"})
	taskID := onlyTaskID(t, load(t, root))

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "approve"})

	var pc *mutate.ParentCancelledError
	require.ErrorAs(t, err, &pc)
	require.Equal(t, taskID, pc.NodeID)
	require.Equal(t, "proposed", load(t, root).Tasks[taskID].Status, "a refused approve must not move the task")
}

// TestStartTaskRefusesUnderFeatureCancelledFromActive pins the post-gate half:
// cancel is legal from active, so a cancelled parent may have a fully approved
// plan behind it, which is exactly why this refusal is its own error and not a
// third ungated state.
func TestStartTaskRefusesUnderFeatureCancelledFromActive(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa", "starter")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounTask, "form-engine/certification-form-loading/starter", mutate.TransitionInput{Verb: "start"})
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "cancel", Reason: "priorities moved"})
	taskID := taskAtKey(t, load(t, root), "form-engine/certification-form-loading/alfa").ID

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})

	var pc *mutate.ParentCancelledError
	require.ErrorAs(t, err, &pc)
	require.Equal(t, "ready", load(t, root).Tasks[taskID].Status, "a refused start must not move the task")
}

// TestSubmitTaskRefusesUnderCancelledFeature: work already in flight when the
// human cancels the feature may not reach review under the dead parent.
func TestSubmitTaskRefusesUnderCancelledFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	taskID := onlyTaskID(t, load(t, root))
	driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})
	driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "check"})
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "cancel", Reason: "priorities moved"})

	pr := 35
	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "submit", PR: &pr})

	var pc *mutate.ParentCancelledError
	require.ErrorAs(t, err, &pc)
	require.Equal(t, "check", load(t, root).Tasks[taskID].Status, "a refused submit must not move the task")
}

// TestCheckTaskRefusesUnderCancelledFeature gives the check verb its own net
// against the cancelled arm, the sibling of TestCheckTaskRefusesUnderDraftFeature.
func TestCheckTaskRefusesUnderCancelledFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	taskID := onlyTaskID(t, load(t, root))
	driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "cancel", Reason: "priorities moved"})
	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)

	_, err := mutate.Check(tree, deps, stubRev(root), task, mutate.CheckInput{})

	var pc *mutate.ParentCancelledError
	require.ErrorAs(t, err, &pc)
	require.Equal(t, "in_progress", load(t, root).Tasks[taskID].Status, "a refused check must not move the task")
}

// TestCancelTaskStillWorksUnderCancelledFeature pins the remedy the blocked
// answer names: cancel is not a doing verb, so retiring the stranded task must
// stay possible or the refusal points at a door that does not open.
func TestCancelTaskStillWorksUnderCancelledFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "cancel", Reason: "scope cut"})
	taskID := onlyTaskID(t, load(t, root))

	require.NoError(t, tryTransition(t, root, deps, store.NounTask, taskID,
		mutate.TransitionInput{Verb: "cancel", Reason: "feature was cancelled"}))
	require.Equal(t, "cancelled", load(t, root).Tasks[taskID].Status)
}

// TestApproveRefreshBasisBypassesCancelledParent extends the promised escape
// hatch to the new arm: re-pinning is not a doing verb either.
func TestApproveRefreshBasisBypassesCancelledParent(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "cancel", Reason: "scope cut"})
	taskID := onlyTaskID(t, load(t, root))

	require.NoError(t, tryTransition(t, root, deps, store.NounTask, taskID,
		mutate.TransitionInput{Verb: "approve", RefreshBasis: true}))
	require.Equal(t, "proposed", load(t, root).Tasks[taskID].Status)
}

// setEpicStatus hand-lays an epic status onto epic.yml, mirroring setTaskStatus:
// an archived epic normally arrives via sync's done rollup (every non-cancelled
// feature done) followed by a human's archive, a path unit fixtures cannot reach
// cheaply, and the guard has to read whatever status a legacy tree already holds
// on disk regardless of how it got there.
func setEpicStatus(t *testing.T, root, epicKey, from, to string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "epic.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: "+from, "status: "+to, 1)
	require.NotEqual(t, string(data), out, "the replace must land or the fixture lies")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// TestSpecFeatureRefusesUnderCancelledEpic is issue #36's front door at the
// feature grain: cancel an epic and the features beneath it may not keep
// moving toward work that can never roll up.
func TestSpecFeatureRefusesUnderCancelledEpic(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa") // feature ends specced; epic form-engine is draft
	driveWith(t, root, deps, store.NounEpic, "form-engine",
		mutate.TransitionInput{Verb: "cancel", Reason: "programme killed"})

	err := tryTransition(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve"})

	var pe *mutate.ParentEpicRetiredError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, "cancelled", pe.EpicState)
	require.Equal(t, store.NounFeature, pe.Noun)
	require.Equal(t, "specced", load(t, root).Features[pe.NodeID].Status, "a refused approve must not move the feature")
}

// TestSpecFeatureRefusesUnderArchivedEpic is the archived sibling of
// TestSpecFeatureRefusesUnderCancelledEpic: archive is the epic's own terminal
// close (done -> archived), and it refuses the same way cancelled does rather
// than opening a third answer (issue #36; JC's call 2026-08-02 that archived
// refuses like cancelled). Reaching archived for real needs a full rollup unit
// fixtures cannot cheaply drive, so this hand-lays the status with
// setEpicStatus, the same legacy-tree shape setTaskStatus already exercises
// one grain down.
func TestSpecFeatureRefusesUnderArchivedEpic(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	setEpicStatus(t, root, "form-engine", "draft", "archived")

	err := tryTransition(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve"})

	var pe *mutate.ParentEpicRetiredError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, "archived", pe.EpicState)
	require.Equal(t, store.NounFeature, pe.Noun)
	require.Equal(t, "specced", load(t, root).Features[pe.NodeID].Status, "a refused approve must not move the feature")
}

// TestTaskVerbsRefuseUnderLiveFeatureCancelledEpic pins the grandparent walk:
// cancel epic is legal from ready while a feature is active, so without this
// arm the #35 arc replays one grain up.
func TestTaskVerbsRefuseUnderLiveFeatureCancelledEpic(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa", "starter")
	driveWith(t, root, deps, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounTask, "form-engine/certification-form-loading/starter", mutate.TransitionInput{Verb: "start"})
	driveWith(t, root, deps, store.NounEpic, "form-engine",
		mutate.TransitionInput{Verb: "cancel", Reason: "programme killed"})
	taskID := taskAtKey(t, load(t, root), "form-engine/certification-form-loading/alfa").ID

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})

	var pe *mutate.ParentEpicRetiredError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, store.NounTask, pe.Noun)
	require.Equal(t, "ready", load(t, root).Tasks[taskID].Status)
}

// TestSubmitTaskRefusesUnderRetiredEpicLiveFeature is task 4's net 2 (issue
// #40), the submit half, the in-flight arc: work already checked when the
// epic is cancelled may not reach review either, even with the feature
// directly above the task still active. Mirrors
// TestSubmitTaskRefusesUnderCancelledFeature exactly, one grain up
// (cancelling the epic rather than the feature); TestCheckTaskRefusesUnderRetiredEpicLiveFeature
// (check_test.go) gives net 2's check half.
func TestSubmitTaskRefusesUnderRetiredEpicLiveFeature(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	taskID := onlyTaskID(t, load(t, root))
	driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "start"})
	driveWith(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "check"})
	driveWith(t, root, deps, store.NounEpic, "form-engine",
		mutate.TransitionInput{Verb: "cancel", Reason: "programme killed"})

	pr := 40
	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "submit", PR: &pr})

	var pe *mutate.ParentEpicRetiredError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, store.NounTask, pe.Noun)
	require.Equal(t, "cancelled", pe.EpicState)
	require.Equal(t, "check", load(t, root).Tasks[taskID].Status, "a refused submit must not move the task")
}

// TestFeatureArmWinsOverEpicArmForTasks: when both the feature and the epic
// are retired, the refusal names the feature, the node the caller can act on.
func TestFeatureArmWinsOverEpicArmForTasks(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "cancel", Reason: "scope cut"})
	driveWith(t, root, deps, store.NounEpic, "form-engine",
		mutate.TransitionInput{Verb: "cancel", Reason: "programme killed"})
	taskID := onlyTaskID(t, load(t, root))

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "approve"})

	var pc *mutate.ParentCancelledError
	require.ErrorAs(t, err, &pc, "the deepest actionable node names the refusal")
}

// TestEpicArmWinsOverPreGateArmForDraftFeatureTask pins issue #38: a task
// under a DRAFT feature under a cancelled epic used to answer
// ParentPlanUngatedError, naming `tiller spec feature` as the remedy, a
// remedy that itself refuses ParentEpicRetiredError once run, a one-hop dead
// end. JC's call 2026-08-03: the epic answer wins over the pre-gate arms,
// since the pre-gate seam is a dead end exactly when the epic is retired.
func TestEpicArmWinsOverPreGateArmForDraftFeatureTask(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := layDraftPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounEpic, "form-engine",
		mutate.TransitionInput{Verb: "cancel", Reason: "programme killed"})
	taskID := onlyTaskID(t, load(t, root))

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "approve"})

	var pe *mutate.ParentEpicRetiredError
	require.ErrorAs(t, err, &pe, "the epic answer wins over the draft pre-gate arm")
	require.Equal(t, "cancelled", pe.EpicState)
	require.Equal(t, store.NounTask, pe.Noun)
	require.Equal(t, "proposed", load(t, root).Tasks[taskID].Status, "a refused approve must not move the task")
}

// TestEpicArmWinsOverPreGateArmForSpeccedFeatureTask is the specced sibling
// of TestEpicArmWinsOverPreGateArmForDraftFeatureTask: the same dead end
// replays one pre-gate state later, plan written but not yet approved.
func TestEpicArmWinsOverPreGateArmForSpeccedFeatureTask(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounEpic, "form-engine",
		mutate.TransitionInput{Verb: "cancel", Reason: "programme killed"})
	taskID := onlyTaskID(t, load(t, root))

	err := tryTransition(t, root, deps, store.NounTask, taskID, mutate.TransitionInput{Verb: "approve"})

	var pe *mutate.ParentEpicRetiredError
	require.ErrorAs(t, err, &pe, "the epic answer wins over the specced pre-gate arm")
	require.Equal(t, "cancelled", pe.EpicState)
	require.Equal(t, store.NounTask, pe.Noun)
	require.Equal(t, "proposed", load(t, root).Tasks[taskID].Status, "a refused approve must not move the task")
}

// TestCancelFeatureStillWorksUnderCancelledEpic pins the remedy open at the
// feature grain, the same promise #35 pinned for tasks.
func TestCancelFeatureStillWorksUnderCancelledEpic(t *testing.T) {
	t.Parallel()
	deps := seqDeps()
	root := laySpeccedPlan(t, deps, "alfa")
	driveWith(t, root, deps, store.NounEpic, "form-engine",
		mutate.TransitionInput{Verb: "cancel", Reason: "programme killed"})

	require.NoError(t, tryTransition(t, root, deps, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "cancel", Reason: "epic was cancelled"}))
}

// writeTaskSpec overwrites one task's task.md with real implementation prose,
// the non-stub state the merged specify stage requires.
func writeTaskSpec(t testing.TB, root, taskKey, body string) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir("form-engine", "certification-form-loading", taskKey), store.TaskProseFile)
	require.NoError(t, os.WriteFile(path, []byte("# "+taskKey+"\n\n"+body+"\n"), 0o644))
}

// trySpec runs `spec feature` on the fixture feature and returns the error alone,
// the shape the blocked-versus-misuse assertions read.
func trySpec(t *testing.T, root string) error {
	t.Helper()
	return tryTransition(t, root, testDeps(), store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "spec"})
}

// tryTransition applies a transition and returns its error, for the refusals a
// test is asserting rather than driving through.
func tryTransition(t *testing.T, root string, deps mutate.Deps, noun, raw string, in mutate.TransitionInput) error {
	t.Helper()
	tree := load(t, root)
	n := resolveNode(t, tree, noun, raw)
	_, err := mutate.Transition(tree, deps, n, in)
	return err
}

// blobOf is the git_blob token of a file's current working-tree content, the
// same value the basis records, so a pin assertion names the file it must be
// over rather than only that some value was written.
func blobOf(t *testing.T, root, rel string) string {
	t.Helper()
	hash, err := vcs.New(root).WorktreeBlobHash(rel)
	require.NoError(t, err)
	return "git:" + hash
}

// distinct is the set of values in a map, for asserting two pins are not one.
func distinct(m map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, v := range m {
		out[v] = true
	}
	return out
}

// localKeyT is the last segment of a canonical key, the folder name at that
// node's own grain.
func localKeyT(canonical string) string {
	parts := strings.Split(canonical, "/")
	return parts[len(parts)-1]
}

// taskIDs lists a feature's task ids, the targets the per-task events must name.
func taskIDs(tree *store.Tree, featureID string) []string {
	var out []string
	for _, tk := range tree.FeatureTasks[featureID] {
		out = append(out, tk.ID)
	}
	return out
}

// featureIDIn is the fixture feature's minted id, for fixtures whose sequence
// minter makes it unpredictable.
func featureIDIn(t *testing.T, root string) string {
	t.Helper()
	tree := load(t, root)
	require.Len(t, tree.Features, 1)
	for id := range tree.Features {
		return id
	}
	return ""
}

// onlyTaskID is the id of the single task a one-task fixture laid down.
func onlyTaskID(t *testing.T, tree *store.Tree) string {
	t.Helper()
	require.Len(t, tree.Tasks, 1)
	for id := range tree.Tasks {
		return id
	}
	return ""
}

func driveTo(t testing.TB, root, noun, raw string, in mutate.TransitionInput) mutate.TransitionResult {
	t.Helper()
	return driveWith(t, root, testDeps(), noun, raw, in)
}

// driveWith is driveTo with the caller's own Deps, for the fixtures that need
// distinct ids across the several nodes one gate moves.
func driveWith(t testing.TB, root string, deps mutate.Deps, noun, raw string, in mutate.TransitionInput) mutate.TransitionResult {
	t.Helper()
	tree := load(t, root)
	n := resolveNode(t, tree, noun, raw)
	res, err := mutate.Transition(tree, deps, n, in)
	require.NoError(t, err)
	return res
}

// setFeatureStatusOnDisk rewrites a feature.yml's status field, the cheap way to
// stage a done child feature for a rollup test without driving the full
// merge-observed sync path.
func setFeatureStatusOnDisk(t *testing.T, root, epicKey, featKey, status string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", featKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: draft", "status: "+status, 1)
	require.NotEqual(t, string(data), out, "expected a status: draft line to rewrite")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

func writeOpenQuestion(t *testing.T, root, qid, blocks string) {
	t.Helper()
	writeQuestionWithStatus(t, root, qid, blocks, "open")
}

func writeQuestionWithStatus(t *testing.T, root, qid, blocks, status string) {
	t.Helper()
	questions := store.QuestionsFile{Questions: []*store.Question{{
		ID:        qid,
		Statement: "Which schema version is canonical?",
		Status:    status,
		Blocks:    []string{blocks},
	}}}
	body, err := yaml.Marshal(questions)
	require.NoError(t, err)
	dir := filepath.Join(root, ".tiller", "decisions")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "open-questions.yml"), body, 0o644))
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func resolveOne(tree *store.Tree, noun, raw string) (store.Node, error) {
	return resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: noun, Raw: raw})
}

var _ = strings.TrimSpace

// A local task submission records its own branch and observed head.
func TestTransitionSubmitRecordsReviewRef(t *testing.T) {
	t.Parallel()
	root := layCheckedTask(t)

	head, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	branch := load(t, root).Tasks["T-aaaaaa"].Branch
	ref := &store.ReviewRef{Branch: branch, Head: head}
	res := driveTo(t, root, store.NounTask, "T-aaaaaa",
		mutate.TransitionInput{Verb: "submit", ReviewRef: ref})
	require.Equal(t, "in_review", res.Status)
	require.NotNil(t, res.ReviewRef)
	require.Equal(t, head.Value, res.ReviewRef.Head.Value)
	require.Nil(t, res.PR)

	tk := load(t, root).Tasks["T-aaaaaa"]
	require.Equal(t, "in_review", tk.Status)
	require.NotNil(t, tk.ReviewRef, "the review ref is persisted on the task")
	require.Equal(t, branch, tk.ReviewRef.Branch)
	require.Equal(t, store.KindGitCommit, tk.ReviewRef.Head.Kind)
	require.Equal(t, head.Value, tk.ReviewRef.Head.Value)
	require.Nil(t, tk.PR, "the offline case never fakes a PR number")

	// The submit event carries the review ref.
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var found bool
	for _, e := range evs {
		if e.Type == "submit.task" && containsStr(e.Targets, "T-aaaaaa") {
			found = true
			rr, ok := e.After["review_ref"].(map[string]any)
			require.True(t, ok, "the submit event records the review ref")
			require.Equal(t, branch, rr["branch"])
			require.Equal(t, head.Value, rr["head"])
		}
	}
	require.True(t, found, "a submit.task event should be recorded")
}

// layCheckedTask walks the shared fixture to one task sitting at check under an
// active feature: the state submit is aimed at. It moves through the lifecycle
// verbs alone, so it needs no git repo and no configured gate commands.
func layCheckedTask(t *testing.T) string {
	t.Helper()
	root := layActiveFeature(t)
	branch := load(t, root).Tasks["T-aaaaaa"].Branch
	gitRunM(t, root, "checkout", "-b", branch)
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "check"})
	return root
}

// Task start activates its feature without assigning it a task-grain branch.
func TestTransitionStartTaskActivatesFeatureWithoutAggregateBranch(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	res := driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, "in_progress", res.Status)
	require.Empty(t, res.AggregateBranch)
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status)
	require.Empty(t, load(t, root).Features["F-aaaaaa"].Branch)
}

// TestTransitionStartKeepsAnExistingBranch is the recorded-over-recomputed rule:
// a feature that already carries a branch keeps it, and the result names the one
// on record. sync reads the same field, so naming the pattern where the two
// differ would print a branch sync will never observe.
func TestTransitionStartKeepsAnExistingBranch(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	_, err := mutate.Configure(load(t, root), testDeps(), mutate.ConfigureInput{
		Noun: store.NounFeature, Key: "form-engine/certification-form-loading", ReviewGrain: "feature"})
	require.NoError(t, err)
	setFeatureBranchOnDisk(t, root, "form-engine", "certification-form-loading", "wip/hand-cut")

	res := driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, "wip/hand-cut", res.AggregateBranch)
	require.Equal(t, "wip/hand-cut", load(t, root).Features["F-aaaaaa"].Branch,
		"start leaves a branch already on record alone")
}

// Branch creation guidance belongs to start, not to unrelated state writes.
func TestTransitionBranchOnlyOnStart(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeStage2Output(t, root)

	res := driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	require.Empty(t, res.Branch, "an epic carries no branch")
	res = driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	require.Empty(t, res.Branch, "spec records no branch")
	res = driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	require.Empty(t, res.Branch, "approve records no branch")

	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: "form-engine/certification-form-loading/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	res = driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "approve"})
	require.Empty(t, res.Branch, "approval does not start a branch")
	res = driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	require.Equal(t, "tiller/form-engine/certification-form-loading/transcribe-the-schema", res.Branch)

	// approve --refresh-basis returns without a state move, off an active feature
	// whose branch is on record: the branch belongs to the start, not to every
	// later write of the same doc. The basis is a git_blob, so this arm needs the
	// feature files under version control.
	gitInitM(t, root)
	gitCommitAllM(t, root, "lay tree")
	gitRunM(t, root, "branch", "-M", "main")
	res = driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", RefreshBasis: true})
	require.Empty(t, res.Branch, "refresh-basis records no branch")

	// Submit reports the review surface, not instructions to create a branch.
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "check"})
	pr := 41
	gitRunM(t, root, "checkout", "-b", load(t, root).Tasks["T-aaaaaa"].Branch)
	res = driveTo(t, root, store.NounTask, "T-aaaaaa",
		mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.Equal(t, "in_review", res.Status)
	require.Empty(t, res.Branch, "submit names its review surface, not the branch start recorded")
}

// TestTransitionAssumeQuestionOnActiveFeatureNamesNoBranch closes the arm the
// guard above misses. The plain re-start escapes the branch clause only because
// nothing mutated and the transition early-returns; --assume-question against an
// already-active feature records the assumption, so the write path runs and the
// result is Changed. The state still does not move and no branch is recorded, so
// the result must name none: carrying one would report "this transition began
// work on that branch" for a transition that did nothing of the kind.
func TestTransitionAssumeQuestionOnActiveFeatureNamesNoBranch(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})

	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})

	writeOpenQuestion(t, root, "Q-zzzzzz", "F-aaaaaa")
	res := driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading",
		mutate.TransitionInput{Verb: "approve", AssumeQuestion: "Q-zzzzzz", Reason: "assume the 2025 revision"})
	require.True(t, res.Changed, "recording the assumption is a real write, not the no-op early return")
	require.Equal(t, "active", res.Status, "the feature was already active; the state did not move")
	require.Empty(t, res.Branch, "this transition recorded no branch, so it names none")

	reloaded := load(t, root).Features["F-aaaaaa"]
	require.Contains(t, reloaded.AssumptionBasis, "Q-zzzzzz")
	require.Empty(t, reloaded.Branch)
}

// setFeatureBranchOnDisk inserts a branch onto a feature.yml, the cheap way to
// stage a feature that already carries a hand-set working branch before start.
func setFeatureBranchOnDisk(t *testing.T, root, epicKey, featKey, branch string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", featKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: ready\n", "status: ready\nbranch: "+branch+"\n", 1)
	require.NotEqual(t, string(data), out, "expected a status: ready line to rewrite")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// TestResumeFeatureIsMisuse pins the noun rule that falls out of a PR always
// representing a task, or, at a coarse review grain, the coarser node itself
// (artifact-schema.md, "A PR always represents a task", softened by
// docs/spec/cli-surface.md).
// resume has no coarser-grain meaning of its own at any review grain, so
// naming a feature on it is still a caller that lost track of what it is
// operating on, the checksum class the grammar reserves exit 2 for
// (cli-surface.md, "The grammar"), not the exit-1 "not legal from here" a
// merely absent table row would produce. submit's own former twin here,
// TestSubmitFeatureIsMisuse, is retired by
// TestSubmitFeatureNoLongerSpecialCasedInTheLifecycleEngine below: submit
// now carries its own feature-grain meaning, served by mutate.SubmitNode
// (submit_node.go), so this engine no longer treats it as a checksum misuse
// at all.
func TestResumeFeatureIsMisuse(t *testing.T) {
	t.Parallel()
	root := layActiveFeature(t)
	_, err := mutate.Transition(load(t, root), testDeps(),
		resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading"),
		mutate.TransitionInput{Verb: "resume", Reason: "review asked for changes"})
	requireTaskOnlyMisuse(t, err, "resume")
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status,
		"a refused resume records nothing")
}

// TestSubmitFeatureNoLongerSpecialCasedInTheLifecycleEngine is
// TestSubmitFeatureIsMisuse's replacement: submit is gone from
// taskOnlyFeatureVerbs (transition.go), so a feature-noun submit reaching
// mutate.Transition directly (something the CLI itself never does once it
// routes `submit feature` to mutate.SubmitNode instead) now falls through to
// the engine's ordinary answer for a verb the feature transitions table
// carries no row for: IllegalTransitionError, the same "not legal from here"
// exit-1 shape any other unmodeled feature verb gets. It is no longer a
// checksum misuse, because submit is no longer a caller-confused noun at
// this grain; it is a real feature-grain verb the engine itself just does
// not implement, SubmitNode does.
func TestSubmitFeatureNoLongerSpecialCasedInTheLifecycleEngine(t *testing.T) {
	t.Parallel()
	root := layActiveFeature(t)
	pr := 41
	_, err := mutate.Transition(load(t, root), testDeps(),
		resolveNode(t, load(t, root), store.NounFeature, "form-engine/certification-form-loading"),
		mutate.TransitionInput{Verb: "submit", PR: &pr})
	require.Error(t, err)
	var ill *lifecycle.IllegalTransitionError
	require.ErrorAs(t, err, &ill, "submit is now an ordinary unmodeled feature verb, not a checksum misuse")
	require.Equal(t, "active", load(t, root).Features["F-aaaaaa"].Status,
		"a refused submit records nothing")
}

// requireTaskOnlyMisuse asserts a refusal is the exit-2 misuse class and that it
// names the task noun, so the caller learns the move rather than only that this
// one was wrong.
func requireTaskOnlyMisuse(t *testing.T, err error, verb string) {
	t.Helper()
	require.Error(t, err)
	var ill *lifecycle.IllegalTransitionError
	require.False(t, errors.As(err, &ill),
		"a feature-noun %s is misuse (exit 2), not an illegal transition (exit 1)", verb)
	var coded interface{ ExitCode() int }
	require.ErrorAs(t, err, &coded)
	require.Equal(t, 2, coded.ExitCode())
	require.Contains(t, err.Error(), "task")
}

// layActiveFeature walks the shared one-epic-one-feature fixture to an active
// feature with its branch recorded, the state a submit or resume would be aimed
// at if either still took the feature noun.
func layActiveFeature(t *testing.T) string {
	t.Helper()
	root := layEpicFeature(t)
	writeStage2Output(t, root)
	driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	driveTo(t, root, store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	return root
}

// writeFeatureProseAt is writeFeatureProse generalized over epic and feature
// key: the round-trip sweep below spans three features across two epics, so
// the shared fixture's hardcoded pair does not reach all of them.
func writeFeatureProseAt(t *testing.T, root, epicKey, featureKey string) {
	t.Helper()
	dir := filepath.Join(root, ".tiller", "epics", epicKey, "features", featureKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# "+featureKey+"\n\nProse for "+featureKey+".\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: It works\n    polarity: introduce\n    binding: { type: test, ref: spec/form_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
}

// writeTaskSpecAt is writeTaskSpec generalized over epic and feature key, the
// same reason writeFeatureProseAt exists beside it.
func writeTaskSpecAt(t testing.TB, root, epicKey, featureKey, taskKey, body string) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir(epicKey, featureKey, taskKey), store.TaskProseFile)
	require.NoError(t, os.WriteFile(path, []byte("# "+taskKey+"\n\n"+body+"\n"), 0o644))
}

// TestEveryTaskInATreeLoadsRoundTripsAndResolvesBothWays is the property this
// refactor turns on, checked across the input space rather than on one node: for
// every task in a multi-feature tree, the loader places it, its folder holds
// both files, both addresses resolve to it, and a transition rewrites its own
// task.yml without disturbing a sibling or a cousin. A writer that computed the
// wrong path would land a sibling's state on this task's file, which only a
// sweep like this catches.
func TestEveryTaskInATreeLoadsRoundTripsAndResolvesBothWays(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	// Every node needs a distinct id here, since the sweep addresses each task
	// by id as well as by key; the shared fixed minter would collide them.
	seq := 0
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		seq++
		return fmt.Sprintf("%s-%06d", prefix, seq)
	}
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "form-engine"})
	require.NoError(t, err)
	_, err = mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "reporting"})
	require.NoError(t, err)

	want := map[string]string{} // canonical task key -> title
	for _, featureKey := range []string{"form-engine/certification-form-loading", "form-engine/audit-export", "reporting/rollups"} {
		_, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: featureKey, ChangeType: "create"})
		require.NoError(t, err)
		var firstLocal string
		for i, local := range []string{"zulu", "alfa", "mike"} {
			key := featureKey + "/" + local
			title := "Do " + key
			_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{Key: key, Title: title})
			require.NoError(t, err)
			_, err = mutate.ClaimTask(load(t, root), deps, key, "")
			require.NoError(t, err)
			want[key] = title
			if i == 0 {
				firstLocal = local
			}
		}

		// The parent-plan gate now stands ahead of every doing task verb
		// (guardParentState), so this sweep has to carry each feature across
		// it before the per-task loop below can drive a task: write real prose
		// and one real task spec (the bar validateSpec asks for), then spec and
		// approve the feature, which pins the whole initial explosion, every
		// task under it, proposed -> ready, in the same act (approveFeaturePlan).
		seg := strings.Split(featureKey, "/")
		writeFeatureProseAt(t, root, seg[0], seg[1])
		writeTaskSpecAt(t, root, seg[0], seg[1], firstLocal, "## Approach\n\nThe implementation spec for "+firstLocal+".")
		driveWith(t, root, deps, store.NounFeature, featureKey, mutate.TransitionInput{Verb: "spec"})
		driveWith(t, root, deps, store.NounFeature, featureKey, mutate.TransitionInput{Verb: "approve"})
	}

	tree := load(t, root)
	require.Len(t, tree.Tasks, len(want), "every task laid down is a task loaded")

	for key, title := range want {
		tree := load(t, root)
		seg := strings.Split(key, "/")
		dir := filepath.Join(root, store.TaskRelDir(seg[0], seg[1], seg[2]))
		require.FileExists(t, filepath.Join(dir, store.TaskProseFile), "prose for %s", key)
		require.FileExists(t, filepath.Join(dir, store.TaskStateFile), "state for %s", key)

		byKey := resolveNode(t, tree, store.NounTask, key)
		byID := resolveNode(t, tree, store.NounTask, byKey.NodeID())
		require.Equal(t, byKey.NodeID(), byID.NodeID(), "both addresses answer the same node for %s", key)
		require.Equal(t, title, tree.Tasks[byKey.NodeID()].Title)

		// A write lands on this task's own file: after it, this task moved and
		// every other task is exactly where it was. start, not approve, is the
		// verb this sweep drives here: the plan gate above already carried every
		// task from proposed to ready as part of approving its own feature's
		// plan, so start (ready -> in_progress) is the individual-task write
		// left for this loop to pin.
		before := statusByKey(t, load(t, root))
		_, err := mutate.Transition(tree, deps, byKey, mutate.TransitionInput{Verb: "start"})
		require.NoError(t, err)
		after := statusByKey(t, load(t, root))

		require.Equal(t, "in_progress", after[key], "the started task moved: %s", key)
		for other := range want {
			if other == key {
				continue
			}
			require.Equal(t, before[other], after[other], "writing %s must not touch %s", key, other)
		}
	}
}

// statusByKey maps every task's canonical key to its stored status, the snapshot
// the round-trip sweep compares across a write.
func statusByKey(t *testing.T, tree *store.Tree) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tk := range tree.Tasks {
		require.NotNil(t, tk.Key, "task %s carries no key", tk.ID)
		out[*tk.Key] = tk.Status
	}
	return out
}

// TestATaskWriteCannotLandOnAnotherTask is the write half of the folder/key
// guard, pinned on its own so neither half rests on the other. A write derives
// its file from the addressed task's key; if that derivation lands anywhere
// else, the write must refuse rather than rewrite whatever it found. Cancelling
// the wrong node is bad; the same path on the sync arm crosses the wrong task to
// done, which does not reverse.
//
// The disagreement is staged in memory because store.Load now refuses it on disk
// (checkKeyNamesItsFolder), which is the point: this pins that the write site
// refuses on its own, without the loader having caught it first.
func TestATaskWriteCannotLandOnAnotherTask(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	seq := 0
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		seq++
		return fmt.Sprintf("%s-%06d", prefix, seq)
	}
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "e1"})
	require.NoError(t, err)
	for _, fk := range []string{"e1/f1", "e1/f2"} {
		_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: fk, ChangeType: "create"})
		require.NoError(t, err)
		_, err = mutate.AddTask(load(t, root), deps, mutate.TaskInput{Key: fk + "/alpha", Title: "under " + fk})
		require.NoError(t, err)
	}

	tree := load(t, root)
	addressed := taskAtKey(t, tree, "e1/f1/alpha")
	victim := taskAtKey(t, tree, "e1/f2/alpha")
	require.NotEqual(t, addressed.ID, victim.ID)

	// The shape a hand-moved folder leaves: the loader placed this task under f2
	// (the folder it now sits in) while its task.yml still records the f1 key, so
	// rebuilding the path from the key reaches f2's OWN alpha, the victim.
	addressed.FeatureID = victim.FeatureID

	_, err = mutate.Transition(tree, deps, addressed, mutate.TransitionInput{Verb: "cancel", Reason: "x"})
	require.Error(t, err, "a write that reached another task's file must refuse")
	require.Contains(t, err.Error(), addressed.ID, "the refusal names the task that was addressed")
	require.Contains(t, err.Error(), victim.ID, "and the task whose file it reached")

	after := load(t, root)
	require.Equal(t, "proposed", after.Tasks[victim.ID].Status, "the innocent sibling is untouched")
	require.Equal(t, "proposed", after.Tasks[addressed.ID].Status, "and so is the addressed task")
}

// taskAtKey is the tree's task at one canonical key, the addressing a test means
// when it names a node the fixture laid down.
func taskAtKey(t *testing.T, tree *store.Tree, key string) *store.Task {
	t.Helper()
	for _, tk := range tree.Tasks {
		if tk.Key != nil && *tk.Key == key {
			return tk
		}
	}
	t.Fatalf("no task at key %s", key)
	return nil
}

// TestTransitionRefusesBlankReason pins the widening this task makes: return
// and resume already demanded a reason, and cancel is the tool's only removal
// verb, so a whitespace-only --reason must satisfy none of the three
// (cli-surface.md, the verb table).
func TestTransitionRefusesBlankReason(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"cancel", "return", "resume"} {
		t.Run(verb+"/blank", func(t *testing.T) {
			root := layEpicFeature(t)
			tree := load(t, root)
			node := tree.Features["F-aaaaaa"]

			_, err := mutate.Transition(tree, testDeps(), node,
				mutate.TransitionInput{Verb: verb, Reason: "   "})

			require.Error(t, err, "a whitespace-only reason is no reason at all")
			var ue interface {
				Reason() string
				ExitCode() int
			}
			require.ErrorAs(t, err, &ue)
			require.Equal(t, "missing_reason", ue.Reason())
			require.Equal(t, 2, ue.ExitCode(), "a missing required input is misuse, not a blocked business answer")
		})
	}
}

// TestTransitionAcceptsNonBlankReason confirms a real reason clears the new
// check and cancel proceeds to its ordinary legality path.
func TestTransitionAcceptsNonBlankReason(t *testing.T) {
	t.Parallel()
	root := layEpicFeature(t)
	tree := load(t, root)
	node := tree.Features["F-aaaaaa"]

	_, err := mutate.Transition(tree, testDeps(), node,
		mutate.TransitionInput{Verb: "cancel", Reason: "subsumed by the table model"})

	require.NoError(t, err)
}

// layClaimGateTask claims one task to jc and advances it to the requested state.
func layClaimGateTask(t *testing.T, state string) (root string, deps mutate.Deps) {
	t.Helper()
	deps = claimDeps()
	root = writeTwoOwnerRepo(t)
	_, err := mutate.AddEpic(load(t, root), depsAs(deps, "jc"), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), depsAs(deps, "jc"), mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	writeFeatureProse(t, root)
	_, err = mutate.AddTask(load(t, root), depsAs(deps, "jc"),
		mutate.TaskInput{Key: "form-engine/certification-form-loading/alfa", Title: "Alfa"})
	require.NoError(t, err)
	writeTaskSpec(t, root, "alfa", "## Approach\n\nDo the work.")
	_, err = mutate.ClaimTask(load(t, root), depsAs(deps, "jc"), "T-000001", "")
	require.NoError(t, err)

	// The plan gate (feature spec, then approve) flips the initial "alfa"
	// task proposed -> ready in the same act as the feature's own
	// specced -> ready move (approveFeaturePlan), so there is no separate
	// task-grain approve to drive, and none would be legal here: "ready" is
	// already the task's state once the feature crosses the gate.
	driveWith(t, root, depsAs(deps, "jc"), store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, depsAs(deps, "jc"), store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	if state == "ready" {
		return root, deps
	}
	started := driveWith(t, root, depsAs(deps, "jc"), store.NounTask, "T-000001", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "checkout", "-b", started.Branch)
	if state == "in_progress" {
		return root, deps
	}
	driveWith(t, root, depsAs(deps, "jc"), store.NounTask, "T-000001", mutate.TransitionInput{Verb: "check"})
	if state == "check" {
		return root, deps
	}
	pr := 1
	driveWith(t, root, depsAs(deps, "jc"), store.NounTask, "T-000001", mutate.TransitionInput{Verb: "submit", PR: &pr})
	return root, deps // "in_review"
}

// workVerbInput builds the TransitionInput a work-gate matrix cell drives:
// submit needs a PR, resume needs a non-blank --reason (reasonRequired's own
// rule, transition.go), start needs neither.
func workVerbInput(verb string) mutate.TransitionInput {
	switch verb {
	case "submit":
		pr := 7
		return mutate.TransitionInput{Verb: verb, PR: &pr}
	case "resume":
		return mutate.TransitionInput{Verb: verb, Reason: "review asked for changes"}
	default:
		return mutate.TransitionInput{Verb: verb}
	}
}

// Execution verbs use the task's claim, while started work cannot be reassigned.
func TestWorkGateRefuseMatrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ verb, state string }{
		{"start", "ready"}, {"submit", "check"}, {"resume", "in_review"},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			root, deps := layClaimGateTask(t, tc.state)
			before := snapshotDir(t, root)
			err := tryTransition(t, root, depsAs(deps, "pat"), store.NounTask, "T-000001", workVerbInput(tc.verb))
			var held *mutate.WorkClaimHeldError
			require.ErrorAs(t, err, &held)
			require.Equal(t, "jc", held.Holder)
			require.Equal(t, "T-000001", held.NodeID)
			requireNoWrites(t, before, snapshotDir(t, root))
			require.NoError(t, tryTransition(t, root, deps, store.NounTask, "T-000001", workVerbInput(tc.verb)))
			require.NotEqual(t, tc.state, load(t, root).Tasks["T-000001"].Status)
		})
	}
	t.Run("start requires a claim", func(t *testing.T) {
		root, deps := layClaimGateTask(t, "ready")
		_, err := mutate.ReleaseTask(load(t, root), deps, "T-000001")
		require.NoError(t, err)
		before := snapshotDir(t, root)
		err = tryTransition(t, root, deps, store.NounTask, "T-000001", mutate.TransitionInput{Verb: "start"})
		var unclaimed *mutate.WorkUnclaimedError
		require.ErrorAs(t, err, &unclaimed)
		require.Equal(t, "T-000001", unclaimed.NodeID)
		requireNoWrites(t, before, snapshotDir(t, root))
	})
}

// Returning an unstarted task for specification rework does not require a claim.
func TestTaskReturnStaysUngatedOnAnUnclaimedTask(t *testing.T) {
	t.Parallel()
	root, deps := layClaimGateTask(t, "ready")
	_, err := mutate.ReleaseTask(load(t, root), depsAs(deps, "jc"), "T-000001")
	require.NoError(t, err)
	require.Empty(t, load(t, root).Tasks["T-000001"].Owner)

	res, err := mutate.Transition(load(t, root), depsAs(deps, "jc"),
		resolveNode(t, load(t, root), store.NounTask, "T-000001"),
		mutate.TransitionInput{Verb: "return", Reason: "rework needed"})
	require.NoError(t, err, "return must not require a claim the work-gate never binds it to")
	require.Equal(t, "proposed", res.Status)
}

// layPatClaimedActiveFeature activates a feature by starting Pat's task.
func layPatClaimedActiveFeature(t *testing.T) (root string, deps mutate.Deps) {
	t.Helper()
	deps = claimDeps()
	root = writeTwoOwnerRepo(t)
	_, err := mutate.AddEpic(load(t, root), depsAs(deps, "jc"), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), depsAs(deps, "jc"), mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	writeStage2Output(t, root)
	_, err = mutate.ReassignTask(load(t, root), depsAs(deps, "jc"), "T-aaaaaa", "pat", "Pat implements this task")
	require.NoError(t, err)
	driveWith(t, root, depsAs(deps, "jc"), store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	driveWith(t, root, depsAs(deps, "jc"), store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	driveWith(t, root, depsAs(deps, "pat"), store.NounTask, "T-aaaaaa", mutate.TransitionInput{Verb: "start"})
	return root, deps
}

// A registered teammate can approve a task reserved by another developer.
func TestApproveTaskImmuneToAnotherOwnersClaim(t *testing.T) {
	t.Parallel()
	root, deps := layPatClaimedActiveFeature(t)
	added, err := mutate.AddTask(load(t, root), depsAs(deps, "jc"),
		mutate.TaskInput{Key: "form-engine/certification-form-loading/mid-flight", Title: "Mid flight"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), added.ID, "")
	require.NoError(t, err)
	writeTaskSpec(t, root, "mid-flight", "## Approach\n\nAdd the missing validator.")

	res, err := mutate.Transition(load(t, root), depsAs(deps, "jc"),
		resolveNode(t, load(t, root), store.NounTask, added.ID), mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err, "approve must succeed for jc despite pat holding the claim")
	require.Equal(t, "ready", res.Status)
}

// TestCheckTaskImmuneToAnotherOwnersClaim pins check's immunity. pat, the
// actual claimant, does the legitimate work of starting the task (a
// work-verb call the gate would refuse from jc); jc then runs the judgment
// verb over pat's work, exactly the design's own example ("a teammate
// answers a question against work they do not hold").
func TestCheckTaskImmuneToAnotherOwnersClaim(t *testing.T) {
	t.Parallel()
	root, deps := layPatClaimedActiveFeature(t)

	tree := load(t, root)
	res, err := mutate.Check(tree, depsAs(deps, "jc"), stubRev(root),
		resolveNode(t, tree, store.NounTask, "T-aaaaaa"), mutate.CheckInput{})
	require.NoError(t, err, "check must succeed for jc despite pat holding the claim")
	require.Equal(t, "check", res.Status)
}

// A teammate can attest aggregate acceptance independently of implementation.
func TestAttestImmuneToAnotherOwnersClaim(t *testing.T) {
	t.Parallel()
	root, deps := layPatClaimedActiveFeature(t)
	tree := load(t, root)
	res, err := mutate.Attest(tree, depsAs(deps, "jc"),
		resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading"),
		mutate.AttestInput{Gate: "code_review", Status: store.GateStatusPass})
	require.NoError(t, err, "attest must succeed for jc despite pat holding the claim")
	require.True(t, res.Satisfied)
}

// A teammate can evaluate the feature contract independently of implementation.
func TestEvalImmuneToAnotherOwnersClaim(t *testing.T) {
	t.Parallel()
	root, deps := layPatClaimedActiveFeature(t)
	tree := load(t, root)
	_, err := mutate.Eval(tree, depsAs(deps, "jc"), stubRev(root),
		resolveNode(t, tree, store.NounFeature, "form-engine/certification-form-loading"), mutate.EvalInput{})
	require.NoError(t, err, "eval must succeed for jc despite pat holding the claim")
}

// A teammate can answer a question on another developer's implementation.
func TestAnswerImmuneToAnotherOwnersClaim(t *testing.T) {
	t.Parallel()
	root, deps := layPatClaimedActiveFeature(t)
	_, err := mutate.AddQuestion(load(t, root), depsAs(deps, "jc"), mutate.QuestionInput{
		Title:     "Canonical schema version",
		Statement: "Which schema version is canonical?", Blocks: []string{"F-000001"},
	})
	require.NoError(t, err)

	_, err = mutate.AnswerQuestion(load(t, root), depsAs(deps, "jc"), stubRev(root),
		mutate.AnswerInput{Node: "Q-000001", Answer: "The 2025 revision; 2022 is read-only reference."})
	require.NoError(t, err, "answer must succeed for jc despite pat holding the claim")
}
