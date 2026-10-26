package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// This file drives the real `tiller drift` command over scratch projects
// (the CLI test idiom internal/cli/lifecycle_cmds_test.go and e2e_done_test.go
// already use): a real temp git repo, a hand-authored manifest, and the exact
// command flow an agent or a human would run, never a fixture that hand-sets
// a pin or a completion record the real commands are supposed to populate
// (the one deliberate exception, markFeatureDone below, is exactly the "hand-
// write a done feature" fixture the drift design asks for).

// allDriftChecks enables every marker the schema plants, for a test that
// wants the full sweep and needs to confirm no OTHER marker also fires.
const allDriftChecks = "stale_spec, active_basis, answered_question, missing_surface, orphan_eval"

// driftManifestTemplate is the minimal owner-bearing manifest (add_test.go's
// own ownedManifest) plus a drift.checks block a caller fills in, since an
// absent drift: block reads as "every check disabled" (drift.go's own
// enabledSet doc comment) and every test in this file needs to control its
// sweep scope deliberately.
const driftManifestTemplate = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
drift:
  checks: [%s]
`

// initDriftGitRepo lays a temp git repo with one commit and a manifest whose
// drift.checks carries exactly checks (comma-joined marker names), then
// chdirs into it. The one commit exists because approve pins HEAD as the
// accepted basis's commit anchor, and HEAD cannot resolve before a first
// commit exists (mirrors check_test.go's own initGatedGitRepo).
func initDriftGitRepo(t *testing.T, checks string) string {
	t.Helper()
	dir := t.TempDir()
	testgit.Init(t, dir)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"),
		[]byte(fmt.Sprintf(driftManifestTemplate, checks)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")

	t.Chdir(dir)
	return dir
}

// driftFeatureDir is the on-disk folder of the one fixture feature
// layDriftFeature lays under dir.
func driftFeatureDir(dir string) string {
	return filepath.Join(dir, ".tiller", "epics", "form-engine", "features", "certification-form-loading")
}

const driftFeatureKey = "form-engine/certification-form-loading"

// layDriftFeature adds one epic and one feature, writes a real spec and
// acceptance contract, adds one task with its own implementation spec, then
// runs the merged specify gate and the plan gate. It leaves the feature and
// its initial task both `ready` with a freshly pinned, clean accepted basis,
// the real command flow having populated AcceptedAgainst itself (mirrors
// lifecycle_cmds_test.go's own laySpeccableFeature): nothing here hand-sets a
// pin, which is what makes a later edit to feature.md an honest active_basis
// drift rather than a fixture artifact.
func layDriftFeature(t *testing.T, dir string) {
	t.Helper()
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", driftFeatureKey,
		"--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "epic", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code)

	fdir := driftFeatureDir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(fdir, "feature.md"),
		[]byte("# Certification form loading\n\nLoad the CERT-LTI-02-A form from its JSON Schema and render it.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fdir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: The form loads\n"+
			"    polarity: introduce\n    binding: { type: test, ref: spec/form_spec.rb }\n---\n\n# Acceptance\n"), 0o644))

	_, _, code = execCLI(t, "add", "task", driftFeatureKey+"/transcribe-the-schema",
		"--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NoError(t, os.WriteFile(filepath.Join(fdir, store.TaskDirName, "transcribe-the-schema", store.TaskProseFile),
		[]byte("# Transcribe the schema\n\n## Approach\n\nTranscribe the schema.\n"), 0o644))

	_, _, code = execCLI(t, "spec", "feature", driftFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "approve", "feature", driftFeatureKey, "--contributor", "jc")
	require.Equal(t, 0, code)
}

// markFeatureDone hand-edits one feature's status to done directly on disk,
// leaving verified_against absent, since `add` never writes one: this is the
// "hand-write a done feature with no
// verified_against into an otherwise clean tree", a node that reached done
// some way other than a real sync crossing and so carries no completion
// record for stale_spec to compare against. It is the one place in this file
// that does not drive a real command, deliberately: there is no verb that
// produces this state, which is exactly why it is Uncovered rather than
// clean.
func markFeatureDone(t *testing.T, dir, epicKey, featureLocal string) {
	t.Helper()
	path := filepath.Join(dir, ".tiller", "epics", epicKey, "features", featureLocal, "feature.yml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "status: draft", "add feature always lands in draft")
	out := strings.Replace(string(raw), "status: draft", "status: done", 1)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// driftEnvelope is the JSON shape every test in this file parses: just enough
// of DriftView's fields, typed loosely (candidates/uncovered as raw maps) so
// a test asserts the fields it actually cares about without pinning every
// row's full shape.
type driftEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Reason     string                      `json:"reason"`
		Candidates map[string][]map[string]any `json:"candidates"`
		Uncovered  []map[string]any            `json:"uncovered"`
		Disabled   []string                    `json:"disabled"`
		Exempted   map[string]int              `json:"exempted"`
		Coverage   map[string]map[string]int   `json:"coverage"`
		Compared   int                         `json:"compared"`
	} `json:"data"`
}

func parseDriftEnvelope(t *testing.T, out string) driftEnvelope {
	t.Helper()
	var env driftEnvelope
	require.NoError(t, json.Unmarshal([]byte(out), &env), "drift --json must parse: %s", out)
	return env
}

// TestDriftCleanAssertsCoverage drives a healthy, freshly approved feature and
// task through the whole five-marker sweep: exit 0, ok true, no candidates,
// no uncovered nodes, and the human output states what was compared rather
// than printing a bare "no drift", because exit 0 is itself an assertion of
// coverage (cli-surface.md, the drift paragraph).
func TestDriftCleanAssertsCoverage(t *testing.T) {
	dir := initDriftGitRepo(t, allDriftChecks)
	layDriftFeature(t, dir)

	out, errOut, code := execCLI(t, "drift", "--json")
	require.Equal(t, 0, code, "stdout=%s stderr=%s", out, errOut)

	env := parseDriftEnvelope(t, out)
	require.True(t, env.OK)
	require.Empty(t, env.Data.Candidates, "a clean sweep names no candidates")
	require.Empty(t, env.Data.Uncovered, "a clean sweep leaves no node uncovered")
	require.Empty(t, env.Data.Reason, "reason rides only on the blocked path")
	require.Greater(t, env.Data.Compared, 0, "the approved feature and its task were actually compared, not skipped in silence")

	// Coverage: active_basis swept the feature and its task (2 nodes checked),
	// answered_question swept the feature's empty assumption_basis (1 node
	// checked), and orphan_eval's one introduce criterion has not started its
	// clock yet (the feature is ready, not done). stale_spec and
	// missing_surface never touch a ready, surfaceless feature at all, so
	// neither bucket exists in the map, unlike being present with a zero
	// count (round-1 review, Finding 1: this exact shape was previously
	// asserted nowhere, so a regression to any of it still passed make test).
	require.Equal(t, map[string]int{"checked": 2}, env.Data.Coverage["active_basis"])
	require.Equal(t, map[string]int{"checked": 1}, env.Data.Coverage["answered_question"])
	require.Equal(t, map[string]int{"not_yet_in_scope": 1}, env.Data.Coverage["orphan_eval"])
	require.NotContains(t, env.Data.Coverage, "stale_spec")
	require.NotContains(t, env.Data.Coverage, "missing_surface")

	human, humanErr, code := execCLI(t, "drift")
	require.Equal(t, 0, code)
	require.Empty(t, humanErr, "a clean run's human output is not blocked, so it prints to stdout")
	require.Contains(t, human, "Compared", "clean output states what was compared")
	require.Contains(t, human, "nothing moved")
	require.NotContains(t, strings.ToLower(human), "no drift",
		"exit 0 asserts coverage; it must never read as a bare absence-of-news line")

	// The same three coverage lines, rendered with their own unit named
	// (round-1 review, Finding 1): node(s) for the two node-grained markers,
	// criteria (left unpluralized on purpose, Finding 2) for orphan_eval.
	require.Contains(t, human, "Coverage:")
	require.Contains(t, human, "active_basis: 2 node(s) checked")
	require.Contains(t, human, "answered_question: 1 node(s) checked")
	require.Contains(t, human, "orphan_eval: 1 criteria not yet in scope")
}

// TestDriftCandidatesExitOne edits the approved feature's own spec (no
// commit; the working-tree comparison catches it immediately, the same
// active-basis behavior e2e_done_test.go's own basis-guard arc exercises at
// the transition verbs) and asserts: exit 1, ok false, reason drift_found,
// data.candidates.active_basis names the feature by id and key, and the
// human block prints on stderr (blocked answers print there, per resolution
// 1).
func TestDriftCandidatesExitOne(t *testing.T) {
	dir := initDriftGitRepo(t, "active_basis")
	layDriftFeature(t, dir)
	fid := soleFeatureID(t)

	mdPath := filepath.Join(driftFeatureDir(dir), "feature.md")
	require.NoError(t, os.WriteFile(mdPath,
		[]byte("# Certification form loading\n\nRevised: scope changed since acceptance.\n"), 0o644))

	out, errOut, code := execCLI(t, "drift", "--json")
	require.Equal(t, 1, code)
	require.Empty(t, errOut, "json mode always writes the envelope to stdout, blocked or not")

	env := parseDriftEnvelope(t, out)
	require.False(t, env.OK)
	require.Equal(t, "drift_found", env.Data.Reason)

	abRows := env.Data.Candidates["active_basis"]
	require.Len(t, abRows, 1, "one candidate: the feature whose spec changed")
	require.Equal(t, fid, abRows[0]["id"])
	require.Equal(t, driftFeatureKey, abRows[0]["key"])
	require.Contains(t, abRows[0]["detail"], "feature.md")
	require.NotContains(t, abRows[0], "ref", "id and key ride as separate fields; ref is a human-only label, never a composed one")

	// The record's commit anchor rides as a structured kind/value/observed_at
	// object (resolution 4), not a composed string: approve
	// pinned this feature's basis at HEAD, so Since is populated.
	since, ok := abRows[0]["since"].(map[string]any)
	require.True(t, ok, "since projects structurally: %v", abRows[0]["since"])
	require.Equal(t, "git_commit", since["kind"])
	require.NotEmpty(t, since["value"])
	require.NotEmpty(t, since["observed_at"])

	_, humanErr, code := execCLI(t, "drift")
	require.Equal(t, 1, code)
	require.Contains(t, humanErr, "active_basis (1):")
	require.Contains(t, humanErr, driftFeatureKey)
	require.Contains(t, humanErr, fid)
	require.Contains(t, humanErr, "feature.md changed since the accepted basis")
}

// TestDriftUncoveredAloneExitsOne hand-writes a done feature with no
// verified_against into an otherwise clean tree (Task 8's found-plus-failed
// case's sibling: here nothing is found, one node just could not be
// compared). It asserts NO candidates, one uncovered row, and STILL exit 1,
// because a run that could not check must never look like a run that found
// nothing. This is decision 11's whole point; it is not weakened here.
func TestDriftUncoveredAloneExitsOne(t *testing.T) {
	dir := initDriftGitRepo(t, allDriftChecks)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", driftFeatureKey,
		"--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	fid := soleFeatureID(t)

	markFeatureDone(t, dir, "form-engine", "certification-form-loading")

	out, errOut, code := execCLI(t, "drift", "--json")
	require.Equal(t, 1, code, "a run that could not check must never look like a run that found nothing")
	require.Empty(t, errOut)

	env := parseDriftEnvelope(t, out)
	require.False(t, env.OK)
	require.Equal(t, "drift_found", env.Data.Reason)
	require.Empty(t, env.Data.Candidates, "no candidates: this node was never compared, so nothing can be reported as moved")
	require.Len(t, env.Data.Uncovered, 1)
	require.Equal(t, "stale_spec", env.Data.Uncovered[0]["marker"])
	require.Equal(t, "no_completion_record", env.Data.Uncovered[0]["reason"])
	require.Equal(t, fid, env.Data.Uncovered[0]["id"])
	require.Equal(t, driftFeatureKey, env.Data.Uncovered[0]["key"])

	_, humanErr, code := execCLI(t, "drift")
	require.Equal(t, 1, code)
	require.Contains(t, humanErr, "Uncovered (1):")
	require.Contains(t, humanErr, "no_completion_record")
	require.NotContains(t, humanErr, "stale_spec (", "no candidate section header: this node produced no candidate")
}

// The disabled footer names every omitted marker without enabling new checks.
func TestDriftDisabledFooter(t *testing.T) {
	initDriftGitRepo(t, "active_basis")

	out, _, code := execCLI(t, "drift", "--json")
	require.Equal(t, 0, code)

	env := parseDriftEnvelope(t, out)
	require.Equal(t,
		[]string{"stale_spec", "answered_question", "missing_surface", "orphan_eval", "stale_design", "shifted_design"},
		env.Data.Disabled,
		"checks left out of drift.checks, in the schema's own order")
}

// TestDriftEndToEndArc is the one e2e arc: a
// scratch project that starts clean (exit 0), then a single tree carries
// BOTH an induced active_basis candidate (an edit to the approved feature's
// spec) and an induced uncovered node (a second feature hand-set to done
// with no completion record), asserting the exit codes read 0 then 1, the
// JSON carries both lists, and the human output carries both sections plus
// the footer.
func TestDriftEndToEndArc(t *testing.T) {
	dir := initDriftGitRepo(t, allDriftChecks)
	layDriftFeature(t, dir)

	cleanOut, cleanErr, code := execCLI(t, "drift", "--json")
	require.Equal(t, 0, code, "the freshly approved feature and task start clean")
	require.Contains(t, cleanOut, `"ok": true`)
	require.Empty(t, cleanErr)

	// Induce the candidate: edit the approved feature's spec, no commit.
	mdPath := filepath.Join(driftFeatureDir(dir), "feature.md")
	require.NoError(t, os.WriteFile(mdPath,
		[]byte("# Certification form loading\n\nRevised after acceptance.\n"), 0o644))

	// Induce the uncovered node: a second, unrelated feature hand-set to done
	// with no verified_against, the same fixture TestDriftUncoveredAloneExitsOne
	// isolates on its own.
	_, _, code = execCLI(t, "add", "epic", "widget", "--title", "Widget", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "widget/assembly",
		"--title", "Assemble the widget", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	markFeatureDone(t, dir, "widget", "assembly")

	out, errOut, code := execCLI(t, "drift", "--json")
	require.Equal(t, 1, code)
	require.Empty(t, errOut)

	env := parseDriftEnvelope(t, out)
	require.False(t, env.OK)
	require.Equal(t, "drift_found", env.Data.Reason)
	require.NotEmpty(t, env.Data.Candidates["active_basis"], "the edited spec is named as a candidate")
	require.Len(t, env.Data.Uncovered, 1, "the hand-set done feature is named as uncovered")
	require.Equal(t, "stale_spec", env.Data.Uncovered[0]["marker"])

	_, humanErr, code := execCLI(t, "drift")
	require.Equal(t, 1, code)
	require.Contains(t, humanErr, "active_basis (")
	require.Contains(t, humanErr, "Uncovered (")
	require.Contains(t, humanErr, "Compared")
	require.Contains(t, humanErr, "widget/assembly")
	require.Contains(t, humanErr, driftFeatureKey)
}

// TestDriftExemptedFooter closes round-1 review's Finding 1: the Exempted
// loop in writeDriftFooter had no coverage in either output mode. The
// fixture is the reviewer's own recipe: a feature carries a recorded surface
// path (`tiller link --surface`) and is then cancelled, which
// sweepMissingSurface (drift.go) counts as an exemption rather than a
// candidate or an uncovered node, because the human's cancel decision took
// the node off the table (artifact-schema.md's exemption paragraph). The
// feature is never spec'd or approved, so no other marker touches it: the
// run is otherwise clean (Compared: 0, no candidates, no uncovered), exit 0,
// with the exemption visible in both JSON and human output.
func TestDriftExemptedFooter(t *testing.T) {
	initDriftGitRepo(t, allDriftChecks)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", driftFeatureKey,
		"--title", "Certification form loading", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	// README.md already exists (initDriftGitRepo's own first commit), so it
	// is a real, resolvable path: link --surface records surface.paths
	// without needing a second fixture file.
	_, _, code = execCLI(t, "link", "feature", driftFeatureKey, "--surface", "README.md", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "cancel", "feature", driftFeatureKey, "--reason", "not needed", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "drift", "--json")
	require.Equal(t, 0, code, "an exemption is not a finding: it does not redden the run")

	env := parseDriftEnvelope(t, out)
	require.True(t, env.OK)
	require.Empty(t, env.Data.Candidates)
	require.Empty(t, env.Data.Uncovered)
	require.Equal(t, map[string]int{"missing_surface": 1}, env.Data.Exempted)

	human, _, code := execCLI(t, "drift")
	require.Equal(t, 0, code)
	require.Contains(t, human, "Exempted: missing_surface: 1")
}

// TestDriftContextLinesRender closes round-1 review's Finding 1: the
// `context:` lines in writeCandidateRow had no coverage. It induces the ONE
// active_basis shape that populates Context at all (activeBasisDetail): one
// pinned file DELETED (feature.eval.md, which lands in both Files and the
// leading Detail clause, "... deleted since the accepted basis") alongside
// another pinned file CHANGED BUT STILL PRESENT (feature.md, which rides
// beside it as Context, "... also changed since the accepted basis") on the
// SAME node, so the two moved-file facts are not lost against each other.
func TestDriftContextLinesRender(t *testing.T) {
	dir := initDriftGitRepo(t, "active_basis")
	layDriftFeature(t, dir)

	fdir := driftFeatureDir(dir)
	require.NoError(t, os.Remove(filepath.Join(fdir, "feature.eval.md")))
	require.NoError(t, os.WriteFile(filepath.Join(fdir, "feature.md"),
		[]byte("# Certification form loading\n\nRevised after acceptance.\n"), 0o644))

	out, errOut, code := execCLI(t, "drift", "--json")
	require.Equal(t, 1, code)
	require.Empty(t, errOut)

	env := parseDriftEnvelope(t, out)
	abRows := env.Data.Candidates["active_basis"]
	require.Len(t, abRows, 1)
	require.Contains(t, abRows[0]["detail"], "feature.eval.md deleted since the accepted basis")
	context, ok := abRows[0]["context"].([]any)
	require.True(t, ok, "context rides as a list: %v", abRows[0]["context"])
	require.Equal(t, []any{"feature.md also changed since the accepted basis"}, context)

	_, humanErr, code := execCLI(t, "drift")
	require.Equal(t, 1, code)
	require.Contains(t, humanErr, "feature.eval.md deleted since the accepted basis")
	require.Contains(t, humanErr, "      context: feature.md also changed since the accepted basis")
}
