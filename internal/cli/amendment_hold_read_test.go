package cli

import (
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the CLI read surfaces that say a draft is held (decision 7):
// tree, status, show amendment, approve task --feature, and the approve JSON.

// stageHeldFeature lays the amendment fixture, opens amendment "reshape", and
// stages one held feature under a new held epic, returning the amendment id and
// the two held node ids.
func stageHeldFeature(t *testing.T) (aid, epicID, featureID string) {
	t.Helper()
	layAmendmentTree(t)
	aid = openAmendmentCLI(t, "reshape", "Reshape the forms")
	for _, args := range [][]string{
		{"--add-epic", "hardening", "--title", "Hardening"},
		{"--add-feature", "hardening/csp-headers", "--title", "CSP headers", "--change-type", "create"},
	} {
		_, stderr, code := execCLI(t, append(append([]string{"amend", aid}, args...), "--contributor", "jc")...)
		require.Equal(t, 0, code, stderr)
	}
	ops := loadCwdTree(t).Amendments[aid].Operations
	return aid, ops[0].Node, ops[1].Node
}

func TestTreeMarksAHeldNode(t *testing.T) {
	_, epicID, featureID := stageHeldFeature(t)

	out, stderr, code := execCLI(t, "tree")
	require.Equal(t, 0, code, stderr)
	var held, unheld []string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "("+epicID+")"), strings.Contains(line, "("+featureID+")"):
			held = append(held, line)
		case strings.Contains(line, "form-engine"), strings.Contains(line, "certification-form-loading"):
			unheld = append(unheld, line)
		}
	}
	require.Len(t, held, 2, out)
	for _, line := range held {
		require.Contains(t, line, "(in amendment reshape)")
	}
	require.NotEmpty(t, unheld, out)
	for _, line := range unheld {
		require.NotContains(t, line, "in amendment")
	}

	jsonOut, _, code := execCLI(t, "tree", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, jsonOut, `"in_amendment": "reshape"`)
}

func TestStatusMarksAHeldNode(t *testing.T) {
	aid, _, featureID := stageHeldFeature(t)

	out, stderr, code := execCLI(t, "status", "feature", featureID)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "state: draft (in amendment reshape)")
	require.Contains(t, out, "next:  tiller approve amendment reshape")
	var next string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "next:") {
			next = line
		}
	}
	require.Equal(t, "  next:  tiller approve amendment reshape (held draft: the amendment's approval is the only move)", next)

	js, _, code := execCLI(t, "status", "feature", featureID, "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, js)
	hold := data["amendment_hold"].(map[string]any)
	require.Equal(t, aid, hold["id"])
	require.Equal(t, "reshape", hold["key"])
	require.Empty(t, data["legal_next"])

	// An unheld node reads as before.
	out, _, code = execCLI(t, "status", "feature", "form-engine/certification-form-loading")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "in amendment")
	require.NotContains(t, out, "approve amendment")

	// The project totals keep counting a held draft as a draft.
	sum, _, code := execCLI(t, "status", "--json")
	require.Equal(t, 0, code)
	counts := jsonData(t, sum)["counts"].(map[string]any)
	require.EqualValues(t, 2, counts["epics"], "the held epic still counts")
	require.EqualValues(t, 2, counts["features"], "the held feature still counts")
}

func TestShowAmendmentNamesItsRule(t *testing.T) {
	aid, _, featureID := stageHeldFeature(t)

	out, stderr, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "New work: held drafts, promoted at approval")
	require.NotContains(t, out, "created at approval")
	require.Contains(t, out, "held draft hardening/csp-headers ("+featureID+")")

	// An amendment staged before schema 15 carries no node on its adds.
	legacy := openAmendmentCLI(t, "legacy", "Staged before schema 15")
	appendAmendmentOpOnDisk(t, legacy, store.AmendmentOp{Op: store.AmendmentOpAddEpic, Key: "old-epic", Title: "Old epic"})
	out, stderr, code = execCLI(t, "show", "amendment", legacy)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "New work: created at approval (staged before schema 15)")
	require.NotContains(t, out, "held drafts")
	require.NotContains(t, out, "held draft ")

	// An amendment with no add operation names no rule.
	none := openAmendmentCLI(t, "plain", "No adds")
	out, _, code = execCLI(t, "show", "amendment", none)
	require.Equal(t, 0, code)
	require.NotContains(t, out, "New work")
}

func TestShowAmendmentReportsARemovedDraftWithItsReason(t *testing.T) {
	aid, epicID, featureID := stageHeldFeature(t)
	_, stderr, code := execCLI(t, "remove", "feature", featureID, "--reason", "folded into another feature", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	out, stderr, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "removed: folded into another feature")
	require.Contains(t, out, "held draft hardening ("+epicID+")")

	js, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code)
	op := jsonData(t, js)["operations"].([]any)[1].(map[string]any)
	require.Equal(t, "folded into another feature", op["removed"])
}

func TestShowAmendmentReportsAMissingHeldNode(t *testing.T) {
	aid, _, _ := stageHeldFeature(t)
	appendAmendmentOpOnDisk(t, aid, store.AmendmentOp{Op: store.AmendmentOpAddEpic, Key: "ghost", Title: "Ghost", Node: "E-ghost0"})

	out, stderr, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr+out)
	require.Contains(t, out, "held draft E-ghost0: missing")
	require.Contains(t, out, "held draft hardening", "the read still shows the operations that resolve")
}

// A removed operation is out of the proposal, so show amendment reports no
// precondition verdict for it.
func TestShowAmendmentReportsNoPreconditionForARemovedOperation(t *testing.T) {
	aid, _, _ := stageHeldFeature(t)
	appendAmendmentOpOnDisk(t, aid, store.AmendmentOp{
		Op: store.AmendmentOpAddEpic, Key: "dropped", Title: "Dropped", Node: "E-drop00", Removed: "not this quarter",
		Preconditions: map[string]string{"target_status": "done"},
	})

	js, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, js)
	op := jsonData(t, js)["operations"].([]any)[2].(map[string]any)
	pcs := op["preconditions"].([]any)
	require.Len(t, pcs, 1)
	require.Empty(t, pcs[0].(map[string]any)["status"], "no live verdict for a removed operation")
}

func TestApproveTaskByFeatureNamesTheAmendmentHoldingItsProposedTasks(t *testing.T) {
	layAmendmentTree(t)
	approveFixtureFeatureOnDisk(t)
	// The fixture's own proposed tasks are approved so only the held one remains.
	_, stderr, code := execCLI(t, "approve", "task", "--feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	aid := openAmendmentCLI(t, "more-work", "More work")
	_, stderr, code = execCLI(t, "amend", aid, "--add-task", "form-engine/certification-form-loading/load-offline",
		"--title", "Load offline", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	out, stderr, code := execCLI(t, "approve", "task", "--feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "No proposed tasks on feature")
	require.Contains(t, out, "held by an open amendment")
	require.Contains(t, out, "tiller approve amendment more-work")
}

func TestApproveAmendmentJSONFlagsPromotedDraftsApartFromCreatedAtApproval(t *testing.T) {
	aid, _, _ := stageHeldFeature(t)
	appendAmendmentOpOnDisk(t, aid, store.AmendmentOp{Op: store.AmendmentOpAddEpic, Key: "old-epic", Title: "Old epic"})

	out, stderr, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr+out)
	applied := jsonData(t, out)["applied"].([]any)
	require.Len(t, applied, 3)
	require.Equal(t, true, applied[0].(map[string]any)["held_draft"])
	require.Equal(t, true, applied[1].(map[string]any)["held_draft"])
	_, flagged := applied[2].(map[string]any)["held_draft"]
	require.False(t, flagged, "an add created at approval is not a promoted draft")
}

func TestShowAmendmentSaysNothingAlarmingForFinishedAmendments(t *testing.T) {
	aid, epicID, _ := stageHeldFeature(t)
	_, stderr, code := execCLI(t, "withdraw", "amendment", aid, "--reason", "not now", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	out, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code)
	require.NotContains(t, out, "missing")
	require.Contains(t, out, "draft "+epicID+" removed when the amendment was withdrawn")

	// An approved amendment whose promoted task was removed since.
	approveFixtureFeatureOnDisk(t)
	aid2 := openAmendmentCLI(t, "second", "Second")
	_, stderr, code = execCLI(t, "amend", aid2, "--add-task", "form-engine/certification-form-loading/later", "--title", "Later", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	later := loadCwdTree(t).Amendments[aid2].Operations[0].Node
	out, stderr, code = execCLI(t, "approve", "amendment", aid2, "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	out, _, _ = execCLI(t, "show", "amendment", aid2)
	require.Contains(t, out, "promoted draft form-engine/certification-form-loading/later ("+later+")")
	_, stderr, code = execCLI(t, "return", "task", later, "--reason", "rework", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = execCLI(t, "remove", "task", later, "--reason", "gone", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	out, _, _ = execCLI(t, "show", "amendment", aid2)
	require.NotContains(t, out, "missing")
	require.NotContains(t, out, "held draft ")
	require.NotContains(t, out, "promoted draft")
}

func TestTreeKeepsTheAmendmentMarkerWhenTheTitleIsCut(t *testing.T) {
	m := gen.TreeModel{Project: "p", Nodes: []gen.TreeNode{{
		Noun: "feature", ID: "F-aaaaaa", Key: "e/f", Status: "draft", InAmendment: "reshape",
		Title: strings.Repeat("long title ", 10),
	}}}
	out := renderTreeHuman(m, treeFlags{}, 50)
	require.Contains(t, out, "(in amendment reshape)")
	require.Contains(t, out, "…")
}
