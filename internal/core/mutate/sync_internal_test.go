package mutate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// rollupTree mirrors gen's activity_test fixture: one feature, the evals gate
// required at feature grain.
func rollupTree(status string, gates map[string]store.Gate, taskStatuses ...string) (*store.Tree, *store.Feature) {
	key := "foundation/repo-tooling"
	f := &store.Feature{ID: "F-aaaaaa", Key: &key, Status: status,
		GatesBlock: store.GatesBlock{Gates: gates}}
	tree := &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
			"evals": {Kind: "command", On: store.StringList{"feature"}},
		}}},
		Features: map[string]*store.Feature{f.ID: f},
		Tasks:    map[string]*store.Task{},
	}
	for i, ts := range taskStatuses {
		id := string(rune('A' + i))
		tree.Tasks["T-"+id] = &store.Task{ID: "T-" + id, Status: ts, FeatureID: f.ID}
	}
	return tree, f
}

// TestFeatureRollupReadyIsTheAwaitingSyncPhase pins the refactor: the rollup
// fires exactly when the derived phase says awaiting_sync, on every shape of
// the truth table, so the display and the crossing can never disagree.
func TestFeatureRollupReadyIsTheAwaitingSyncPhase(t *testing.T) {
	t.Parallel()
	checked := map[string]store.Gate{"evals": {Required: true, Status: store.GateStatusPending}}
	cases := []struct {
		name   string
		status string
		gates  map[string]store.Gate
		tasks  []string
	}{
		{"no tasks", "active", nil, nil},
		{"in flight", "active", nil, []string{"done", "in_progress"}},
		{"all done unchecked", "active", nil, []string{"done", "done"}},
		{"all done checked", "active", checked, []string{"done"}},
		{"cancelled only", "active", nil, []string{"cancelled"}},
		{"not active", "ready", checked, []string{"done"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, f := rollupTree(tc.status, tc.gates, tc.tasks...)
			want := gen.FeatureActivity(tree, f) == gen.FeatureActivityAwaitingSync
			require.Equal(t, want, featureRollupReady(tree, f))
		})
	}
}

// TestChecksDueNamesAwaitingCheckFeaturesOnly is the incident in miniature: a
// tree whose feature has every task done and no recorded feature gates is
// named; one with gates recorded (awaiting sync) and one still in flight are
// not.
func TestChecksDueNamesAwaitingCheckFeaturesOnly(t *testing.T) {
	t.Parallel()
	tree, f := rollupTree("active", nil, "done", "done")
	due := checksDue(tree)
	require.Len(t, due, 1)
	require.Equal(t, CheckDue{Noun: store.NounFeature, ID: f.ID, Key: "foundation/repo-tooling"}, due[0])

	checkedTree, _ := rollupTree("active",
		map[string]store.Gate{"evals": {Status: store.GateStatusPending}}, "done")
	require.Empty(t, checksDue(checkedTree), "awaiting_sync never lands in ChecksDue")

	inFlight, _ := rollupTree("active", nil, "done", "in_progress")
	require.Empty(t, checksDue(inFlight))
}

// TestFeatureQuestionBlockersMirrorsTheGuard verifies that the shared blocker
// projection gathers open questions blocking a feature and respects
// assumption_basis to clear them.
func TestFeatureQuestionBlockersMirrorsTheGuard(t *testing.T) {
	t.Parallel()
	tree, f := rollupTree("active", nil, "done")
	tree.Questions = map[string]*store.Question{
		"Q-open": {ID: "Q-open", Status: "open", Statement: "which program first?",
			Blocks: []string{f.ID}},
	}
	require.Len(t, gen.FeatureQuestionBlockers(tree, f), 1)

	// An assumption covering the question clears the blocker, the guard's rule.
	f.AssumptionBasis = store.AssumptionBasis{"Q-open": store.Assumption{}}
	require.Empty(t, gen.FeatureQuestionBlockers(tree, f))
}

func TestTouchedDesignNormalizesDeclaredAliasesWithoutReadingDescendants(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	external := t.TempDir()
	require.NoError(t, os.Symlink(external, filepath.Join(root, "legacy")))
	canonicalRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	canonicalExternal, err := filepath.EvalSymlinks(external)
	require.NoError(t, err)
	described, err := filepath.Rel(canonicalRoot, filepath.Join(canonicalExternal, "module"))
	require.NoError(t, err)
	tree := &store.Tree{
		Root: root,
		Manifest: &store.Manifest{Inputs: store.InputsBlock{
			Code: []store.CodeRoot{{Root: "legacy", Mode: store.CodeModeReference}},
		}},
		Baseline: store.BaselineLedger{Docs: []store.BaselineDoc{
			{Path: "legacy.md", Describes: []string{filepath.ToSlash(described)}},
			{Path: "sibling.md", Describes: []string{filepath.ToSlash(described) + "-other"}},
			{Path: "project.md", Describes: []string{"."}},
		}},
	}
	// Historical surface aliases retain both their printed coordinate and the
	// declaration's canonical coordinate, even after a descendant disappeared.
	require.Equal(t, []string{"legacy.md", "project.md"},
		touchedDesign(tree, []string{"legacy/module/../module/vanished.go"}))
	require.Equal(t, []string{"legacy.md"},
		touchedDesign(tree, []string{filepath.Join(canonicalExternal, "module/vanished.go")}))
}
