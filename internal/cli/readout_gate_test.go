package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// epicDirFor returns epicKey's on-disk folder under the current fixture's
// .tiller/epics, named here once for the fixtures that lay an epic's prose by
// hand rather than through the add-stub.
func epicDirFor(t *testing.T, epicKey string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(cwd, ".tiller", "epics", epicKey)
}

// writeFile writes name under dir with the given content, the small helper
// epicDirFor's caller uses to lay real epic.md prose directly rather than
// leaving the add-stub in place.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// This file pins the contract: the human gates (approve,
// answer question, attest, sync) print their existing confirmation
// unchanged, then the moment's readout block; every other command (add, spec,
// start feature/task, return, resume, cancel, archive, submit) is untouched.
// The brief's own three tests
// (TestApproveEmitsTheReadoutAfterItsConfirmation,
// TestAddFeatureEmitsNoReadout,
// TestApproveJSONCarriesTheReadoutAndKeepsItsExistingFields) are reproduced
// here adapted to this repo's actual fixtures (layReadTree, laySpeccableFeature,
// execCLI: the brief's scratchProjectWithDraftEpics and runCLI do not exist
// verbatim, the same substitution readout_test.go already made for Task 5).
// The remaining gates each get their own test, since the brief scoped its
// worked example to approve alone but its Step 4 is explicit that every one
// of them needs its expectations proven, not just pinned by inference from
// one.

// TestApproveEmitsTheReadoutAfterItsConfirmation is the brief's own Step 1
// test: the per-node confirmation is the record and stays, and the block
// follows it, never replaces it.
func TestApproveEmitsTheReadoutAfterItsConfirmation(t *testing.T) {
	layReadTree(t)
	out, _, code := execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)

	require.Contains(t, out, "is now ready.")
	require.Contains(t, out, "**Approval recorded**")
	requireNeedsYouSection(t, out)
	require.Less(t, strings.Index(out, "is now ready."), strings.Index(out, "**Approval recorded**"))
}

// TestAddFeatureEmitsNoReadout confirms a non-gate command carries no block:
// a decompose runs roughly eighty adds, and a block on each would bury the
// signal (design doc, "Settled decisions": "Seam boundaries and human gates,
// never every command").
func TestAddFeatureEmitsNoReadout(t *testing.T) {
	layReadTree(t)
	out, _, code := execCLI(t, "add", "feature", "form-engine/checkout-flow",
		"--title", "Checkout flow", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	// A decompose runs ~80 adds; a block on each would bury the signal.
	requireNoReadoutBlock(t, out)
}

// TestApproveJSONCarriesTheReadoutAndKeepsItsExistingFields confirms the
// readout field rides additively under --json: every existing field
// (status among them) is untouched, and readout is new. It also pins the
// same additive treatment for the acting-contributor "actor" field (design doc,
// "The fallback speaks"): approve is a gate (emitReportWithReadout), so this
// is the representative pin that the readout and actor map-merges compose
// rather than one replacing the other.
func TestApproveJSONCarriesTheReadoutAndKeepsItsExistingFields(t *testing.T) {
	layReadTree(t)
	raw, _, code := execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	data := env["data"].(map[string]any)
	require.Equal(t, "ready", data["status"], "existing fields are untouched")
	require.NotEmpty(t, data["readout"], "the block rides additively")
	actorField, ok := data["actor"].(map[string]any)
	require.True(t, ok, "the actor field rides additively alongside readout")
	require.Equal(t, "jc", actorField["id"])
	require.Equal(t, "--contributor", actorField["source"])
}

// TestApproveBatchEmitsOneReadoutAfterAllConfirmations confirms the batch
// shape the design doc calls out explicitly: "on a batch approve those
// lines are the per-node record of what transitioned, and a summary cannot
// replace them." Every node's own confirmation line stays, and the block
// prints exactly once, after all of them and the mixed-outcome summary,
// never once per node.
func TestApproveBatchEmitsOneReadoutAfterAllConfirmations(t *testing.T) {
	layReadTree(t)
	execCLI(t, "add", "epic", "payments", "--title", "Payments", "--contributor", "jc")

	out, _, code := execCLI(t, "approve", "epic", "form-engine", "payments", "--contributor", "jc")
	require.Equal(t, 0, code)

	require.Contains(t, out, "epic form-engine")
	require.Contains(t, out, "epic payments")
	require.Equal(t, 1, strings.Count(out, "**Approval recorded**"),
		"one block after the whole batch, not one per node")
	last := strings.LastIndex(out, "epic payments")
	require.Less(t, last, strings.Index(out, "**Approval recorded**"),
		"both per-node lines precede the block")
}

// Starting a task activates its feature without adding a human gate readout.
func TestStartTaskEmitsNoReadout(t *testing.T) {
	laySpeccableFeature(t)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")

	claimTaskAsOwner(t, cliFeatureKey+"/transcribe-the-schema", "jc")
	out, _, code := execCLI(t, "start", "task", cliFeatureKey+"/transcribe-the-schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	// Start is the agent's own move, not a gate.
	requireNoReadoutBlock(t, out)
}

// TestNoAcceptCommandRemains pins the verb's removal at the command surface,
// the one place a stale muscle-memory invocation would land: `tiller accept`
// is not a registered command at all, so cobra answers with its unknown-command
// error rather than a gate readout (artifact-schema.md, "Done above a task is
// computed, and accept is gone"). Epic done arrives through sync, whose own
// readout is pinned by TestSyncEmitsTheReadout below.
func TestNoAcceptCommandRemains(t *testing.T) {
	initGatedGitRepo(t, "true")
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")

	out, _, code := execCLI(t, "accept", "epic", "form-engine", "--contributor", "jc")
	require.NotEqual(t, 0, code, "accept is gone; it must not quietly succeed")
	require.NotContains(t, out, "**Acceptance recorded**")
	require.NotContains(t, out, "is now done.")

	// The refused invocation left the epic exactly where approve put it.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Len(t, tree.Epics, 1)
	for _, e := range tree.Epics {
		require.Equal(t, "ready", e.Status)
	}
}

// TestAnswerQuestionEmitsTheReadout drives the question ledger's happy path
// (question_test.go's own TestQuestionBlocksArcEndToEnd fixture, trimmed)
// to a successful answer and confirms the confirmation is followed by the
// block.
func TestAnswerQuestionEmitsTheReadout(t *testing.T) {
	ids := prepActiveFeatureWithTasks(t)
	gitifyCwd(t)

	out, _, code := execCLI(t, "add", "question", "Which schema version is canonical?",
		"--title", "Canonical schema version", "--blocks", ids[0], "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	qid := extractID(t, out, "Q-")

	out, _, code = execCLI(t, "answer", "question", qid, "--answer", "The 2025 revision.", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "is now answered.")
	require.Contains(t, out, "**Question answered**")
	requireNeedsYouSection(t, out)
	require.Less(t, strings.Index(out, "is now answered."), strings.Index(out, "**Question answered**"))
}

// TestAttestEmitsTheReadout drives attest_test.go's own recordC2 fixture (a
// needs_attestation criterion) to a passing attest and confirms the
// confirmation is followed by the block.
func TestAttestEmitsTheReadout(t *testing.T) {
	key := recordC2(t)

	out, _, code := execCLI(t, "attest", "feature", key, "--criterion", "C2", "--status", "pass", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Attested")
	require.Contains(t, out, "**Attestation recorded**")
	requireNeedsYouSection(t, out)
	require.Less(t, strings.Index(out, "Attested"), strings.Index(out, "**Attestation recorded**"))
}

// TestSyncEmitsTheReadout drives a task through review and merges its feature's
// branch, so sync records real crossings, and confirms the confirmation is
// followed by the block.
//
// The fixture used to submit the FEATURE, which is a task-only verb now (a PR
// always represents a task), so the run crossed nothing and the test passed on a
// headline that named crossings there were none of
// (task-9-findings-round-1.md, small item 3). The zero-crossing headline has its
// own test below.
func TestSyncEmitsTheReadout(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	_, _, code := execCLI(t, "add", "task", cliTaskKey, "--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := taskIDByKey(t, cliTaskKey)
	for _, argv := range [][]string{
		{"approve", "task", tid, "--contributor", "jc"},
		{"start", "task", tid, "--contributor", "jc"},
		{"check", "task", tid, "--contributor", "jc"},
		{"submit", "task", tid, "--pr", "41", "--contributor", "jc"},
		{"check", "feature", cliFeatureKey, "--contributor", "jc"},
	} {
		_, _, code = execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
		if argv[0] == "start" {
			checkoutTaskBranchCLI(t, tid)
		}
	}
	mergeTaskBranchCLI(t, dir, loadTestTree(t).Tasks[tid].Branch)

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Sync: recorded")
	require.Contains(t, out, "**Crossings recorded**")
	requireNeedsYouSection(t, out)
}

// TestSyncReadoutDoesNotClaimCrossingsItDidNotRecord pins the headline against
// the ordinary case: sync runs on every serve refresh and on demand, so most
// runs observe nothing, and "Crossings recorded" printed directly under "no
// merged branches to record" is the block contradicting the line above it
// (task-9-findings-round-1.md, small item 3).
func TestSyncReadoutDoesNotClaimCrossingsItDidNotRecord(t *testing.T) {
	initGatedGitRepo(t, "true")
	driveFeatureToReadyCLI(t)

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "no merged branches to record")
	require.NotContains(t, out, "**Crossings recorded**")
	require.Contains(t, out, "**No crossings to record**")
}

// TestEmitReportWithReadoutOmitsTheBlockOnALoadFailure pins the brief's own
// rule directly: "A readout that fails to derive must be logged and omitted
// rather than failing the command, since the gate already succeeded, and
// turning a reporting problem into a transition failure would be worse than
// a missing block." Round-1 review found this path behaved correctly but
// carried no test at all: the reviewer wrote a throwaway one, confirmed it,
// and deleted it. root here is an empty directory with no .tiller at all,
// so renderReadoutFor's own store.Load fails the way a real gate's post-
// mutation reload could (a tree that stops loading between the write and
// the report, however unlikely); emitReportWithReadout must still return a
// nil error, print the original confirmation unchanged, print no block, and
// log the failure rather than swallow it.
func TestEmitReportWithReadoutOmitsTheBlockOnALoadFailure(t *testing.T) {
	root := t.TempDir()
	cmd := &cobra.Command{}
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)

	err := emitReportWithReadout(cmd, "approve", true, map[string]any{"status": "ready"},
		"epic x is now ready.", 0, views.MomentApprove, root, nil)

	require.NoError(t, err, "a readout that fails to derive must not fail the command")
	require.Contains(t, out.String(), "epic x is now ready.", "the original confirmation still prints")
	require.NotContains(t, out.String(), "**Approval recorded**", "no block when it failed to derive")
	requireNoReadoutBlock(t, out.String())
	require.Contains(t, errb.String(), "readout for approve not available", "the failure is logged, not silently dropped")
}
