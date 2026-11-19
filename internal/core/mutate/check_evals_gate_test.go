package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// These tests pin how check finds the derived evals gate: by its manifest key,
// `evals`, never by reading the gate's command text. The key is what every
// other reader already uses (attest, eval, submit, the readout), so a gate is
// the derived one exactly when it is declared under that key, and any other
// gate is an ordinary command whatever its command happens to say.

// extraGateManifestTmpl carries passing build and tests gates plus whatever
// further required gate lines a test supplies.
const extraGateManifestTmpl = `schema_version: 16
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
gates:
  required:
    build: { kind: command, run: "true", on: [feature, task] }
    tests: { kind: command, run: "true", on: [feature, task] }
%s`

// layExtraGateTask lays a repo whose manifest carries the given extra gate
// lines and drives it to one started task.
func layExtraGateTask(t *testing.T, gateLines string) (root, taskID string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	writeManifest(t, root, fmt.Sprintf(extraGateManifestTmpl, gateLines))
	gitInitM(t, root)
	gitCommitAllM(t, root, "init")
	return root, driveGatedTask(t, root)
}

// TestCheckDerivesTheEvalsGateWhateverItsCommandSays covers the seeded command
// and two other spellings of the gate declared under the `evals` key. Each
// must be derived from the eval contract inside check and never shelled: a
// shelled `tiller eval` is a second mutation of the node check is about to
// write, which leaves the outer check stale.
//
// Serial: t.Setenv puts a stand-in tiller first on PATH for the whole process.
func TestCheckDerivesTheEvalsGateWhateverItsCommandSays(t *testing.T) {
	// The stand-in records every invocation in the gate's working directory, so
	// a shelled evals gate is observed directly and no installed tiller runs.
	shim := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(shim, "tiller"),
		[]byte("#!/bin/sh\necho \"$*\" >> tiller-was-shelled\n"), 0o755))
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))

	for name, run := range map[string]string{
		"seeded command":    "tiller eval ${node.noun} ${node.id}",
		"global flag first": "tiller --json eval ${node.noun} ${node.id}",
		"flags and spacing": "tiller  --contributor jc   eval ${node.noun} ${node.id}",
	} {
		t.Run(name, func(t *testing.T) {
			root, taskID := layExtraGateTask(t,
				fmt.Sprintf("    evals: { kind: command, run: %q, on: [feature, task] }\n", run))

			tree := load(t, root)
			task := resolveNode(t, tree, store.NounTask, taskID)
			p := &recordingCheckProgress{}
			res, err := mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{Progress: p})
			require.NoError(t, err)

			require.NoFileExists(t, filepath.Join(root, "tiller-was-shelled"),
				"the evals gate is derived inside check, never run as a child command")
			require.ElementsMatch(t, []string{"build", "tests", "eval-C1"}, p.started,
				"check shells the command gates and the contract's own criterion, nothing else")

			var evals []mutate.GateOutcome
			for _, g := range res.Gates {
				if g.Name == "evals" {
					evals = append(evals, g)
				}
			}
			require.Len(t, evals, 1, "one evals outcome, the derived one")
			require.Equal(t, store.GateStatusPass, evals[0].Status)
			require.True(t, res.Passed)

			got := load(t, root).Tasks[taskID]
			require.Equal(t, "check", got.Status)
			recorded := got.GatesBlock.Gates["evals"]
			require.Equal(t, store.GateStatusPass, recorded.Status)
			require.Equal(t, store.GateStatusPass, recorded.Criteria["C1"].Status,
				"the recorded gate carries the criteria it is derived from")
			require.Empty(t, recorded.Evidence, "a derived gate has no command output of its own")
		})
	}
}

// TestCheckRunsACommandGateThatMentionsTillerEval is the other direction: a
// gate under any other key is a command gate even when its command contains
// the words `tiller eval`. It runs, its result is recorded under its own name,
// `check --gate` knows it, and it does not conjure an evals gate on a grain
// the manifest scopes evals away from.
func TestCheckRunsACommandGateThatMentionsTillerEval(t *testing.T) {
	t.Parallel()
	root, taskID := layExtraGateTask(t,
		"    docs: { kind: command, run: \"echo 'tiller eval is documented' > docs-gate-ran\", on: [feature, task] }\n"+
			"    evals: { kind: command, run: \"tiller eval ${node.noun} ${node.id}\", on: [feature] }\n")

	marker := filepath.Join(root, "docs-gate-ran")
	tree := load(t, root)
	task := resolveNode(t, tree, store.NounTask, taskID)
	res, err := mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{})
	require.NoError(t, err)

	require.FileExists(t, marker, "the command gate actually ran")
	require.True(t, res.Passed, "nothing but the three passing command gates applies to the task")
	var names []string
	for _, g := range res.Gates {
		names = append(names, g.Name)
	}
	require.ElementsMatch(t, []string{"build", "docs", "tests"}, names)

	got := load(t, root).Tasks[taskID].GatesBlock.Gates
	require.Equal(t, store.GateStatusPass, got["docs"].Status)
	require.NotEmpty(t, got["docs"].Evidence, "a command gate records its output as evidence")
	require.NotContains(t, got, "evals", "evals is scoped to the feature and is not recorded on the task")

	require.NoError(t, os.Remove(marker))
	tree = load(t, root)
	task = resolveNode(t, tree, store.NounTask, taskID)
	_, err = mutate.Check(tree, testDeps(), stubRev(root), task, mutate.CheckInput{Gates: []string{"docs"}, Rerun: true})
	require.NoError(t, err, "check --gate names the gate by its own key")
	require.FileExists(t, marker, "the named gate ran again")
}
