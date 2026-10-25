package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Approving a correct-eval: the gate proves a done feature's contract changed
// only in its bindings, then re-pins verified_against.eval so the stale_spec
// sweep stops reporting the reviewed edit forever (issue #141).

// verifiedEvalMD is the two-criterion contract the fixture feature crosses to
// done with, so a correction has a criterion it can retire and one it keeps.
func verifiedEvalMD(featureID, c1Run string) string {
	return "---\nnode: " + featureID + "\ncriteria:\n" +
		"  - id: C1\n    statement: A logged session completes in under three taps\n" +
		"    polarity: introduce\n    binding: { type: command, run: \"" + c1Run + "\" }\n    required: true\n" +
		"  - id: C2\n    statement: The log screen opens offline\n" +
		"    polarity: introduce\n    binding: { type: test, ref: app/log_test.go }\n---\n\n# Acceptance\n"
}

// writeEval replaces the fixture feature's working-tree feature.eval.md.
func writeEval(t *testing.T, root, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, lanternEvalRel), []byte(content), 0o644))
}

// correctableFixture is the lantern fixture's feature crossed to done: its
// contract committed, and a completion record whose spec and eval legs pin the
// committed blobs, the record sync writes at a real crossing.
func correctableFixture(t *testing.T) (root string, deps mutate.Deps, featureID string) {
	t.Helper()
	root, deps, featureID, _ = lanternFixture(t)
	writeEval(t, root, verifiedEvalMD(featureID, "true"))
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	layCompletionRecord(t, root, featureID, store.VerifiedAgainst{
		Spec:     store.RevisionRef{Kind: store.KindGitBlob, Value: blobOf(t, root, lanternSpecRel), Path: lanternSpecRel, ObservedAt: at},
		Eval:     store.RevisionRef{Kind: store.KindGitBlob, Value: blobOf(t, root, lanternEvalRel), Path: lanternEvalRel, ObservedAt: at},
		Surfaces: []store.SurfaceFingerprint{},
	})
	gitCommitAllM(t, root, "cross fast-logging to done")
	return root, deps, featureID
}

// layCompletionRecord hand-lays done and a verified_against onto the fixture
// feature's yml, the way setFeatureVerifiedCommit lays a done target.
func layCompletionRecord(t *testing.T, root, featureID string, record store.VerifiedAgainst) {
	t.Helper()
	path := filepath.Join(root, lanternFeatureDir, "feature.yml")
	var doc store.Feature
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Equal(t, featureID, doc.ID)
	doc.Status = "done"
	doc.VerifiedAgainst = record
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// staleSpecFor is the stale_spec candidate the real drift sweep reports for
// the feature, or nil when it reports none. The fixture's manifest enables no
// drift checks, so the sweep is switched on here, in memory.
func staleSpecFor(t *testing.T, root, featureID string) *drift.Candidate {
	t.Helper()
	tree := load(t, root)
	tree.Manifest.Drift.Checks = []string{drift.MarkerStaleSpec}
	rep, err := drift.Run(tree, amendDeps(root).Rev)
	require.NoError(t, err)
	for i, c := range rep.Candidates {
		if c.Marker == drift.MarkerStaleSpec && c.ID == featureID {
			return &rep.Candidates[i]
		}
	}
	for _, u := range rep.Uncovered {
		require.NotEqual(t, featureID, u.ID, "the feature's stale_spec comparison must run, not land Uncovered")
	}
	return nil
}

// stageCorrection opens an amendment and stages one correct-eval on the feature.
func stageCorrection(t *testing.T, root string, deps mutate.Deps, key string, retire ...string) *store.Amendment {
	t.Helper()
	a := openAmendment(t, root, deps, key)
	return stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCorrectEval, Target: "app-experience/fast-logging",
		Reason: "the binding moved", Retire: retire,
	})
}

func correctEvalEvents(t *testing.T, root string) []events.Event {
	t.Helper()
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	return eventsOfType(evs, "correct-eval.feature")
}

func TestApproveAmendmentCorrectEvalRepinsTheCompletionRecord(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	require.Nil(t, staleSpecFor(t, root, featureID), "the crossing's record matches the committed contract")
	before := load(t, root).Features[featureID]

	writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
	require.NotNil(t, staleSpecFor(t, root, featureID), "a binding edit alone stales the completion record")

	a := stageCorrection(t, root, deps, "eval-bindings")
	res := approve(t, root, deps, a)

	require.Equal(t, store.AmendmentStatusApproved, res.Status)
	require.Len(t, res.Applied, 1)
	require.Equal(t, mutate.AppliedOperation{
		Index: 0, Op: store.AmendmentOpCorrectEval, Noun: store.NounFeature,
		NodeID: featureID, Key: "app-experience/fast-logging", Status: "done",
		Note: "re-pinned verified_against.eval to " + blobOf(t, root, lanternEvalRel)[:len("git:")+7] +
			"; commit " + filepath.ToSlash(lanternEvalRel) + " with this change, since a later correction reads it back from git",
	}, res.Applied[0])

	after := load(t, root).Features[featureID]
	require.Equal(t, store.RevisionRef{
		Kind: store.KindGitBlob, Value: blobOf(t, root, lanternEvalRel), Path: lanternEvalRel,
		ObservedAt: deps.Clock.Now().UTC(),
	}, after.VerifiedAgainst.Eval)
	require.Equal(t, before.VerifiedAgainst.Spec, after.VerifiedAgainst.Spec, "only the eval leg moves")
	require.Equal(t, before.VerifiedAgainst.Surfaces, after.VerifiedAgainst.Surfaces)
	require.Equal(t, "done", after.Status)
	require.Equal(t, before.AcceptedAgainst, after.AcceptedAgainst, "the accepted basis is not the completion record")
	require.Equal(t, before.Gates, after.Gates)

	evs := correctEvalEvents(t, root)
	require.Len(t, evs, 1)
	ev := evs[0]
	require.Equal(t, []string{featureID, a.ID}, ev.Targets)
	require.Equal(t, "approve amendment "+a.ID, ev.Command)
	require.Equal(t, before.VerifiedAgainst.Eval.Value, ev.Before["eval"])
	require.Equal(t, after.VerifiedAgainst.Eval.Value, ev.After["eval"])
	require.Equal(t, "the binding moved", ev.After["reason"])
	require.EqualValues(t, 0, ev.After["operation_index"])
	require.Equal(t, "done", ev.After["status"])
	require.Equal(t, a.ID, ev.After["amendment"])
	require.NotContains(t, ev.After, "retired", "a correction that retires nothing records no retired list")

	require.Nil(t, staleSpecFor(t, root, featureID), "the reviewed correction clears stale_spec")
}

func TestApproveAmendmentCorrectEvalRetiresANamedCriterion(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	contract := verifiedEvalMD(featureID, "true")
	c2 := contract[strings.Index(contract, "  - id: C2"):strings.Index(contract, "---\n\n#")]
	writeEval(t, root, strings.Replace(contract, c2, "", 1))

	a := stageCorrection(t, root, deps, "retire-offline", " C2 ")
	require.Equal(t, []string{"C2"}, a.Operations[0].Retire, "staging stores the trimmed id")
	approve(t, root, deps, a)

	evs := correctEvalEvents(t, root)
	require.Len(t, evs, 1)
	require.Equal(t, []any{"C2"}, evs[0].After["retired"])
	require.Nil(t, staleSpecFor(t, root, featureID))
}

// A second correction reads the first one's committed blob as its base: the
// completion record's eval leg is what the gate compares against, whatever
// produced it (issue #141 Review Focus 1).
func TestApproveAmendmentCorrectEvalChainsOffACommittedCorrection(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
	approve(t, root, deps, stageCorrection(t, root, deps, "first-correction"))
	firstPin := load(t, root).Features[featureID].VerifiedAgainst.Eval.Value
	gitCommitAllM(t, root, "commit the first correction")

	writeEval(t, root, verifiedEvalMD(featureID, "test -n ok"))
	a := stageCorrection(t, root, deps, "second-correction")
	approve(t, root, deps, a)

	evs := correctEvalEvents(t, root)
	require.Len(t, evs, 2)
	var second events.Event
	for _, ev := range evs {
		if ev.After["amendment"] == a.ID {
			second = ev
		}
	}
	require.Equal(t, firstPin, second.Before["eval"], "the second correction's base is the first's pin")
	require.Equal(t, blobOf(t, root, lanternEvalRel), second.After["eval"])
	require.Nil(t, staleSpecFor(t, root, featureID))
}

// requireRefusedWithNoWrites approves a, expecting an exit-2 refusal carrying
// reason, and that nothing on disk moved.
func requireRefusedWithNoWrites(t *testing.T, root string, deps mutate.Deps, a *store.Amendment, reason string) error {
	t.Helper()
	before := snapshotDir(t, root)
	err := tryApprove(t, root, deps, a)
	require.Equal(t, reason, reasonOf(t, err), err.Error())
	var coder interface{ ExitCode() int }
	require.ErrorAs(t, err, &coder)
	require.Equal(t, 2, coder.ExitCode())
	requireNoWrites(t, before, snapshotDir(t, root))
	require.Equal(t, store.AmendmentStatusProposed, load(t, root).Amendments[a.ID].Status)
	return err
}

// The first correction was approved but never committed, so its pin names a
// blob git never stored. The second correction has no readable base, and the
// refusal says to commit the first one (issue #141 Review Focus 5).
func TestApproveAmendmentCorrectEvalRefusesAnUncommittedBase(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
	approve(t, root, deps, stageCorrection(t, root, deps, "first-correction"))
	firstPin := load(t, root).Features[featureID].VerifiedAgainst.Eval.Value

	writeEval(t, root, verifiedEvalMD(featureID, "test -n ok"))
	a := stageCorrection(t, root, deps, "second-correction")
	err := requireRefusedWithNoWrites(t, root, deps, a, "correct_eval_base_unreadable")
	require.Contains(t, err.Error(), firstPin)
	require.Contains(t, err.Error(), "git cannot read blob "+firstPin+" as the verified contract")
	require.Contains(t, err.Error(), "if an earlier correction or done crossing is uncommitted, commit it first")
}

// Approval's precondition check still answers first: an eval edited after
// staging is the exit-1 stale answer naming eval_revision, not a correction
// judged over bytes nobody reviewed.
func TestApproveAmendmentCorrectEvalRefusesAContractEditedAfterStaging(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
	a := stageCorrection(t, root, deps, "eval-bindings")
	writeEval(t, root, verifiedEvalMD(featureID, "test -n unreviewed"))
	before := snapshotDir(t, root)

	err := tryApprove(t, root, deps, a)
	var stale *mutate.AmendmentPreconditionsStaleError
	require.ErrorAs(t, err, &stale)
	require.Len(t, stale.Stale, 1)
	require.Equal(t, "eval_revision", stale.Stale[0].Precondition)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestApproveAmendmentCorrectEvalRefusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		prepare    func(t *testing.T, root, featureID string)
		retire     []string
		wantReason string
		wantMsg    string
	}{
		{
			name:       "an unchanged contract",
			wantReason: "correct_eval_no_change",
		},
		{
			name: "a statement change",
			prepare: func(t *testing.T, root, featureID string) {
				writeEval(t, root, strings.Replace(verifiedEvalMD(featureID, "true"), "opens offline", "opens offline fast", 1))
			},
			wantReason: "correct_eval_changes_contract",
			wantMsg:    "criterion C2 changes its statement",
		},
		{
			name: "a removal not named in retire",
			prepare: func(t *testing.T, root, featureID string) {
				contract := verifiedEvalMD(featureID, "true")
				c2 := contract[strings.Index(contract, "  - id: C2"):strings.Index(contract, "---\n\n#")]
				writeEval(t, root, strings.Replace(contract, c2, "", 1))
			},
			wantReason: "correct_eval_changes_contract",
			wantMsg:    "removes criterion C2 without --retire",
		},
		{
			name: "a corrected contract that is not a valid contract",
			prepare: func(t *testing.T, root, featureID string) {
				writeEval(t, root, strings.Replace(verifiedEvalMD(featureID, "true"), "type: test", "type: vibes", 1))
			},
			wantReason: "eval_contract_invalid",
			wantMsg:    "unrecognized binding type",
		},
		{
			name: "a completion record with no eval leg",
			prepare: func(t *testing.T, root, featureID string) {
				f := load(t, root).Features[featureID]
				record := f.VerifiedAgainst
				record.Eval = store.RevisionRef{}
				layCompletionRecord(t, root, featureID, record)
				writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
			},
			wantReason: "correct_eval_unverified",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, deps, featureID := correctableFixture(t)
			if tc.prepare != nil {
				tc.prepare(t, root, featureID)
			}
			a := stageCorrection(t, root, deps, "eval-bindings", tc.retire...)
			err := requireRefusedWithNoWrites(t, root, deps, a, tc.wantReason)
			require.Contains(t, err.Error(), "operation 0")
			if tc.wantMsg != "" {
				require.Contains(t, err.Error(), tc.wantMsg)
			}
		})
	}
}

// A hand edit that drops the staged eval_revision pin leaves approval nothing
// proving the bytes it would pin are the bytes the human reviewed, so it
// refuses and sends the human back through staging, which pins them.
func TestApproveAmendmentCorrectEvalRefusesAnUnpinnedOperation(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
	a := stageCorrection(t, root, deps, "eval-bindings")
	unpinned := a.Operations[0]
	unpinned.Preconditions = nil
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{unpinned})

	err := requireRefusedWithNoWrites(t, root, deps, a, "correct_eval_unpinned")
	require.Contains(t, err.Error(), "operation 0")
	require.Contains(t, err.Error(), "withdraw")
}

func TestApproveAmendmentCorrectEvalRefusesAFeatureThatIsNotDone(t *testing.T) {
	t.Parallel()
	root, deps, featureID, _ := lanternFixture(t)
	a := stageCorrection(t, root, deps, "eval-bindings")
	err := requireRefusedWithNoWrites(t, root, deps, a, "correct_eval_target_not_done")
	require.Contains(t, err.Error(), "operation 0 cannot correct "+featureID+": it is active")
	require.Contains(t, err.Error(), "through revise")
}

// A hand-edited batch can name a task, which staging would have refused.
func TestApproveAmendmentCorrectEvalRefusesATaskTarget(t *testing.T) {
	t.Parallel()
	root, deps, _, taskID := lanternFixture(t)
	a := openAmendment(t, root, deps, "eval-bindings")
	a = writeAmendmentOps(t, root, a, []store.AmendmentOp{
		{Op: store.AmendmentOpCorrectEval, Target: taskID, Reason: "r"},
	})
	err := requireRefusedWithNoWrites(t, root, deps, a, "correct_eval_not_feature")
	require.Contains(t, err.Error(), taskID+" is a task")
}

func TestApproveAmendmentCorrectEvalRefusesTwoCorrectionsOfOneFeature(t *testing.T) {
	t.Parallel()
	root, deps, featureID := correctableFixture(t)
	writeEval(t, root, verifiedEvalMD(featureID, "exit 0"))
	a := stageCorrection(t, root, deps, "eval-bindings")
	a = stage(t, root, deps, a, mutate.StageInput{
		Op: store.AmendmentOpCorrectEval, Target: featureID, Reason: "again"})
	err := requireRefusedWithNoWrites(t, root, deps, a, "correct_eval_duplicate_target")
	require.Contains(t, err.Error(), "operation 1")
}
