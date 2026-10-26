package gen

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// TestProsePathPerNoun pins the read-side prose derivation for every noun: the
// folder-backed nodes name their .md by the directory layout, and the question
// ledger item reports no file at all because its prose rides on the node itself.
func TestProsePathPerNoun(t *testing.T) {
	tr := mapTree()
	tr.Questions["Q-000001"] = &store.Question{ID: "Q-000001", Statement: "Which dialect?"}

	p, ok := ProsePath(tr, tr.Epics["E-founda"])
	require.True(t, ok)
	require.Equal(t, ".tiller/epics/foundation/epic.md", p)

	p, ok = ProsePath(tr, tr.Features["F-dbcore"])
	require.True(t, ok)
	require.Equal(t, ".tiller/epics/foundation/features/db-core/feature.md", p)

	p, ok = ProsePath(tr, tr.Tasks["T-task01"])
	require.True(t, ok)
	require.Equal(t, ".tiller/epics/form-engine/features/certification-form-loading/tasks/schema-transcription/task.md", p)

	_, ok = ProsePath(tr, tr.Questions["Q-000001"])
	require.False(t, ok, "a question's prose handle is its statement and answer")
}

// TestProsePathAmendment pins the flat, non-folder-backed layout an amendment's
// narrative lives under: .tiller/amendments/<A-id>-<key>.md
// (artifact-schema.md, Amendments), the same naming mutate's own
// amendmentRelMD builds.
func TestProsePathAmendment(t *testing.T) {
	tr := mapTree()
	key := "iphone-only"
	tr.Amendments = map[string]*store.Amendment{
		"A-000001": {ID: "A-000001", Key: &key, Title: "Remove native iPad support", Status: store.AmendmentStatusProposed},
	}

	p, ok := ProsePath(tr, tr.Amendments["A-000001"])
	require.True(t, ok)
	require.Equal(t, ".tiller/amendments/A-000001-iphone-only.md", p)
}

// A task whose key or whose parent feature the tree cannot place has no
// resolvable folder, so ProsePath answers no path rather than a guessed one.
func TestProsePathRefusesAnUnplaceableTask(t *testing.T) {
	tr := mapTree()
	tr.Tasks["T-nokey"] = &store.Task{ID: "T-nokey", FeatureID: "F-loadng"}
	orphanKey := "form-engine/gone/orphan"
	tr.Tasks["T-orphan"] = &store.Task{ID: "T-orphan", Key: &orphanKey, FeatureID: "F-gone"}

	_, ok := ProsePath(tr, tr.Tasks["T-nokey"])
	require.False(t, ok, "a task with no key cannot name its own folder")

	_, ok = ProsePath(tr, tr.Tasks["T-orphan"])
	require.False(t, ok, "a task whose parent feature is gone cannot name its folder either")
}

// TestProsePathMatchesTreeDrillTarget pins the two surfaces to one answer: the
// path the node page hands the console is byte-for-byte the drill target
// `tree --files` prints, because both read the same derivation.
func TestProsePathMatchesTreeDrillTarget(t *testing.T) {
	tr := mapTree()
	m := Tree(tr)

	epicPath, ok := ProsePath(tr, tr.Epics["E-founda"])
	require.True(t, ok)
	require.Equal(t, nodeByKey(m.Nodes, "foundation").Path, epicPath)

	featPath, ok := ProsePath(tr, tr.Features["F-dbcore"])
	require.True(t, ok)
	found := nodeByKey(m.Nodes, "foundation")
	require.Equal(t, nodeByKey(found.Children, "foundation/db-core").Path, featPath)
}

// TestEvalPathPerNoun pins the read-side eval-contract path derivation for
// every noun: a feature's feature.eval.md follows the same directory layout
// ProsePath resolves feature.md from, and every other noun reports no eval
// file at all. The feature is the one grain where acceptance lives: an epic's
// bar is the sum of its features' contracts, and a task's and a question's
// acceptance rides on the node itself (artifact-schema.md, Epics; Gates).
// Presence on disk is a separate question the store answers on load
// (Feature.HasEval); this derivation is the path alone.
func TestEvalPathPerNoun(t *testing.T) {
	tr := mapTree()
	tr.Questions["Q-000001"] = &store.Question{ID: "Q-000001", Statement: "Which dialect?"}

	p, ok := EvalPath(tr, tr.Features["F-dbcore"])
	require.True(t, ok)
	require.Equal(t, ".tiller/epics/foundation/features/db-core/feature.eval.md", p)

	_, ok = EvalPath(tr, tr.Epics["E-founda"])
	require.False(t, ok, "an epic's bar is the sum of its features' contracts, not a file of its own")

	_, ok = EvalPath(tr, tr.Tasks["T-task01"])
	require.False(t, ok, "a task binds to its feature's contract, carrying no eval file of its own")

	_, ok = EvalPath(tr, tr.Questions["Q-000001"])
	require.False(t, ok, "a question carries no eval contract")
}
