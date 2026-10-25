package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const stabilityManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Stability Test
  key: stability
  kind: greenfield
contributors: []
`

func TestAPIRefusesProjectionWhoseLoadedTreeBecameStale(t *testing.T) {
	for _, projectionErr := range []error{nil, errors.New("raw projection error")} {
		name := "success"
		if projectionErr != nil {
			name = "projection_error"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			tiller := filepath.Join(root, ".tiller")
			require.NoError(t, os.MkdirAll(tiller, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(tiller, "tiller.yml"), []byte(stabilityManifest), 0o644))

			handler := api(root, func(tree *store.Tree, _ *http.Request) (any, error) {
				markdown := filepath.Join(tiller, "design", "new.md")
				require.NoError(t, os.MkdirAll(filepath.Dir(markdown), 0o755))
				require.NoError(t, os.WriteFile(markdown, []byte("new generation prose\n"), 0o644))
				generation := filepath.Join(tiller, ".cache", "generation")
				require.NoError(t, os.MkdirAll(filepath.Dir(generation), 0o755))
				require.NoError(t, os.WriteFile(generation, []byte("next-generation"), 0o600))
				return map[string]any{"old_project": tree.Manifest.Project.Key, "new_markdown": "new generation prose"}, projectionErr
			})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/test", nil))
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			require.Contains(t, recorder.Body.String(), "Tiller state changed while it was being read")
			require.NotContains(t, recorder.Body.String(), "new generation prose")
			require.NotContains(t, recorder.Body.String(), "raw projection error")
		})
	}
}

func TestStaleRecapDoesNotConsumeVisitHorizon(t *testing.T) {
	root := t.TempDir()
	tiller := filepath.Join(root, ".tiller")
	require.NoError(t, os.MkdirAll(tiller, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tiller, "tiller.yml"), []byte(stabilityManifest), 0o644))
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	prior := now.Add(-2 * time.Hour)
	writeLastVisit(root, prior)

	tree, err := store.Load(root)
	require.NoError(t, err)
	eventPath, eventData, err := events.File(tree, events.Event{
		ID:          "EV-stable1",
		Type:        "start.feature",
		At:          now.Add(-time.Hour),
		Contributor: "jc",
		Command:     "start feature stability/work",
		Targets:     []string{"F-stable1"},
	})
	require.NoError(t, err)
	eventRel, err := store.RepositoryRelativePath(tree.Root, eventPath)
	require.NoError(t, err)
	require.NoError(t, store.WriteRepositoryFileAtomic(tree.Root, eventRel, eventData))

	visits := &visitState{now: func() time.Time { return now }}
	stale := recapHandlerWithEvents(root, visits, func(tree *store.Tree) ([]events.Event, error) {
		evs, listErr := events.List(tree)
		generation := filepath.Join(tiller, ".cache", "generation")
		require.NoError(t, os.WriteFile(generation, []byte("next-generation"), 0o600))
		return evs, listErr
	})
	recorder := httptest.NewRecorder()
	stale.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/recap", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, prior, readLastVisit(root))

	freshVisits := &visitState{now: func() time.Time { return now }}
	recorder = httptest.NewRecorder()
	recapHandler(root, freshVisits).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/recap", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "start feature stability/work")
}
