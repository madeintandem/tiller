package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

const twoOwnerManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
  - { id: pat, name: Pat Nguyen }
`

func writeTwoOwnerRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"), []byte(twoOwnerManifest), 0o644))
	gitInitM(t, dir)
	gitCommitAllM(t, dir, "init")
	return dir
}

func claimDeps() mutate.Deps {
	d := testDeps()
	seqs := map[string]int{}
	d.Mint = func(prefix string) string { seqs[prefix]++; return fmt.Sprintf("%s-%06d", prefix, seqs[prefix]) }
	return d
}

func depsAs(deps mutate.Deps, owner string) mutate.Deps { deps.Contributor = owner; return deps }

func layClaimFeature(t *testing.T, deps mutate.Deps) (root, ref string) {
	t.Helper()
	root = writeTwoOwnerRepo(t)
	_, err := mutate.AddEpic(load(t, root), depsAs(deps, "jc"), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), depsAs(deps, "jc"), mutate.FeatureInput{Key: "form-engine/nrca-form-loading", Title: "NRCA form loading", ChangeType: "create"})
	require.NoError(t, err)
	return root, "form-engine/nrca-form-loading"
}

func addTaskNamed(t *testing.T, root string, deps mutate.Deps, localKey string) string {
	t.Helper()
	res, err := mutate.AddTask(load(t, root), depsAs(deps, "jc"), mutate.TaskInput{Key: "form-engine/nrca-form-loading/" + localKey, Title: localKey})
	require.NoError(t, err)
	return res.ID
}

func requireBlockedNotUsage(t *testing.T, err error) {
	t.Helper()
	_, hasExitCode := err.(interface{ ExitCode() int })
	require.False(t, hasExitCode, "a blocked answer must not implement ExitCode: %v", err)
}

func lastEvent(t *testing.T, root, typ string) events.Event {
	t.Helper()
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var found *events.Event
	for i := range evs {
		if evs[i].Type == typ {
			found = &evs[i]
		}
	}
	require.NotNil(t, found, "no %s event recorded", typ)
	return *found
}

func TestTaskReservationChangesOnlyNamedTask(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root, _ := layClaimFeature(t, deps)
	first, sibling := addTaskNamed(t, root, deps, "first"), addTaskNamed(t, root, deps, "sibling")
	_, err := mutate.ClaimTask(load(t, root), deps, first, "")
	require.NoError(t, err)
	tree := load(t, root)
	require.Equal(t, "jc", tree.Tasks[first].Owner)
	require.Empty(t, tree.Tasks[sibling].Owner)
	require.False(t, tree.Tasks[first].Started)
	require.Empty(t, tree.Contributors["F-000001"])
	ev := lastEvent(t, root, "claim.task")
	require.Equal(t, []string{first}, ev.Targets)
	_, err = mutate.ReassignTask(tree, deps, first, "pat", "human handoff")
	require.NoError(t, err)
	require.Equal(t, "pat", load(t, root).Tasks[first].Owner)
	require.Empty(t, load(t, root).Tasks[sibling].Owner)
	_, err = mutate.ReleaseTask(load(t, root), depsAs(deps, "pat"), first)
	require.NoError(t, err)
	require.Empty(t, load(t, root).Tasks[first].Owner)
}

func TestTaskClaimHolderDisciplineAndIdempotency(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root, _ := layClaimFeature(t, deps)
	id := addTaskNamed(t, root, deps, "first")
	_, err := mutate.ClaimTask(load(t, root), deps, id, "")
	require.NoError(t, err)
	before := snapshotDir(t, root)
	again, err := mutate.ClaimTask(load(t, root), deps, id, "")
	require.NoError(t, err)
	require.False(t, again.Changed)
	requireNoWrites(t, before, snapshotDir(t, root))
	_, err = mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), id, "")
	var held *mutate.ClaimHeldError
	require.ErrorAs(t, err, &held)
	require.Equal(t, "jc", held.Holder)
	_, err = mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), id, "pat")
	var mismatch *mutate.ClaimHolderMismatchError
	require.ErrorAs(t, err, &mismatch)
	_, err = mutate.ReleaseTask(load(t, root), depsAs(deps, "pat"), id)
	var notClaimant *mutate.NotClaimantError
	require.ErrorAs(t, err, &notClaimant)
	requireNoWrites(t, before, snapshotDir(t, root))
	taken, err := mutate.ClaimTask(load(t, root), depsAs(deps, "pat"), id, "jc")
	require.NoError(t, err)
	require.True(t, taken.Takeover)
	require.Equal(t, "pat", load(t, root).Tasks[id].Owner)
}

func TestTaskReassignValidatesRecipientAndReasonBeforeWrites(t *testing.T) {
	t.Parallel()
	deps := claimDeps()
	root, _ := layClaimFeature(t, deps)
	id := addTaskNamed(t, root, deps, "first")
	for _, c := range []struct{ to, reason string }{{"", "handoff"}, {"pat", " "}, {"unknown", "handoff"}} {
		before := snapshotDir(t, root)
		_, err := mutate.ReassignTask(load(t, root), deps, id, c.to, c.reason)
		require.Error(t, err)
		requireNoWrites(t, before, snapshotDir(t, root))
	}
	_, err := mutate.ClaimTask(load(t, root), depsAs(deps, "unknown"), id, "")
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Empty(t, load(t, root).Tasks[id].Owner)
}
