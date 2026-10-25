package mutate

import (
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// verifiedContract is the contract a done feature was verified against: a
// command criterion, a test criterion, and a required preserve criterion with
// its baseline, so every field checkEvalCorrection judges has a value to move.
func verifiedContract() *store.EvalFile {
	return &store.EvalFile{
		Node: "F-000002",
		Criteria: []store.EvalCriterion{
			{ID: "C1", Statement: "A session logs in three taps", Polarity: store.PolarityIntroduce,
				Binding: store.EvalBinding{Type: store.BindingCommand, Run: "true"}, Required: true},
			{ID: "C2", Statement: "The log screen opens offline", Polarity: store.PolarityIntroduce,
				Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"app/log_test.go"}}},
			{ID: "C3", Statement: "Existing exports are unchanged", Polarity: store.PolarityPreserve, Required: true,
				Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"app/export_test.go"}},
				Baseline: &store.EvalBaseline{
					Surface:    "app/export",
					CapturedAt: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abc1234", ObservedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
					Artifact:   "testdata/export.golden",
				}},
		},
	}
}

// evalPinOf and checkEvalPin are the gate's proof that the bytes it pins are the bytes the
// staged eval_revision named. checkPreconditions compares the same value
// earlier in phase 1, so the mismatch arm is only reachable when the file
// moves between that read and the judge's; it is unit-tested here because no
// seam reaches that window from outside.
func TestCheckEvalPin(t *testing.T) {
	t.Parallel()
	key := "app/fast-logging"
	r := func(pre map[string]string) resolvedOp {
		return resolvedOp{index: 2, op: store.AmendmentOp{Op: store.AmendmentOpCorrectEval, Target: "F-000002", Preconditions: pre},
			target: &store.Feature{ID: "F-000002", Key: &key}}
	}

	t.Run("a matching pin passes", func(t *testing.T) {
		op := r(map[string]string{"eval_revision": "git:aaa"})
		pinned, err := evalPinOf(op)
		require.NoError(t, err)
		require.Equal(t, "git:aaa", pinned)
		require.NoError(t, checkEvalPin("A-000001", op, pinned, "git:aaa"))
	})

	t.Run("no pin refuses unpinned", func(t *testing.T) {
		_, err := evalPinOf(r(map[string]string{"target_status": "done"}))
		var ue *usageError
		require.ErrorAs(t, err, &ue)
		require.Equal(t, "correct_eval_unpinned", ue.Reason())
		require.Contains(t, err.Error(), "operation 2 cannot correct F-000002")
		require.Contains(t, err.Error(), "withdraw")
	})

	t.Run("a pin the judged bytes no longer match is stale", func(t *testing.T) {
		err := checkEvalPin("A-000001", r(map[string]string{"eval_revision": "git:aaa"}), "git:aaa", "git:bbb")
		var stale *AmendmentPreconditionsStaleError
		require.ErrorAs(t, err, &stale)
		require.Equal(t, "A-000001", stale.AmendmentID)
		require.Equal(t, []StalePrecondition{{
			Index: 2, Op: store.AmendmentOpCorrectEval, Target: "F-000002", Precondition: "eval_revision",
			Expected: "git:aaa", Actual: "git:bbb",
			Note: "feature.eval.md changed while approval was judging it",
		}}, stale.Stale)
	})
}

func TestCheckEvalCorrection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    func(ef *store.EvalFile)
		retire  []string
		wantMsg string // empty means the correction passes
	}{
		{name: "a binding-only change passes", edit: func(ef *store.EvalFile) {
			ef.Criteria[0].Binding = store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"app/taps_test.go"}, Run: "go test ./app"}
		}},
		{name: "a ref scalar becoming a list passes", edit: func(ef *store.EvalFile) {
			ef.Criteria[1].Binding.Ref = store.BindingRefs{"app/log_test.go", "app/offline_test.go"}
		}},
		{name: "a baseline artifact change passes", edit: func(ef *store.EvalFile) {
			ef.Criteria[2].Baseline.Artifact = "testdata/exports/v1.golden"
		}},
		{name: "a statement change refuses", edit: func(ef *store.EvalFile) {
			ef.Criteria[0].Statement = "A session logs in two taps"
		}, wantMsg: "criterion C1 changes its statement"},
		{name: "a polarity change refuses", edit: func(ef *store.EvalFile) {
			ef.Criteria[1].Polarity = store.PolarityPreserve
		}, wantMsg: "criterion C2 changes its polarity"},
		{name: "a required flip refuses", edit: func(ef *store.EvalFile) {
			ef.Criteria[1].Required = true
		}, wantMsg: "criterion C2 changes its required flag"},
		{name: "a baseline surface change refuses", edit: func(ef *store.EvalFile) {
			ef.Criteria[2].Baseline.Surface = "app/export/v2"
		}, wantMsg: "criterion C3 changes its baseline.surface"},
		{name: "a baseline captured_at change refuses", edit: func(ef *store.EvalFile) {
			ef.Criteria[2].Baseline.CapturedAt.Value = "git:def5678"
		}, wantMsg: "criterion C3 changes its baseline.captured_at"},
		{name: "a dropped baseline refuses on its surface", edit: func(ef *store.EvalFile) {
			ef.Criteria[2].Baseline = nil
		}, wantMsg: "criterion C3 changes its baseline.surface"},
		{name: "a node change refuses", edit: func(ef *store.EvalFile) {
			ef.Node = "F-000009"
		}, wantMsg: "changes node from F-000002 to F-000009"},
		{name: "an added criterion refuses", edit: func(ef *store.EvalFile) {
			ef.Criteria = append(ef.Criteria, store.EvalCriterion{ID: "C4", Statement: "new", Binding: store.EvalBinding{Type: store.BindingManual}})
		}, wantMsg: "adds criterion C4; new acceptance belongs to forward work"},
		{name: "a removed criterion without retire refuses", edit: func(ef *store.EvalFile) {
			ef.Criteria = append(ef.Criteria[:1], ef.Criteria[2:]...)
		}, wantMsg: "removes criterion C2 without --retire"},
		{name: "a removed criterion named in retire passes", edit: func(ef *store.EvalFile) {
			ef.Criteria = append(ef.Criteria[:1], ef.Criteria[2:]...)
		}, retire: []string{"C2"}},
		{name: "a retire id with stray whitespace still matches", edit: func(ef *store.EvalFile) {
			ef.Criteria = append(ef.Criteria[:1], ef.Criteria[2:]...)
		}, retire: []string{" C2 "}},
		{name: "retiring a criterion the corrected contract still carries refuses",
			retire: []string{"C3"}, wantMsg: "retires C3, which the corrected contract still carries"},
		{name: "retiring a criterion the verified contract never carried refuses",
			retire: []string{"C9"}, wantMsg: "retires C9, which the verified contract never carried"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old, current := verifiedContract(), verifiedContract()
			if tc.edit != nil {
				tc.edit(current)
			}
			err := checkEvalCorrection(old, current, tc.retire)
			if tc.wantMsg == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			var ue *usageError
			require.ErrorAs(t, err, &ue)
			require.Equal(t, "correct_eval_changes_contract", ue.Reason())
			require.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}
