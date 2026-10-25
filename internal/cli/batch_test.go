package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// Tests for the batch verbs (issue #14): multi-node approve and approve task
// --feature. The contract under test: resolution is
// all-or-nothing (any bad address is exit 2 and NOTHING mutates), application is
// per node in argument order (a blocked node does not stop the rest), exit 0
// only when every node succeeded, exit 1 when any node was blocked. A
// single-node invocation keeps today's envelope byte for byte; a multi-node
// invocation (or --feature) emits data.results plus data.summary.

// batchEnvelope is the parsed shape of a batch --json envelope.
type batchEnvelope struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Data    struct {
		Results []map[string]any `json:"results"`
		Summary struct {
			Total     int `json:"total"`
			Changed   int `json:"changed"`
			Unchanged int `json:"unchanged"`
			Blocked   int `json:"blocked"`
		} `json:"summary"`
	} `json:"data"`
}

func parseBatch(t *testing.T, out string) batchEnvelope {
	t.Helper()
	var env batchEnvelope
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	return env
}

// loadTestTree loads the current temp repo's tree.
func loadTestTree(t *testing.T) *store.Tree {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	return tree
}

// TestApproveMultipleEpics is the headline batch move: three draft epics
// approved in one invocation, one event per node, every entry in data.results.
func TestApproveMultipleEpics(t *testing.T) {
	layReadTree(t)
	execCLI(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	execCLI(t, "add", "epic", "reports", "--title", "Reports", "--contributor", "jc")
	eventsBefore := countEvents(t)

	out, _, code := execCLI(t, "approve", "epic", "form-engine", "payments", "reports", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)

	env := parseBatch(t, out)
	require.True(t, env.OK)
	require.Equal(t, "approve", env.Command)
	require.Len(t, env.Data.Results, 3)
	require.Equal(t, 3, env.Data.Summary.Total)
	require.Equal(t, 3, env.Data.Summary.Changed)
	require.Equal(t, 0, env.Data.Summary.Blocked)

	// Argument order, not map order.
	require.Equal(t, "form-engine", env.Data.Results[0]["key"])
	require.Equal(t, "payments", env.Data.Results[1]["key"])
	require.Equal(t, "reports", env.Data.Results[2]["key"])

	// One event per node.
	require.Equal(t, eventsBefore+3, countEvents(t))

	// All three moved on disk.
	for _, e := range loadTestTree(t).Epics {
		require.Equal(t, "ready", e.Status)
	}
}

// TestApproveBatchMixedUnchanged confirms the idempotent no-change semantics
// hold per node: an already-ready epic is an unchanged success, not a block, so
// the batch exits 0 with per-node changed flags.
func TestApproveBatchMixedUnchanged(t *testing.T) {
	layReadTree(t)
	execCLI(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")

	out, _, code := execCLI(t, "approve", "epic", "form-engine", "payments", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)

	env := parseBatch(t, out)
	require.True(t, env.OK)
	require.Equal(t, 2, env.Data.Summary.Total)
	require.Equal(t, 1, env.Data.Summary.Changed)
	require.Equal(t, 1, env.Data.Summary.Unchanged)
	require.Equal(t, false, env.Data.Results[0]["changed"])
	require.Equal(t, true, env.Data.Results[1]["changed"])
}

// TestApproveBatchCarriesEachPlanGatesTasks pins the batch envelope over the
// plan gate: a multi-node feature approve keeps data.results plus data.summary,
// and each entry carries the same fields the single-node block carries, the
// tasks that gate moved included (cli-surface.md, "Output and exit codes": a
// batch entry holds the single-node fields plus its own ok).
func TestApproveBatchCarriesEachPlanGatesTasks(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "add", "feature", "form-engine/checkout-flow",
		"--title", "Checkout flow", "--change-type", "create", "--contributor", "jc")
	require.NoError(t, os.WriteFile(
		filepath.Join(featureDir(t), "..", "checkout-flow", "feature.md"),
		[]byte("# Checkout flow\n\nReal spec prose for the second feature.\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(featureDir(t), "..", "checkout-flow", "feature.eval.md"),
		[]byte("---\nnode: F-bbbbbb\ncriteria:\n  - id: C1\n    statement: Checkout works\n    polarity: introduce\n    binding: { type: test, ref: spec/checkout_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
	_, _, code := execCLI(t, "add", "task", "form-engine/checkout-flow/wire-the-cart",
		"--title", "Wire the cart", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NoError(t, os.WriteFile(
		filepath.Join(featureDir(t), "..", "checkout-flow", store.TaskDirName, "wire-the-cart", store.TaskProseFile),
		[]byte("# Wire the cart\n\n## Approach\n\nWire it.\n"), 0o644))

	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	for _, key := range []string{"form-engine/nrca-form-loading", "form-engine/checkout-flow"} {
		_, _, code := execCLI(t, "spec", "feature", key, "--contributor", "jc")
		require.Equal(t, 0, code)
	}

	out, _, code := execCLI(t, "approve", "feature",
		"form-engine/nrca-form-loading", "form-engine/checkout-flow", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	env := parseBatch(t, out)
	require.True(t, env.OK)
	require.Equal(t, 2, env.Data.Summary.Total)
	require.Equal(t, 2, env.Data.Summary.Changed)
	for i, r := range env.Data.Results {
		require.Equal(t, "ready", r["status"], "entry %d", i)
		tasks, ok := r["tasks"].([]any)
		require.True(t, ok, "entry %d carries the tasks its gate moved", i)
		require.Len(t, tasks, 1)
		require.Equal(t, "ready", tasks[0].(map[string]any)["status"])
	}
	for _, tk := range loadTestTree(t).Tasks {
		require.Equal(t, "ready", tk.Status, "every plan's tasks moved with its feature")
		require.NotNil(t, tk.AcceptedAgainst)
	}
}

// TestApproveBatchBlockedExits1 confirms a blocked node does not stop the rest:
// the draft feature (illegal approve) is reported per node, the specced feature
// still transitions, and the whole batch exits 1.
func TestApproveBatchBlockedExits1(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "add", "feature", "form-engine/checkout-flow",
		"--title", "Checkout flow", "--change-type", "create", "--contributor", "jc")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")

	// The blocked (still draft) feature comes FIRST, proving application continues.
	out, _, code := execCLI(t, "approve", "feature",
		"form-engine/checkout-flow", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "any blocked node makes the batch exit 1")

	env := parseBatch(t, out)
	require.False(t, env.OK)
	require.Equal(t, 2, env.Data.Summary.Total)
	require.Equal(t, 1, env.Data.Summary.Changed)
	require.Equal(t, 1, env.Data.Summary.Blocked)

	require.Equal(t, false, env.Data.Results[0]["ok"])
	require.Equal(t, "illegal_transition", env.Data.Results[0]["reason"])
	require.Equal(t, "draft", env.Data.Results[0]["current_state"])
	require.Equal(t, true, env.Data.Results[1]["ok"])
	require.Equal(t, "ready", env.Data.Results[1]["status"])

	// The specced feature transitioned despite the earlier block.
	tree := loadTestTree(t)
	byKey := map[string]string{}
	for _, f := range tree.Features {
		byKey[*f.Key] = f.Status
	}
	require.Equal(t, "ready", byKey["form-engine/nrca-form-loading"])
	require.Equal(t, "draft", byKey["form-engine/checkout-flow"])
}

// TestApproveBatchBogusIDMutatesNothing is the all-or-nothing resolution rule:
// a bad address anywhere in the list is exit 2 misuse and NOTHING mutates, even
// the nodes before it that would have succeeded.
func TestApproveBatchBogusIDMutatesNothing(t *testing.T) {
	layReadTree(t)
	eventsBefore := countEvents(t)

	out, _, code := execCLI(t, "approve", "epic", "form-engine", "E-zzzzzz", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, "a resolution failure anywhere is misuse")
	require.Contains(t, out, "\"error\"")
	require.Contains(t, out, "\"reason\": \"not_found\"")

	// The first (resolvable) epic did not transition, and no event was written.
	for _, e := range loadTestTree(t).Epics {
		require.Equal(t, "draft", e.Status)
	}
	require.Equal(t, eventsBefore, countEvents(t))
}

// TestApproveBatchHumanSummary confirms the human output: per-node confirmation
// lines plus one summary line when the outcomes are mixed. Each node's full
// line is asserted with its real minted id read off disk, binding every ref to
// its own outcome, so swapped results (payments' outcome printed against
// form-engine's ref) cannot pass.
func TestApproveBatchHumanSummary(t *testing.T) {
	layReadTree(t)
	execCLI(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")

	stdout, _, code := execCLI(t, "approve", "epic", "form-engine", "payments", "--contributor", "jc")
	require.Equal(t, 0, code)

	// The minted ids are random, so read them off disk by key.
	epicIDByKey := map[string]string{}
	for id, e := range loadTestTree(t).Epics {
		require.NotNil(t, e.Key)
		epicIDByKey[*e.Key] = id
	}
	require.Contains(t, stdout, fmt.Sprintf("epic form-engine (%s) is already ready (no change).", epicIDByKey["form-engine"]))
	require.Contains(t, stdout, fmt.Sprintf("epic payments (%s) is now ready.", epicIDByKey["payments"]))
	require.Contains(t, stdout, "2 total, 1 changed, 1 unchanged, 0 blocked")
}

// TestApproveBatchBlockedHumanLines is the human-mode sibling of
// TestApproveBatchBlockedExits1: a mixed batch prints one line per node, the
// blocked line bound by ref to the draft feature and the success line bound by
// ref to the specced one, plus the mixed-outcome summary. Batch lines all go
// to stdout via emitBatch, but the assertion reads stdout+stderr so it holds
// even if a blocked line ever moves to the single-node stderr stream.
func TestApproveBatchBlockedHumanLines(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "add", "feature", "form-engine/checkout-flow",
		"--title", "Checkout flow", "--change-type", "create", "--contributor", "jc")
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")

	// The blocked (still draft) feature comes FIRST, proving application continues.
	stdout, stderr, code := execCLI(t, "approve", "feature",
		"form-engine/checkout-flow", "form-engine/nrca-form-loading", "--contributor", "jc")
	require.Equal(t, 1, code, "any blocked node makes the batch exit 1")

	// The minted ids are random, so read them off disk by key.
	featIDByKey := map[string]string{}
	for id, f := range loadTestTree(t).Features {
		require.NotNil(t, f.Key)
		featIDByKey[*f.Key] = id
	}
	out := stdout + stderr
	require.Contains(t, out, fmt.Sprintf("Blocked: cannot approve feature form-engine/checkout-flow (%s) from %q.",
		featIDByKey["form-engine/checkout-flow"], "draft"))
	require.Contains(t, out, fmt.Sprintf("feature form-engine/nrca-form-loading (%s) is now ready.",
		featIDByKey["form-engine/nrca-form-loading"]))
	require.Contains(t, out, "Summary: 2 total, 1 changed, 0 unchanged, 1 blocked.")
}

// prepActiveFeatureWithTasks starts the initial task, then appends three
// proposed tasks in ledger (add) order.
func prepActiveFeatureWithTasks(t *testing.T) []string {
	t.Helper()
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	_, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	var ids []string
	// The fixture's own task rode the plan gate to ready already; only the three
	// added here are the mid-flight proposed set this helper is about.
	seen := map[string]bool{}
	for id := range loadTestTree(t).Tasks {
		seen[id] = true
	}
	for _, tk := range []struct{ local, title string }{
		{"schema-transcription", "Transcribe the schema"},
		{"form-render", "Render the form"},
		{"validation", "Wire validation"},
	} {
		_, _, code := execCLI(t, "add", "task", "form-engine/nrca-form-loading/"+tk.local, "--title", tk.title, "--contributor", "jc")
		require.Equal(t, 0, code)
		for id := range loadTestTree(t).Tasks {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	require.Len(t, ids, 3)
	return ids
}

// TestApproveTaskByFeature confirms --feature approves exactly the proposed set
// in ledger order, and a second run is the zero-set success.
func TestApproveTaskByFeature(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	// Approve the first task singly; the batch must cover only the remaining two.
	_, _, code := execCLI(t, "approve", "task", ids[0], "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "approve", "task", "--feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	env := parseBatch(t, out)
	require.True(t, env.OK)
	require.Len(t, env.Data.Results, 2)
	require.Equal(t, ids[1], env.Data.Results[0]["id"], "ledger order")
	require.Equal(t, ids[2], env.Data.Results[1]["id"], "ledger order")
	require.Equal(t, 2, env.Data.Summary.Changed)

	tree := loadTestTree(t)
	for _, id := range ids {
		require.Equal(t, "ready", tree.Tasks[id].Status)
	}
	require.Equal(t, "in_progress", tree.Tasks[taskIDByKey(t, cliTaskKey)].Status,
		"approving proposed work must not reset the already-started task")

	// Zero proposed tasks is not an error. The feature is named by the reference
	// convention; its minted id is random, so the assertion brackets it.
	stdout, _, code := execCLI(t, "approve", "task", "--feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, stdout, "No proposed tasks on feature form-engine/nrca-form-loading (F-")

	out, _, code = execCLI(t, "approve", "task", "--feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	env = parseBatch(t, out)
	require.True(t, env.OK)
	require.NotNil(t, env.Data.Results)
	require.Len(t, env.Data.Results, 0)
	require.Equal(t, 0, env.Data.Summary.Total)
}

// TestApproveTaskByFeatureZeroArmRefusesCorruptIdentity pins issue #58: the
// zero-proposed-tasks arm of `approve task --feature` used to return its
// "nothing to do" success before deps() ever ran, so a corrupt
// .tiller/identity.yml was silently skipped on that arm alone while every
// other approve path, including this same command while tasks are still
// proposed, refuses identity_unreadable first. With the feature's proposed
// set already exhausted, no --contributor, and no TILLER_CONTRIBUTOR, the zero-tasks arm
// must refuse the same way rather than reporting the empty-batch success.
func TestApproveTaskByFeatureZeroArmRefusesCorruptIdentity(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	out, _, code := execCLI(t, "approve", "task", "--feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	env := parseBatch(t, out)
	require.Equal(t, len(ids), env.Data.Summary.Changed, "every proposed task approved, leaving zero proposed")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	corruptIdentityFile(t, cwd)
	t.Setenv("TILLER_CONTRIBUTOR", "")

	out, _, code = execCLI(t, "approve", "task", "--feature", "form-engine/nrca-form-loading", "--json")
	require.Equal(t, 3, code, "a corrupt identity file refuses even on the zero-proposed-tasks arm")
	require.Contains(t, out, store.ReasonIdentityUnreadable)
}

// TestApproveTaskByFeatureFollowsDependencyOrder pins the argument order the
// spec names for this batch arm: `approve task --feature` applies a feature's
// proposed tasks in dependency order (cli-surface.md, "The high-repetition
// verb batches"), so the per-node results read dependency-first. The three
// tasks record a chain whose ranked order (schema-transcription, validation,
// form-render) is the reverse of nothing in particular but is decidedly not
// their key order (form-render, schema-transcription, validation), which is
// the order the folders are read in.
func TestApproveTaskByFeatureFollowsDependencyOrder(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/nrca-form-loading", "--contributor", "jc")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	_, stderr, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	base := "form-engine/nrca-form-loading/"
	for _, tk := range []struct{ local, follows string }{
		{"schema-transcription", ""},
		{"validation", "schema-transcription"},
		{"form-render", "validation"},
	} {
		args := []string{"add", "task", base + tk.local, "--title", tk.local, "--contributor", "jc"}
		if tk.follows != "" {
			args = append(args, "--depends-on", base+tk.follows)
		}
		_, _, code := execCLI(t, args...)
		require.Equal(t, 0, code, "add task %s", tk.local)
	}

	out, _, code := execCLI(t, "approve", "task", "--feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	env := parseBatch(t, out)
	require.True(t, env.OK)

	keys := make([]string, 0, len(env.Data.Results))
	for _, r := range env.Data.Results {
		keys = append(keys, r["key"].(string))
	}
	require.Equal(t, []string{
		base + "schema-transcription",
		base + "validation",
		base + "form-render",
	}, keys, "the batch applies the chain in order, not in folder order")
}

// TestApproveFeatureFlagMisuse pins the --feature misuse cases: mixing the flag
// with positional nodes, and using it with a noun other than task.
func TestApproveFeatureFlagMisuse(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)

	_, _, code := execCLI(t, "approve", "task", ids[0], "--feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, "--feature is mutually exclusive with positional nodes")

	_, _, code = execCLI(t, "approve", "epic", "--feature", "form-engine/nrca-form-loading", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, "--feature applies to approve task only")

	// Nothing mutated by either misuse: the three mid-flight tasks are still
	// proposed. The fixture's own task rode the plan gate to ready before them,
	// so the assertion names the set it is about rather than every task there is.
	for _, id := range ids {
		require.Equal(t, "proposed", loadTestTree(t).Tasks[id].Status)
	}
}

// countEvents counts the event files under .tiller/events/.
func countEvents(t *testing.T) int {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	count := 0
	_ = filepath.WalkDir(filepath.Join(cwd, ".tiller", "events"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".yml" {
			count++
		}
		return nil
	})
	return count
}
