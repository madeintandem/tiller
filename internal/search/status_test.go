package search_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin the status filter: Options.Status keeps the nodes in one
// stored status, or with "live" every node that is not done, cancelled, or
// archived. Documents have no status, so any status filter leaves them out.

// statusFixture titles every node of the shelf fixture to match "ledger"
// and spreads them over statuses: a ready epic, a done feature, a cancelled
// task, a proposed bug, a done chore, an in_progress tweak, an open question,
// and one design doc that matches too.
func statusFixture(t *testing.T) (string, *store.Tree) {
	t.Helper()
	root, tree := searchShelfFixture(t)
	tree.Epics["E-aaaaaa"].Title, tree.Epics["E-aaaaaa"].Status = "Ledger epic", "ready"
	tree.Features["F-aaaaaa"].Title, tree.Features["F-aaaaaa"].Status = "Ledger feature", "done"
	tree.Tasks["T-aaaaaa"].Title, tree.Tasks["T-aaaaaa"].Status = "Ledger task", "cancelled"
	tree.Bugs["B-aaaaaa"].Title, tree.Bugs["B-aaaaaa"].Status = "Ledger bug", "proposed"
	tree.Chores["CH-bbbbbb"].Title, tree.Chores["CH-bbbbbb"].Status = "Ledger chore", "done"
	tree.Tweaks["TW-cccccc"].Title, tree.Tweaks["TW-cccccc"].Status = "Ledger tweak", "in_progress"
	tree.Questions["Q-aaaaaa"].Title = "Ledger question"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "02-ledger.md"),
		[]byte("# Ledger notes\n"), 0o644))
	return root, tree
}

func searchStatus(t *testing.T, root string, tree *store.Tree, opts search.Options) search.Result {
	t.Helper()
	res, err := search.Search(root, tree, "ledger", opts)
	require.NoError(t, err)
	return res
}

func nodeIDs(res search.Result) []string {
	out := []string{}
	for _, n := range res.Nodes {
		out = append(out, n.ID)
	}
	return out
}

// TestLiveLeavesOutFinishedAndAbandonedWork is the defect: checking whether
// something is already tracked returned done and cancelled nodes beside the
// live ones.
func TestLiveLeavesOutFinishedAndAbandonedWork(t *testing.T) {
	root, tree := statusFixture(t)
	require.Len(t, searchStatus(t, root, tree, search.Options{}).Nodes, 7, "unfiltered, every node matches")

	res := searchStatus(t, root, tree, search.Options{Status: "live"})
	require.ElementsMatch(t, []string{"E-aaaaaa", "B-aaaaaa", "TW-cccccc", "Q-aaaaaa"}, nodeIDs(res))
	require.Equal(t, "live", res.Status)
	require.Equal(t, 4, res.Count)

	tree.Epics["E-aaaaaa"].Status = "archived"
	require.NotContains(t, nodeIDs(searchStatus(t, root, tree, search.Options{Status: "live"})), "E-aaaaaa")
}

// TestAStatusNamesOneStoredStatus: the fuller form keeps exactly the nodes
// whose stored status is the one named.
func TestAStatusNamesOneStoredStatus(t *testing.T) {
	root, tree := statusFixture(t)
	require.ElementsMatch(t, []string{"F-aaaaaa", "CH-bbbbbb"}, nodeIDs(searchStatus(t, root, tree, search.Options{Status: "done"})))
	require.Equal(t, []string{"T-aaaaaa"}, nodeIDs(searchStatus(t, root, tree, search.Options{Status: "cancelled"})))
	require.Equal(t, []string{"Q-aaaaaa"}, nodeIDs(searchStatus(t, root, tree, search.Options{Status: "open"})))
	require.Empty(t, nodeIDs(searchStatus(t, root, tree, search.Options{Status: "specced"})))
}

// TestAStatusFilterLeavesDocumentsOut: a document has no status, so it is in
// no status. Asking for documents in a status is a legal empty answer.
func TestAStatusFilterLeavesDocumentsOut(t *testing.T) {
	root, tree := statusFixture(t)
	require.Len(t, searchStatus(t, root, tree, search.Options{}).Documents, 1)

	res := searchStatus(t, root, tree, search.Options{Status: "live"})
	require.Empty(t, res.Documents)
	require.NotNil(t, res.Documents, "still an array")

	res = searchStatus(t, root, tree, search.Options{Status: "live", Kind: search.KindDocument})
	require.Zero(t, res.Count)
	require.Empty(t, res.Nodes)
	require.Empty(t, res.Documents)
}

// TestStatusComposesWithKind: both filters hold at once, and kinds tallies
// what the status filter kept across every kind, so a reader narrowed to one
// kind still sees what the others hold in that status.
func TestStatusComposesWithKind(t *testing.T) {
	root, tree := statusFixture(t)
	res := searchStatus(t, root, tree, search.Options{Status: "done", Kind: "chore"})
	require.Equal(t, []string{"CH-bbbbbb"}, nodeIDs(res))
	require.Equal(t, 1, res.Count)
	require.Equal(t, []search.KindCount{{Kind: "feature", Count: 1}, {Kind: "chore", Count: 1}}, res.Kinds)

	got, err := json.Marshal(res)
	require.NoError(t, err)
	require.Contains(t, string(got), `"kind":"chore","status":"done","count":1`)

	got, err = json.Marshal(searchStatus(t, root, tree, search.Options{}))
	require.NoError(t, err)
	require.Contains(t, string(got), `{"query":"ledger","count":8,`, "no filter, no status field")
}

// TestAnUnknownStatusIsTypedMisuse: a status no node can hold is refused the
// way an unknown kind is.
func TestAnUnknownStatusIsTypedMisuse(t *testing.T) {
	root, tree := statusFixture(t)
	res, err := search.Search(root, tree, "ledger", search.Options{Status: "finished"})
	require.Equal(t, search.Result{}, res)
	var unknown *search.UnknownStatusError
	require.ErrorAs(t, err, &unknown)
	require.Equal(t, 2, unknown.ExitCode())
	require.Equal(t, "unknown_status", unknown.Reason())
	require.Contains(t, unknown.Error(), "live")
}
