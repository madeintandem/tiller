package views_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

func baselineConfirmation() *store.BaselineConfirmation {
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	return &store.BaselineConfirmation{
		By: "jc", Commit: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:abcdef0", ObservedAt: at},
		Surfaces: []store.SurfaceFingerprint{{Path: ".", Kind: store.KindGitTree, Value: "git:1234567", ObservedAt: at}},
	}
}

// withoutKind drops every candidate of one kind, keeping the rest in order.
func withoutKind(cands []views.NextCandidate, kind string) []views.NextCandidate {
	out := []views.NextCandidate{}
	for _, c := range cands {
		if c.Kind != kind {
			out = append(out, c)
		}
	}
	return out
}

func TestBaselinePreemptsAndReleasesEveryExistingRoute(t *testing.T) {
	for _, kind := range []string{store.KindBrownfield, store.KindLegacyMigration} {
		for name, build := range routeArmFixtures {
			t.Run(kind+"/"+name, func(t *testing.T) {
				tree := build(t)
				previous := nextFor(tree, "jc", workflow.Input{}).Candidates
				tree.Manifest.Project.Kind = kind
				n := nextFor(tree, "jc", workflow.Input{})
				require.Len(t, n.Candidates, 1, "deriving the baseline preempts every other move")
				require.Equal(t, "derive_baseline", n.Candidates[0].Kind)
				require.Nil(t, n.Candidates[0].Node)
				require.False(t, n.CaughtUp)
				tree.Baseline.Docs = []store.BaselineDoc{{Path: "accepted.md", Describes: []string{"."}}, {Path: "pending.md", Describes: []string{"."}}}
				require.Equal(t, []string{"confirm_design", "confirm_design"}, candidateKinds(nextFor(tree, "jc", workflow.Input{}).Candidates),
					"registered docs await confirmation, still exclusively")
				tree.Baseline.Docs[0].Confirmed = baselineConfirmation()
				released := nextFor(tree, "jc", workflow.Input{}).Candidates
				require.Len(t, candidatesOfKind(released, "confirm_design"), 1, "the pending sibling stays a move")
				require.Equal(t, previous, withoutKind(released, "confirm_design"), "one confirmation must restore every prior move, not gate on pending siblings")
				tree.Manifest.Project.Kind = store.KindGreenfield
				tree.Baseline.Docs[0].Confirmed = nil
				require.Equal(t, previous, withoutKind(nextFor(tree, "jc", workflow.Input{}).Candidates, "confirm_design"))
			})
		}
	}
}

func TestBaselineReleasePreservesCheckedWorkAndQuestionSkip(t *testing.T) {
	tree, _, _, legal := questionHeldRouteTree(t)
	legal.Status = "check"
	tree.Manifest.Project.Kind = store.KindBrownfield
	tree.Baseline.Docs = []store.BaselineDoc{{Path: "accepted.md", Describes: []string{"."}, Confirmed: baselineConfirmation()}}
	top := topMove(t, tree, "jc", workflow.Input{})
	require.Equal(t, "continue", top.Kind)
	require.Equal(t, legal.ID, top.Node.ID)
	// A checked held task is still no move after release; its questions are.
	tree.Tasks["T-held"].Status = "check"
	n := nextFor(tree, "jc", workflow.Input{})
	require.Equal(t, legal.ID, n.Candidates[0].Node.ID)
	require.Empty(t, candidatesFor(n.Candidates, "T-held"))
	require.Equal(t, "answer_question", candidatesFor(n.Candidates, "Q-alpha")[0].Kind)
}

func TestBaselineReadoutRetainsCompleteRevisionCoverageAndCurrentQuestions(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Manifest.Project.Kind = store.KindBrownfield
	present := map[string]bool{}
	for i := range 6 {
		doc := store.BaselineDoc{Path: fmt.Sprintf("module-%d.md", i), Describes: []string{fmt.Sprintf("src/module-%d", i)}}
		doc.DerivedFrom = []store.RevisionRef{{Path: "docs/source.md", Kind: store.KindGitBlob, Value: fmt.Sprintf("git:revision-%d", i%2)}}
		tree.Baseline.Docs = append(tree.Baseline.Docs, doc)
		present[doc.Path] = i != 5
	}
	tree.Questions = map[string]*store.Question{
		"Q-old":      {ID: "Q-old", Status: "open"},
		"Q-new":      {ID: "Q-new", Status: "open"},
		"Q-answered": {ID: "Q-answered", Status: "answered"},
	}
	r := views.ReadoutOfInput(tree, views.MomentDeriveBaseline, views.ReadoutInput{DesignDocPresent: present})
	require.NotNil(t, r.Baseline)
	require.Len(t, r.Baseline.Docs, 6)
	require.Zero(t, r.Withheld)
	require.Len(t, r.Baseline.Sources, 2, "separate revisions of one source must not collapse")
	require.Equal(t, []string{"module-0.md", "module-2.md", "module-4.md"}, r.Baseline.Sources[0].Docs)
	require.Equal(t, []string{"module-1.md", "module-3.md", "module-5.md"}, r.Baseline.Sources[1].Docs)
	require.Equal(t, 2, r.Baseline.OpenQuestions)
	require.Equal(t, 6, r.Baseline.Pending)
	require.NotNil(t, r.Baseline.Docs[5].Present)
	require.False(t, *r.Baseline.Docs[5].Present)
	require.Len(t, r.Next.Candidates, 6, "one confirmation move per pending doc")
	require.Equal(t, "human", topNext(t, r).Actor)
	require.Equal(t, "confirm_design", topNext(t, r).Kind)
	require.NotNil(t, r.Baseline.Review)
	correction := views.CorrectionOfInput(tree, views.MomentDeriveBaseline, views.CorrectionInput{DesignDocPresent: present})
	require.Equal(t, r.Next, correction.Next)
	require.Equal(t, r.Baseline, correction.Baseline)
	unknown := views.ReadoutOf(tree, views.MomentDeriveBaseline)
	require.Nil(t, unknown.Baseline.Docs[5].Present, "pure views never assert unobserved existence")
	// Existing confirmations are historical facts, not an all-doc startup gate.
	tree.Baseline.Docs[0].Confirmed = baselineConfirmation()
	r = views.ReadoutOf(tree, views.MomentOrient)
	require.Equal(t, 1, r.Baseline.Confirmed)
	require.Equal(t, 5, r.Baseline.Pending)
	require.Empty(t, r.Baseline.Docs, "ordinary execution readouts must not repeat the full baseline")
	require.Empty(t, r.Baseline.Sources)
	require.True(t, r.Next.Unplanned, "orient resumes ordinary routing after first confirmation: with no to-be doc, the no-work-planned end state")
	require.Contains(t, r.Next.Hint, "no work is planned yet")
	require.Equal(t, []string{"confirm_design", "confirm_design", "confirm_design", "confirm_design", "confirm_design", "answer_question", "answer_question"}, nextKinds(r),
		"each doc still pending stays a confirmation move beside the rest")
	finished := views.ReadoutOf(tree, views.MomentDeriveBaseline)
	require.NotNil(t, finished.Baseline.Review, "finishing the baseline seam still hands pending docs to the human")
	require.Equal(t, r.Next, finished.Next, "Next is one list whatever the moment")
}

func TestMissingConfirmedBaselineNeedsRepairNotConfirmation(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Manifest.Project.Kind = store.KindBrownfield
	tree.Baseline.Docs = []store.BaselineDoc{
		{Path: "accepted.md", Describes: []string{"."}, Confirmed: baselineConfirmation()},
	}
	input := views.ReadoutInput{DesignDocPresent: map[string]bool{"accepted.md": false}}
	full := views.ReadoutOfInput(tree, views.MomentDeriveBaseline, input)
	require.Equal(t, 1, full.Baseline.Confirmed)
	require.Zero(t, full.Baseline.Pending)
	require.Equal(t, 1, full.Baseline.Missing)
	require.Nil(t, full.Baseline.Review)
	require.NotNil(t, full.Baseline.Repair)
	require.Equal(t, "human", full.Baseline.Repair.Owner)
	require.Contains(t, full.Baseline.Repair.Text, "tiller unlink design")
	require.False(t, *full.Baseline.Docs[0].Present)
	require.Empty(t, full.Next.Candidates, "repair must not reopen the baseline startup route")
	require.Contains(t, full.Next.Hint, "no work is planned yet")
	correction := views.CorrectionOfInput(tree, views.MomentDeriveBaseline, views.CorrectionInput{DesignDocPresent: input.DesignDocPresent})
	require.Equal(t, full.Baseline, correction.Baseline)
	require.Equal(t, full.Next, correction.Next)

	brief := views.ReadoutOfInput(tree, views.MomentOrient, input)
	require.Equal(t, full.Baseline.Repair, brief.Baseline.Repair)
	require.Equal(t, 1, brief.Baseline.Missing)
	require.Nil(t, brief.Baseline.Docs, "ordinary readouts retain only the brief attention summary")
	require.Nil(t, brief.Baseline.Sources)
	require.Contains(t, brief.Next.Hint, "no work is planned yet")

	unknown := views.ReadoutOf(tree, views.MomentDeriveBaseline)
	require.Zero(t, unknown.Baseline.Missing)
	require.Nil(t, unknown.Baseline.Repair, "unobserved presence cannot create a repair claim")
	require.Nil(t, unknown.Baseline.Docs[0].Present)
	input.DesignDocPresent["accepted.md"] = true
	restored := views.ReadoutOfInput(tree, views.MomentDeriveBaseline, input)
	require.Zero(t, restored.Baseline.Missing)
	require.Nil(t, restored.Baseline.Repair)
	require.Equal(t, 1, restored.Baseline.Confirmed)
}

func TestDesignApprovalReadoutReportsTheActNotALifecycleMove(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	tree.Manifest.Project.Kind = store.KindBrownfield
	tree.Baseline.Docs = []store.BaselineDoc{
		{Path: "first.md", Describes: []string{"."}, Confirmed: baselineConfirmation()},
		{Path: "prior.md", Describes: []string{"."}, Confirmed: baselineConfirmation()},
		{Path: "pending.md", Describes: []string{"."}},
	}
	r := views.ReadoutOfInput(tree, views.MomentApprove, views.ReadoutInput{Act: &views.GateAct{DesignApproval: &views.DesignApprovalAct{Confirmed: []string{"first.md"}, Unchanged: []string{"prior.md"}}}})
	require.Equal(t, []views.ReadoutCheck{{Label: "Design docs confirmed", Result: "1"}, {Label: "Confirmations unchanged", Result: "1"}}, r.Checks)
	require.Equal(t, []string{"confirm_design"}, nextKinds(r), "the doc still pending stays a move after startup releases")
	require.Equal(t, "tiller approve design pending.md", topNext(t, r).Command)
	require.Contains(t, r.Next.Hint, "no work is planned yet")
	require.NotContains(t, r.Checks, views.ReadoutCheck{Label: "Moved to ready", Result: "1 of 2"})
}

func TestDesignCorrectionEventsKeepPathMeaningAfterUnlink(t *testing.T) {
	tree := treeWithDraftEpics(t, 0)
	for _, typ := range []string{"link.design", "unlink.design", "approve.design"} {
		t.Run(typ, func(t *testing.T) {
			event := events.Event{Type: typ, Targets: []string{".tiller/design/-owner's guide.md"}, After: map[string]any{
				"doc": "-owner's guide.md", "describes_added": []any{"src/module"}, "describes_removed": []string{"gone/module"},
				"from_added": []any{"docs/source.md"}, "from_updated": []any{"docs/new.md"}, "from_removed": []any{"docs/gone.md"}, "commit": "git:abcdef0",
			}}
			c := views.CorrectionOfInput(tree, views.MomentDeriveBaseline, views.CorrectionInput{Events: []events.Event{event}})
			require.Len(t, c.Changed, 1)
			require.Contains(t, c.Changed[0], ".tiller/design/-owner's guide.md")
			require.NotContains(t, c.Changed[0], "edge")
			if typ == "approve.design" {
				require.Contains(t, c.Changed[0], "git:abcdef0")
				require.NotContains(t, c.Changed[0], "ready")
			} else {
				for _, path := range []string{"src/module", "gone/module", "docs/source.md", "docs/new.md", "docs/gone.md"} {
					require.True(t, strings.Contains(c.Changed[0], path), path)
				}
			}
		})
	}
}
