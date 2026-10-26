package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// layReadTree lays a one-epic-one-feature draft tree (the Task 8 shape) and
// chdirs into the repo, the fixture the read commands are exercised against.
func layReadTree(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine",
		"--title", "Form Engine", "--source", "design/04-form-engine.md", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/certification-form-loading",
		"--title", "Certification form loading", "--change-type", "create",
		"--source", "design/04-form-engine.md", "--contributor", "jc")
	require.Equal(t, 0, code)
}

// soleFeatureID reads the one feature's minted id off disk, so a by-id test can
// address it without parsing the random token out of the add output.
func soleFeatureID(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Len(t, tree.Features, 1)
	for id := range tree.Features {
		return id
	}
	return ""
}

// TestShowFeatureGolden pins the full-node envelope for `show feature <key>`,
// resolved by canonical key, with the random id normalized.
func TestShowFeatureGolden(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "show_feature.json"))
	require.NoError(t, err)

	layReadTree(t)
	out, _, code := execCLI(t, "show", "feature", "form-engine/certification-form-loading", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, string(golden), normalizeIDs(out))
}

// enrichedFeatureYML is a feature.yml carrying the full integrity block a human
// audits through `show`: a green build gate, an evals gate with a per-criterion map
// (C1 attested pass, C2 a recorded rubric awaiting attestation), a human attestation,
// a merge-time policy violation, and the accepted/verified basis (verified_against
// records surfaces as the empty list for this create feature). Every value is fixed
// so the golden is deterministic (the random id is normalized to F-xxxxxx).
const enrichedFeatureYML = `id: F-aaaaaa
key: form-engine/certification-form-loading
title: Certification form loading
status: active
blocked: false
change_type: create
accepted_against:
  spec: { kind: git_blob, value: "git:5pec0aa" }
  eval: { kind: git_blob, value: "git:e7a10aa" }
verified_against:
  spec: { kind: git_blob, value: "git:5pec1bb" }
  eval: { kind: git_blob, value: "git:e7a11bb" }
  surfaces: []
gates:
  build:
    required: true
    kind: command
    status: pass
    run_by: jc
    run_at: 2026-06-27T12:00:00Z
    against: { kind: git_commit, value: "git:abc1234" }
    evidence:
      - { kind: command_output, ref: artifacts/tiller/gates/build-abc1234.txt }
  evals:
    required: true
    kind: command
    status: pending
    run_by: jc
    run_at: 2026-06-27T12:00:00Z
    against: { kind: git_commit, value: "git:abc1234" }
    criteria:
      C1:
        status: pass
      C2:
        status: needs_attestation
        evidence:
          - { kind: rubric_output, ref: artifacts/tiller/evals/F-aaaaaa-C2.json }
attestations:
  - id: AT-aaaaaa
    target: { kind: criterion, id: C1 }
    status: pass
    by: jc
    at: 2026-06-27T12:00:00Z
policy_violations:
  - gate: evals
    observed: { kind: git_commit, value: "git:abc1234" }
    status: pending
    evidence_ref: artifacts/tiller/evals/F-aaaaaa-C2.json
`

// TestShowFeatureEnrichedGolden pins the enriched full-node envelope for `show
// feature`: the gates block with the evals per-criterion map, the attestations, the
// policy_violations, and the accepted/verified basis (B6). It lays the read tree,
// overwrites the feature.yml with a deterministic integrity fixture, and goldens the
// JSON so the audit surface stays stable.
func TestShowFeatureEnrichedGolden(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "show_feature_enriched.json"))
	require.NoError(t, err)

	layReadTree(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	ymlPath := filepath.Join(cwd, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
	require.NoError(t, os.WriteFile(ymlPath, []byte(enrichedFeatureYML), 0o644))

	out, _, code := execCLI(t, "show", "feature", "form-engine/certification-form-loading", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, string(golden), normalizeIDs(out))
}

// TestShowFeatureEnrichedHuman confirms the human (non-json) show surfaces the same
// integrity block: the gates with the evals per-criterion statuses, the attestations,
// the policy_violations, and the accepted/verified basis, so a human auditing through
// the CLI without --json still sees integrity (B6).
func TestShowFeatureEnrichedHuman(t *testing.T) {
	layReadTree(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	ymlPath := filepath.Join(cwd, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
	require.NoError(t, os.WriteFile(ymlPath, []byte(enrichedFeatureYML), 0o644))

	out, _, code := execCLI(t, "show", "feature", "form-engine/certification-form-loading")
	require.Equal(t, 0, code)
	require.Contains(t, out, "gates:")
	require.Contains(t, out, "evals: pending")
	require.Contains(t, out, "C2: needs_attestation")
	require.Contains(t, out, "attestations:")
	require.Contains(t, out, "criterion C1 pass by jc")
	require.Contains(t, out, "policy_violations:")
	require.Contains(t, out, "accepted_against:")
	require.Contains(t, out, "verified_against:")
}

// crossedNonGreenNowPassYML is the field case issue #44's qualifier closes: a
// feature that crossed to done while its evals gate was red under advisory
// enforcement (the recorded policy_violations entry), and the gate has since
// gone green (a re-run, or a later attestation) without the violation record
// itself changing, since a policy_violations entry is a merge-time record,
// never rewritten after the fact. A reader meets "evals: pass" in the gates
// block right next to "evals: fail" in policy_violations with nothing saying
// the two are consistent, not contradictory.
const crossedNonGreenNowPassYML = `id: F-aaaaaa
key: form-engine/certification-form-loading
title: Certification form loading
status: active
blocked: false
change_type: create
gates:
  evals:
    required: true
    kind: command
    status: pass
    run_by: jc
    run_at: 2026-06-27T12:00:00Z
    against: { kind: git_commit, value: "git:abc1234" }
policy_violations:
  - gate: evals
    observed: { kind: git_commit, value: "git:abc1234" }
    status: fail
    evidence_ref: artifacts/tiller/evals/F-aaaaaa.json
`

// crossedNonGreenStillRedYML is the same shape with the gate still not
// passing, the control case: the qualifier must stay silent while the
// violated gate has not actually recovered.
const crossedNonGreenStillRedYML = `id: F-aaaaaa
key: form-engine/certification-form-loading
title: Certification form loading
status: active
blocked: false
change_type: create
gates:
  evals:
    required: true
    kind: command
    status: fail
    run_by: jc
    run_at: 2026-06-27T12:00:00Z
    against: { kind: git_commit, value: "git:abc1234" }
policy_violations:
  - gate: evals
    observed: { kind: git_commit, value: "git:abc1234" }
    status: fail
    evidence_ref: artifacts/tiller/evals/F-aaaaaa.json
`

// writeFeatureYML overwrites the sole read-tree feature's feature.yml with
// yml, the shared setup TestShowFeatureEnrichedGolden and this task's own
// qualifier tests both need.
func writeFeatureYML(t *testing.T, yml string) {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	ymlPath := filepath.Join(cwd, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
	require.NoError(t, os.WriteFile(ymlPath, []byte(yml), 0o644))
}

// TestShowPolicyViolationQualifierWhenGateNowPasses pins the violations
// qualifier's positive case (issue #44): a policy_violations line naming a
// gate whose CURRENT status in the same view is pass gains the suffix
// dating the record to the crossing, so a reader is not left decoding
// "evals: pass" next to "evals: fail" as a contradiction.
func TestShowPolicyViolationQualifierWhenGateNowPasses(t *testing.T) {
	layReadTree(t)
	writeFeatureYML(t, crossedNonGreenNowPassYML)

	out, _, code := execCLI(t, "show", "feature", "form-engine/certification-form-loading")
	require.Equal(t, 0, code)
	require.Contains(t, out, "evals: fail (recorded at the crossing; gate now pass)")

	// The JSON payload carries no schema or store change: the qualifier is a
	// render-local fact the human renderer alone adds (NodeView already
	// carries Gates and PolicyViolations together), so a machine caller's
	// decode is untouched.
	jout, _, code := execCLI(t, "show", "feature", "form-engine/certification-form-loading", "--json")
	require.Equal(t, 0, code)
	require.NotContains(t, jout, "recorded at the crossing")
}

// TestShowPolicyViolationQualifierAbsentWhileGateStillNotPass pins the
// qualifier's negative case: the same shape with the gate still not passing
// prints the violation bare, since the suffix would misstate the gate's
// current status if it fired unconditionally.
func TestShowPolicyViolationQualifierAbsentWhileGateStillNotPass(t *testing.T) {
	layReadTree(t)
	writeFeatureYML(t, crossedNonGreenStillRedYML)

	out, _, code := execCLI(t, "show", "feature", "form-engine/certification-form-loading")
	require.Equal(t, 0, code)
	require.Contains(t, out, "evals: fail")
	require.NotContains(t, out, "recorded at the crossing",
		"the gate has not recovered, so the crossing qualifier does not fire")
}

// TestLsFeaturesByStatusGolden pins the list envelope for `ls features --status
// draft`: the homogeneous noun, the count, and the one matching item.
func TestLsFeaturesByStatusGolden(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "ls_features_draft.json"))
	require.NoError(t, err)

	layReadTree(t)
	out, _, code := execCLI(t, "ls", "features", "--status", "draft", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, string(golden), normalizeIDs(out))
}

// TestStatusFeatureByIDGolden pins the typed status oracle for `status feature
// <id>`: state and recorded gates. The legal-next-moves are a Phase 3 seam
// (T10/T11 add the lifecycle engine and widen this payload with legal_next); a
// draft feature with no gates carries neither yet, which this golden records.
func TestStatusFeatureByIDGolden(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "status_feature.json"))
	require.NoError(t, err)

	layReadTree(t)
	fid := soleFeatureID(t)
	out, _, code := execCLI(t, "status", "feature", fid, "--json")
	require.Equal(t, 0, code)
	require.Equal(t, string(golden), normalizeIDs(out))
}

// TestShowNounMismatchExits2 confirms the redundant noun is a checksum: showing
// a feature id as an epic is misuse (exit 2) with the noun_mismatch reason.
func TestShowNounMismatchExits2(t *testing.T) {
	layReadTree(t)
	fid := soleFeatureID(t)
	out, _, code := execCLI(t, "show", "epic", fid, "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, reasonNounMismatch)
}

// TestShowNotFoundExits2 confirms an unresolved address is misuse (exit 2).
func TestShowNotFoundExits2(t *testing.T) {
	layReadTree(t)
	out, _, code := execCLI(t, "show", "feature", "no-such/feature", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, reasonNotFound)
}

// TestShowUnknownNounExits2 confirms an unrecognized noun is rejected as misuse.
func TestShowUnknownNounExits2(t *testing.T) {
	layReadTree(t)
	out, _, code := execCLI(t, "show", "widget", "form-engine/certification-form-loading", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "unknown_noun")
}

// TestLsEmptyListIsDeterministic confirms a filter that matches nothing returns
// a zero count and an empty (not null) nodes array.
func TestLsEmptyListIsDeterministic(t *testing.T) {
	layReadTree(t)
	out, _, code := execCLI(t, "ls", "features", "--status", "active", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"count\": 0")
	require.Contains(t, out, "\"nodes\": []")
}

// TestLsOwnerFilter confirms the --contributor filter narrows by the owner field. The
// draft tree carries no owner, so an owner filter excludes the feature.
func TestLsOwnerFilter(t *testing.T) {
	layReadTree(t)
	out, _, code := execCLI(t, "ls", "features", "--owner", "sam", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"count\": 0")
}

// TestLsAcceptsSingularAndPlural confirms the noun grammar is uniform: ls takes
// both the singular and the plural of every noun (add/show/status take singular),
// and the two forms produce identical output.
func TestLsAcceptsSingularAndPlural(t *testing.T) {
	layReadTree(t)
	for _, pair := range [][2]string{
		{"epic", "epics"},
		{"feature", "features"},
		{"task", "tasks"},
		{"question", "questions"},
	} {
		sing, plur := pair[0], pair[1]

		outSing, _, codeSing := execCLI(t, "ls", sing, "--json")
		require.Equalf(t, 0, codeSing, "ls %s should work", sing)
		outPlur, _, codePlur := execCLI(t, "ls", plur, "--json")
		require.Equalf(t, 0, codePlur, "ls %s should work", plur)

		require.Equalf(t, outPlur, outSing, "ls %s and ls %s produce identical output", sing, plur)
	}
}

// TestLsUnknownNounExits2 confirms an unrecognized plural noun is misuse.
func TestLsUnknownNounExits2(t *testing.T) {
	layReadTree(t)
	out, _, code := execCLI(t, "ls", "widgets", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, "unknown_noun")
}

// TestLsTaskByFeatureFilter confirms `ls task --feature <feature>` narrows the task
// list to one parent feature's tasks, resolved by the feature's key (B7).
func TestLsTaskByFeatureFilter(t *testing.T) {
	initOwnedRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/certification-form-loading",
		"--title", "Certification", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/pdf-export",
		"--title", "PDF", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "task", "form-engine/certification-form-loading/transcribe-the-schema",
		"--title", "Transcribe the schema", "--contributor", "jc")
	require.Equal(t, 0, code)

	// The named feature's task is listed.
	out, _, code := execCLI(t, "ls", "task", "--feature", "form-engine/certification-form-loading", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"count\": 1")
	require.Contains(t, out, "Transcribe the schema")

	// A sibling feature with no tasks lists none: the filter actually narrows.
	out, _, code = execCLI(t, "ls", "task", "--feature", "form-engine/pdf-export", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"count\": 0")
}

// TestLsTaskUnknownFeatureExits2 confirms a --feature value that resolves to no
// feature is misuse (exit 2), the same class as any unresolved address.
func TestLsTaskUnknownFeatureExits2(t *testing.T) {
	layReadTree(t)
	_, _, code := execCLI(t, "ls", "task", "--feature", "no-such/feature", "--json")
	require.Equal(t, 2, code)
}

// TestLsFeatureByEpicFilter confirms `ls feature --epic <epic>` narrows the feature
// list to one epic's features (B7).
func TestLsFeatureByEpicFilter(t *testing.T) {
	initOwnedRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "epic", "billing", "--title", "Billing", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "form-engine/certification-form-loading",
		"--title", "Certification", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "feature", "billing/invoices",
		"--title", "Invoices", "--change-type", "create", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "ls", "feature", "--epic", "form-engine", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"count\": 1")
	require.Contains(t, out, "form-engine/certification-form-loading")
	require.NotContains(t, out, "billing/invoices")
}

// TestStatusNodeOutsideRepoExits3 confirms the typed oracle refuses outside a
// Tiller repo with an environment error (exit 3).
func TestStatusNodeOutsideRepoExits3(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "status", "feature", "F-aaaaaa", "--json")
	require.Equal(t, 3, code)
}

// TestStatusOneArgIsMisuse confirms a lone noun with no node is rejected (exit 2),
// the same arg-count misuse cobra enforces elsewhere.
func TestStatusOneArgIsMisuse(t *testing.T) {
	layReadTree(t)
	_, _, code := execCLI(t, "status", "feature", "--json")
	require.Equal(t, 2, code)
}

// TestLsFeatureByEpicKeyEqualsID confirms `ls features --epic <key>` lists the
// same rows as `ls features --epic <id>` (issue #23's parity rule): a parent
// filter resolves through ResolveParentFilters, so naming the parent either
// way compares against the same stable identity.
func TestLsFeatureByEpicKeyEqualsID(t *testing.T) {
	layReadTree(t)

	byKey, _, code := execCLI(t, "ls", "features", "--epic", "form-engine", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, byKey, "\"count\": 1")
	require.Contains(t, byKey, "Certification form loading")

	byID, _, code := execCLI(t, "ls", "features", "--epic", soleEpicID(t), "--json")
	require.Equal(t, 0, code)
	require.Equal(t, byKey, byID, "the key and id filters must list identical output")
}

// TestLsFeatureByBogusEpicExits2 confirms an unresolved --epic value is misuse
// (exit 2), the same class as an unresolved --feature, rather than a false
// empty success.
func TestLsFeatureByBogusEpicExits2(t *testing.T) {
	layReadTree(t)
	out, _, code := execCLI(t, "ls", "features", "--epic", "no-such-epic", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, reasonNotFound)
}

// TestLsEmptyNamesFilters pins the empty-result phrasing (dry-run paper cut):
// an empty filtered list names the filters that produced it, and an unfiltered
// empty list keeps the plain fallback.
func TestLsEmptyNamesFilters(t *testing.T) {
	layReadTree(t)

	out, _, code := execCLI(t, "ls", "features", "--status", "ready")
	require.Equal(t, 0, code)
	require.Equal(t, "No features with status ready.\n", out)

	out, _, code = execCLI(t, "ls", "tasks", "--status", "ready", "--owner", "jc")
	require.Equal(t, 0, code)
	require.Equal(t, "No tasks with status ready, owner jc.\n", out)

	out, _, code = execCLI(t, "ls", "questions")
	require.Equal(t, 0, code)
	require.Equal(t, "No questions found.\n", out)
}

// TestLsEpicsFollowDependencyOrder pins Task A1 (issue #27) end to end at the
// CLI surface: `ls epics`' rows come out in dependency order, not key order.
// "alpha" records --depends-on "zeta", so key order (alpha before zeta) and
// dependency order (zeta, the foundation, before alpha, what rests on it)
// disagree. layReadTree's single-epic fixture never exercises this.
func TestLsEpicsFollowDependencyOrder(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "zeta", "--title", "Zeta", "--contributor", "jc")
	mustRun(t, "add", "epic", "alpha", "--title", "Alpha", "--depends-on", "zeta", "--contributor", "jc")

	out, _, code := execCLI(t, "ls", "epics", "--json")
	require.Equal(t, 0, code)
	zetaIdx := strings.Index(out, `"key": "zeta"`)
	alphaIdx := strings.Index(out, `"key": "alpha"`)
	require.GreaterOrEqual(t, zetaIdx, 0, "zeta is listed")
	require.Greater(t, alphaIdx, zetaIdx,
		"zeta (the dependency) leads alpha in the nodes array, though alpha's key sorts first alphabetically")
}

// titleColumns returns, per line, the column the text after the reference
// starts at: the index of the first non-space rune following the run of spaces
// that closes the label column. It is how the layout tests below compare rows
// without hard-coding a width that is meant to be computed.
func titleColumns(t *testing.T, out string) []int {
	t.Helper()
	var cols []int
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		i := strings.Index(line, ") ")
		require.GreaterOrEqual(t, i, 0, "every row names a node by reference: %q", line)
		head := line[:i+1] // the status column and the reference, through its closing paren
		rest := line[i+1:] // the gutter, then the title
		// A space is one byte and one rune, so counting the gutter in bytes and
		// the head in runes measures the same units the renderer pads in
		// (utf8.RuneCountInString, via col in tree.go).
		gutter := len(rest) - len(strings.TrimLeft(rest, " "))
		cols = append(cols, utf8.RuneCountInString(head)+gutter)
	}
	return cols
}

// TestLsSizesItsLabelColumnToTheWidestReference pins the fix for the fixed
// 44-column label budget, which was sized when the widest row a list could hold
// was a two-segment feature key. A task's
// canonical reference is three segments plus the id parenthetical
// (cli-surface.md, Human references), which overruns 44 on ordinary keys, and an
// overrunning row pushed its own title right while every shorter row stayed put.
//
// The rule is the one `tiller tree` already follows: the column is this list's
// own widest reference, so labels never truncate and titles line up
// (cli-surface.md, "The tree map sizes itself").
func TestLsSizesItsLabelColumnToTheWidestReference(t *testing.T) {
	laySpeccableFeature(t)
	addSpeccedTask(t, "b", "A short key")

	out, _, code := execCLI(t, "ls", "tasks")
	require.Equal(t, 0, code)
	require.Contains(t, out, "form-engine/certification-form-loading/transcribe-the-schema (T-")
	require.Contains(t, out, "form-engine/certification-form-loading/b (T-")

	cols := titleColumns(t, out)
	require.Len(t, cols, 2)
	require.Equal(t, cols[0], cols[1], "titles line up across rows:\n%s", out)
}

// TestLsNeverTruncatesAReference is the other half of the same rule: a reference
// is an address, so it renders whole however long it is. A budget that clipped
// it would print a key that resolves to nothing.
func TestLsNeverTruncatesAReference(t *testing.T) {
	laySpeccableFeature(t)

	out, _, code := execCLI(t, "ls", "tasks")
	require.Equal(t, 0, code)
	require.Contains(t, out, "form-engine/certification-form-loading/transcribe-the-schema (T-")
	require.NotContains(t, out, "…")
}

// TestShowOmitsTheEvalLegWhereTheGrainHasNone pins small item 5 of the round-1
// findings' also-fix list at the grain that exposed it. The acceptance contract
// enters the tree at feature grain (artifact-schema.md, Gates), so a task's
// accepted_against carries a spec leg and nothing else, and the renderer printed
// the eval label with an empty value after it. Every sibling block in show.go
// returns early on empty rather than printing a label with nothing behind it.
func TestShowOmitsTheEvalLegWhereTheGrainHasNone(t *testing.T) {
	layApprovedFeatureWithTask(t)
	tid := soleTaskID(t)

	out, _, code := execCLI(t, "show", "task", tid)
	require.Equal(t, 0, code)
	require.Contains(t, out, "accepted_against: spec git:", "the plan gate pinned this task's task.md")
	require.NotContains(t, out, "eval ", "a task has no eval leg to name")

	// The feature above it carries both legs, so the label is not simply gone.
	out, _, code = execCLI(t, "show", "feature", cliFeatureKey)
	require.Equal(t, 0, code)
	require.Contains(t, out, "accepted_against: spec git:")
	require.Contains(t, out, ", eval git:")
}
