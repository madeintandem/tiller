package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// These tests pin the on: scoping rules the full-arc dry run broke (plan
// 2026-07-02, Workstream 1 items 1.1 and 1.2): the evals gate is recorded only
// on the grains the manifest's on: list names, and a recorded gate the manifest
// no longer applies to a noun is dropped on the next check, so a rescope plus
// re-check unblocks the node.

// featureScopedManifestTmpl scopes the evals gate to features only: the shape a
// rescoped project uses when the acceptance contract gates the feature and tasks
// gate on build and tests alone.
const featureScopedManifestTmpl = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
gates:
  required:
    build: { kind: command, run: "%s", on: [feature, task] }
    tests: { kind: command, run: "%s", on: [feature, task] }
    evals: { kind: command, run: "tiller eval ${node.noun} ${node.id}", on: [feature] }
`

// writeManifest (over)writes the repo's tiller.yml, the rescope move a human
// makes mid-project.
func writeManifest(t *testing.T, root, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(body), 0o644))
}

// TestCheckTaskHonorsEvalsOnScoping is item 1.1: with the manifest's evals gate
// scoped on: [feature], a fresh task's check records NO evals gate and computes
// passed from the applicable gates only, even though the parent feature carries
// a feature.eval.md the derived-evals path could grade.
func TestCheckTaskHonorsEvalsOnScoping(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	writeManifest(t, dir, fmt.Sprintf(featureScopedManifestTmpl, "true", "true"))
	// driveGatedTask approves a feature and a task, both of which now pin a
	// commit anchor: HEAD has to resolve before that walk starts.
	gitInitM(t, dir)
	gitCommitAllM(t, dir, "init")
	taskID := driveGatedTask(t, dir)

	tree := load(t, dir)
	task := resolveNode(t, tree, store.NounTask, taskID)
	res, err := mutate.Check(tree, testDeps(), stubRev(dir), task, mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed, "passed considers only the gates applicable to a task")
	for _, g := range res.Gates {
		require.NotEqual(t, "evals", g.Name, "no evals outcome on a grain the manifest scopes evals away from")
	}

	got := load(t, dir).Tasks[taskID].GatesBlock.Gates
	require.Contains(t, got, "build")
	require.Contains(t, got, "tests")
	require.NotContains(t, got, "evals", "a task records no evals gate under evals on: [feature]")
}

// TestCheckFeatureStillDerivesEvalsWhenScoped is the regression guard around
// item 1.1: where the evals gate IS applicable (check feature under on:
// [feature]), the derived-from-criteria behavior is unchanged, so check never
// clobbers what tiller eval and attest recorded.
func TestCheckFeatureStillDerivesEvalsWhenScoped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	writeManifest(t, dir, fmt.Sprintf(featureScopedManifestTmpl, "true", "true"))
	// driveGatedTask approves a feature and a task, both of which now pin a
	// commit anchor: HEAD has to resolve before that walk starts.
	gitInitM(t, dir)
	gitCommitAllM(t, dir, "init")
	driveGatedTask(t, dir)
	gitCommitAllM(t, dir, "lay tree")

	tree := load(t, dir)
	feat := resolveNode(t, tree, store.NounFeature, "form-engine/nrca-form-loading")
	res, err := mutate.Check(tree, testDeps(), vcs.New(dir), feat, mutate.CheckInput{})
	require.NoError(t, err)

	var sawEvals bool
	for _, g := range res.Gates {
		if g.Name == "evals" {
			sawEvals = true
		}
	}
	require.True(t, sawEvals, "the evals outcome is reported where the manifest applies it")

	got := load(t, dir).Features["F-aaaaaa"].GatesBlock.Gates
	require.Contains(t, got, "evals")
	require.NotEmpty(t, got["evals"].Criteria, "the evals gate stays derived from the recorded criteria")
}

// TestCheckDropsGateRecordAfterRescope is item 1.2: a recorded evals gate on a
// task is dropped by the next check once the manifest rescopes evals to
// on: [feature], so the rescope plus a re-check unblocks the task and no pinned
// required: true outlives the config. Every record the re-check leaves behind
// shares one against baseline.
func TestCheckDropsGateRecordAfterRescope(t *testing.T) {
	t.Parallel()
	root, taskID := layGatedTask(t, "true", "true")

	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)
	_, err := mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	require.Contains(t, load(t, root).Tasks[taskID].GatesBlock.Gates, "evals",
		"under on: [feature, task] the task records an evals gate")

	// The human rescopes the manifest: the contract now gates the feature only.
	writeManifest(t, root, fmt.Sprintf(featureScopedManifestTmpl, "true", "true"))

	tree = load(t, root)
	task = resolveNode(t, tree, store.NounTask, taskID)
	res, err := mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{})
	require.NoError(t, err)
	require.True(t, res.Passed, "the re-check computes passed from the applicable gates only")

	got := load(t, root).Tasks[taskID].GatesBlock.Gates
	require.NotContains(t, got, "evals", "the stale evals record follows the manifest and is dropped")
	require.Contains(t, got, "build")
	require.Contains(t, got, "tests")

	baseline := got["build"].Against.Value
	for name, g := range got {
		require.Equal(t, baseline, g.Against.Value, "gate %s shares the run's one against baseline", name)
	}
}

// TestCheckKeepsAttestationGateRecord pins the merge rule's other half: a gate
// the manifest does not know, an attestation-backed review gate recorded by
// tiller attest, is preserved across a check rather than dropped.
func TestCheckKeepsAttestationGateRecord(t *testing.T) {
	t.Parallel()
	root, taskID := layGatedTask(t, "true", "true")

	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)
	_, err := mutate.Attest(tree, testDeps(), task, mutate.AttestInput{
		Gate: "design-review", Status: store.GateStatusPass,
	})
	require.NoError(t, err)

	tree = load(t, root)
	task = resolveNode(t, tree, store.NounTask, taskID)
	_, err = mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{})
	require.NoError(t, err)

	got := load(t, root).Tasks[taskID].GatesBlock.Gates
	require.Contains(t, got, "design-review", "an attested review gate outlives a check")
	require.Equal(t, store.GateKindAttestation, got["design-review"].Kind)
	require.Equal(t, store.GateStatusPass, got["design-review"].Status)
}
