package mutate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const startedHistoryManifest = `schema_version: 15
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

// segmentTree lays a manifest and one segment holding evs, and returns the
// loaded tree with the segment's absolute path.
func segmentTree(t *testing.T, evs []events.Event) (*store.Tree, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(startedHistoryManifest), 0o644))
	data, err := events.EncodeSegment(evs)
	require.NoError(t, err)
	path := filepath.Join(root, events.SegmentRelPath(evs[0].At, data))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
	tree, err := store.Load(root)
	require.NoError(t, err)
	return tree, path
}

func startEvent(id string, at time.Time, target string) events.Event {
	return events.Event{ID: id, Type: "start.task", At: at, Contributor: "jc", Command: "start", Targets: []string{target}}
}

func TestStartedTaskHistoryGuardsACompactedStartEvent(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tree, segment := segmentTree(t, []events.Event{startEvent("ev-000001", at, "T-000001")})

	started, guards, err := startedTaskHistory(tree)

	require.NoError(t, err)
	require.True(t, started["T-000001"])
	require.Len(t, guards, 1)
	require.Equal(t, segment, guards[0].Path)
}

func TestStartedTaskHistoryGuardsASegmentOnce(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tree, _ := segmentTree(t, []events.Event{
		startEvent("ev-000001", at, "T-000001"),
		startEvent("ev-000002", at.Add(time.Minute), "T-000002"),
	})

	started, guards, err := startedTaskHistory(tree)

	require.NoError(t, err)
	require.True(t, started["T-000001"])
	require.True(t, started["T-000002"])
	require.Len(t, guards, 1)
}
