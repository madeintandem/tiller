package cli

// The multi-engineer arc keeps reservations separate from permanent contributorship:
// task claims do not cascade, start freezes the named owner, and historical
// contribution survives cancellation and roster removal.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// lifecycleFeatureEval is a minimal, valid feature.eval.md body: this
// package's own copy of the fixture lifecycle_cmds_test.go's writeRealSpec
// keeps under a different node id, the same keep-each-file's-own-copy
// convention singlePathRef's comment documents, since the two files address
// different feature keys.
const lifecycleFeatureEval = `---
node: F-aaaaaa
criteria:
  - id: C1
    statement: The feature behaves as specified
    polarity: introduce
    binding: { type: test, ref: spec/lifecycle_spec.rb }
---

# Acceptance
`

// initLifecycleGitRepo scaffolds a fresh Tiller project through the real
// `tiller init`, then turns the checkout into a real git repo on a branch
// named "main": the manifest's recorded integration branch defaults to
// "main" (init.go's detectIntegrationBranch) whenever no git repo exists yet
// at init time, and `tiller sync` later in this arc resolves "into" from
// that recorded default, so the actual branch has to match it. approve and
// spec also need a resolvable HEAD to pin their accepted basis against
// (mirrors e2e_contributor_registration_test.go's own bootstrap).
func initLifecycleGitRepo(t *testing.T) string {
	t.Helper()
	dir := initRepo(t)
	testgit.Init(t, dir)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")
	return dir
}

// writeLifecycleDesignDoc satisfies readiness's design/ non-empty gap: a
// bare `tiller init` leaves .tiller/design/ empty (init_test.go's own
// "plain init leaves design/ empty" pin), so a real project's readiness
// stays red until a human adds something under it.
func writeLifecycleDesignDoc(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller", "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "design", "notes.md"),
		[]byte("# Billing\n\nBilling epic notes for the contributor lifecycle arc.\n"), 0o644))
}

// lifecycleFeatureDir is a feature's folder under the working directory
// (store's own epics/<epic>/features/<feature> layout), the address this
// file's spec-writing helpers write into directly, the same shape
// lifecycle_cmds_test.go's featureDir keeps for its own fixed feature.
func lifecycleFeatureDir(epicKey, featureKey string) string {
	return filepath.Join(".tiller", "epics", epicKey, "features", featureKey)
}

// writeLifecycleFeatureSpec writes a non-stub feature.md and a valid
// feature.eval.md, the pair `spec feature` validates before it will move a
// feature draft->specced.
func writeLifecycleFeatureSpec(t *testing.T, epicKey, featureKey, title string) {
	t.Helper()
	dir := lifecycleFeatureDir(epicKey, featureKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte(fmt.Sprintf("# %s\n\n%s is specified for the contributor lifecycle arc.\n", title, title)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(lifecycleFeatureEval), 0o644))
}

// writeLifecycleTaskSpec overwrites a task's auto-generated stub task.md
// with real prose, the non-stub state `spec feature` requires of every task
// under the feature it is validating.
func writeLifecycleTaskSpec(t *testing.T, epicKey, featureKey, taskKey, body string) {
	t.Helper()
	path := filepath.Join(lifecycleFeatureDir(epicKey, featureKey), store.TaskDirName, taskKey, store.TaskProseFile)
	require.NoError(t, os.WriteFile(path, []byte("# "+taskKey+"\n\n"+body+"\n"), 0o644))
}

// eventsByType returns every event of typ recorded on the working
// directory's tree, in stream order (events.List's own timestamp-then-id
// order).
func eventsByType(t *testing.T, typ string) []events.Event {
	t.Helper()
	tree := loadCwdTree(t)
	evs, err := events.List(tree)
	require.NoError(t, err)
	var out []events.Event
	for _, e := range evs {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

// Exercise independent task claims, activation, immutable contributorship, and roster
// holdings through the same command surface developers use.
func TestEndToEndContributorLifecycleMultiEngineerArc(t *testing.T) {
	dir := initLifecycleGitRepo(t)
	writeLifecycleDesignDoc(t, dir)
	registerContributor(t, "jc", "JC Grubbs")
	registerContributor(t, "sam", "Sam Rivera")
	registerContributor(t, "dana", "Dana Ochoa")
	mustRun(t, "join", "--as", "jc")
	mustRun(t, "add", "epic", "billing", "--title", "Billing", "--contributor", "dana")
	mustRun(t, "add", "feature", "billing/invoicing", "--title", "Invoicing", "--contributor", "dana")
	for _, key := range []string{"write-model", "wire-api"} {
		mustRun(t, "add", "task", "billing/invoicing/"+key, "--title", key, "--contributor", "dana")
		writeLifecycleTaskSpec(t, "billing", "invoicing", key, "Implement "+key+".")
	}
	writeLifecycleFeatureSpec(t, "billing", "invoicing", "Invoicing")
	mustRun(t, "approve", "epic", "billing", "--contributor", "jc")
	mustRun(t, "spec", "feature", "billing/invoicing", "--contributor", "jc")
	mustRun(t, "approve", "feature", "billing/invoicing", "--contributor", "jc")
	const model = "billing/invoicing/write-model"
	const api = "billing/invoicing/wire-api"
	modelID := taskIDByKey(t, model)
	apiID := taskIDByKey(t, api)
	featureID := featureIDByKey(t, "billing/invoicing")
	tree := loadCwdTree(t)
	require.Empty(t, tree.Tasks[modelID].Owner)
	require.Empty(t, tree.Tasks[apiID].Owner)
	require.Empty(t, tree.Contributors[featureID])

	claimTaskAsOwner(t, model, "sam")
	out, _, code := execCLI(t, "claim", "task", model, "--contributor", "dana", "--json")
	require.Equal(t, 1, code, out)
	require.Equal(t, "claim_held", jsonData(t, out)["reason"])
	mustRun(t, "claim", "task", model, "--contributor", "dana", "--from", "sam")
	tree = loadCwdTree(t)
	require.Equal(t, "dana", tree.Tasks[modelID].Owner)
	require.Empty(t, tree.Tasks[apiID].Owner, "taking one reservation must not claim its sibling")
	require.Empty(t, tree.Contributors[featureID], "reservations are not contributions")

	mustRun(t, "start", "task", model, "--contributor", "dana")
	tree = loadCwdTree(t)
	require.Equal(t, "active", tree.Features[featureID].Status)
	require.True(t, tree.Tasks[modelID].Started)
	require.Equal(t, []store.Contributor{{ID: "dana", Name: "Dana Ochoa"}}, tree.Contributors[featureID])
	for _, args := range [][]string{
		{"claim", "task", model, "--contributor", "sam", "--from", "dana"},
		{"release", "task", model, "--contributor", "dana"},
		{"reassign", "task", model, "--to", "sam", "--reason", "handoff", "--contributor", "jc"},
	} {
		out, stderr, code := execCLI(t, args...)
		require.Equal(t, 1, code, out+stderr)
		require.Equal(t, "dana", loadCwdTree(t).Tasks[modelID].Owner)
	}

	// The second task remains independently reservable, including release and
	// explicit human reassignment, until its own start.
	claimTaskAsOwner(t, api, "dana")
	out, stderr, code := execCLI(t, "reassign", "task", api, "--to", "sam",
		"--reason", "Sam implements the API", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	require.Equal(t, "dana", jsonData(t, out)["from"])
	require.Equal(t, "sam", jsonData(t, out)["to"])
	mustRun(t, "release", "task", api, "--contributor", "sam")
	require.Empty(t, loadCwdTree(t).Tasks[apiID].Owner)
	claimTaskAsOwner(t, api, "sam")
	mustRun(t, "start", "task", api, "--contributor", "sam")
	tree = loadCwdTree(t)
	require.Equal(t, "dana", tree.Tasks[modelID].Owner)
	require.Equal(t, "sam", tree.Tasks[apiID].Owner)
	require.NotEqual(t, tree.Tasks[modelID].Branch, tree.Tasks[apiID].Branch)
	require.Equal(t, []store.Contributor{{ID: "dana", Name: "Dana Ochoa"}, {ID: "sam", Name: "Sam Rivera"}}, tree.Contributors[featureID])

	// Removing a contributor with unfinished task work refuses, but cancellation
	// leaves immutable history rather than a live roster holding.
	out, _, code = execCLI(t, "remove", "contributor", "dana", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Equal(t, float64(1), jsonData(t, out)["tasks"])
	mustRun(t, "cancel", "task", model, "--reason", "scope retired", "--contributor", "jc")
	mustRun(t, "remove", "contributor", "dana", "--contributor", "jc")
	tree = loadCwdTree(t)
	require.Equal(t, "dana", tree.Tasks[modelID].Owner)
	require.True(t, tree.Tasks[modelID].Started)
	require.Equal(t, []store.Contributor{{ID: "dana", Name: "dana"}, {ID: "sam", Name: "Sam Rivera"}}, tree.Contributors[featureID])
	for _, typ := range []string{"claim.task", "release.task", "reassign.task", "contributor.add", "contributor.remove"} {
		require.NotEmpty(t, eventsByType(t, typ), "event stream carries %s", typ)
	}
}
