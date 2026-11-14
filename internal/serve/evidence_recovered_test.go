package serve_test

// These tests pin the evidence endpoint's scope over retained recovery
// history. A recovered review's policy snapshots keep the evidence refs the
// historical gates recorded, and a crossing copies them onto the node's
// source-pinned policy violations. The console links both (RecoveredReview and
// IntegrityNotes), so a ref the ledger keeps only there must serve even after
// the current gate that first recorded it has moved on, while a file that
// merely sits beside it in an evidence directory must not.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const (
	recoveredSnapshotRef  = "artifacts/tiller/gates/build-sha256-0123456789ab.txt"
	recoveredUnsafeRef    = "artifacts/tiller/gates/../../../.tiller/identity.yml"
	recoveredAliasRef     = "artifacts/tiller/gates/alias.txt"
	recoveredUnrecordedAt = "artifacts/tiller/gates/unrecorded-sha256-000000000000.txt"
)

// recoveredEvidenceServer lays the recovered feature fixture with the
// snapshot's one observation recording refs, hands the record to the named
// aggregate owner, and returns a server over the real console Handler. The
// current gates record no evidence at all, so a ref is reachable here only
// through the retained snapshot.
func recoveredEvidenceServer(t *testing.T, owner string, refs ...string) (*httptest.Server, string) {
	t.Helper()
	root, feature, save, featurePath := recoveredFeatureFixture(t)
	feature.PolicyViolations = nil
	feature.RecoveredReview.PolicySnapshots[1].Observations[0].EvidenceRefs = refs
	if owner == store.NounEpic {
		tree, err := store.Load(root)
		require.NoError(t, err)
		var epic *store.Epic
		for _, candidate := range tree.Epics {
			if candidate.Key != nil && *candidate.Key == "form-engine" {
				epic = candidate
			}
		}
		require.NotNil(t, epic)
		epic.ReviewGrain, epic.Branch = store.ReviewGrainEpic, feature.Branch
		epic.ReviewRef, epic.RecoveredReview = feature.ReviewRef, feature.RecoveredReview
		feature.ReviewGrain, feature.ReviewRef, feature.RecoveredReview = "", nil, nil
		save(".tiller/epics/form-engine/epic.yml", epic)
	}
	save(featurePath, feature)
	tree, err := store.Load(root)
	require.NoError(t, err)
	for _, f := range tree.Features {
		require.Equal(t, owner == store.NounFeature && f.ID == feature.ID, f.RecoveredReview != nil)
	}
	for _, e := range tree.Epics {
		require.Equal(t, owner == store.NounEpic, e.RecoveredReview != nil)
	}

	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Tiller</title>")}}
	srv := httptest.NewServer(serve.Handler(root, assets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)
	return srv, root
}

func getEvidence(t *testing.T, srv *httptest.Server, ref string) (int, string) {
	t.Helper()
	resp, err := srv.Client().Get(evidenceURL(srv, ref))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// TestEvidenceEndpointServesRecoveredSnapshotEvidence reads, for each
// aggregate owner, an artifact the tree records only in a recovered review's
// policy snapshot. The same fixture proves the record widens nothing: an
// unrecorded neighbour stays a 404, a snapshot-recorded ref that climbs into a
// dot-prefixed path stays a 400, and a snapshot-recorded symlink stays a 400.
func TestEvidenceEndpointServesRecoveredSnapshotEvidence(t *testing.T) {
	for _, owner := range []string{store.NounFeature, store.NounEpic} {
		t.Run(owner, func(t *testing.T) {
			srv, root := recoveredEvidenceServer(t, owner, recoveredUnsafeRef, recoveredAliasRef, recoveredSnapshotRef)
			writeFixtureFile(t, root, recoveredSnapshotRef, []byte("historical build output\n"))
			writeFixtureFile(t, root, recoveredUnrecordedAt, []byte("super-secret-log\n"))
			writeFixtureFile(t, root, ".tiller/identity.yml", []byte("owner: super-secret\n"))
			writeFixtureFile(t, root, "config/master.key", []byte("super-secret-key\n"))
			require.NoError(t, os.Symlink("../../../config/master.key", filepath.Join(root, filepath.FromSlash(recoveredAliasRef))))

			status, body := getEvidence(t, srv, recoveredSnapshotRef)
			require.Equal(t, http.StatusOK, status, body)
			require.Equal(t, "historical build output\n", body)

			for _, refused := range []struct {
				name   string
				ref    string
				status int
			}{
				{"unrecorded neighbour", recoveredUnrecordedAt, http.StatusNotFound},
				{"unrecorded checkout file", "config/master.key", http.StatusNotFound},
				{"recorded traversal into a hidden path", recoveredUnsafeRef, http.StatusBadRequest},
				{"recorded symlink", recoveredAliasRef, http.StatusBadRequest},
			} {
				t.Run(refused.name, func(t *testing.T) {
					status, body := getEvidence(t, srv, refused.ref)
					require.Equal(t, refused.status, status, body)
					require.NotContains(t, body, "super-secret")
				})
			}
		})
	}
}

// TestEvidenceEndpointServesHistoricalViolationEvidence covers the record a
// recovered crossing leaves behind once the review record itself is consumed:
// the source-pinned policy violation keeps the snapshot's evidence refs as a
// list, and the node page links each one.
func TestEvidenceEndpointServesHistoricalViolationEvidence(t *testing.T) {
	root, feature, save, featurePath := recoveredFeatureFixture(t)
	feature.PolicyViolations[0].EvidenceRefs = []string{recoveredSnapshotRef}
	save(featurePath, feature)
	writeFixtureFile(t, root, recoveredSnapshotRef, []byte("historical build output\n"))
	writeFixtureFile(t, root, recoveredUnrecordedAt, []byte("super-secret-log\n"))
	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Tiller</title>")}}
	srv := httptest.NewServer(serve.Handler(root, assets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)

	status, body := getEvidence(t, srv, recoveredSnapshotRef)
	require.Equal(t, http.StatusOK, status, body)
	require.Equal(t, "historical build output\n", body)

	status, body = getEvidence(t, srv, recoveredUnrecordedAt)
	require.Equal(t, http.StatusNotFound, status, body)
	require.NotContains(t, body, "super-secret")
}
