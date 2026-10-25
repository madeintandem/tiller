package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// This file tests the five amendment verbs (amendment_cmds.go): envelope
// contracts, usage refusals (exit 2), the blocked stale-preconditions answer
// (exit 1), show's LIVE validity rendering, and ls/status support. It mirrors
// the rest of internal/cli's test conventions: execCLI drives the real cobra
// tree, assertions read the pretty-printed JSON by substring the way
// lifecycle_cmds_test.go does throughout, and a real git repo backs every
// fixture (initOwnedRepo) because AddAmendment pins base_revision from HEAD
// and approve reads git for spec_revision/eval_revision content hashes.

// layAmendmentTree lays a one-epic, one-feature, two-task tree in a real git
// repo, the shape the amendment verbs are exercised against. The feature
// stays in draft (unapproved): a fresh draft is what most staged operations
// target, and the one test that needs a different state drives it there
// itself.
func layAmendmentTree(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/nrca-form-loading",
		"--title", "NRCA form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", "form-engine/nrca-form-loading/schema-transcription",
		"--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", "form-engine/nrca-form-loading/schema-review",
		"--title", "Review the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
}

// openAmendmentCLI opens a proposed amendment through the CLI and returns its
// minted A-id, read back off disk (the same way soleFeatureID reads a minted
// F-id) so a test can address it without parsing the id out of stdout.
func openAmendmentCLI(t *testing.T, key, title string) string {
	t.Helper()
	_, stderr, code := execCLI(t, "add", "amendment", key, "--title", title, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	tree := loadCwdTree(t)
	for id, a := range tree.Amendments {
		if a.Key != nil && *a.Key == key {
			return id
		}
	}
	t.Fatalf("amendment %q not found after add", key)
	return ""
}

// jsonData unmarshals an envelope's data block into a map for field-by-field
// assertions, the same shape TestApproveJSONCarriesTheReadoutAndKeepsItsExistingFields
// (readout_gate_test.go) reads.
func jsonData(t *testing.T, raw string) map[string]any {
	t.Helper()
	var env map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	data, ok := env["data"].(map[string]any)
	require.True(t, ok, "envelope carries a data block: %s", raw)
	return data
}

// soleTaskIDByKey reads a task's minted id off disk by its canonical key,
// the address a fixture with more than one task (layAmendmentTree lays two,
// for supersede/cancel-with-replacement tests) needs rather than
// soleTaskID's single-task assumption.
func soleTaskIDByKey(t *testing.T, key string) string {
	t.Helper()
	tree := loadCwdTree(t)
	for id, tk := range tree.Tasks {
		if tk.Key != nil && *tk.Key == key {
			return id
		}
	}
	t.Fatalf("task %q not found", key)
	return ""
}

// vanishTargetOnDisk hand-rewrites a staged operation's target to an id the
// tree does not carry, the shape only a hand edit (or a partially applied
// batch) can produce; staging itself always resolves a target before
// writing it (mirrors mutate_test's writeAmendmentOps).
func vanishTargetOnDisk(t *testing.T, aid string) {
	t.Helper()
	tree := loadCwdTree(t)
	a := tree.Amendments[aid]
	require.NotNil(t, a)
	path := filepath.Join(".tiller", "amendments", a.ID+"-"+*a.Key+".yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Amendment
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.Operations)
	doc.Operations[0].Target = "F-vanished"
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// setAmendmentSourceOnDisk hand-lays a source onto an amendment's yml,
// manufacturing the intake-minted shape (doc.Source != nil) nothing in this
// build writes yet (intake, a later task, is the one writer): the same
// disk-level poke vanishTargetOnDisk uses to reach a state only a hand edit
// can produce.
func setAmendmentSourceOnDisk(t *testing.T, aid, path, revision string) {
	t.Helper()
	tree := loadCwdTree(t)
	a := tree.Amendments[aid]
	require.NotNil(t, a)
	ymlPath := filepath.Join(".tiller", "amendments", a.ID+"-"+*a.Key+".yml")
	data, err := os.ReadFile(ymlPath)
	require.NoError(t, err)
	var doc store.Amendment
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Source = &store.AmendmentSource{Path: path, Revision: revision}
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(ymlPath, out, 0o644))
}

// setOpSourceOnDisk hand-writes one staged operation's source to value, the
// shape only a hand edit can produce for anything but a non-blank string:
// StageOperation itself refuses a blank --source on an intake-minted
// amendment, so a blank or whitespace-only value reaches a state staging
// never permits, the same disk-level poke vanishTargetOnDisk and
// setAmendmentSourceOnDisk above use (mirrors mutate_test's own
// writeAmendmentOps).
func setOpSourceOnDisk(t *testing.T, aid string, index int, value string) {
	t.Helper()
	tree := loadCwdTree(t)
	a := tree.Amendments[aid]
	require.NotNil(t, a)
	path := filepath.Join(".tiller", "amendments", a.ID+"-"+*a.Key+".yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Amendment
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Greater(t, len(doc.Operations), index)
	doc.Operations[index].Source = value
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// blankOpSourceOnDisk is setOpSourceOnDisk's own empty-string case, kept as
// a named helper since it is the common one every existing op-source test
// here reaches for.
func blankOpSourceOnDisk(t *testing.T, aid string, index int) {
	t.Helper()
	setOpSourceOnDisk(t, aid, index, "")
}

// appendAmendmentOpOnDisk hand-appends one more operation to an amendment's
// yml, the shape only a hand edit can produce for an op staging itself
// would refuse (a supersede's --by or a cancel's --superseded-by naming a
// staged EPIC key refuses unknown_target at stage time,
// resolveStageReplacement's own deliberate epic exclusion, amendment.go) —
// the same disk-level poke the other helpers above use, appending rather
// than rewriting so operations staged legitimately before it survive
// untouched (mirrors mutate_test's own writeAmendmentOps, which replaces the
// whole list instead; this package's tests build most of a batch through
// the real CLI first, so appending is the one that fits here).
func appendAmendmentOpOnDisk(t *testing.T, aid string, op store.AmendmentOp) {
	t.Helper()
	tree := loadCwdTree(t)
	a := tree.Amendments[aid]
	require.NotNil(t, a)
	path := filepath.Join(".tiller", "amendments", a.ID+"-"+*a.Key+".yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Amendment
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Operations = append(doc.Operations, op)
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func TestAmendMoveFeatureStagesShowsAndApprovesIdentityPreservingMove(t *testing.T) {
	layAmendmentTree(t)
	featureID := soleFeatureID(t)
	before := loadCwdTree(t)
	_, stderr, code := execCLI(t, "add", "epic", "back-office", "--title", "Back office", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	aid := openAmendmentCLI(t, "move-loading", "Move loading")
	out, stderr, code := execCLI(t, "amend", aid, "--move-feature", "form-engine/nrca-form-loading",
		"--to", "back-office/loading", "--reason", "Keep oversight together", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
	op := ops[0].(map[string]any)
	require.Equal(t, "move-feature", op["op"])
	require.Equal(t, "back-office/loading", op["key"])
	require.Equal(t, featureID, op["target"].(map[string]any)["id"])
	require.Equal(t, "Keep oversight together", op["reason"])
	require.IsType(t, map[string]any{}, op["move"])
	move := op["move"].(map[string]any)
	require.Equal(t, "form-engine/nrca-form-loading", move["source_key"])
	require.Equal(t, "back-office", move["destination_epic_key"])
	require.Equal(t, "append", move["order_effect"])
	require.Equal(t, "form-engine/nrca-form-loading", *loadCwdTree(t).Features[featureID].Key,
		"staging must leave the feature at its original address")
	out, stderr, code = execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, stderr)
	shown := jsonData(t, out)["operations"].([]any)[0].(map[string]any)
	require.Equal(t, op["move"], shown["move"])
	out, stderr, code = execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "form-engine/nrca-form-loading")
	require.Contains(t, out, "back-office/loading")
	require.Contains(t, out, "Keep oversight together")
	out, stderr, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	applied := jsonData(t, out)["applied"].([]any)[0].(map[string]any)
	require.Equal(t, featureID, applied["id"])
	require.Equal(t, "back-office/loading", applied["key"])
	require.Equal(t, op["move"], applied["move"])
	after := loadCwdTree(t)
	require.Equal(t, "back-office/loading", *after.Features[featureID].Key)
	for id, task := range before.Tasks {
		require.Equal(t, strings.Replace(*task.Key, "form-engine/nrca-form-loading/", "back-office/loading/", 1), *after.Tasks[id].Key)
		require.Equal(t, task.Status, after.Tasks[id].Status)
	}
}

func TestAmendMoveFeatureShowsStaleReviewedSnapshot(t *testing.T) {
	layAmendmentTree(t)
	featureID := soleFeatureID(t)
	_, stderr, code := execCLI(t, "add", "epic", "back-office", "--title", "Back office", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	aid := openAmendmentCLI(t, "move-validity", "Move validity")
	out, stderr, code := execCLI(t, "amend", aid, "--move-feature", featureID,
		"--to", "back-office/loading", "--reason", "Keep oversight together", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	showStatus := func(want string) {
		t.Helper()
		out, stderr, code := execCLI(t, "show", "amendment", aid, "--json")
		require.Equal(t, 0, code, out+stderr)
		op := jsonData(t, out)["operations"].([]any)[0].(map[string]any)
		preconditions, ok := op["preconditions"].([]any)
		require.True(t, ok, "review must expose move snapshot validity")
		require.Len(t, preconditions, 1)
		pc := preconditions[0].(map[string]any)
		require.Equal(t, "move", pc["name"])
		require.Equal(t, want, pc["status"])
	}
	showStatus("valid")
	out, stderr, code = execCLI(t, "rename", "feature", featureID, "--title", "Changed after review", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	showStatus("stale")
	_, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Equal(t, "form-engine/nrca-form-loading", *loadCwdTree(t).Features[featureID].Key)
}

func TestAmendMoveFeatureRefusesInvalidFlagsWithoutStaging(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "move-flags", "Move flags")
	for _, tc := range []struct {
		name   string
		flags  []string
		reason string
	}{
		{"exclusive", []string{"--move-feature", "form-engine/nrca-form-loading", "--cancel", "form-engine/nrca-form-loading", "--to", "back-office/loading", "--reason", "Move"}, "multiple_operations"},
		{"missing destination", []string{"--move-feature", "form-engine/nrca-form-loading", "--reason", "Move"}, "to_required"},
		{"missing reason", []string{"--move-feature", "form-engine/nrca-form-loading", "--to", "back-office/loading"}, "reason_required"},
		{"blank reason", []string{"--move-feature", "form-engine/nrca-form-loading", "--to", "back-office/loading", "--reason", " "}, "reason_required"},
		{"invalid destination", []string{"--move-feature", "form-engine/nrca-form-loading", "--to", "loading", "--reason", "Move"}, "invalid_key"},
		{"misplaced destination", []string{"--revise", "form-engine/nrca-form-loading", "--to", "back-office/loading"}, "to_misplaced"},
		{"empty misplaced destination", []string{"--amends-design", "design/a.md", "--to="}, "to_misplaced"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"amend", aid, "--json", "--contributor", "jc"}, tc.flags...)
			out, stderr, code := execCLI(t, args...)
			require.Equal(t, 2, code, out+stderr)
			require.Contains(t, out, `"`+tc.reason+`"`)
			require.Empty(t, loadCwdTree(t).Amendments[aid].Operations)
		})
	}
}

// --- add amendment ---

func TestAddAmendmentEnvelopeShape(t *testing.T) {
	layAmendmentTree(t)
	out, _, code := execCLI(t, "add", "amendment", "iphone-only",
		"--title", "Remove native iPad support from v1", "--implements", "ADR-3",
		"--triggered-by", "design/04-platform-and-constraints.md", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)

	data := jsonData(t, out)
	require.Equal(t, "iphone-only", data["key"])
	require.Equal(t, "proposed", data["status"])
	require.Equal(t, "Remove native iPad support from v1", data["title"])
	require.Equal(t, "ADR-3", data["implements"])
	require.NotEmpty(t, data["id"])
	require.Equal(t, []any{}, data["operations"], "a fresh amendment opens with no operations staged")
}

// TestAddAmendmentDuplicateKeyRefused confirms a second open amendment cannot
// reuse a PROPOSED amendment's key (mutate.AddAmendment's own refusal,
// amendment_exists), the one add verb that is not idempotent on a repeat key.
func TestAddAmendmentDuplicateKeyRefused(t *testing.T) {
	layAmendmentTree(t)
	openAmendmentCLI(t, "iphone-only", "Remove native iPad support from v1")

	out, _, code := execCLI(t, "add", "amendment", "iphone-only", "--title", "second one", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "amendment_exists"`)
}

// --- amend: usage refusals ---

func TestAmendRefusesMoreThanOneOperationKind(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--cancel", taskID, "--reason", "x", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "multiple_operations"`)
}

func TestAmendRefusesNothingToStage(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "nothing_to_stage"`)
}

func TestAmendRefusesPreconditionWithoutOp(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--precondition", "target_status=done", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "precondition_without_op"`)
}

func TestAmendRefusesMalformedPrecondition(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--precondition", "no-equals-sign", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "precondition_malformed"`)
}

func TestAmendAddFeatureRefusesInvalidChangeType(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--add-feature", "form-engine/table-backed-forms",
		"--change-type", "bogus", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "invalid_change_type"`)
}

func TestAmendAddEpicRefusesInvalidKey(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "Not_A_Key", "--title", "Hardening", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "invalid_key"`)
}

func TestAmendAddTaskRequiresTitle(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/extra-task", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "title_required"`)
}

func TestAmendCancelRequiresReason(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--cancel", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "reason_required"`)
}

func TestAmendSupersedeRequiresBy(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--supersede", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "supersede_by_required"`)
}

// TestAmendSupersedeRefusesSelfSupersession confirms mutate's own refusal
// (self_supersession) surfaces at exit 2 through the CLI.
func TestAmendSupersedeRefusesSelfSupersession(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--supersede", taskID, "--by", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "self_supersession"`)
}

// --- amend: --by / --superseded-by forwarded on every arm ---
//
// buildStageInput (amendment_cmds.go) must forward By and SupersededBy on
// all five operation arms, not just the ones that use them, so a misplaced
// flag reaches mutate.buildStagedOp's own supersede_by_misplaced /
// superseded_by_misplaced refusals (amendment.go) rather than being silently
// dropped before mutate ever sees it. Before the fix, an arm that did not
// forward the field staged exit 0 with the flag ignored; these pin the
// refusal on both flags across all four arms where each is illegal.

func TestAmendByMisplacedOnRevise(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "supersede_by_misplaced"`)
}

func TestAmendByMisplacedOnCancel(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--cancel", taskID, "--reason", "x", "--by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "supersede_by_misplaced"`)
}

func TestAmendByMisplacedOnAddFeature(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--add-feature", "form-engine/table-backed-forms",
		"--by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "supersede_by_misplaced"`)
}

func TestAmendByMisplacedOnAddTask(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/extra-task",
		"--title", "Extra task", "--by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "supersede_by_misplaced"`)
}

func TestAmendSupersededByMisplacedOnRevise(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--superseded-by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "superseded_by_misplaced"`)
}

func TestAmendSupersededByMisplacedOnSupersede(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--supersede", taskID, "--by", task2,
		"--superseded-by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "superseded_by_misplaced"`)
}

func TestAmendByMisplacedOnAddEpic(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "hardening",
		"--by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "supersede_by_misplaced"`)
}

func TestAmendSupersededByMisplacedOnAddFeature(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--add-feature", "form-engine/table-backed-forms",
		"--superseded-by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "superseded_by_misplaced"`)
}

func TestAmendSupersededByMisplacedOnAddTask(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/extra-task",
		"--title", "Extra task", "--superseded-by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "superseded_by_misplaced"`)
}

func TestAmendSupersededByMisplacedOnAddEpic(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "hardening",
		"--superseded-by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "superseded_by_misplaced"`)
}

func TestAmendByMisplacedOnAddQuestion(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--add-question", "Which auth provider?",
		"--title", "Authentication provider", "--by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "supersede_by_misplaced"`)
}

func TestAmendSupersededByMisplacedOnAddQuestion(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--add-question", "Which auth provider?",
		"--title", "Authentication provider", "--superseded-by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "superseded_by_misplaced"`)
}

// TestAmendBlocksMisplacedOnRevise pins --blocks refusing loudly on any op
// but add-question, the same misplaced-field treatment --by and
// --superseded-by already get: buildStageInput now forwards Blocks on every
// arm (amendment_cmds.go), so mutate.buildStagedOp's own blocks_misplaced
// refusal (amendment.go) is actually reachable rather than the flag being
// silently dropped before mutate ever saw it.
func TestAmendBlocksMisplacedOnRevise(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--blocks", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "blocks_misplaced"`)
}

func TestAmendBlocksMisplacedOnAddEpic(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "hardening", "--blocks", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "blocks_misplaced"`)
}

func TestAmendOnATerminalAmendmentRefusesAmendmentNotOpen(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	_, _, code := execCLI(t, "withdraw", "amendment", aid, "--reason", "dropped", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "amendment_not_open"`)
}

// --- amend: happy paths ---

func TestAmendStagesAddFeatureAndAddTask(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--add-feature", "form-engine/table-backed-forms",
		"--change-type", "replace", "--title", "Table-backed forms",
		"--precondition", "key_absent=form-engine/table-backed-forms", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
	op0 := ops[0].(map[string]any)
	require.Equal(t, "add-feature", op0["op"])
	require.Equal(t, "form-engine/table-backed-forms", op0["key"])
	require.Equal(t, "replace", op0["change_type"])

	out, _, code = execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/iphone-acceptance",
		"--title", "iPhone-only acceptance", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data = jsonData(t, out)
	ops = data["operations"].([]any)
	require.Len(t, ops, 2, "the amendment now carries both staged operations")
}

// TestAmendStagesAddEpic pins add-epic's own CLI arm (task-2-brief.md): a
// bare key and title, no --change-type (epics carry none) and no parent
// resolution, unlike add-feature and add-task one grain down.
func TestAmendStagesAddEpic(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "hardening",
		"--title", "Hardening", "--source", "docs/feedback/opus-review.md#L1-9",
		"--precondition", "key_absent=hardening", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
	op0 := ops[0].(map[string]any)
	require.Equal(t, "add-epic", op0["op"])
	require.Equal(t, "hardening", op0["key"])
	require.Equal(t, "Hardening", op0["title"])
}

// TestAmendApproveMaterializesAnAddEpicWithItsSourceCitation is the CLI-level
// arc for add-epic's own acceptance criterion (task-2-brief.md): stage then
// approve lands a draft epic whose node page (show epic --json) carries the
// citation as its source line, the same generic Source rendering every epic
// already gets (views/node.go), now reachable through the amendment batch.
func TestAmendApproveMaterializesAnAddEpicWithItsSourceCitation(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "hardening-epic", "Land the hardening epic")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "hardening",
		"--title", "Hardening", "--source", "docs/feedback/opus-review.md#L1-9", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	applied, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, applied)
	appliedData := jsonData(t, applied)
	appliedOps := appliedData["applied"].([]any)
	require.Len(t, appliedOps, 1)
	op0 := appliedOps[0].(map[string]any)
	require.Equal(t, "add-epic", op0["op"])
	require.Equal(t, "draft", op0["status"])
	newEpicID := op0["id"].(string)
	require.NotEmpty(t, newEpicID)

	show, _, code := execCLI(t, "show", "epic", "hardening", "--json")
	require.Equal(t, 0, code, show)
	showData := jsonData(t, show)
	require.Equal(t, "docs/feedback/opus-review.md#L1-9", showData["source"],
		"the epic's node page shows the op's citation as its source line")
	require.Equal(t, "draft", showData["status"])

	status, _, code := execCLI(t, "status", "--json")
	require.Equal(t, 0, code, status)
	statusData := jsonData(t, status)
	require.Equal(t, []any{newEpicID}, statusData["order"],
		"the project status reload carries the applied epic ID in root order without filling older unlisted epics")
}

// TestAmendStagesAddQuestion pins add-question's CLI contract: the positional
// flag value is the full statement while shared --title is the independent,
// required one-line scanning title. JSON exposes both without overloading.
func TestAmendStagesAddQuestion(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--add-question", "Which auth provider?",
		"--title", "Authentication provider", "--blocks", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
	op0 := ops[0].(map[string]any)
	require.Equal(t, "add-question", op0["op"])
	require.Equal(t, "Authentication provider", op0["title"])
	require.Equal(t, "Which auth provider?", op0["statement"])
	require.Empty(t, op0["key"], "a question carries no key")
}

func TestAmendAddQuestionRequiresStatementAndOneLineTitle(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantReason string
	}{
		{name: "missing statement", args: []string{"--add-question", "", "--title", "Authentication provider"}, wantReason: "statement_required"},
		{name: "missing title", args: []string{"--add-question", "Which auth provider?"}, wantReason: "title_required"},
		{name: "line feed in title", args: []string{"--add-question", "Which auth provider?", "--title", "Authentication\nprovider"}, wantReason: "title_one_line"},
		{name: "carriage return in title", args: []string{"--add-question", "Which auth provider?", "--title", "Authentication\rprovider"}, wantReason: "title_one_line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layAmendmentTree(t)
			aid := openAmendmentCLI(t, "question-shape", "Question shape")
			args := append([]string{"amend", aid}, tt.args...)
			args = append(args, "--contributor", "jc", "--json")
			out, _, code := execCLI(t, args...)
			require.Equal(t, 2, code, out)
			require.Contains(t, out, `"reason": "`+tt.wantReason+`"`)
			require.Empty(t, loadCwdTree(t).Amendments[aid].Operations)
		})
	}
}

// TestAmendApproveMaterializesAnAddQuestionWithItsSourceCitation is the
// CLI-level arc for add-question's own acceptance criterion
// (task-3-brief.md): stage then approve lands the question open on the
// shared ledger, carrying the op's Source and its Blocks resolved to the
// live task's id. Read back through the loaded tree rather than `show
// question --json`: views.Node's Question case does not yet surface source
// or blocks at all (a pre-existing gap task-3 does not own; task-5's brief,
// "source honesty at show and approve", is the likelier owner).
func TestAmendApproveMaterializesAnAddQuestionWithItsSourceCitation(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "hardening-question", "Land the hardening question")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--add-question", "Keep the legacy parser?",
		"--title", "Legacy parser", "--blocks", taskID, "--source", "docs/feedback/opus-review.md#L20-24", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	applied, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, applied)
	appliedData := jsonData(t, applied)
	appliedOps := appliedData["applied"].([]any)
	require.Len(t, appliedOps, 1)
	op0 := appliedOps[0].(map[string]any)
	require.Equal(t, "add-question", op0["op"])
	require.Equal(t, "open", op0["status"])
	qid := op0["id"].(string)

	tree := loadCwdTree(t)
	q := tree.Questions[qid]
	require.NotNil(t, q)
	require.Equal(t, "Legacy parser", q.Title)
	require.Equal(t, "Keep the legacy parser?", q.Statement)
	require.Equal(t, "docs/feedback/opus-review.md#L20-24", q.Source)
	require.Equal(t, []string{taskID}, q.Blocks)
}

// TestShowAmendmentJSONCarriesOpSourceAndBlocks pins Task 5's ledger
// addendum, closed by Task 8: AmendmentOperationView needed source and
// blocks FIELDS, not just a headline arm, so `show amendment --json`
// displays what a staged question blocks and every op's citation. Blocks
// resolves through amendmentRefView, the identical rule Target/By/
// SupersededBy already use: a live target's id and key, or a staged key
// marked "staged" when it names an add op on this same amendment.
func TestShowAmendmentJSONCarriesOpSourceAndBlocks(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--add-question", "Which auth provider?",
		"--title", "Authentication provider", "--blocks", taskID, "--source", "docs/feedback/opus-review.md#L20-24", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	show, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, show)
	data := jsonData(t, show)
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
	op0 := ops[0].(map[string]any)
	require.Equal(t, "docs/feedback/opus-review.md#L20-24", op0["source"])
	blocks := op0["blocks"].([]any)
	require.Len(t, blocks, 1)
	b0 := blocks[0].(map[string]any)
	require.Equal(t, taskID, b0["id"])
}

// TestShowAmendmentJSONResolvesAnEpicBlockTarget pins a case amendmentRefView
// was never built for: store.AmendmentOp's own doc comment says Blocks names
// "an existing epic, feature, or task" (resolveStageBlockTarget's own doc
// comment: "extended to epics here since a question can block one"), unlike
// Target/By/SupersededBy, which per cli-surface.md never name an epic
// ("epics reshape through their features"). An epic block must resolve to
// its full id-and-key ref, the same as a feature or task block does, not
// degrade to the bare-id row a genuinely vanished target gets.
func TestShowAmendmentJSONResolvesAnEpicBlockTarget(t *testing.T) {
	layAmendmentTree(t)
	tree := loadCwdTree(t)
	var epicID string
	for id := range tree.Epics {
		epicID = id
	}
	require.NotEmpty(t, epicID)

	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	out, _, code := execCLI(t, "amend", aid, "--add-question", "Drop the whole epic?",
		"--title", "Whole epic", "--blocks", epicID, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	show, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, show)
	data := jsonData(t, show)
	op0 := data["operations"].([]any)[0].(map[string]any)
	blocks := op0["blocks"].([]any)
	require.Len(t, blocks, 1)
	b0 := blocks[0].(map[string]any)
	require.Equal(t, epicID, b0["id"])
	require.Equal(t, "form-engine", b0["key"],
		"an existing epic block resolves to its full key, not the bare-id row a vanished target degrades to")
}

// TestAmendApproveAddQuestionBlocksAFeatureAddedInTheSameAmendment is the
// CLI-level arc for the acceptance's heart (task-3-brief.md): a question can
// block a feature the same amendment stages.
func TestAmendApproveAddQuestionBlocksAFeatureAddedInTheSameAmendment(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "table-backed", "Table-backed forms")

	const stagedKey = "form-engine/table-backed-forms"
	out, _, code := execCLI(t, "amend", aid, "--add-feature", stagedKey,
		"--change-type", "replace", "--title", "Table-backed forms", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	out, _, code = execCLI(t, "amend", aid, "--add-question", "Which storage engine?",
		"--title", "Storage engine", "--blocks", stagedKey, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	applied, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, applied)
	appliedData := jsonData(t, applied)
	appliedOps := appliedData["applied"].([]any)
	require.Len(t, appliedOps, 2)
	newFeatureID := appliedOps[0].(map[string]any)["id"].(string)
	qid := appliedOps[1].(map[string]any)["id"].(string)

	tree := loadCwdTree(t)
	require.NotNil(t, tree.Features[newFeatureID])
	q := tree.Questions[qid]
	require.NotNil(t, q)
	require.Equal(t, []string{newFeatureID}, q.Blocks, "the staged key resolved to the materialized feature's id")
}

func TestAmendStagesSupersedeAndCancelWithReplacement(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task1 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "amend", aid, "--supersede", task1, "--by", task2, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
	op0 := ops[0].(map[string]any)
	require.Equal(t, "supersede", op0["op"])
	target := op0["target"].(map[string]any)
	require.Equal(t, task1, target["id"])
	by := op0["by"].(map[string]any)
	require.Equal(t, task2, by["id"])
}

func TestAmendAmendsDesignRidesAloneAndBesideAnOp(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--amends-design", "design/04-platform-and-constraints.md", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	require.Equal(t, []any{"design/04-platform-and-constraints.md"}, data["amends_design"])
	require.Empty(t, data["operations"].([]any), "amends-design alone stages no operation")

	out, _, code = execCLI(t, "amend", aid, "--revise", taskID,
		"--amends-design", "design/05-forms.md", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data = jsonData(t, out)
	require.Equal(t, []any{"design/04-platform-and-constraints.md", "design/05-forms.md"}, data["amends_design"])
	require.Len(t, data["operations"].([]any), 1, "the op staged alongside amends-design also lands")
}

// --- amend: --source ---

// TestAmendSourceStagesOntoTheOpAndPersists pins the interface task-1-brief.md
// pins: `tiller amend <addr> --<op> ... --source "<citation>"` stages the
// citation onto that op, and it survives a reload off disk.
func TestAmendSourceStagesOntoTheOpAndPersists(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID,
		"--source", "docs/feedback/opus-review.md#L12-18", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	tree := loadCwdTree(t)
	a := tree.Amendments[aid]
	require.Len(t, a.Operations, 1)
	require.Equal(t, "docs/feedback/opus-review.md#L12-18", a.Operations[0].Source)
}

// TestAmendRefusesSourcelessOpWhenAmendmentCarriesSource pins the staging
// rule's exact refusal text on the real CLI path: an intake-minted amendment
// (Source set, manufactured here by hand since intake itself is a later
// task) refuses an op staged with no --source.
func TestAmendRefusesSourcelessOpWhenAmendmentCarriesSource(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	setAmendmentSourceOnDisk(t, aid, ".tiller/feedback/opus-review.md", "git:9f2a1c0")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "source_required"`)
	require.Contains(t, out, "intake amendments trace every operation: pass --source citing the feedback document")

	tree := loadCwdTree(t)
	require.Empty(t, tree.Amendments[aid].Operations, "a refused stage must not write")
}

// TestAmendAllowsSourcedOpWhenAmendmentCarriesSource is the other half: an
// intake-minted amendment stages an op that DOES carry --source clean.
func TestAmendAllowsSourcedOpWhenAmendmentCarriesSource(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	setAmendmentSourceOnDisk(t, aid, ".tiller/feedback/opus-review.md", "git:9f2a1c0")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID,
		"--source", "docs/feedback/opus-review.md#L20-25", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
}

// TestAmendFlagsACitationThatWillNotResolve pins #143's staging warning:
// nothing edits a staged operation, so a --source written as a document
// citation that provenance cannot resolve is named while it is still cheap to
// restage. The operation still stages; the citation is kept as opaque text.
func TestAmendFlagsACitationThatWillNotResolve(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	citation := "docs/feedback/opus-review.md#L12-18; L40"

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--source", citation, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	op := jsonData(t, out)["operations"].([]any)[0].(map[string]any)
	require.Equal(t, true, op["source_unresolved"])

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, human)
	require.Contains(t, human, "source: not a citation Tiller can resolve (path.md#Lstart-Lend, more ranges after commas)")
}

func TestAmendHumanWarnsWhenItsCitationWillNotResolve(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--revise", taskID,
		"--source", "docs/feedback/opus-review.md#L12–18", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, `Warning: --source "docs/feedback/opus-review.md#L12–18" is not a citation Tiller can resolve`)
}

func TestAmendAcceptsSeveralRangesAndFreeTextWithoutWarning(t *testing.T) {
	for _, source := range []string{"docs/feedback/opus-review.md#L12-18, #L40", "Raised in the planning call"} {
		t.Run(source, func(t *testing.T) {
			layAmendmentTree(t)
			aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
			taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

			out, _, code := execCLI(t, "amend", aid, "--revise", taskID, "--source", source, "--contributor", "jc")
			require.Equal(t, 0, code, out)
			require.NotContains(t, out, "Warning")
		})
	}
}

// TestShowAmendmentHumanRendersAddEpicAndAddQuestionHeadlines pins
// add-question's title-led human shape with the full statement retained as
// detail, alongside the trailing source citation.
func TestShowAmendmentHumanRendersAddEpicAndAddQuestionHeadlines(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "hardening", "Land hardening work")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "hardening-epic",
		"--title", "Hardening", "--source", "docs/feedback/opus-review.md#L1-9", "--contributor", "jc")
	require.Equal(t, 0, code, out)

	out, _, code = execCLI(t, "amend", aid, "--add-question", "Which auth provider?",
		"--title", "Authentication provider", "--blocks", taskID,
		"--source", "docs/feedback/opus-review.md#L20-24", "--contributor", "jc")
	require.Equal(t, 0, code, out)

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, human)
	require.Contains(t, human, `add-epic hardening-epic: "Hardening" · docs/feedback/opus-review.md#L1-9`)
	require.Contains(t, human, `add-question Authentication provider: "Which auth provider?" · docs/feedback/opus-review.md#L20-24`)
}

// TestShowAmendmentHumanRendersSourceOnAPreExistingOpKind pins the review
// round's Important finding: renderOperationHeadline's trailing source
// suffix was widened to all seven op kinds, not only add-epic/add-question,
// but that widening was only ever pinned on the two new kinds. A sourced
// revise is the COMMON case in a real intake batch, not an edge case:
// StageOperation requires every operation on an intake-minted amendment to
// carry a Source (mutate's own sourceless-op refusal), so a revise almost
// always carries one there. This locks the exact trailing format in on
// revise, one of the five pre-existing kinds.
func TestShowAmendmentHumanRendersSourceOnAPreExistingOpKind(t *testing.T) {
	layAmendmentTree(t)
	featureID := soleFeatureID(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--revise", "form-engine/nrca-form-loading",
		"--source", "docs/feedback/opus-review.md#L12-18", "--contributor", "jc")
	require.Equal(t, 0, code, out)

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, human)
	require.Contains(t, human, fmt.Sprintf(
		"revise form-engine/nrca-form-loading (%s) · docs/feedback/opus-review.md#L12-18", featureID))
}

// TestShowAmendmentHumanOmitsCitationForAWhitespaceOnlySource pins the
// whitespace double-render fold-in carried from #57's own review: a
// hand-blanked source that is whitespace ONLY, not empty, still trips
// SourceMissing (mutate.EvaluateAmendmentOpSources runs strings.TrimSpace
// before comparing to ""), but before this fix renderOperationHeadline's own
// check was a bare op.Source != "", which is true for whitespace, so the
// headline ALSO rendered it as a trailing citation (" · " followed by
// invisible whitespace) right beside the "source: missing" line calling the
// same operation's citation absent. The fix trims before deciding whether to
// render a citation at all.
func TestShowAmendmentHumanOmitsCitationForAWhitespaceOnlySource(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	setAmendmentSourceOnDisk(t, aid, ".tiller/feedback/opus-review.md", "git:9f2a1c0")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "billing", "--title", "Billing",
		"--source", "docs/feedback/opus-review.md#L1-2", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	setOpSourceOnDisk(t, aid, 0, "   ")

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, human)
	require.Contains(t, human, "source: missing (intake amendments cite every operation)")

	var headline string
	for _, line := range strings.Split(human, "\n") {
		if strings.Contains(line, "add-epic billing") {
			headline = line
			break
		}
	}
	require.NotEmpty(t, headline, "the add-epic headline must render: %s", human)
	require.NotContains(t, headline, " · ",
		"a whitespace-only source must not still render as a trailing citation on the operation's own headline")
}

// --- show amendment: live validity ---

// TestShowAmendmentRendersLiveValidity is the fixture task-4-brief.md asks
// for: one operation carrying a precondition that holds against the live
// tree (target_status, matching the feature's actual draft status) beside
// one that does not (key_absent naming the key the feature itself already
// holds), and asserts show renders each with its own LIVE status.
func TestShowAmendmentRendersLiveValidity(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	_, _, code := execCLI(t, "amend", aid, "--revise", "form-engine/nrca-form-loading",
		"--precondition", "target_status=draft",
		"--precondition", "key_absent=form-engine/nrca-form-loading",
		"--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
	pre := ops[0].(map[string]any)["preconditions"].([]any)
	require.Len(t, pre, 2)

	byName := map[string]map[string]any{}
	for _, p := range pre {
		m := p.(map[string]any)
		byName[m["name"].(string)] = m
	}
	require.Equal(t, "valid", byName["target_status"]["status"])
	require.Equal(t, "draft", byName["target_status"]["actual"])
	require.Equal(t, "stale", byName["key_absent"]["status"])
	require.NotEmpty(t, byName["key_absent"]["note"], "a stale key_absent explains who holds the key")

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code)
	require.Contains(t, human, "[valid]")
	require.Contains(t, human, "[stale]")
}

// TestShowAmendmentRendersUnknownForAStagedKey is the third of the three LIVE
// validity states task-4-brief.md names: an add-task's own staged key is not
// a live node until approve materializes it, so a precondition that needs a
// target (anything but key_absent) cannot be compared yet. That is not a
// stale verdict, nothing has changed; it is simply not knowable until apply.
func TestShowAmendmentRendersUnknownForAStagedKey(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	_, _, code := execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/iphone-acceptance",
		"--title", "iPhone-only acceptance", "--precondition", "target_status=done", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	pre := data["operations"].([]any)[0].(map[string]any)["preconditions"].([]any)
	require.Len(t, pre, 1)
	p := pre[0].(map[string]any)
	require.Equal(t, "unknown", p["status"])
	require.Nil(t, p["actual"], "nothing to compare against exists yet")
	require.NotEmpty(t, p["note"])

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code)
	require.Contains(t, human, "[unknown]")
}

// A supersede may be staged before its target is done, but approval enforces
// done intrinsically. Without target_status, show explains that the optional
// stale guard was omitted rather than weakening the lifecycle rule.
func TestShowAmendmentSupersedeMechanismNote(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task1 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	_, _, code := execCLI(t, "amend", aid, "--supersede", task1, "--by", task2, "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	op0 := data["operations"].([]any)[0].(map[string]any)
	require.Equal(t, "approve requires the supersede target to be done even without a target_status precondition; stage target_status=done when the reviewed batch should also prove the state has not moved.", op0["note"])

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code)
	require.Contains(t, human, "note:")
}

func TestShowAmendmentSupersedeNoNoteWhenTargetStatusStaged(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	task1 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	_, _, code := execCLI(t, "amend", aid, "--supersede", task1, "--by", task2,
		"--precondition", "target_status=done", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	op0 := data["operations"].([]any)[0].(map[string]any)
	require.Nil(t, op0["note"], "staging target_status silences the mechanism note")
}

func TestShowAmendmentNotFound(t *testing.T) {
	layAmendmentTree(t)
	out, _, code := execCLI(t, "show", "amendment", "A-nonexist", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "not_found"`)
}

// TestShowAmendmentDegradesWhenTheBatchDoesNotResolve confirms show never
// hard-fails just because a staged batch has drifted out of resolving (a
// vanished target here): it is a read, and a stuck amendment is exactly what
// a human runs show to diagnose. The static shape still renders and the one
// reason live validity is unavailable is named once.
func TestShowAmendmentDegradesWhenTheBatchDoesNotResolve(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	_, _, code := execCLI(t, "amend", aid, "--revise", "form-engine/nrca-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	vanishTargetOnDisk(t, aid)

	out, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	require.Equal(t, "iphone-only", data["key"], "the static shape still renders")
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
	require.NotEmpty(t, data["validity_error"], "the one reason live validity could not run is named")

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code)
	require.Contains(t, human, "live validity unavailable")
}

// TestJoinLiveReadErrorsDedupesIdenticalMessages is the fold-in review
// finding: mutate.EvaluateAmendmentPreconditions and
// mutate.EvaluateAmendmentSource both open with the identical
// loadAmendmentDoc read (amendment_validity.go), so a doc-load failure (a
// corrupt or hand-mangled amendment yml) surfaces from BOTH evaluators as
// the exact same error text — reproducing that exact pairing through the
// CLI's own load-then-evaluate flow is not possible (loadAmendmentTree's
// own initial full-tree load would already refuse identically, before
// runShowAmendment ever reaches either evaluator), so this pins the pure
// join function directly: two error values with the same message join to
// ONE line in validity_error, not "X; X".
func TestJoinLiveReadErrorsDedupesIdenticalMessages(t *testing.T) {
	same := "mutate: A-aaaaaa-x.yml holds amendment A-bbbbbb, not the addressed A-aaaaaa"
	require.Equal(t, same, joinLiveReadErrors(errors.New(same), errors.New(same)),
		"the identical failure from both evaluators must be named once, not twice")

	require.Empty(t, joinLiveReadErrors(nil, nil), "no errors joins to nothing")
	require.Equal(t, "only preconditions failed", joinLiveReadErrors(errors.New("only preconditions failed"), nil))
	require.Equal(t, "only source failed", joinLiveReadErrors(nil, errors.New("only source failed")))
	require.Equal(t, "preconditions broke; source broke",
		joinLiveReadErrors(errors.New("preconditions broke"), errors.New("source broke")),
		"two genuinely distinct failures still join, preconditions first")
}

// --- show amendment: op-source validity (#57) ---

// TestShowAmendmentJSONRendersSourceMissingForABlankedOpSource is the
// pre-flight pin #57 asks for: a hand-blanked per-op source citation on an
// intake-minted amendment was invisible at show until approve refused
// op_source_required; now show itself flags it, in --json and in the human
// render, before approve ever runs.
func TestShowAmendmentJSONRendersSourceMissingForABlankedOpSource(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	setAmendmentSourceOnDisk(t, aid, ".tiller/feedback/opus-review.md", "git:9f2a1c0")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "billing", "--title", "Billing",
		"--source", "docs/feedback/opus-review.md#L1-2", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	blankOpSourceOnDisk(t, aid, 0)

	show, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, show)
	data := jsonData(t, show)
	op0 := data["operations"].([]any)[0].(map[string]any)
	require.Equal(t, true, op0["source_missing"],
		"a hand-blanked op source is visible at show, before approve ever refuses")

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, human)
	require.Contains(t, human, "source: missing (intake amendments cite every operation)")
}

// TestShowAmendmentJSONRendersShapeErrorForADuplicateStagedKey is the
// pre-flight pin #57 asks for, DUPLICATE STAGED KEY's own sibling to
// TestShowAmendmentJSONRendersSourceMissingForABlankedOpSource: two add-epic
// operations staging the same key was invisible at show until approve
// refused duplicate_staged_key; now show flags the SECOND occurrence
// itself, in --json and in the human render, before approve ever runs.
// Staging permits this cleanly
// (TestApproveAmendmentRefusesDuplicateStagedAddEpicKeysWithNoWrites,
// mutate_test, pins that only approve refuses it), so no hand-edit is
// needed.
func TestShowAmendmentJSONRendersShapeErrorForADuplicateStagedKey(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "hardening", "Land hardening work")

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "hardening", "--title", "Hardening one", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	out, _, code = execCLI(t, "amend", aid, "--add-epic", "hardening", "--title", "Hardening two", "--contributor", "jc")
	require.Equal(t, 0, code, out)

	show, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, show)
	data := jsonData(t, show)
	ops := data["operations"].([]any)
	op0 := ops[0].(map[string]any)
	op1 := ops[1].(map[string]any)
	require.Empty(t, op0["shape_error"], "the FIRST operation to stage the key is clean")
	require.Equal(t, "duplicate_staged_key", op1["shape_error"],
		"the second operation to stage the same key is flagged, before approve ever refuses")

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, human)
	require.Contains(t, human, "shape: duplicate staged key")
}

// TestShowAmendmentJSONRendersShapeErrorForAStagedEpicReplacement is the
// pre-flight pin #57 asks for, STAGED-EPIC REPLACEMENT's own sibling: a
// supersede naming an earlier staged add-epic key as its --by was invisible
// at show until approve refused staged_epic_replacement; now show flags it
// itself, before approve ever runs. Staging itself refuses this shape
// (resolveStageReplacement's own deliberate epic exclusion, amendment.go),
// so the batch is hand-appended the same way
// TestApproveAmendmentRefusesAStagedAddEpicKeyAsAReplacementWithNoWrites
// (mutate_test) reaches it.
func TestShowAmendmentJSONRendersShapeErrorForAStagedEpicReplacement(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "hardening", "Land hardening work")
	featureID := soleFeatureID(t)

	out, _, code := execCLI(t, "amend", aid, "--add-epic", "hardening", "--title", "Hardening", "--contributor", "jc")
	require.Equal(t, 0, code, out)

	appendAmendmentOpOnDisk(t, aid, store.AmendmentOp{
		Op: store.AmendmentOpSupersede, Target: featureID, By: "hardening",
	})

	show, _, code := execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, show)
	data := jsonData(t, show)
	op1 := data["operations"].([]any)[1].(map[string]any)
	require.Equal(t, "staged_epic_replacement", op1["shape_error"],
		"the supersede naming an earlier staged epic key is flagged, before approve ever refuses")

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, human)
	require.Contains(t, human, "shape: staged epic replacement")
}

// TestShowAmendmentRendersEveryHandEditDefectAlongsideTheResolutionFailure
// is the full pre-flight #57 asks for: an amendment carrying ALL THREE
// hand-edit defect classes (a blanked op source, a duplicate staged key, a
// staged-epic replacement) plus a vanished target renders every per-op
// verdict AND validity_error from the ONE show call. It grew from
// TestShowAmendmentRendersSourceMissingAlongsideAResolutionFailure (#57's own
// first pass, which pinned only the blanked source beside the vanished
// target); the shape defects are ADDED to that same batch rather than pinned
// in isolation, because the point is that none of the four signals silences
// any other: EvaluateAmendmentOpSources and EvaluateAmendmentShape are both
// pure over the doc (no tree, no resolveAmendment), so both keep reporting
// even though EvaluateAmendmentPreconditions' own resolution fails.
//
// Operation 3 (the cancel) is ALSO hand-blanked, on top of its own shape
// defect: SourceMissing and ShapeError are orthogonal facts an operation can
// carry AT ONCE (AmendmentOperationView's own doc comment,
// amendment_cmds.go), and this is where that claim is actually pinned rather
// than merely asserted in a comment — a single operation renders both
// verdicts from the one show call.
//
// Operation 0 (the revise) also carries a STAGED precondition
// (target_status=draft), a fold-in from #57's own review: without one
// staged there is no live column to prove blank, only validity_error's own
// non-empty string, which is a weaker pin than asserting the precondition's
// own status renders empty directly.
func TestShowAmendmentRendersEveryHandEditDefectAlongsideTheResolutionFailure(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	setAmendmentSourceOnDisk(t, aid, ".tiller/feedback/opus-review.md", "git:9f2a1c0")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	// Operation 0: a revise, later hand-blanked (source_missing) and hand-
	// vanished (the resolution failure), carrying a staged precondition so
	// its own live column has something to prove blank.
	out, _, code := execCLI(t, "amend", aid, "--revise", "form-engine/nrca-form-loading",
		"--precondition", "target_status=draft",
		"--source", "docs/feedback/opus-review.md#L1-2", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	// Operations 1 and 2: a duplicate staged key (operation 2 repeats
	// operation 1's own).
	out, _, code = execCLI(t, "amend", aid, "--add-epic", "hardening", "--title", "Hardening one",
		"--source", "docs/feedback/opus-review.md#L3-4", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	out, _, code = execCLI(t, "amend", aid, "--add-epic", "hardening", "--title", "Hardening two",
		"--source", "docs/feedback/opus-review.md#L5-6", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)

	// Operation 3: a cancel naming operation 1's staged epic key as its
	// replacement, a shape staging itself refuses, so it is hand-appended.
	// Its own Source is left blank on the same append, so this one
	// operation carries BOTH defect classes at once.
	appendAmendmentOpOnDisk(t, aid, store.AmendmentOp{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "replaced under iPad removal",
		SupersededBy: "hardening", Source: "",
	})

	blankOpSourceOnDisk(t, aid, 0)
	vanishTargetOnDisk(t, aid)

	out, _, code = execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	ops := data["operations"].([]any)
	require.Len(t, ops, 4)
	op0 := ops[0].(map[string]any)
	op1 := ops[1].(map[string]any)
	op2 := ops[2].(map[string]any)
	op3 := ops[3].(map[string]any)

	require.Equal(t, true, op0["source_missing"],
		"the op-source evaluator is doc-only and needs no resolution, so it still renders even though the batch cannot resolve")
	require.Empty(t, op0["shape_error"], "a revise is not a shape defect either predicate covers")
	precond0 := op0["preconditions"].([]any)[0].(map[string]any)
	require.Empty(t, precond0["status"],
		"the staged precondition's own live column renders blank, pinned directly rather than only through validity_error's own non-empty string")

	require.Empty(t, op1["shape_error"], "the FIRST operation to stage the key is clean")
	require.Equal(t, "duplicate_staged_key", op2["shape_error"],
		"the shape evaluator is doc-only too and still flags the duplicate even though the batch cannot resolve")

	require.Equal(t, true, op3["source_missing"],
		"operation 3 is ALSO hand-blanked, on top of its own shape defect")
	require.Equal(t, "staged_epic_replacement", op3["shape_error"],
		"the same operation carries both verdicts at once, neither one silencing the other")

	require.NotEmpty(t, data["validity_error"], "the resolution failure is named too")

	human, _, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, human)
	require.Contains(t, human, "source: missing (intake amendments cite every operation)")
	require.Contains(t, human, "shape: duplicate staged key")
	require.Contains(t, human, "shape: staged epic replacement")
	require.Contains(t, human, "live validity unavailable")
}

// --- approve amendment ---

func TestApproveAmendmentAppliesTheBatchAndPrintsTheReadout(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	_, _, code := execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/iphone-acceptance",
		"--title", "iPhone-only acceptance",
		"--precondition", "key_absent=form-engine/nrca-form-loading/iphone-acceptance", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "amend", aid, "--cancel", taskID, "--reason", "superseded under the iPhone-only decision",
		"--superseded-by", "form-engine/nrca-form-loading/iphone-acceptance",
		"--precondition", "target_status=proposed", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "operation 0 (add-task)")
	require.Contains(t, out, "operation 1 (cancel)")
	require.Contains(t, out, "**Approval recorded**", "the gate readout follows the confirmation")
	// The deferred item this task closes: the gate readout used to pass
	// act:nil and print no Verification section at all for this command.
	// It now reports what the batch actually applied, tallied by op kind in
	// mutate's own fixed order (cancel before add-task).
	require.Contains(t, out, "Operations applied: 2")
	require.Contains(t, out, "By kind: 1 cancel, 1 add-task")

	tree := loadCwdTree(t)
	require.Equal(t, "approved", tree.Amendments[aid].Status)
}

// A new task created by amendment is unclaimed even when a sibling is assigned.
func TestApproveAmendmentAddTaskDoesNotInheritSiblingOwnership(t *testing.T) {
	layAmendmentTree(t)
	mustRun(t, "add", "task", cliTaskKey, "--title", "Transcribe the schema", "--contributor", "jc")
	claimTaskAsOwner(t, cliTaskKey, "jc")
	aid := openAmendmentCLI(t, "claimed-add-task", "Add a task beside assigned work")

	_, _, code := execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/iphone-acceptance",
		"--title", "iPhone-only acceptance",
		"--precondition", "key_absent=form-engine/nrca-form-loading/iphone-acceptance", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out)

	minted := soleTaskIDByKey(t, "form-engine/nrca-form-loading/iphone-acceptance")
	require.Empty(t, loadCwdTree(t).Tasks[minted].Owner)
	require.Equal(t, "jc", loadCwdTree(t).Tasks[soleTaskIDByKey(t, cliTaskKey)].Owner)
}

// A same-statement amendment unions new question blocks without minting a duplicate.
func TestApproveAmendmentDoesNotCountUpdatedQuestionAsMinted(t *testing.T) {
	layAmendmentTree(t)
	statement := "Which billing provider is canonical?"
	firstBlock := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	secondBlock := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	out, _, code := execCLI(t, "add", "question", statement, "--title", "Billing currency", "--blocks", firstBlock, "--contributor", "jc")
	require.Equal(t, 0, code, out)

	aid := openAmendmentCLI(t, "billing-question-update", "Add billing and widen the existing question")
	_, _, code = execCLI(t, "amend", aid, "--add-epic", "billing", "--title", "Billing",
		"--precondition", "key_absent=billing", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "amend", aid, "--add-question", statement,
		"--title", "Billing currency", "--blocks", secondBlock, "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out)

	q := questionByStatement(t, loadCwdTree(t), statement)
	require.ElementsMatch(t, []string{firstBlock, secondBlock}, q.Blocks,
		"AppliedUpdated unions the new block without minting another question")
}

// An idempotent question amendment preserves its existing block set.
func TestApproveAmendmentDoesNotCountNoChangeQuestionAsMinted(t *testing.T) {
	layAmendmentTree(t)
	statement := "Which table format is canonical?"
	block := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	out, _, code := execCLI(t, "add", "question", statement, "--title", "Table question", "--blocks", block, "--contributor", "jc")
	require.Equal(t, 0, code, out)

	aid := openAmendmentCLI(t, "table-question-no-change", "Add table forms and retain the existing question")
	_, _, code = execCLI(t, "amend", aid, "--add-feature", "form-engine/table-backed-forms",
		"--title", "Table-backed forms", "--change-type", "create",
		"--precondition", "key_absent=form-engine/table-backed-forms", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "amend", aid, "--add-question", statement,
		"--title", "Table question", "--blocks", block, "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, out)

	q := questionByStatement(t, loadCwdTree(t), statement)
	require.Equal(t, []string{block}, q.Blocks,
		"AppliedNoChange leaves the existing question untouched")
}

// TestApproveAmendmentBareIdentitySpeaksTheFallbackAndCreditsTheActor is the
// design's combined acceptance, adapted for birth-unclaimed: a bare approve
// on a checkout whose identity.yml names jc resolves through the fallback
// chain and speaks it on both surfaces, the human headline's "acting as"
// line and the --json envelope's generic actor field. It no longer also
// claims jc as the OWNER of what the batch minted: both amendments here
// mint an add-task alone, and a minted task carries no ownership clause at
// all, regardless of the feature's claim state (poolMinted's own doc
// comment: a task's real landing owner is not something Op alone can
// honestly report). The actor field is a different fact, "who ran this",
// true regardless of what got minted or who it landed with, so it is
// pinned unchanged.
func TestApproveAmendmentBareIdentitySpeaksTheFallbackAndCreditsTheActor(t *testing.T) {
	layAmendmentTree(t)
	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	aid1 := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	_, _, code = execCLI(t, "amend", aid1, "--add-task", "form-engine/nrca-form-loading/iphone-acceptance",
		"--title", "iPhone-only acceptance",
		"--precondition", "key_absent=form-engine/nrca-form-loading/iphone-acceptance", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "approve", "amendment", aid1)
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "acting as jc (.tiller/identity.yml)",
		"a bare approve falls through to the identity file and says so")
	require.Contains(t, out, ": 1 operation(s) applied.",
		"the batch's one mint is a task, which carries no ownership clause at all")
	require.NotContains(t, out, "minted nodes owned by")
	require.NotContains(t, out, "epic(s) minted and owned by")
	require.NotContains(t, out, "question(s) minted and owned by")
	require.NotContains(t, out, "into the pool")

	aid2 := openAmendmentCLI(t, "android-only", "Remove native Android support")
	_, _, code = execCLI(t, "amend", aid2, "--add-task", "form-engine/nrca-form-loading/android-acceptance",
		"--title", "Android-only acceptance",
		"--precondition", "key_absent=form-engine/nrca-form-loading/android-acceptance", "--contributor", "jc")
	require.Equal(t, 0, code)

	jsonOut, _, code := execCLI(t, "approve", "amendment", aid2, "--json")
	require.Equal(t, 0, code, jsonOut)
	data := jsonData(t, jsonOut)
	actorField, ok := data["actor"].(map[string]any)
	require.True(t, ok, "the actor field names the acting contributor on the JSON surface")
	require.Equal(t, "jc", actorField["id"])
	require.Equal(t, ".tiller/identity.yml", actorField["source"])
}

func TestApproveAmendmentRefusesSupersedingNonDoneTarget(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "non-done-supersede", "Replace unfinished schema work")
	task1 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")

	_, _, code := execCLI(t, "amend", aid, "--supersede", task1, "--by", task2, "--contributor", "jc")
	require.Equal(t, 0, code, "staging remains legal before the target is done")

	stdout, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, stdout, `"reason": "supersede_target_not_done"`)
	require.NotContains(t, stdout, `"applied"`)
}

func TestApproveAmendmentPinsCrossGrainLineageReason(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "cross-grain-lineage", "Reject mixed-grain lineage")
	taskID := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")

	_, _, code := execCLI(t, "amend", aid, "--supersede", taskID, "--by", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)

	stdout, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, stdout, `"reason": "supersession_noun_mismatch"`)
	require.NotContains(t, stdout, `"applied"`)
}

func TestApproveAmendmentPinsConflictingLineageReason(t *testing.T) {
	layAmendmentTree(t)
	task1 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-transcription")
	task2 := soleTaskIDByKey(t, "form-engine/nrca-form-loading/schema-review")
	setTaskStatusOnDisk(t, "schema-transcription", "done")

	first := openAmendmentCLI(t, "first-lineage", "Record the first replacement")
	_, _, code := execCLI(t, "amend", first, "--supersede", task1, "--by", task2, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, stderr, code := execCLI(t, "approve", "amendment", first, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	replacement := "form-engine/nrca-form-loading/replacement-schema"
	second := openAmendmentCLI(t, "conflicting-lineage", "Reject a conflicting replacement")
	_, _, code = execCLI(t, "amend", second, "--add-task", replacement, "--title", "Replacement schema work", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "amend", second, "--supersede", task1, "--by", replacement, "--contributor", "jc")
	require.Equal(t, 0, code)

	stdout, _, code := execCLI(t, "approve", "amendment", second, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, stdout, `"reason": "supersession_conflict"`)
	require.NotContains(t, stdout, `"applied"`)
}

// TestApproveAmendmentBlockedStalePreconditions confirms the blocked answer
// (exit 1, reason amendment_preconditions_stale) names the stale operation
// and leaves the amendment proposed: approve refuses the WHOLE batch on one
// bad precondition, writing nothing.
func TestApproveAmendmentBlockedStalePreconditions(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	_, _, code := execCLI(t, "amend", aid, "--revise", "form-engine/nrca-form-loading",
		"--precondition", "target_status=done", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.Contains(t, out, `"reason": "amendment_preconditions_stale"`)
	require.Contains(t, out, `"precondition": "target_status"`)
	require.Contains(t, out, `"expected": "done"`)
	require.Contains(t, out, `"actual": "draft"`)

	_, stderr, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "stale precondition")
	require.Contains(t, stderr, "operation 0 (revise)")

	tree := loadCwdTree(t)
	require.Equal(t, "proposed", tree.Amendments[aid].Status, "a refused approve leaves the amendment proposed")
}

// TestApproveAmendmentRefusesAReviseWithNoContractOnDisk is the carried defect
// at the surface a human sees it on. layAmendmentTree's feature is a bare draft:
// `tiller add feature` writes feature.md and no feature.eval.md at all, so the
// revise's basis re-pin has no contract to hash. That used to exit 2 with
// `vcs: git could not resolve path .../feature.eval.md`, an environment error
// for a fact about the tree, which tells a reader nothing about which operation
// caused it or what to do next.
//
// The exit code is the same 2 it always was (the mutate arms hold that ruling,
// amendment_approve.go's reviseTargetIncompleteErr); what this pins is that the
// answer now names the operation, the file, and the way out, and that no raw
// `vcs:` text reaches the reader on either stream.
func TestApproveAmendmentRefusesAReviseWithNoContractOnDisk(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	_, _, code := execCLI(t, "amend", aid, "--revise", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code, "staging asserts nothing about the working tree")

	out, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "revise_target_incomplete"`)
	require.Contains(t, out, "feature.eval.md")
	require.NotContains(t, out, "vcs:")

	stdout, stderr, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 2, code)
	human := stdout + stderr
	require.Contains(t, human, "operation 0 revises feature")
	require.Contains(t, human, "feature.eval.md")
	require.Contains(t, human, "drop the operation")
	require.NotContains(t, human, "vcs:")

	require.Equal(t, "proposed", loadCwdTree(t).Amendments[aid].Status,
		"a refused approve leaves the amendment proposed")

	// The way out the refusal names works: write the contract, then approve. The
	// contract names the feature this tree really minted, not a stand-in id: what
	// the flow produces is the only fixture worth asserting against, and nothing
	// validates the node field to catch a stand-in (store.EvalFile.ValidateContract
	// never reads it).
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.eval.md"),
		[]byte(amendE2EEvalMD(featureIDByKey(t, cliFeatureKey), "The form loads")), 0o644))
	_, stderr, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "approved", loadCwdTree(t).Amendments[aid].Status)
}

// TestApproveAmendmentRefusesWhenNotOpen confirms a second approve of an
// already-approved amendment refuses amendment_not_open at exit 2, the same
// state check every amendment verb runs before touching anything.
func TestApproveAmendmentRefusesWhenNotOpen(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	_, _, code := execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/iphone-acceptance",
		"--title", "iPhone-only acceptance", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "amendment_not_open"`)
}

// TestApproveAmendmentTakesExactlyOneAddress confirms none of the batch/
// refresh-basis/reviewed-assumption acts under the shared `approve` verb
// apply to an amendment.
func TestApproveAmendmentTakesExactlyOneAddress(t *testing.T) {
	layAmendmentTree(t)
	aid1 := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	aid2 := openAmendmentCLI(t, "table-forms", "Table-backed forms")

	out, _, code := execCLI(t, "approve", "amendment", aid1, aid2, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "bad_args"`)

	out, _, code = execCLI(t, "approve", "amendment", aid1, "--refresh-basis", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "bad_args"`)
}

// --- withdraw amendment ---

func TestWithdrawAmendmentHappyPath(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "withdraw", "amendment", aid, "--reason", "decided against it", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	require.Equal(t, "withdrawn", data["status"])
	require.Equal(t, "decided against it", data["reason"])
}

// TestWithdrawAmendmentRequiresReason mirrors TestCancelRequiresReasonFlag
// (lifecycle_cmds_test.go): cobra refuses the command before it ever reaches
// the core, naming the flag the human forgot.
func TestWithdrawAmendmentRequiresReason(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	_, stderr, code := execCLI(t, "withdraw", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 2, code, "a missing required flag is misuse")
	require.Contains(t, stderr, "reason", "the refusal must name the flag the human forgot")
}

func TestWithdrawAmendmentRefusesWhenAlreadyTerminal(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	_, _, code := execCLI(t, "withdraw", "amendment", aid, "--reason", "first", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "withdraw", "amendment", aid, "--reason", "again", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "amendment_not_open"`)
}

// TestWithdrawThenReopenLeavesTheKeyAddressable drives the whole cycle the
// duplicate-key rule invites: open a reshape, withdraw it, open another under the
// same key, then keep working through that key. Before resolve preferred the open
// proposal (internal/core/resolve's preferOpenAmendment) every one of the calls
// below exited 2 as an ambiguous key, so the second reshape could only be reached
// by its id, and the readout's own copyable `tiller approve amendment <key>` row
// was a command that could not run. mutate's own refusal invites the reuse
// ("withdraw or approve it before reusing the key"), so this is the supported
// path, not an edge case, and it is asserted through the binary rather than in
// the resolver's unit fixtures because the poisoned surface was the CLI's.
func TestWithdrawThenReopenLeavesTheKeyAddressable(t *testing.T) {
	layAmendmentTree(t)
	first := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	_, _, code := execCLI(t, "withdraw", "amendment", first, "--reason", "wrong shape", "--contributor", "jc")
	require.Equal(t, 0, code)

	// Read the second id off `add`'s own envelope rather than through
	// openAmendmentCLI, which finds a key's holder by ranging the tree and cannot
	// say which of two holders is the new one.
	out, stderr, code := execCLI(t, "add", "amendment", "iphone-only",
		"--title", "Remove native iPad support, take two", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	second, _ := jsonData(t, out)["id"].(string)
	require.NotEmpty(t, second)
	require.NotEqual(t, first, second)

	out, stderr, code = execCLI(t, "show", "amendment", "iphone-only", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, second, jsonData(t, out)["id"], "the key addresses the amendment still open")

	out, stderr, code = execCLI(t, "amend", "iphone-only",
		"--cancel", "form-engine/nrca-form-loading/schema-review", "--reason", "subsumed",
		"--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, second, jsonData(t, out)["id"], "and staging lands on it")

	out, stderr, code = execCLI(t, "approve", "amendment", "iphone-only", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, second, jsonData(t, out)["id"], "and so does the gate the queue row prints")

	// Both retired amendments keep their own id as an address, which is how a
	// human reads the proposal that was dropped.
	for _, aid := range []string{first, second} {
		_, stderr, code := execCLI(t, "show", "amendment", aid)
		require.Equalf(t, 0, code, "%s stays addressable by id: %s", aid, stderr)
	}

	// With no open holder left, the key now matches two retired amendments and
	// there is no honest winner, so it refuses rather than guessing.
	out, _, code = execCLI(t, "show", "amendment", "iphone-only", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "ambiguous", "two retired holders and no open one is a real ambiguity")
}

func TestWithdrawRefusesAnyNounButAmendment(t *testing.T) {
	layAmendmentTree(t)
	out, _, code := execCLI(t, "withdraw", "epic", "form-engine", "--reason", "x", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "unknown_noun"`)
}

// --- ls amendments / status amendment ---

func TestLsAmendments(t *testing.T) {
	layAmendmentTree(t)
	openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "ls", "amendments", "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	nodes := data["nodes"].([]any)
	require.Len(t, nodes, 1)
	row := nodes[0].(map[string]any)
	require.Equal(t, "iphone-only", row["key"])
	require.Equal(t, "Remove native iPad support", row["title"])
	require.Equal(t, "proposed", row["status"])
}

// TestStatusAmendmentOracle confirms the typed oracle's state and legal next
// moves for an amendment: proposed -> approve|withdraw, and terminal once
// approved.
func TestStatusAmendmentOracle(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "status", "amendment", aid, "--json")
	require.Equal(t, 0, code)
	data := jsonData(t, out)
	require.Equal(t, "proposed", data["status"])
	require.Equal(t, []any{"approve", "withdraw"}, data["legal_next"])

	_, _, code = execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/iphone-acceptance",
		"--title", "iPhone-only acceptance", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code = execCLI(t, "status", "amendment", aid, "--json")
	require.Equal(t, 0, code)
	data = jsonData(t, out)
	require.Equal(t, "approved", data["status"])
	require.Equal(t, []any{}, data["legal_next"], "approved is terminal")
}

// --- actor model: no code-level refusal ---

// TestAmendmentGatesRunForAnyActingOwnerNoCodeLevelRefusal documents a
// deliberate absence: cli-surface.md's "The actor model" is explicit that
// Tiller never refuses a human-only verb by identity ("on one machine the
// agent and the human share an identity and any such refusal would be
// theater"), and store.Contributor itself carries no agent/human distinction
// ("A developer's agent acts as that developer", manifest.go). approve
// amendment and withdraw amendment are classed ActorHuman in
// lifecycle/table.go's verbActors map (guidance the skills and orientation
// surface), the identical non-enforcement every other human-only verb
// (approve, cancel, archive) already has in this codebase: there is no
// --actor flag, no owner "kind", and nothing here to assert a refusal
// against. This test pins the actual contract: the same acting contributor every
// other mutation resolves through (--contributor/TILLER_CONTRIBUTOR/identity.yml) is
// sufficient to run both gates, exactly as it is for every other verb.
func TestAmendmentGatesRunForAnyActingOwnerNoCodeLevelRefusal(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	_, _, code := execCLI(t, "amend", aid, "--add-task", "form-engine/nrca-form-loading/iphone-acceptance",
		"--title", "iPhone-only acceptance", "--contributor", "jc")
	require.Equal(t, 0, code)

	// "jc" here plays both roles cli-surface.md says share one identity: the
	// same --contributor an agent resolves on a developer's behalf runs the human
	// gate cleanly, because Tiller has no second identity to check it
	// against.
	_, _, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc")
	require.Equal(t, 0, code, "approve amendment is guidance-only human-gate classing, never a code refusal")

	aid2 := openAmendmentCLI(t, "table-forms", "Table-backed forms")
	_, _, code = execCLI(t, "withdraw", "amendment", aid2, "--reason", "not needed", "--contributor", "jc")
	require.Equal(t, 0, code, "withdraw amendment is guidance-only human-gate classing, never a code refusal")
}
