package views_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
	"github.com/stretchr/testify/require"
)

// lineFixture is a tree and its observed workflow input. readerOnly marks a
// fixture whose moves exist only for a reader (own work, own blockers).
type lineFixture struct {
	tree       *store.Tree
	in         workflow.Input
	readerOnly bool
}

// lineFixtures are trees that between them reach every candidate kind: the
// baseline moves, the reader's own work in every flavor, the queue's
// approvals, triage, answers, attests, closeouts, and merges, a blocker, the
// frontier, claimable and in-review tasks, shelf work in review, and
// decompose.
func lineFixtures(t *testing.T) map[string]lineFixture {
	t.Helper()
	execution := treeWithDraftEpics(t, 1)
	execution.Epics["E-aaaaa0"].Status = "ready"
	draftFeatureUnder(t, execution, "F-ddddd0", "epic-0/draft", "epic-0")
	_, claimable := readyFeatureWithTask(t, execution, "F-aaaaa0", "epic-0/claim", "epic-0")
	reviewFeature, reviewed := readyFeatureWithTask(t, execution, "F-bbbbb0", "epic-0/review", "epic-0")
	reviewFeature.Status, reviewFeature.ReviewGrain = "active", store.ReviewGrainTask
	pr := 7
	reviewed.Status, reviewed.Owner, reviewed.Started, reviewed.PR = "in_review", "jc", true, &pr
	execution.Chores = map[string]*store.Chore{
		"CH-aaaaa0": {WorkItem: store.WorkItem{ID: "CH-aaaaa0", Key: strPtr("chores/bump"), Status: "in_review", Owner: "jc"}},
	}
	require.Equal(t, "ready", claimable.Status)

	own := readyEpics(t, 1)
	f, observed := readyFeatureWithTask(t, own, "F-aaaaa0", "epic-0/work", "epic-0")
	f.Status = "active"
	f.Order = []string{observed.ID}
	addTask(own, f, "T-check0", "check", "check", "jc")
	addTask(own, f, "T-claim0", "claimed", "ready", "jc")
	addTask(own, f, "T-going0", "going", "in_progress", "jc")
	own.Bugs = map[string]*store.Bug{
		"B-aaaaa0": {WorkItem: store.WorkItem{ID: "B-aaaaa0", Key: strPtr("bugs/crash"), Title: "Crash", Status: "in_progress", Owner: "jc"}},
	}

	deadline := readyEpics(t, 1)
	_, dueTask := readyFeatureWithTask(t, deadline, "F-aaaaa0", "epic-0/work", "epic-0")
	datedQuestion(deadline, "Q-late00", dueTask.ID, store.QuestionDue{Date: "2026-10-19"})
	datedQuestion(deadline, "Q-soon00", dueTask.ID, store.QuestionDue{Date: "2026-10-22"})

	blocked := treeWithDraftEpics(t, 1)
	blocked.Epics["E-aaaaa0"].Status = "ready"
	held, heldTask := readyFeatureWithTask(t, blocked, "F-aaaaa0", "epic-0/work", "epic-0")
	heldTask.Owner = "jc"

	triage := &store.Tree{
		Manifest: &store.Manifest{},
		Bugs: map[string]*store.Bug{
			"B-prop00": {WorkItem: store.WorkItem{ID: "B-prop00", Key: strPtr("bugs/proposed"), Status: "proposed"}},
			"B-return": {WorkItem: store.WorkItem{ID: "B-return", Key: strPtr("bugs/returned"), Status: "ready", ResolvedBy: "F-gone00"}},
		},
		Chores: map[string]*store.Chore{
			"CH-prop00": {WorkItem: store.WorkItem{ID: "CH-prop00", Key: strPtr("chores/proposed"), Status: "proposed"}},
		},
		Tweaks: map[string]*store.Tweak{
			"TW-prop00": {WorkItem: store.WorkItem{ID: "TW-prop00", Key: strPtr("tweaks/proposed"), Status: "proposed"}},
		},
	}
	derive := &store.Tree{Manifest: &store.Manifest{}}
	derive.Manifest.Project.Kind = store.KindBrownfield

	checkTree, _ := awaitingCheckTree()
	featureReview, featureIn := reviewPhaseTree(t, store.ReviewGrainFeature)
	epicReview, epicIn := reviewPhaseTree(t, store.ReviewGrainEpic)
	return map[string]lineFixture{
		"attention":      {tree: attentionSummaryTree()},
		"execution":      {tree: execution},
		"own work":       {tree: own, in: observedOn(observed.ID, "t/x", "jc"), readerOnly: true},
		"deadline":       {tree: deadline},
		"blocker":        {tree: blocked, in: workflow.Input{Eligibility: map[string]string{held.ID: "accepted basis changed"}}, readerOnly: true},
		"triage":         {tree: triage},
		"check":          {tree: checkTree, in: workflowContracts(checkTree)},
		"feature review": {tree: featureReview, in: featureIn},
		"epic review":    {tree: epicReview, in: epicIn},
		"derive":         {tree: derive},
		"confirm":        {tree: confirmedBrownfield(t)},
		"decompose":      {tree: &store.Tree{Manifest: &store.Manifest{}, ToBeDesign: []string{"vision.md"}}},
	}
}

// yourWord matches "your" as a word, in any case.
var yourWord = regexp.MustCompile(`(?i)\byour\b`)

// Every candidate kind's printed line, for no reader and for a reader who
// touched every feature: with no reader nothing is "your" anything, and no
// reason restates the line's own node, command, or branch, which the line
// already prints. Each fixture is read raw (every move, before dedupe) and
// ranked (the tier reasons a surface shows).
func TestEveryKindsLineReadsRightWithAndWithoutAReader(t *testing.T) {
	kinds := append([]string{"derive_baseline", "confirm_design", "continue", "claim_task",
		"resolve_blocker", "specify", "merge_task", "decompose"}, views.QueueKinds...)
	readerOnly := map[string]bool{"continue": true, "resolve_blocker": true}
	seen := map[string]map[string]bool{"": {}, "jc": {}}

	for name, fx := range lineFixtures(t) {
		var touches []events.Event
		for id := range fx.tree.Features {
			touches = append(touches, locateEvent("EV-"+id, "spec.feature", "jc", 1, id))
		}
		for _, reader := range []string{"", "jc"} {
			if fx.readerOnly && reader == "" {
				continue
			}
			in := views.NextInput{Reader: reader, Workflow: fx.in, Now: locateNow}
			if reader != "" {
				in.Recent = touches
			}
			rows := append(views.CandidatesOf(fx.tree, gen.DependencyOrder(fx.tree), in), views.NextOf(fx.tree, in).Candidates...)
			for _, c := range rows {
				seen[reader][c.Kind] = true
				line := views.NextLine(c)
				if reader == "" {
					require.Falsef(t, yourWord.MatchString(line), "%s, no reader: %s line says your: %s", name, c.Kind, line)
				}
				for _, own := range []string{c.Command, c.Branch} {
					if own != "" {
						require.NotContainsf(t, c.Reason, own, "%s, reader %q: %s reason restates its own command or branch", name, reader, c.Kind)
					}
				}
				if c.Node != nil {
					require.NotContainsf(t, c.Reason, c.Node.Ref, "%s, reader %q: %s reason restates its own node", name, reader, c.Kind)
				}
				require.Falsef(t, strings.Contains(c.Reason, "\n"), "%s: %s reason is one line", name, c.Kind)
			}
		}
	}
	for _, kind := range kinds {
		require.Truef(t, seen["jc"][kind], "no fixture reaches %s for a reader", kind)
		if !readerOnly[kind] {
			require.Truef(t, seen[""][kind], "no fixture reaches %s without a reader", kind)
		}
	}
}
