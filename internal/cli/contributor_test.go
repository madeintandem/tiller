package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// TestContributorAddRegistersAFreshContributor is the core acceptance: `add contributor <id>
// --name "<display>"` on a repo with an empty roster puts the person on
// tiller.yml's committed contributors block, and no `tiller join` is required to do
// it.
func TestContributorAddRegistersAFreshContributor(t *testing.T) {
	dir := initRepo(t)

	out, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs")
	require.Equal(t, 0, code, "add contributor registers a fresh id")
	require.Contains(t, out, "Registered jc (JC Grubbs)", "summary names who was registered")

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 1)
	require.Equal(t, "jc", m.Contributors[0].ID)
	require.Equal(t, "JC Grubbs", m.Contributors[0].Name)

	// join is not needed for the roster write: no identity.yml exists yet.
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "identity.yml"),
		"add contributor records no identity; that is still join's job")
}

// TestContributorAddUpdatesAnExistingName confirms re-adding a registered id with a
// different --name updates the display name in place rather than appending a
// duplicate row.
func TestContributorAddUpdatesAnExistingName(t *testing.T) {
	dir := initRepo(t)
	_, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Updated jc's name to \"JC Grubbs\"", "summary names the update")

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 1, "the name update never appends a second row")
	require.Equal(t, "JC Grubbs", m.Contributors[0].Name)
}

// TestContributorAddSameNameIsANoOp confirms re-adding an id with the identical name
// is an idempotent no-op that says so, rather than a silent success
// indistinguishable from a real registration.
func TestContributorAddSameNameIsANoOp(t *testing.T) {
	dir := initRepo(t)
	_, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs")
	require.Equal(t, 0, code)

	before, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
	require.NoError(t, err)

	out, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs")
	require.Equal(t, 0, code, "a no-op re-add still succeeds")
	require.Contains(t, out, "already on the roster", "the summary names the no-op")

	after, err := os.ReadFile(filepath.Join(dir, ".tiller", "tiller.yml"))
	require.NoError(t, err)
	require.Equal(t, before, after, "an identical re-add writes nothing")
}

// TestContributorAddSecondContributorKeepsFirst confirms a second registration appends
// without disturbing the first, the same order-preserving upsert the roster
// has always carried (store.AddContributor), now reached only through this command.
func TestContributorAddSecondContributorKeepsFirst(t *testing.T) {
	dir := initRepo(t)
	_, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "contributor", "bob", "--name", "Bob")
	require.Equal(t, 0, code)

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 2)
	require.Equal(t, "jc", m.Contributors[0].ID, "the first registration is preserved")
	require.Equal(t, "bob", m.Contributors[1].ID, "the second is appended")
}

// TestContributorAddRefusesBlankIDOrName confirms both <id> and --name must be
// non-blank: this is the registration gate the incident this design closes
// depends on, so it refuses rather than defaults either half quietly. Each
// half has its own reason token (blank_contributor_id, blank_contributor_name), pinned
// per case rather than sharing one coarse token, the package's usual
// convention (init's invalid_name, invalid_key, invalid_kind, for one).
func TestContributorAddRefusesBlankIDOrName(t *testing.T) {
	initRepo(t)

	out, _, code := execCLI(t, "add", "contributor", "jc", "--json")
	require.Equal(t, 2, code, "a blank --name is misuse")
	require.Contains(t, out, "\"reason\": \"blank_contributor_name\"")

	out, _, code = execCLI(t, "add", "contributor", " ", "--name", "JC Grubbs", "--json")
	require.Equal(t, 2, code, "a blank id is misuse")
	require.Contains(t, out, "\"reason\": \"blank_contributor_id\"")
}

// TestContributorAddOutsideRepoExitsThree confirms add contributor refuses outside a
// Tiller repo with the environment exit code, before it writes anything, the
// same not_a_repo posture every other mutating command carries.
func TestContributorAddOutsideRepoExitsThree(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	out, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs", "--json")
	require.Equal(t, 3, code, "add contributor outside a Tiller repo is an environment failure")
	require.Contains(t, out, "not_a_repo")
}

// TestContributorAddPreservesRosterEditAfterTreeLoad simulates a human roster edit
// landing after mutate.AddContributor's stable Tree.Load but before its manifest
// snapshot. The contributor merge must derive from the exact bytes protected by that
// snapshot, not the tree's older in-memory roster. This relocates the coverage
// TestJoinOwnerPreservesRosterEditAfterTreeLoad carried before registration
// moved out of `tiller join`.
func TestContributorAddPreservesRosterEditAfterTreeLoad(t *testing.T) {
	dir := initRepo(t)
	tree, err := store.Load(dir)
	require.NoError(t, err)

	manifestPath := filepath.Join(dir, ".tiller", "tiller.yml")
	before, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	humanOwner := store.Contributor{ID: "pat", Name: "Pat Nguyen"}
	humanEdit, err := store.RenderContributors(before, []store.Contributor{humanOwner})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, humanEdit, 0o644))

	registeredOwner := store.Contributor{ID: "jc", Name: "JC Grubbs"}
	res, err := mutate.AddContributor(tree, mutate.Deps{}, registeredOwner)
	require.NoError(t, err)
	require.Equal(t, mutate.ContributorAddRegistered, res.Outcome)
	require.Equal(t, []store.Contributor{humanOwner, registeredOwner}, res.Roster)

	onDisk, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Equal(t, []store.Contributor{humanOwner, registeredOwner}, onDisk.Contributors)
}

// TestContributorAddRecordsEvent confirms add contributor now writes the contributor.add
// event it should always have written (design doc, "The verbs": "add contributor
// gains the event it should always have written"). The envelope command
// field is "add contributor", not the bare "owner" it used to carry, since a
// second sub-verb (remove contributor) makes the distinction a real one.
func TestContributorAddRecordsEvent(t *testing.T) {
	dir := initRepo(t)
	out, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, `"command": "add contributor"`)
	data := jsonData(t, out)
	require.Equal(t, map[string]any{"id": "jc", "name": "JC Grubbs"}, data["contributor"])
	require.NotContains(t, data, "owner")
	require.NotContains(t, data, "actor", "bootstrap registration needs no selected actor")

	tree, err := store.Load(dir)
	require.NoError(t, err)
	evs, err := events.List(tree)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, "contributor.add", evs[0].Type)
	require.Equal(t, "jc", evs[0].Contributor, "the event is about who joined; there is no distinct actor for a bootstrap act")
	require.Contains(t, evs[0].Targets, "jc")
	require.Equal(t, "JC Grubbs", evs[0].After["name"])
}

// --- remove contributor ---

func TestContributorRemoveRetiresARegisteredContributorAndRecordsEvent(t *testing.T) {
	dir := initRepo(t)
	_, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "contributor", "pat", "--name", "Pat Nguyen")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "remove", "contributor", "pat", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, `"command": "remove contributor"`)
	data := jsonData(t, out)
	removed := data["removed"].(map[string]any)
	require.Equal(t, "pat", removed["id"])

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 1, "pat's row is gone")
	require.Equal(t, "jc", m.Contributors[0].ID)

	tree, err := store.Load(dir)
	require.NoError(t, err)
	evs, err := events.List(tree)
	require.NoError(t, err)
	last := evs[len(evs)-1]
	require.Equal(t, "contributor.remove", last.Type)
	require.Equal(t, "jc", last.Contributor, "the event's acting contributor is who removed pat, not pat")
	require.Contains(t, last.Targets, "pat")
}

// TestContributorRemoveRefusesSelfRemoval pins the design's own no-self-lockout
// rule: the acting contributor cannot remove themselves.
func TestContributorRemoveRefusesSelfRemoval(t *testing.T) {
	dir := initRepo(t)
	_, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "remove", "contributor", "jc", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"reason": "cannot_remove_self"`)

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 1, "the refusal writes nothing")
}

func TestContributorRemoveRefusesAnUnknownID(t *testing.T) {
	initRepo(t)
	_, _, code := execCLI(t, "add", "contributor", "jc", "--name", "JC Grubbs")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "remove", "contributor", "nobody", "--contributor", "jc", "--json")
	require.Equal(t, 3, code, out)
	require.Contains(t, out, `"reason": "contributor_unknown"`)
}

// A live task reservation prevents removing its owner from the roster.
func TestContributorRemoveRefusesLiveHoldingsNamingCounts(t *testing.T) {
	layTwoOwnerFeature(t)
	mustRun(t, "add", "task", cliTaskKey, "--title", "Transcribe the schema", "--contributor", "jc")
	_, _, code := execCLI(t, "claim", "task", cliTaskKey, "--contributor", "pat")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "remove", "contributor", "pat", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	data := jsonData(t, out)
	require.Equal(t, "contributor_holds_live_work", data["reason"])
	require.Equal(t, float64(1), data["tasks"])
	require.Equal(t, float64(0), data["questions"])

}

// Removing a contributor requires clearing both the live task and open question.
func TestContributorRemoveEndToEndBlockedThenUnblockedByReassignAndAnswer(t *testing.T) {
	layTwoOwnerFeature(t)
	_, _, code := execCLI(t, "add", "task", cliFeatureKey+"/transcribe-the-schema",
		"--title", "Transcribe the schema", "--contributor", "pat")
	require.Equal(t, 0, code)
	claimTaskAsOwner(t, cliTaskKey, "pat")
	_, _, code = execCLI(t, "add", "question", "Which schema version is canonical?", "--title", "Canonical schema version", "--contributor", "pat")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "remove", "contributor", "pat", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	data := jsonData(t, out)
	require.Equal(t, float64(1), data["tasks"])
	require.Equal(t, float64(1), data["questions"])

	_, _, code = execCLI(t, "reassign", "task", cliTaskKey, "--to", "jc", "--reason", "pat is leaving the team", "--contributor", "jc")
	require.Equal(t, 0, code)

	tree := loadCwdTree(t)
	require.Len(t, tree.Questions, 1)
	var qid string
	for id := range tree.Questions {
		qid = id
	}
	_, _, code = execCLI(t, "answer", "question", qid, "--answer", "v2 is canonical.", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code = execCLI(t, "remove", "contributor", "pat", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out, "both remedies landed: task reassigned and question answered")
}

// TestContributorAddRemoveSurviveManifestLayouts drives add and remove through the
// CLI against hand-formatted rosters that a rewrite keyed on indentation alone
// corrupts: an indentless sequence and a flow sequence, each followed by a
// comment-led key. The roster must end exactly as asked and the neighbors survive.
func TestContributorAddRemoveSurviveManifestLayouts(t *testing.T) {
	for name, roster := range map[string]string{
		"indentless": "contributors:\n- { id: jc, name: JC }\n\n- { id: pat, name: Pat }\n",
		"flow":       "contributors: [{ id: jc, name: JC }, { id: pat, name: Pat }]\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := initRepo(t)
			path := filepath.Join(dir, ".tiller", "tiller.yml")
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			text := string(raw)
			start := strings.Index(text, "contributors:")
			require.GreaterOrEqual(t, start, 0)
			rest := text[start:]
			nl := strings.Index(rest, "\n")
			body := text[:start] + roster + "# keep me\n" + rest[nl+1:]
			require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

			out, _, code := execCLI(t, "add", "contributor", "sam", "--name", "Sam", "--json")
			require.Equal(t, 0, code, out)
			out, _, code = execCLI(t, "remove", "contributor", "pat", "--contributor", "jc", "--json")
			require.Equal(t, 0, code, out)

			m, err := store.LoadManifest(dir)
			require.NoError(t, err)
			require.Equal(t, []store.Contributor{{ID: "jc", Name: "JC"}, {ID: "sam", Name: "Sam"}}, m.Contributors)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(after), "# keep me\n")
		})
	}
}

// TestNullQuestionEntryFailsFullTreeReadWithEnvelope proves a null entry in the
// questions ledger reaches the user as an ordinary JSON error, not a panic, and
// that the failed read leaves the ledger and event log as they were.
func TestNullQuestionEntryFailsFullTreeReadWithEnvelope(t *testing.T) {
	dir := initRepo(t)
	ledger := filepath.Join(dir, ".tiller", "decisions", "open-questions.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(ledger), 0o755))
	require.NoError(t, os.WriteFile(ledger, []byte("questions:\n  - null\n"), 0o644))

	var out string
	var code int
	require.NotPanics(t, func() { out, _, code = execCLI(t, "status", "--json") })
	require.NotEqual(t, 0, code)
	require.Contains(t, out, `"ok": false`)
	require.Contains(t, out, "open-questions.yml[0]")

	after, err := os.ReadFile(ledger)
	require.NoError(t, err)
	require.Equal(t, "questions:\n  - null\n", string(after))
}

// TestContributorAddRefusedRenderWritesNothing drives a roster rewrite that the
// post-render check must refuse (another key aliases an anchor on the roster) and
// asserts the manifest bytes and event log are exactly as before.
func TestContributorAddRefusedRenderWritesNothing(t *testing.T) {
	dir := initRepo(t)
	path := filepath.Join(dir, ".tiller", "tiller.yml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	body := strings.Replace(string(raw), "contributors: []", "contributors: &roster\n  - { id: jc, name: JC }", 1) + "\nshared_roster: *roster\n"
	require.NotEqual(t, string(raw), body)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	tree, err := store.Load(dir)
	require.NoError(t, err)
	eventsBefore, err := events.List(tree)
	require.NoError(t, err)

	out, _, code := execCLI(t, "add", "contributor", "sam", "--name", "Sam", "--json")
	require.NotEqual(t, 0, code, out)
	require.Contains(t, out, `"ok": false`)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, body, string(after))
	tree, err = store.Load(dir)
	require.NoError(t, err)
	eventsAfter, err := events.List(tree)
	require.NoError(t, err)
	require.Equal(t, eventsBefore, eventsAfter)
}
