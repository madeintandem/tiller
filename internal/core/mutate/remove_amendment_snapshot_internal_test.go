package mutate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRemovalAbortsWhenTheAmendmentChangesBeforeCommit pins the snapshot
// guard on the amendment rewrite a removal of a held draft plans
// (planRemovalAmendments' edit branch, which marks the operation removed): an
// amendment file that changes between the plan and the commit aborts the
// whole removal, so the draft stays and the changed file is not overwritten.
// No seam reaches that window from outside, so the plan and the commit are
// driven here directly.
func TestRemovalAbortsWhenTheAmendmentChangesBeforeCommit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel string, data []byte) {
		t.Helper()
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, data, 0o644))
	}
	marshal := func(v any) []byte {
		t.Helper()
		data, err := yaml.Marshal(v)
		require.NoError(t, err)
		return data
	}
	write(".tiller/tiller.yml", []byte("schema_version: 15\nschema_epoch: initial\nproject:\n  name: Acme\n  key: acme\n  kind: greenfield\n"+
		"contributors:\n  - { id: lantern, name: Lantern }\n"))
	epicKey, amendmentKey := "hardening", "reshape"
	write(".tiller/epics/hardening/epic.yml", marshal(store.Epic{ID: "E-aaaaaa", Key: &epicKey, Title: "Hardening", Status: statusDraft}))
	write(".tiller/epics/hardening/epic.md", []byte("# Hardening\n"))
	amendment := store.Amendment{ID: "A-aaaaaa", Key: &amendmentKey, Title: "Reshape", Status: store.AmendmentStatusDraft,
		Operations: []store.AmendmentOp{{Op: store.AmendmentOpAddEpic, Key: epicKey, Title: "Hardening", Node: "E-aaaaaa"}}}
	amendmentRel := ".tiller/amendments/A-aaaaaa-reshape.yml"
	write(amendmentRel, marshal(amendment))
	write(".tiller/amendments/A-aaaaaa-reshape.md", []byte("# Reshape\n"))

	tree, err := store.Load(root)
	require.NoError(t, err)
	require.Equal(t, "A-aaaaaa", tree.AmendmentHolds["E-aaaaaa"])
	removed, err := collectRemovedNodes(tree, tree.Epics["E-aaaaaa"])
	require.NoError(t, err)
	var marked []string
	plan, err := planRemoval(tree, removed, amendmentRemovalEdit{edit: markHeldOpsRemoved(tree, removedNodeSet(removed), "not now", &marked)})
	require.NoError(t, err)
	require.Equal(t, []string{"A-aaaaaa"}, marked, "the plan rewrites the amendment")

	amendment.Title = "Reshape, edited after the plan"
	changed := marshal(amendment)
	write(amendmentRel, changed)

	err = commitMutation(tree, "remove epic", plan.writes, nil)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)
	require.FileExists(t, filepath.Join(root, ".tiller/epics/hardening/epic.yml"), "nothing was removed")
	after, err := os.ReadFile(filepath.Join(root, amendmentRel))
	require.NoError(t, err)
	require.Equal(t, string(changed), string(after), "the changed amendment is not overwritten")
}
