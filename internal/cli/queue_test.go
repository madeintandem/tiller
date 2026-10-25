package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// layQueueCommandTree creates a draft epic, an active feature, an in-review
// task, and shelf rows for triage and a cancelled carrier. The feature and task
// each carry a criterion awaiting attestation, and the task has two distinct
// optional review gates. That gives the command enough mixed kind/noun rows to
// distinguish every filter path and prove filtering never derives eligibility.
func layQueueCommandTree(t *testing.T) *store.Tree {
	t.Helper()
	layReadTree(t)
	_, _, code := execCLI(t, "add", "epic", "private-backlog", "--title", "Private backlog", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", "form-engine/nrca-form-loading/private-review-task",
		"--title", "Private review task", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "bug", "private-triage", "--title", "Private triage bug", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "tweak", "private-adjustment", "--title", "Private adjustment", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "chore", "private-triage", "--title", "Private triage chore", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "bug", "private-carrier", "--title", "Private carrier bug", "--contributor", "jc")
	require.Equal(t, 0, code)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)

	tree.Manifest.Gates.Optional = map[string]store.GateSpec{
		"private_gate_alpha": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
		"private_gate_beta":  {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
	}
	manifestYAML, err := yaml.Marshal(tree.Manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cwd, ".tiller", "tiller.yml"), manifestYAML, 0o644))

	feature := tree.Features[soleFeatureID(t)]
	feature.Status = "active"
	feature.GatesBlock = store.GatesBlock{Gates: map[string]store.Gate{
		"evals": {Criteria: map[string]store.Criterion{
			"PRIVATE_FEATURE_CRITERION": {Status: store.CriterionStatusNeedsAttestation},
		}},
	}}
	featureYAML, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(featureDir(t), "feature.yml"), featureYAML, 0o644))

	task := tree.Tasks[soleTaskID(t)]
	task.Status = "in_review"
	task.Started = true
	task.Owner = "jc"
	pr := 42
	task.PR = &pr
	task.GatesBlock = store.GatesBlock{Gates: map[string]store.Gate{
		"evals": {Criteria: map[string]store.Criterion{
			"PRIVATE_TASK_CRITERION": {Status: store.CriterionStatusNeedsAttestation},
		}},
	}}
	taskYAML, err := yaml.Marshal(task)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(taskYMLPath(t, "private-review-task"), taskYAML, 0o644))

	carrierKey := store.ShelfCanonicalKey(store.NounBug, "private-carrier")
	var carrier *store.Bug
	for _, bug := range tree.Bugs {
		if bug.Key != nil && *bug.Key == carrierKey {
			carrier = bug
			break
		}
	}
	require.NotNil(t, carrier)
	carrier.Status = "ready"
	carrier.ResolvedBy = "F-gone00"
	carrierYAML, err := yaml.Marshal(carrier)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cwd, store.ShelfRelDir(store.NounBug, "private-carrier"), store.ShelfStateFile(store.NounBug)), carrierYAML, 0o644))

	tree, err = store.Load(cwd)
	require.NoError(t, err)
	return tree
}

func queueCommandData(t *testing.T, raw string) views.Queue {
	t.Helper()
	var env struct {
		Data views.Queue `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	return env.Data
}

func runQueueCommandJSON(t *testing.T, args ...string) (views.Queue, string) {
	t.Helper()
	argv := append([]string{"queue"}, args...)
	argv = append(argv, "--json")
	out, stderr, code := execCLI(t, argv...)
	require.Equal(t, 0, code, stderr)
	return queueCommandData(t, out), out
}

func TestQueueCommandMatchesTheUnfilteredQueueProjection(t *testing.T) {
	tree := layQueueCommandTree(t)
	want := views.QueueOfInput(tree, views.QueueInput{})

	got, _ := runQueueCommandJSON(t)
	require.Equal(t, want, got,
		"the JSON command uses the same queue projection and order as the console API")

	human, stderr, code := execCLI(t, "queue")
	require.Equal(t, 0, code, stderr)
	cursor := -1
	for _, entry := range want.Entries {
		at := strings.Index(human, entry.Command)
		require.Greater(t, at, cursor, "human drill-down keeps queue order and includes %q", entry.Command)
		cursor = at
	}
}

func TestQueueCommandFiltersByKindNounAndBothTogether(t *testing.T) {
	tree := layQueueCommandTree(t)
	all := views.QueueOfInput(tree, views.QueueInput{})

	for _, tc := range []struct {
		name   string
		filter views.QueueFilter
		args   []string
	}{
		{name: "attestation kind", filter: views.QueueFilter{Kind: "attest_gate"}, args: []string{"--kind", "attest_gate"}},
		{name: "bug approval kind", filter: views.QueueFilter{Kind: "approve_bug"}, args: []string{"--kind", "approve_bug"}},
		{name: "tweak approval kind", filter: views.QueueFilter{Kind: "approve_tweak"}, args: []string{"--kind", "approve_tweak"}},
		{name: "chore approval kind", filter: views.QueueFilter{Kind: "approve_chore"}, args: []string{"--kind", "approve_chore"}},
		{name: "cancelled carrier kind", filter: views.QueueFilter{Kind: "carrier_cancelled"}, args: []string{"--kind", "carrier_cancelled"}},
		{name: "task noun", filter: views.QueueFilter{Noun: "task"}, args: []string{"--noun", "task"}},
		{name: "bug noun", filter: views.QueueFilter{Noun: "bug"}, args: []string{"--noun", "bug"}},
		{name: "tweak noun", filter: views.QueueFilter{Noun: "tweak"}, args: []string{"--noun", "tweak"}},
		{name: "chore noun", filter: views.QueueFilter{Noun: "chore"}, args: []string{"--noun", "chore"}},
		{name: "criterion kind and task noun", filter: views.QueueFilter{Kind: "attest_criterion", Noun: "task"}, args: []string{"--kind", "attest_criterion", "--noun", "task"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runQueueCommandJSON(t, tc.args...)
			require.Equal(t, views.FilterQueue(all, tc.filter), got)
		})
	}
}

func TestQueueCommandKeepsMultipleGatesOnOneTask(t *testing.T) {
	layQueueCommandTree(t)

	got, _ := runQueueCommandJSON(t, "--kind", "attest_gate")

	require.Len(t, got.Entries, 2, "two gates on one task are two actionable rows, not one task summary")
	require.Equal(t, got.Entries[0].ID, got.Entries[1].ID)
	require.ElementsMatch(t, []string{"private_gate_alpha", "private_gate_beta"},
		[]string{got.Entries[0].Gate, got.Entries[1].Gate})
}

func TestQueueCommandRendersEmptyMatchesAsAnArray(t *testing.T) {
	layQueueCommandTree(t)

	got, raw := runQueueCommandJSON(t, "--kind", "feature_check")

	require.NotNil(t, got.Entries)
	require.Empty(t, got.Entries)
	require.Contains(t, raw, `"entries": []`)
}

func TestQueueCommandRejectsInvalidFiltersAsUsage(t *testing.T) {
	layQueueCommandTree(t)

	for _, tc := range []struct {
		name, flag, value string
		args              []string
	}{
		{name: "kind", flag: "kind", value: "not-a-kind", args: []string{"queue", "--kind", "not-a-kind"}},
		{name: "noun", flag: "noun", value: "widget", args: []string{"queue", "--noun", "widget"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := execCLI(t, tc.args...)
			message := stdout + stderr
			require.Equal(t, 2, code, message)
			require.Contains(t, message, tc.flag)
			require.Contains(t, message, tc.value)
			require.NotContains(t, message, "unknown command", "the command exists; the filter value is the misuse")
		})
	}
}

// TestQueueCommandAcceptsMergeKinds pins the review-grain queue allowlist
// (controller ruling A): --kind merge_epic and --kind merge_feature are both
// accepted, matching the views-side gravity order (queue.go's own group
// order runs epics before features, after feature_check), and an unknown
// kind still refuses.
func TestQueueCommandAcceptsMergeKinds(t *testing.T) {
	layQueueCommandTree(t)

	for _, kind := range []string{"merge_epic", "merge_feature"} {
		t.Run(kind, func(t *testing.T) {
			got, raw := runQueueCommandJSON(t, "--kind", kind)
			require.NotNil(t, got.Entries)
			require.Contains(t, raw, `"entries": []`, "no merge-ready node in this fixture, but the kind itself is legal")
		})
	}

	stdout, stderr, code := execCLI(t, "queue", "--kind", "not-a-kind")
	message := stdout + stderr
	require.Equal(t, 2, code, message)
	require.Contains(t, message, "not-a-kind")
}

// mergeReadyFeatureFixture drives a one-epic-one-feature tree to a feature
// with its own PR recorded at feature grain: views.queueMergeFeatures' own
// precondition (a recorded PR or review ref, and not done). It uses the
// plain owned manifest (initOwnedRepo, via walkFeatureToActive), which
// carries no required gates. The grain is chosen while the task is still
// ready, before work begins; the task then reaches in_review without a PR
// of its own because the feature is the review surface. Returns the feature's
// id.
func mergeReadyFeatureFixture(t *testing.T) (fid string) {
	t.Helper()
	initOwnedRepo(t)
	gitRun(t, ".", "branch", "-M", "main")
	walkFeatureToActive(t)
	_, _, code := execCLI(t, "configure", "feature", cliFeatureKey, "--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	checkoutTaskBranchCLI(t, tid)
	require.NoError(t, os.WriteFile("work.txt", []byte("feature review implementation\n"), 0o644))
	gitRun(t, ".", "add", "-A")
	gitRun(t, ".", "commit", "-m", "implement feature task")
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "submit", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	assembleTaskForReviewCLI(t, tid, soleFeatureBranch(t))
	_, _, code = execCLI(t, "submit", "feature", cliFeatureKey, "--pr", "99", "--contributor", "jc")
	require.Equal(t, 0, code)
	return featureIDByKey(t, cliFeatureKey)
}

// TestQueueCommandShowsAMergeReadyFeature is the named-risk fix: proof that
// `tiller queue --kind merge_feature` actually renders the row Task 5's
// views.queueMergeFeatures produces, not merely that the kind is accepted.
// A typo drifting the "merge_feature" literal between views/queue.go and
// this CLI's own allowlist (queue.go's queueKindOrder) would leave --kind
// merge_feature accepted (TestQueueCommandAcceptsMergeKinds still passes,
// its refusal check is over --kind alone) but rendering nothing, exactly
// what TestQueueCommandAcceptsMergeKinds's own empty-entries assertion could
// never catch; this test drives past that gap.
func TestQueueCommandShowsAMergeReadyFeature(t *testing.T) {
	fid := mergeReadyFeatureFixture(t)

	got, _ := runQueueCommandJSON(t, "--kind", "merge_feature")
	require.Len(t, got.Entries, 1, "the merge-ready feature must render exactly one row")
	entry := got.Entries[0]
	require.Equal(t, "merge_feature", entry.Kind)
	require.Equal(t, store.NounFeature, entry.Noun)
	require.Equal(t, fid, entry.ID)
	require.Equal(t, cliFeatureKey, entry.Key)
	require.Equal(t, "tiller sync", entry.Command)
}

// TestQueueCommandShowsAMergeReadyEpic mirrors
// TestQueueCommandShowsAMergeReadyFeature one grain up: an epic-grain epic
// with its own PR recorded, over a live feature whose own task is in_review,
// renders one merge_epic row.
func TestQueueCommandShowsAMergeReadyEpic(t *testing.T) {
	mergeReadyEpicFixture(t)

	got, _ := runQueueCommandJSON(t, "--kind", "merge_epic")
	require.Len(t, got.Entries, 1, "the merge-ready epic must render exactly one row")
	entry := got.Entries[0]
	require.Equal(t, "merge_epic", entry.Kind)
	require.Equal(t, store.NounEpic, entry.Noun)
	require.Equal(t, soleEpicID(t), entry.ID)
	require.Equal(t, "form-engine", entry.Key)
	require.Equal(t, "tiller sync", entry.Command)
}

// mergeReadyEpicFixture drives an epic-grain epic to its own recorded PR over
// a live feature whose own task is in_review.
func mergeReadyEpicFixture(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	gitRun(t, ".", "branch", "-M", "main")
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", cliFeatureKey, "--title", "NRCA form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	writeRealSpec(t)
	addSpeccedTask(t, "transcribe-the-schema", "Transcribe the schema")
	claimTaskAsOwner(t, cliFeatureKey+"/transcribe-the-schema", "jc")
	_, _, code = execCLI(t, "spec", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "feature", cliFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "configure", "epic", "form-engine", "--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	checkoutTaskBranchCLI(t, tid)
	require.NoError(t, os.WriteFile("work.txt", []byte("epic review implementation\n"), 0o644))
	gitRun(t, ".", "add", "-A")
	gitRun(t, ".", "commit", "-m", "implement epic task")
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	// The epic grain is chosen before either feature or task work begins.
	// The task therefore reaches in_review without its own PR; the epic's PR
	// below is the one review surface for the round.
	_, _, code = execCLI(t, "submit", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	assembleTaskForReviewCLI(t, tid, loadTestTree(t).Epics[soleEpicID(t)].Branch)
	_, _, code = execCLI(t, "submit", "epic", "form-engine", "--pr", "77", "--contributor", "jc")
	require.Equal(t, 0, code)
}

// TestReadoutNamesAggregateMergeAsks: a queue whose only asks are an
// aggregate merge must not read as "nothing needs you". The readout's
// attention list names the matching queue drill-down for each grain.
func TestReadoutNamesAggregateMergeAsks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fixture func(*testing.T)
		kind    string
	}{
		{"feature", func(t *testing.T) { mergeReadyFeatureFixture(t) }, "merge_feature"},
		{"epic", mergeReadyEpicFixture, "merge_epic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.fixture(t)
			queue, _ := runQueueCommandJSON(t)
			require.Len(t, queue.Entries, 1, "the fixture's queue holds only the merge ask")
			require.Equal(t, tc.kind, queue.Entries[0].Kind)

			out, stderr, code := execCLI(t, "readout", "execute")
			require.Equal(t, 0, code, stderr)
			require.Contains(t, out, needsAttentionHeading)
			require.Contains(t, out, "tiller queue --kind "+tc.kind)
		})
	}
}

func TestQueueCommandSuppressesABasisDriftedFeatureCheck(t *testing.T) {
	layApprovedFeatureWithTask(t)
	_, _, code := execCLI(t, "start", "task", soleTaskID(t), "--contributor", "jc")
	require.Equal(t, 0, code)
	setTaskStatus(t, "transcribe-the-schema", "in_progress", "done")

	clean, _ := runQueueCommandJSON(t, "--kind", "feature_check")
	require.Len(t, clean.Entries, 1, "the clean awaiting-check feature proves the candidate exists")

	specPath := filepath.Join(featureDir(t), "feature.md")
	spec, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(specPath, append(spec, []byte("\nBasis drift after approval.\n")...), 0o644))

	drifted, raw := runQueueCommandJSON(t, "--kind", "feature_check")
	require.NotNil(t, drifted.Entries)
	require.Empty(t, drifted.Entries, "the CLI computes the same basis-drift suppression as /api/queue")
	require.Contains(t, raw, `"entries": []`)
}
