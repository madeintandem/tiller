package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// recordC2 lays the eval-feature fixture and records a passing rubric output for
// C2, leaving it needs_attestation, the precondition the attest tests build on.
// The record run exits 0 (the record mutation succeeded); the criterion is recorded
// needs_attestation, still unsatisfied until a human attests.
func recordC2(t *testing.T) string {
	t.Helper()
	key := layEvalFeature(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "rubric.json"), []byte(rubricOutputPass), 0o644))
	_, _, code := execCLI(t, "eval", "feature", key,
		"--criterion", "C2", "--record-rubric-output", "rubric.json", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, store.CriterionStatusNeedsAttestation, evalsCriterion(t, "C2").Status)
	return key
}

// TestAttestCriterionSatisfies confirms a human attest --criterion --status pass
// flips a needs_attestation criterion to pass, clears it, and records an AT- entry
// with the attesting owner.
func TestAttestCriterionSatisfies(t *testing.T) {
	key := recordC2(t)

	out, _, code := execCLI(t, "attest", "feature", key, "--criterion", "C2", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"ok\": true")
	require.Contains(t, out, "\"satisfied\": true")
	require.Contains(t, out, "\"attestation_id\": \"AT-")
	require.Contains(t, out, "\"target_kind\": \"criterion\"")

	// The criterion is now pass, and the attestation is recorded with provenance.
	c := evalsCriterion(t, "C2")
	require.Equal(t, store.CriterionStatusPass, c.Status)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	feat := tree.Features[soleFeatureID(t)]
	require.Len(t, feat.GatesBlock.Attestations, 1)
	att := feat.GatesBlock.Attestations[0]
	require.Equal(t, "criterion", att.Target.Kind)
	require.Equal(t, "C2", att.Target.ID)
	require.Equal(t, store.GateStatusPass, att.Status)
	require.Equal(t, "jc", att.By)
	require.False(t, att.At.IsZero())
	require.Regexp(t, `^AT-[a-z0-9]{6}$`, att.ID)
}

// TestAgentRubricDoesNotSatisfyWithoutAttest is the boundary the handoff defends: a
// recorded rubric leaves the required eval criterion unsatisfied, and only the human
// attest crosses it. Before attest the criterion is needs_attestation and the evals
// gate is not green; after attest it is pass.
func TestAgentRubricDoesNotSatisfyWithoutAttest(t *testing.T) {
	key := recordC2(t)

	// Without attest: not satisfied.
	c := evalsCriterion(t, "C2")
	require.Equal(t, store.CriterionStatusNeedsAttestation, c.Status)
	require.NotEqual(t, store.CriterionStatusPass, c.Status)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	require.Empty(t, tree.Features[soleFeatureID(t)].GatesBlock.Attestations,
		"no attestation exists until the human attests")

	// With attest: satisfied.
	_, _, code := execCLI(t, "attest", "feature", key, "--criterion", "C2", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Equal(t, store.CriterionStatusPass, evalsCriterion(t, "C2").Status)
}

// TestAttestManualCriterionHasNoDanglingEvidenceRef confirms attesting a manual
// criterion records no evidence_ref (B2): a manual criterion has no graded artifact,
// so the attestation must not point at a rubric-output file that was never created.
func TestAttestManualCriterionHasNoDanglingEvidenceRef(t *testing.T) {
	key := layEvalFeature(t)

	_, _, code := execCLI(t, "attest", "feature", key, "--criterion", "C1", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	feat := tree.Features[soleFeatureID(t)]
	require.Len(t, feat.GatesBlock.Attestations, 1)
	att := feat.GatesBlock.Attestations[0]
	require.Equal(t, "C1", att.Target.ID)
	require.Empty(t, att.EvidenceRef, "a manual criterion attestation references no rubric-output file")
}

// TestAttestEvalCriterionEvidenceRefExists confirms attesting an eval criterion whose
// rubric output was recorded points evidence_ref at the real recorded artifact, a
// file that exists on disk (B2): the attestation's trail is a real file, never a
// dangling path.
func TestAttestEvalCriterionEvidenceRefExists(t *testing.T) {
	key := recordC2(t)

	_, _, code := execCLI(t, "attest", "feature", key, "--criterion", "C2", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	feat := tree.Features[soleFeatureID(t)]
	var att store.Attestation
	for _, a := range feat.GatesBlock.Attestations {
		if a.Target.ID == "C2" {
			att = a
		}
	}
	require.NotEmpty(t, att.EvidenceRef, "an eval criterion attestation points at its recorded rubric output")
	require.FileExists(t, filepath.Join(cwd, att.EvidenceRef))
}

// TestAttestGate confirms --gate sets an attestation-backed optional review gate and
// records the attestation against the gate target.
func TestAttestGate(t *testing.T) {
	key := layEvalFeature(t)
	declareReviewGates(t)

	out, _, code := execCLI(t, "attest", "feature", key, "--gate", "code_review", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, "\"target_kind\": \"gate\"")
	require.Contains(t, out, "\"satisfied\": true")
	require.NotContains(t, out, "\"warnings\"", "the seeded code_review gate is declared, so no warning")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	feat := tree.Features[soleFeatureID(t)]
	g := feat.GatesBlock.Gates["code_review"]
	require.Equal(t, store.GateKindAttestation, g.Kind)
	require.Equal(t, store.GateStatusPass, g.Status)
	require.Equal(t, "jc", g.RunBy)
	require.False(t, g.Required, "an attestation review gate is optional")
}

// TestAttestUndeclaredGateWarns confirms a gate key the manifest does not
// declare is still recorded but warns, naming the declared keys, so a
// misspelling like code-review for the seeded code_review is visible (#138).
func TestAttestUndeclaredGateWarns(t *testing.T) {
	key := layEvalFeature(t)
	declareReviewGates(t)

	out, _, code := execCLI(t, "attest", "feature", key, "--gate", "code-review", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 0, code)
	require.Contains(t, out, `gate \"code-review\" is not declared in tiller.yml (declared: accessibility_review, code_review, performance_review, security_review); recorded anyway`)

	human, _, code := execCLI(t, "attest", "feature", key, "--gate", "code-review", "--status", "pass", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, human, "Warning: gate \"code-review\" is not declared in tiller.yml")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	tree, err := store.Load(cwd)
	require.NoError(t, err)
	feat := tree.Features[soleFeatureID(t)]
	require.Equal(t, store.GateStatusPass, feat.GatesBlock.Gates["code-review"].Status, "an undeclared gate is still recorded")
}

// declareReviewGates appends the four optional review gates tiller init seeds to
// the test manifest, so attest resolves a gate key against a declared set.
func declareReviewGates(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(".tiller", "tiller.yml"), os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	defer f.Close()
	_, err = f.WriteString(`gates:
  optional:
    code_review:          { kind: attestation, on: pr }
    security_review:      { kind: attestation, on: pr }
    performance_review:   { kind: attestation, on: pr }
    accessibility_review: { kind: attestation, on: pr }
`)
	require.NoError(t, err)
}

// TestAttestMissingTarget confirms attest with neither --gate nor --criterion is
// misuse (exit 2).
func TestAttestMissingTarget(t *testing.T) {
	key := layEvalFeature(t)
	_, _, code := execCLI(t, "attest", "feature", key, "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
}

// TestAttestBadStatus confirms a status outside pass/fail is misuse (exit 2).
func TestAttestBadStatus(t *testing.T) {
	key := recordC2(t)
	_, _, code := execCLI(t, "attest", "feature", key, "--criterion", "C2", "--status", "maybe", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
}

// TestAttestUnknownCriterion confirms attesting a criterion absent from the eval
// file is misuse (exit 2).
func TestAttestUnknownCriterion(t *testing.T) {
	key := layEvalFeature(t)
	_, _, code := execCLI(t, "attest", "feature", key, "--criterion", "C9", "--status", "pass", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
}

// TestAttestOffAggregateIsBlocked confirms a feature attest run off its coarse
// aggregate branch is a branch refusal (exit 1) carrying the machine-readable
// reason and the aggregate branch, not a bare failure (issue #150).
func TestAttestOffAggregateIsBlocked(t *testing.T) {
	key := layEvalFeature(t)
	path := filepath.Join(featureDir(t), "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, []byte("review_grain: feature\nbranch: tiller/reviews/pay\n")...), 0o644))

	out, _, code := execCLI(t, "attest", "feature", key, "--criterion", "C1", "--status", "pass", "--contributor", "jc", "--json")

	require.Equal(t, 1, code)
	require.Contains(t, out, `"reason": "attest_branch_mismatch"`)
	require.Contains(t, out, `"branch": "tiller/reviews/pay"`)
	require.Contains(t, out, `"current_branch": "main"`)
}
