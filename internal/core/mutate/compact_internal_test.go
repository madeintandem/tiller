package mutate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestCompactStopsAtOnceWhenTheTreeChangesUnderIt pins the one error that
// does not move on to the next day: a stale commit means the tree changed
// under the run, so no later day is attempted.
func TestCompactStopsAtOnceWhenTheTreeChangesUnderIt(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	manifest := "schema_version: 14\nschema_epoch: initial\nproject:\n  name: Acme\n  key: acme\n  kind: greenfield\n" +
		"contributors:\n  - { id: lantern, name: Lantern }\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(manifest), 0o644))
	for _, e := range []events.Event{
		{ID: "EV-aaaaaa", Type: "add.task", At: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC), Contributor: "lantern", Command: "tiller add task"},
		{ID: "EV-bbbbbb", Type: "add.task", At: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC), Contributor: "lantern", Command: "tiller add task"},
		{ID: "EV-cccccc", Type: "add.task", At: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC), Contributor: "lantern", Command: "tiller add task"},
	} {
		data, err := yaml.Marshal(e)
		require.NoError(t, err)
		path := filepath.Join(root, events.RelPath(e))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, data, 0o644))
	}
	// The first day refuses on its own account; the second finds the tree
	// changed under it.
	calls := 0
	old := commitDay
	commitDay = func(*store.Tree, []store.PlannedWrite) error {
		calls++
		if calls == 1 {
			return &store.EnvError{Code: 3, Reason: "event_unreadable", Msg: "unreadable"}
		}
		return &store.EnvError{Code: 3, Reason: store.ReasonMutationStale, Msg: "generation changed"}
	}
	t.Cleanup(func() { commitDay = old })

	res, err := Compact(root, CompactOptions{Now: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)})

	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason, "the stale refusal is the one a caller finds")
	require.Contains(t, err.Error(), "2026-11-03")
	require.Contains(t, err.Error(), "2026-11-04")
	require.Equal(t, 2, calls, "no day after the stale one is attempted")
	require.Empty(t, res.Days)
	require.FileExists(t, filepath.Join(root, ".tiller/events/2026/11/05/EV-cccccc.yml"))
}
