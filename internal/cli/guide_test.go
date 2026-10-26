package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/skills"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// Named guides deliver the catalog in full without requiring project state.
func TestGuideSkillPrintsTheSkill(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range skills.Names() {
		t.Run(name, func(t *testing.T) {
			want, ok := skills.Lookup(name)
			require.True(t, ok)
			out, stderr, code := execCLI(t, "guide", name)
			require.Equal(t, 0, code, stderr)
			require.Equal(t, strings.TrimSpace(want), strings.TrimSpace(out))

			out, stderr, code = execCLI(t, "guide", name, "--json")
			require.Equal(t, 0, code, stderr)
			data := jsonData(t, out)
			require.Equal(t, name, data["skill"])
			require.Equal(t, want, data["text"])
		})
	}
}

// TestGuideUnknownSkillIsMisuse confirms an unknown skill name exits 2 and lists
// the valid names so the caller can correct the call.
func TestGuideUnknownSkillIsMisuse(t *testing.T) {
	out, _, code := execCLI(t, "guide", "nonsense", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "unknown_skill")
	require.Contains(t, out, "decompose")
}

// TestGuideNoArgSuggestsDecomposeOnFreshRepo is the routing guarantee: on a fresh,
// undecomposed repo the fitting skill is decompose.
// A fresh greenfield repo has nothing under design/ to decompose: guide
// reports the no-work-planned end state instead of suggesting decompose.
func TestGuideNoArgOnFreshRepoReportsNoWorkPlanned(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"skill": "orient"`)
	require.Contains(t, out, "no work is planned yet")
}

func TestGuideNoArgSuggestsDecomposeWithAToBeDoc(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller", "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "design", "roadmap.md"), []byte("# Roadmap\n"), 0o644))

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"skill": "decompose"`)
	require.Contains(t, out, "starting with .tiller/design/roadmap.md", "guide names the to-be doc the route found")
}

// check readiness prints guide's suggestion, not a hard-coded decompose:
// with an owned, claimed bug on a ready project, Next is execute.
func TestCheckReadinessNextFollowsTheRoute(t *testing.T) {
	layBugCLI(t)
	require.NoError(t, os.MkdirAll(filepath.Join(".tiller", "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(".tiller", "design", "roadmap.md"), []byte("# Roadmap\n"), 0o644))
	mustRun(t, "approve", "bug", "alpha", "--contributor", "jc")
	mustRun(t, "claim", "bug", "alpha", "--contributor", "jc")
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	out, stderr, code := execCLI(t, "check", "readiness")
	require.Equal(t, 0, code, out+stderr)
	require.Contains(t, out, "Next (tiller guide execute):")
	require.NotContains(t, out, "Next: decompose")
}

// TestGuideRoutesAnOwnedBugOnAnEmptyTree is the regression case at the CLI: a
// maintenance-only project with no epics and a claimed, approved bug gets
// execute from guide, not decompose.
func TestGuideRoutesAnOwnedBugOnAnEmptyTree(t *testing.T) {
	layBugCLI(t)
	mustRun(t, "approve", "bug", "alpha", "--contributor", "jc")
	mustRun(t, "claim", "bug", "alpha", "--contributor", "jc")
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	out, stderr, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, `"skill": "execute"`)
	require.Contains(t, out, "bugs/alpha")
	require.NotContains(t, out, `"skill": "decompose"`)
}

func TestGuideCarriesTheShelfQueueLine(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)

	human, _, code := execCLI(t, "guide")
	require.Equal(t, 0, code)
	require.Contains(t, human, "queue: 0 bugs, 0 chores, 0 tweaks ready")

	structured, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, structured, "queue: 0 bugs, 0 chores, 0 tweaks ready")
	require.Contains(t, structured, `"queue": "queue: 0 bugs, 0 chores, 0 tweaks ready"`)
}

// TestGuideDegradesOnACorruptIdentityFile pins the read-surface half of issue
// #52's design ruling: guide is read-only (it carries no --contributor flag and
// never calls deps()), so a present but unparseable .tiller/identity.yml
// must not darken it the way it refuses a mutation. Without this pin, a
// future change that pointed guide's owner resolution at
// resolveActingContributorOrRefuse (the mutating, refusing sibling) would turn a
// diagnostic surface into one more thing that goes dark on a corrupt file,
// and nothing here would catch it.
func TestGuideDegradesOnACorruptIdentityFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)
	corruptIdentityFile(t, dir)

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "identity_unreadable", "a read surface degrades to no id rather than refusing on a corrupt file")
}

// caughtUpGuideOutput reads bare guide against the real approved-but-unclaimed
// shape (layFeatureReadyToStart, laid once by the caller before the identity
// loop, not per call): identity selects the three read-only identity states
// that must not change bare guide's answer. The fixture is laid once so every
// identity reads the same task IDs.
func caughtUpGuideOutput(t *testing.T, identity string) (human, structured string) {
	t.Helper()
	t.Setenv("TILLER_CONTRIBUTOR", "")

	switch identity {
	case "resolved":
		t.Setenv("TILLER_CONTRIBUTOR", "jc")
	case "absent":
		// The fixture creates no identity file. An unresolved reader still sees
		// the same ownerless pool because reading it requires no claim.
	case "corrupt":
		root, err := os.Getwd()
		require.NoError(t, err)
		corruptIdentityFile(t, root)
	default:
		t.Fatalf("unknown identity state %q", identity)
	}

	human, stderr, code := execCLI(t, "guide")
	require.Equal(t, 0, code, stderr)
	structured, stderr, code = execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	return human, structured
}

// Claimable task routing is shared project advice, independent of the reader's
// identity or a corrupt local identity file.
func TestGuideNamesClaimableExecutionIndependentlyOfIdentity(t *testing.T) {
	layFeatureReadyToStart(t)
	var want SuggestView
	// corrupt must stay last: it mutates the shared tree's identity.yml, and
	// an earlier "absent" iteration would then silently read that same
	// corrupt file, becoming a second corrupt case instead of a true absent
	// one.
	for i, identity := range []string{"resolved", "absent", "corrupt"} {
		human, structured := caughtUpGuideOutput(t, identity)
		var envelope struct {
			Data SuggestView `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(structured), &envelope))
		got := envelope.Data
		// The list's heading names the reader, so it differs by identity; the
		// claimable move and the suggestion it earns do not.
		require.Len(t, got.Next.Candidates, 1, identity)
		require.Equal(t, "claim_task", got.Next.Candidates[0].Kind, identity)
		require.Contains(t, human, "Suggested skill: execute", identity)
		if i == 0 {
			want = got
			continue
		}
		require.Equalf(t, want.Skill, got.Skill, "skill differs for %s identity", identity)
		require.Equalf(t, want.Reason, got.Reason, "reason differs for %s identity", identity)
		require.Equalf(t, want.Next.Candidates[0].Command, got.Next.Candidates[0].Command, "move differs for %s identity", identity)
	}

	require.Equal(t, "execute", want.Skill)
	require.Equal(t, "tiller claim task "+cliTaskKey, want.Next.Candidates[0].Command)
}

// TestCaughtUpGuideNamesWhyThePoolIsNotStartable pins the terminal the
// claimable arm leaves behind: a pool whose first feature is blocked by
// someone else's claimed work names that block instead of a claim command.
func TestCaughtUpGuideNamesWhyThePoolIsNotStartable(t *testing.T) {
	layFeatureReadyToStart(t)
	// A second feature, specced, approved, and claimed by pat, that the
	// approved one now depends on: the approved one is still unclaimed but
	// no longer startable, and pat's work is pat's. The dependency target
	// has to leave draft (unmetTarget only treats done/cancelled as met, so
	// any live status blocks the edge), because a draft feature is always
	// a frontier specify candidate: leaving it draft routes the tree
	// to "specify: form-engine/schema-engine" and never reaches this
	// terminal at all (confirmed empirically; the exact shape the brief's
	// primary code proposes, add+claim alone, does not clear draft and so
	// does not reach the terminal either).
	_, _, code := execCLI(t, "add", "feature", "form-engine/schema-engine",
		"--title", "Schema engine", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	schemaDir := filepath.Join(featureDir(t), "..", "schema-engine")
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "feature.md"),
		[]byte("# Schema engine\n\nParse and validate the form schema.\n"), 0o644))
	schemaFeatureID := featureIDByKey(t, "form-engine/schema-engine")
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "feature.eval.md"),
		[]byte("---\nnode: "+schemaFeatureID+"\ncriteria:\n  - id: C1\n    statement: The schema parses\n    polarity: introduce\n    binding: { type: test, ref: spec/schema_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
	_, _, code = execCLI(t, "add", "task", "form-engine/schema-engine/parse-the-schema",
		"--title", "Parse the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, store.TaskDirName, "parse-the-schema", store.TaskProseFile),
		[]byte("# Parse the schema\n\n## Approach\n\nThe implementation spec.\n"), 0o644))
	_, _, code = execCLI(t, "spec", "feature", "form-engine/schema-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "feature", "form-engine/schema-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "claim", "task", "form-engine/schema-engine/parse-the-schema", "--contributor", "pat")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "link", "feature", "form-engine/certification-form-loading",
		"--depends-on", "form-engine/schema-engine", "--contributor", "jc")
	require.Equal(t, 0, code)

	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	human, stderr, code := execCLI(t, "guide")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, human, "Suggested skill: orient")
	require.Contains(t, human, "is blocked by form-engine/schema-engine (F-")

	readout, stderr, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code, stderr)
	require.NotContains(t, readout, "and it is", "the pool line renders no ownership clause for anyone")
}

func TestGuideSkipsAClaimableTaskHeldByQuestion(t *testing.T) {
	layFeatureReadyToStart(t)
	const featureKey = "form-engine/certification-form-loading"
	const taskKey = featureKey + "/transcribe-the-schema"
	out, stderr, code := execCLI(t, "add", "question", "Which schema version should the task use?",
		"--title", "Schema version", "--blocks", taskKey, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	questionID := jsonData(t, out)["id"].(string)

	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	guide, stderr, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "orient", jsonData(t, guide)["skill"])
	human, stderr, code := execCLI(t, "guide")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, human, "1. You: answer "+questionID+".", "the holding question is the move")
	require.Contains(t, human, "No agent move is legal right now; the moves above are yours.")
	readout, stderr, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code, stderr)
	_, next := splitAtNext(t, readout)
	require.Contains(t, next, "1. You: answer "+questionID+".", "the holding question is the move, not the held task")
	require.NotContains(t, readout, "tiller claim task "+taskKey)
	require.NotContains(t, readout, "`tiller start task "+taskKey+"`")

	// Claiming does not bypass the task's question hold.
	out, stderr, code = execCLI(t, "claim", "task", taskKey, "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	out, stderr, code = execCLI(t, "start", "task", taskKey, "--contributor", "jc")
	require.Equal(t, 1, code, out+stderr)
	require.Contains(t, out+stderr, questionID)
	out, stderr, code = execCLI(t, "answer", "question", questionID, "--answer", "Use schema version 1", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	out, stderr, code = execCLI(t, "start", "task", taskKey, "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
}

// TestGuideNoArgRoutesByState walks the routing precedence through the real
// CLI: a draft feature under a draft epic names the approve-epic wait through
// orient, and the same feature suggests specify once the epic is
// approved.
func TestGuideNoArgRoutesByState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)
	registerContributor(t, "jc", "jc")
	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	// A draft feature under a draft epic: the decomposition gate is the
	// human's move, so guide names orient and the approve command, never
	// specify.
	_, _, code = execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/certification-form-loading",
		"--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"skill\": \"orient\"")
	require.Contains(t, out, "No agent move is legal right now; the moves above are yours.")
	require.Contains(t, out, "\"kind\": \"approve_epic\"")
	require.Contains(t, out, "\"command\": \"tiller approve epic form-engine\"")
	require.NotContains(t, out, "specify", "the open decomposition gate is never read as an invitation to spec")

	// The human's approve clears the gate: the same draft feature is now
	// the frontier, and specify is the seam.
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	out, _, code = execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"skill\": \"specify\"")
}

// TestTaskLessFeatureNamesNoSeam replaces the break-down route. The task
// explosion is part of the specification now, so validateSpec refuses
// draft -> specced until the feature carries a non-cancelled task whose task.md
// is more than a stub: a feature past draft always carries one, and this test
// has to DELETE the folder to reach the state break-down existed for. On that
// state the honest answer is that no seam is due, not a seam whose skill no
// longer ships. Both surfaces rank with views.NextOf, so both say it.
func TestTaskLessFeatureNamesNoSeam(t *testing.T) {
	laySpeccableFeature(t)
	_, _, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NoError(t, os.RemoveAll(filepath.Join(featureDir(t), store.TaskDirName)))

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "break-down", "there is no break-down skill to route to")
	require.Contains(t, out, "\"skill\": \"orient\"", "the fallback names the router, not a retired seam")
	// The feature this fixture leaves behind is specced, so the honest wait
	// is the plan gate: a human move on the list, with no agent move to
	// suggest a skill for.
	require.Contains(t, out, "No agent move is legal right now; the moves above are yours.")
	require.Contains(t, out, "\"kind\": \"approve_feature\"")

	// The readout's ranked Next list answers the same tree the same way.
	route, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	_, next := splitAtNext(t, route)
	require.NotContains(t, next, "break-down", "guide and the readout answer one question one way")
	require.Contains(t, next, "1. You: approve the spec for form-engine/certification-form-loading (",
		"the plan-gate wait, not an invented seam")
}

// TestGuideNamesThePlanGateWait pins guide's half of the Acme fix: a tree
// whose feature sits specced awaiting `tiller approve feature` must say so
// through guide (skill stays orient: no skill fits a human wait, and
// re-orienting is always safe, but the reason names the actual gate), and
// the readout's Next must agree.
func TestGuideNamesThePlanGateWait(t *testing.T) {
	laySpeccableFeature(t)
	_, _, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"skill\": \"orient\"")
	require.Contains(t, out, "\"kind\": \"approve_feature\"")
	require.NotContains(t, out, "A feature is in draft",
		"the pending gate is never read as an invitation to open the next spec")

	route, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	require.Contains(t, route, "awaits your approval")
	require.Contains(t, route, "**Next:**")
}

// TestPlanGateClearsIntoExecuteOnApprove is the incident arc's second leg:
// the human's one approve moves the feature and its tasks to ready, and the
// route follows the work rather than lingering on a cleared gate.
func TestPlanGateClearsIntoExecuteOnApprove(t *testing.T) {
	laySpeccableFeature(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	// The route below has to land on jc's own task (birth-unclaimed: laySpeccableFeature's
	// --contributor jc no longer makes that true by itself).
	claimTaskAsOwner(t, cliFeatureKey+"/transcribe-the-schema", "jc")
	mustRun(t, "approve", "epic", "form-engine", "--contributor", "jc")
	_, _, code := execCLI(t, "spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"skill\": \"orient\"", "the gate holds the route before the approve")
	require.Contains(t, out, "\"kind\": \"approve_feature\"")

	_, _, code = execCLI(t, "approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code = execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"skill\": \"execute\"", "the approved plan's ready task is the route now")
}

// layApprovedPlanAheadOfADraftFeature lays the tree the two routing surfaces
// used to answer differently for: one approved feature plan whose task is ready,
// and one later feature (by key, so the dependency-free sort puts it second)
// still in draft. Every seam the walk can name is reachable on it, which is why
// it is worth the setup.
func layApprovedPlanAheadOfADraftFeature(t *testing.T) {
	t.Helper()
	laySpeccableFeature(t)
	// The route this fixture exists to test has to land on jc's own ready
	// task (birth-unclaimed: laySpeccableFeature's --contributor jc no longer
	// makes that true by itself).
	claimTaskAsOwner(t, cliFeatureKey+"/transcribe-the-schema", "jc")
	for _, argv := range [][]string{
		{"approve", "epic", "form-engine", "--contributor", "jc"},
		{"spec", "feature", "form-engine/certification-form-loading", "--contributor", "jc"},
		{"approve", "feature", "form-engine/certification-form-loading", "--contributor", "jc"},
		{"add", "feature", "form-engine/zebra-reporting",
			"--title", "Zebra reporting", "--change-type", "create", "--contributor", "jc"},
	} {
		_, _, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
	}
}

// TestGuideAndTheReadoutRouteOneTreeOneWay pins that `tiller guide` and the
// readout's ranked Next list agree rather than happening to agree on easy
// trees. The tree here is the one that separated them: an approved
// plan with a ready task, plus a later draft feature. A frontier-first walk
// answers "specify" on it and an execute-first walk answers "execute", so a
// surface running its own walk shows up here as a disagreement.
//
// The seam both name is execute, which is also the answer that does not send an
// agent to open work downstream while an approved task sits ready (the
// reader's own work ranks first).
func TestGuideAndTheReadoutRouteOneTreeOneWay(t *testing.T) {
	layApprovedPlanAheadOfADraftFeature(t)
	// The execute arm names only work the acting contributor holds (views.routableTask),
	// and the fixture claims its task as jc, so the route has to be read as
	// jc for the precedence this test is about to be reachable at all.
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"skill\": \"execute\"")

	route, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	_, next := splitAtNext(t, route)
	require.True(t, strings.HasPrefix(next, "1. Agent: continue "), "guide and the readout answer one question one way: %s", next)
	require.Less(t, strings.Index(next, "continue"), strings.Index(next, "specify"), "the task in hand outranks the draft")

	tid := soleTaskID(t)
	for _, verb := range []string{"start", "check"} {
		_, _, code = execCLI(t, verb, "task", tid, "--contributor", "jc")
		require.Equal(t, 0, code)
	}
	checkedGuide, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	suggestion := jsonData(t, checkedGuide)
	require.Equal(t, "execute", suggestion["skill"])
	require.Contains(t, suggestion["reason"], tid)
	checkedReadout, _, code := execCLI(t, "readout", "execute")
	require.Equal(t, 0, code)
	require.Contains(t, checkedReadout, tid)
	require.Contains(t, checkedReadout, "Current submissions recorded: 0 of 1")
}

// TestAddVerbsRefuseAParentAddressedByID is the behavior orient's node-reference
// convention now states, asserted against the binary rather than taken on faith.
// The convention used to say bare ids are ALWAYS valid in command position; the
// add verbs parse their argument as the key path the new node will have, so a
// parent named by id is a malformed key, not a lookup. This is the one place an
// agent is most likely to try it, having just read the parent's id off the
// confirmation line `add` itself printed.
func TestAddVerbsRefuseAParentAddressedByID(t *testing.T) {
	laySpeccableFeature(t)

	epicID := extractID(t, mustJSON(t, "show", "epic", "form-engine"), "E-")
	featureID := extractID(t, mustJSON(t, "show", "feature", "form-engine/certification-form-loading"), "F-")

	_, errOut, code := execCLI(t, "add", "feature", epicID+"/by-id", "--contributor", "jc")
	require.Equal(t, 2, code, "an id in the epic position is misuse")
	require.Contains(t, errOut, "must be <epic-key>/<feature-key>")

	_, errOut, code = execCLI(t, "add", "task", featureID+"/by-id", "--title", "X", "--contributor", "jc")
	require.Equal(t, 2, code, "an id in the feature position is misuse")
	require.Contains(t, errOut, "must be <epic-key>/<feature-key>/<task-key>")

	// The other half of the rule, so this test cannot pass by ids being broken
	// everywhere: a verb addressing an EXISTING node takes the id happily.
	_, _, code = execCLI(t, "show", "feature", featureID)
	require.Equal(t, 0, code, "an existing node still takes a bare id")
	_, _, code = execCLI(t, "link", "feature", featureID, "--depends-on", epicID, "--contributor", "jc")
	require.Equal(t, 0, code, "including in a mutating verb and in a --depends-on target")
}

// mustJSON runs a read command with --json and requires it to succeed.
func mustJSON(t *testing.T, argv ...string) string {
	t.Helper()
	out, _, code := execCLI(t, append(argv, "--json")...)
	require.Equalf(t, 0, code, "%v", argv)
	return out
}

// TestGuideNamesTheAmendmentWait is the amendment gate's half of the same
// contract the plan-gate test above pins. A proposed amendment is a human move
// on the ranked list, so it reaches a skill-less harness through the printed
// list while guide suggests orient. The tree here is a draft feature under a
// draft epic: opening a new spec is not legal, because the epic's decomposition
// and the amendment both await the human.
func TestGuideNamesTheAmendmentWait(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "form-engine-rework", "Restructure the form engine")

	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"skill\": \"orient\"", "no skill fits a human wait; the router stays the answer")
	require.Contains(t, out, "\"kind\": \"approve_amendment\"")
	require.Contains(t, out, "\"ref\": \"form-engine-rework ("+aid+")\"",
		"the list names the amendment by its reference")
	require.NotContains(t, out, "A feature is in draft",
		"the staged reshape is never read as an invitation to spec the next feature")

	route, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	_, next := splitAtNext(t, route)
	require.Contains(t, next, "1. You: approve amendment form-engine-rework ("+aid+").",
		"guide and the readout answer one question one way")
}

// TestInitGeneratesOrientationByteStable is the committed-artifact guarantee:
// init writes .tiller/orientation.md, and a second init produces a byte-identical
// file (distribution.md, orientation.md is schema-keyed and re-emits the same
// bytes).
func TestInitGeneratesOrientationByteStable(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)
	orientPath := filepath.Join(dir, ".tiller", "orientation.md")
	require.FileExists(t, orientPath)
	first, err := os.ReadFile(orientPath)
	require.NoError(t, err)
	require.Contains(t, string(first), "Do not hand-edit")

	// A second init re-emits the same bytes: no git churn.
	_, _, code = execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)
	second, err := os.ReadFile(orientPath)
	require.NoError(t, err)
	require.Equal(t, string(first), string(second), "orientation.md must be byte-stable across inits")
}

func TestInitAndJoinDoNotCreateRetiredBoard(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)

	require.NoFileExists(t, filepath.Join(dir, ".tiller", "board.md"))

	registerContributor(t, "jc", "JC Grubbs")
	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	require.NoFileExists(t, filepath.Join(dir, ".tiller", "board.md"))
	require.FileExists(t, filepath.Join(dir, ".tiller", "orientation.md"))
}

// twoOwnerManifest is the roster the cross-owner routing fixture needs: two
// humans who can hold work, which every other manifest in this package lacks.
const twoOwnerManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
  - { id: dana, name: Dana }
gates:
  required:
    build: { kind: command, run: "true", on: [feature, task] }
`

// layForeignTaskAndOwnFrontier lays the tree the earlier lens built: dana holds
// one in_progress task under epic platform, and jc owns four draft features
// under epic reporting with nothing specified. It returns the repo root, and the
// checkout carries no identity, so each test declares whose checkout it is.
func layForeignTaskAndOwnFrontier(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testgit.Init(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"), []byte(twoOwnerManifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")
	t.Chdir(dir)

	for _, argv := range [][]string{
		{"add", "epic", "platform", "--title", "Platform", "--contributor", "dana"},
		{"add", "feature", "platform/auth", "--title", "Auth", "--change-type", "create", "--contributor", "dana"},
		{"approve", "epic", "platform", "--contributor", "dana"},
	} {
		_, _, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
	}
	authDir := filepath.Join(dir, ".tiller", "epics", "platform", "features", "auth")
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "feature.md"),
		[]byte("# Auth\n\nWire the auth form.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: It authenticates\n    polarity: introduce\n    binding: { type: test, ref: spec/auth_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
	_, _, code := execCLI(t, "add", "task", "platform/auth/wire-the-form", "--title", "Wire the form", "--contributor", "dana")
	require.Equal(t, 0, code)
	claimTaskAsOwner(t, "platform/auth/wire-the-form", "dana")
	require.NoError(t, os.WriteFile(filepath.Join(authDir, store.TaskDirName, "wire-the-form", store.TaskProseFile),
		[]byte("# Wire the form\n\n## Approach\n\nThe implementation spec.\n"), 0o644))

	danaTask := taskIDByKey(t, "platform/auth/wire-the-form")
	for _, argv := range [][]string{
		{"spec", "feature", "platform/auth", "--contributor", "dana"},
		{"approve", "feature", "platform/auth", "--contributor", "dana"},
		{"start", "task", danaTask, "--contributor", "dana"},
		// jc's own lane: an epic of features nobody has specified yet.
		{"add", "epic", "reporting", "--title", "Reporting", "--contributor", "jc"},
		{"approve", "epic", "reporting", "--contributor", "jc"},
	} {
		_, _, code := execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
	}
	for _, k := range []string{"alpha", "beta", "gamma", "delta"} {
		_, _, code := execCLI(t, "add", "feature", "reporting/"+k,
			"--title", k, "--change-type", "create", "--contributor", "jc")
		require.Equalf(t, 0, code, "add feature reporting/%s", k)
	}
	return dir
}

// TestAForeignTaskDoesNotSuppressTheActingOwnersFrontier is the earlier lens's
// case, and the cost of putting execute ahead of the frontier: the execute arm
// took the first ready-or-in_progress task tree-wide with no eligibility filter
// at all, so one task held by somebody else answered the route for everybody.
// While the frontier came first that cost nothing; once execute leads it
// suppresses the whole frontier.
//
// The line it produced was worse than a wrong route: it asserted to jc that
// dana's in-flight work was his, on a tree where jc's own four features are
// waiting to be specified and were not named anywhere in the block.
func TestAForeignTaskDoesNotSuppressTheActingOwnersFrontier(t *testing.T) {
	layForeignTaskAndOwnFrontier(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	out, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "wire-the-form", "dana's task is not jc's move")
	_, next := splitAtNext(t, out)
	for _, k := range []string{"alpha", "beta", "gamma", "delta"} {
		require.Contains(t, next, "Agent: specify reporting/"+k+" (", "jc's own frontier is reachable again")
	}

	guide, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, guide, "\"skill\": \"specify\"", "the other routing surface agrees, as always")

	// And the census still reports the in-flight task, so the work is absent
	// from jc's route without being hidden from jc's picture.
	require.Contains(t, out, "Tasks by state: 1 in_progress")
}

// TestTheActingOwnersOwnTaskStillRoutesToExecute is the other side of the same
// filter: on dana's checkout the identical tree routes to dana's task, so the
// filter narrows the arm rather than disabling it.
func TestTheActingOwnersOwnTaskStillRoutesToExecute(t *testing.T) {
	layForeignTaskAndOwnFrontier(t)
	t.Setenv("TILLER_CONTRIBUTOR", "dana")

	out, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	_, next := splitAtNext(t, out)
	require.Contains(t, next, "1. Agent: continue platform/auth/wire-the-form (")

	guide, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, guide, "\"skill\": \"execute\"")
}

// TestAQuestionBlockedTaskDoesNotHoldTheRoute pins the shared routing decision
// through its readout and guide projections. Recorded feature order leads with
// a task that the real command guard refuses, then a later feature supplies a
// legal task. Routing keeps that selection while explaining the first skip.
func TestAQuestionBlockedTaskDoesNotHoldTheRoute(t *testing.T) {
	layForeignTaskAndOwnFrontier(t)
	t.Setenv("TILLER_CONTRIBUTOR", "dana")

	heldID := taskIDByKey(t, "platform/auth/wire-the-form")
	legalFeatureKey := "platform/legal-route"
	_, _, code := execCLI(t, "add", "feature", legalFeatureKey,
		"--title", "Legal route", "--change-type", "create", "--contributor", "dana")
	require.Equal(t, 0, code)

	legalFeatureDir := filepath.Join(".tiller", "epics", "platform", "features", "legal-route")
	require.NoError(t, os.WriteFile(filepath.Join(legalFeatureDir, "feature.md"),
		[]byte("# Legal route\n\nKeep later work independently routable.\n"), 0o644))
	legalFeatureID := featureIDByKey(t, legalFeatureKey)
	require.NoError(t, os.WriteFile(filepath.Join(legalFeatureDir, "feature.eval.md"),
		[]byte("---\nnode: "+legalFeatureID+"\ncriteria:\n  - id: C1\n    statement: Later work stays routable\n    polarity: introduce\n    binding: { type: test, ref: internal/cli/guide_test.go }\n---\n\n# Acceptance\n"), 0o644))

	legalKey := legalFeatureKey + "/alpha-legal"
	_, _, code = execCLI(t, "add", "task", legalKey, "--title", "Legal follow-up", "--contributor", "dana")
	require.Equal(t, 0, code)
	claimTaskAsOwner(t, legalKey, "dana")
	legalDir := filepath.Join(legalFeatureDir, store.TaskDirName, "alpha-legal")
	require.NoError(t, os.WriteFile(filepath.Join(legalDir, store.TaskProseFile),
		[]byte("# Legal follow-up\n\n## Approach\n\nImplement the later legal task.\n"), 0o644))
	for _, argv := range [][]string{
		{"spec", "feature", legalFeatureKey, "--contributor", "dana"},
		{"approve", "feature", legalFeatureKey, "--contributor", "dana"},
		{"order", "feature", "platform/auth", legalFeatureKey, "--contributor", "dana"},
	} {
		_, _, code = execCLI(t, argv...)
		require.Equalf(t, 0, code, "%v", argv)
	}

	type questionFact struct {
		id        string
		title     string
		statement string
	}
	open := make([]questionFact, 0, 3)
	for _, q := range []struct {
		title     string
		statement string
		block     string
	}{
		{
			title:     "Authentication provider",
			statement: strings.Repeat("This is the full authentication-provider question statement. ", 6),
			block:     heldID,
		},
		{
			title:     "Session boundary",
			statement: strings.Repeat("This is the full session-boundary question statement. ", 6),
			block:     "platform/auth",
		},
		{
			title:     "Token migration",
			statement: strings.Repeat("This is the full token-migration question statement. ", 6),
			block:     "platform/auth",
		},
	} {
		qout, _, qcode := execCLI(t, "add", "question", q.statement,
			"--title", q.title, "--blocks", q.block, "--contributor", "dana", "--json")
		require.Equal(t, 0, qcode)
		open = append(open, questionFact{id: extractID(t, qout, "Q-"), title: q.title, statement: q.statement})
	}
	sort.Slice(open, func(i, j int) bool { return open[i].id < open[j].id })

	answeredOut, _, code := execCLI(t, "add", "question",
		"This answered statement must not hold or appear in the route.",
		"--title", "Answered route detail", "--blocks", heldID, "--contributor", "dana", "--json")
	require.Equal(t, 0, code)
	answeredID := extractID(t, answeredOut, "Q-")
	_, _, code = execCLI(t, "answer", "question", answeredID,
		"--answer", "The route can proceed.", "--contributor", "dana")
	require.Equal(t, 0, code)

	// The leading task is still refused by the real work command.
	_, _, code = execCLI(t, "check", "task", heldID, "--contributor", "dana")
	require.Equal(t, 1, code)

	// The surface lists the holding questions and the legal task as moves, never
	// the held task, an answered question, or a full statement.
	assertRanked := func(surface string) {
		t.Helper()
		require.Contains(t, surface, "Agent: continue "+legalKey+" (")
		for _, q := range open {
			require.Contains(t, surface, "You: answer "+q.id+".")
			require.NotContains(t, surface, q.statement,
				"the list uses bounded titles, never full statements")
		}
		require.NotContains(t, surface, "Agent: continue platform/auth/wire-the-form (", "the held task is not a move")
		require.NotContains(t, surface, answeredID)
		require.NotContains(t, surface, "This answered statement must not hold")
	}

	// The readout lists the holding questions and the legal task as moves.
	out, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	_, next := splitAtNext(t, out)
	require.Contains(t, next, "Agent: continue "+legalKey+" (")
	for _, q := range open {
		require.Contains(t, next, "You: answer "+q.id+".")
	}
	require.NotContains(t, next, "Agent: continue platform/auth/wire-the-form (", "the held task is not a move")
	require.NotContains(t, next, "check task", "nor is checking it")
	require.NotContains(t, out, answeredID)

	guide, _, code := execCLI(t, "guide")
	require.Equal(t, 0, code)
	require.Contains(t, guide, "Suggested skill: execute")
	assertRanked(guide)

	guideJSON, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	suggestion := jsonData(t, guideJSON)
	require.Equal(t, "execute", suggestion["skill"])
	require.Contains(t, suggestion["reason"], legalKey, "the suggestion names the legal task, not the held one")
	require.NotContains(t, suggestion["reason"], "platform/auth/wire-the-form")
}

// TestAnUnscopedRouteClaimsNoOwnership pins the earlier finding with the case
// the coordinator drove: TILLER_CONTRIBUTOR naming somebody who is not in the roster
// at all. The execute arm matches nothing for them, which is correct, so the
// route falls to the frontier, and the frontier arm tests ownership against
// nothing. Appending the clause there told that reader a feature jc added was
// theirs.
//
// The ranked list names who makes each move (an agent specifies a draft) and
// never says whose the work is. Only the reader's own task reads as theirs,
// because Task.Owner says so.
func TestAnUnscopedRouteClaimsNoOwnership(t *testing.T) {
	layForeignTaskAndOwnFrontier(t)

	for _, owner := range []string{"nobody", "jc"} {
		t.Setenv("TILLER_CONTRIBUTOR", owner)
		out, _, code := execCLI(t, "readout", "orient")
		require.Equalf(t, 0, code, "owner %s", owner)
		_, next := splitAtNext(t, out)
		require.Containsf(t, next, "Agent: specify reporting/alpha (", "owner %s", owner)
		require.NotContainsf(t, next, "Your task", "owner %s: dana's task is not this reader's", owner)
		require.NotContainsf(t, out, "wire-the-form", "owner %s", owner)
	}

	t.Setenv("TILLER_CONTRIBUTOR", "dana")
	out, _, code := execCLI(t, "readout", "orient")
	require.Equal(t, 0, code)
	_, next := splitAtNext(t, out)
	require.Contains(t, next, "1. Agent: continue platform/auth/wire-the-form (")
	require.Contains(t, next, "Your task is in progress.")
}

// The coarse finish line remains execute work and names the reviewed feature,
// rather than pretending a task is still ready to start.
func TestGuideNamesTheCoarseFinishLineReason(t *testing.T) {
	initOwnedRepo(t)
	walkFeatureToActive(t)
	_, _, code := execCLI(t, "configure", "feature", cliFeatureKey, "--review-grain", "feature", "--contributor", "jc")
	require.Equal(t, 0, code)
	tid := soleTaskID(t)
	_, _, code = execCLI(t, "start", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	checkoutTaskBranchCLI(t, tid)
	_, _, code = execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "submit", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code)
	assembleTaskForReviewCLI(t, tid, soleFeatureBranch(t))
	mustRun(t, "check", "feature", cliFeatureKey, "--contributor", "jc")

	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	out, _, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"skill\": \"execute\"")
	require.Contains(t, out, "submit feature", "the coarse finish line's own next step is named")
}

// TestGuidePrintsTheListFirst pins bare guide's shape: the same ranked block
// `tiller next` prints, a blank line, then the suggestion, with the list also
// carried in the JSON payload.
func TestGuidePrintsTheListFirst(t *testing.T) {
	layApprovedPlanAheadOfADraftFeature(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	human, stderr, code := execCLI(t, "guide")
	require.Equal(t, 0, code, stderr)
	next, _, code := execCLI(t, "next")
	require.Equal(t, 0, code)
	require.True(t, strings.HasPrefix(human, strings.TrimRight(next, "\n")+"\n\nSuggested skill: execute\n"),
		"the list comes first, then a blank line, then the suggestion: %s", human)
	require.Contains(t, human, "Run `tiller guide execute` to load it.")

	structured, stderr, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	nextData, ok := jsonData(t, structured)["next"].(map[string]any)
	require.True(t, ok, "the payload carries the ranked list")
	require.Equal(t, "jc", nextData["reader"])
	candidates, ok := nextData["candidates"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, candidates)
}

// TestGuideSuggestsTheTopAgentMoveBelowHumanMoves pins that the top
// ranked move is often the human's, so the skill comes from the highest agent
// move beneath it. A second feature awaiting the reader's approval ranks above
// the reader's own claimed task, and the suggestion is still execute.
func TestGuideSuggestsTheTopAgentMoveBelowHumanMoves(t *testing.T) {
	layApprovedPlanAheadOfADraftFeature(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	zebraDir := filepath.Join(featureDir(t), "..", "zebra-reporting")
	require.NoError(t, os.WriteFile(filepath.Join(zebraDir, "feature.md"),
		[]byte("# Zebra reporting\n\nReport on the zebras.\n"), 0o644))
	zebraID := featureIDByKey(t, "form-engine/zebra-reporting")
	require.NoError(t, os.WriteFile(filepath.Join(zebraDir, "feature.eval.md"),
		[]byte("---\nnode: "+zebraID+"\ncriteria:\n  - id: C1\n    statement: Zebras report\n    polarity: introduce\n    binding: { type: test, ref: spec/zebra_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
	mustRun(t, "add", "task", "form-engine/zebra-reporting/count-zebras", "--title", "Count zebras", "--contributor", "jc")
	require.NoError(t, os.WriteFile(filepath.Join(zebraDir, store.TaskDirName, "count-zebras", store.TaskProseFile),
		[]byte("# Count zebras\n\n## Approach\n\nThe implementation spec.\n"), 0o644))
	mustRun(t, "spec", "feature", "form-engine/zebra-reporting", "--contributor", "jc")

	out, stderr, code := execCLI(t, "guide", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	candidates := data["next"].(map[string]any)["candidates"].([]any)
	top := candidates[0].(map[string]any)
	require.Equal(t, "approve_feature", top["kind"], "the human's approval ranks first: %s", out)
	require.Equal(t, "human", top["actor"])
	require.Equal(t, "execute", data["skill"], "the suggestion skips the human move for the agent's")
	require.Contains(t, data["reason"], "form-engine/certification-form-loading/transcribe-the-schema")
}
