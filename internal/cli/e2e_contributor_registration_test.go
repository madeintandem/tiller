package cli

import (
	"testing"

	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// This file is the contributor-registration-and-attribution design's own end-to-end
// net (docs/plans/2026-08-12-owner-registration-design.md), replaying the
// Lantern incident the design closes at the real CLI rather than through the
// unit-level pins Tasks 1 and 2 already left in join_test.go,
// identity_resolution_test.go, and amendment_cmds_test.go. Those pins prove
// each mechanism in isolation; this test drives the whole story in order, the
// way a developer or an agent actually meets it: bootstrap, a bare mutation
// that falls through to the recorded identity, an intake-style amendment
// approve that mints new nodes under that same fallback, and the two refusals
// that keep an unregistered id from ever becoming a contributor, whichever source
// named it.

// ownerArcFeedbackDoc is a small feedback artifact landed through the real
// `tiller intake` command, the same mechanism that carried "Allen Marks" into
// Lantern's manifest: a document outside the tree makes a case for new scope,
// and the staged op cites the line it came from.
const ownerArcFeedbackDoc = `# Field note

1. No epic in the tree covers infrastructure hardening: secrets rotation
   and dependency pinning have no home. Recommend a "hardening" epic.
`

// TestEndToEndContributorRegistrationBootstrapAttributionAndRefusals drives the
// design's whole arc through the real CLI: the three-command bootstrap
// (init, a human's add contributor, join --as), a bare mutation that falls through
// to the identity join recorded and says so on both surfaces, an
// intake-landed amendment whose bare approve names the same fallback and
// credits it with what the batch minted, and the two refusals, an unknown
// --as and an unknown TILLER_CONTRIBUTOR, that prove no source of the acting id
// can register a person onto the roster, only `tiller add contributor` can.
func TestEndToEndContributorRegistrationBootstrapAttributionAndRefusals(t *testing.T) {
	// --- bootstrap: init, then a human's add contributor, then join --as ---
	dir := initRepo(t)
	// A real git repo with one commit, the same precondition
	// layIntakeE2EShape and initOwnedRepo already establish: intake's own
	// fingerprinting and approve's accepted_against anchor both need a
	// resolvable HEAD, which a bare `tiller init` alone does not create.
	testgit.Init(t, dir)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")

	registerContributor(t, "jc", "JC Grubbs")

	joinOut, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code, joinOut)
	require.Contains(t, joinOut, "Joined as jc (JC Grubbs)")

	// A bare join with neither --as nor TILLER_CONTRIBUTOR set from here on falls
	// through to whatever .tiller/identity.yml the join above just wrote.
	t.Setenv("TILLER_CONTRIBUTOR", "")

	// --- a mutation with no --contributor and no env: the fallback speaks ---
	//
	// Human output carries the echo (design doc, "The fallback speaks").
	humanOut, _, code := execCLI(t, "add", "epic", "hardening", "--title", "Infrastructure hardening")
	require.Equal(t, 0, code, humanOut)
	require.Contains(t, humanOut, "acting as jc (.tiller/identity.yml)",
		"a bare mutation on a joined checkout falls through to the recorded identity and says so")

	// The --json envelope carries the same fact as a top-level actor field,
	// always present regardless of source (a second, distinct node, so this
	// call has its own fresh key to create).
	jsonOut, _, code := execCLI(t, "add", "feature", "hardening/csp-headers", "--title", "CSP headers", "--json")
	require.Equal(t, 0, code, jsonOut)
	require.Contains(t, jsonOut, `"actor": {`)
	require.Contains(t, jsonOut, `"id": "jc"`)
	require.Contains(t, jsonOut, `"source": ".tiller/identity.yml"`)

	// --- an intake-style amendment approve: the incident's own shape ---
	//
	// Land a feedback document through the real `tiller intake`, the same
	// mechanism that carried "Allen Marks" into Lantern's manifest, stage the
	// op it recommends, then approve BARE, no --contributor, no env, so the
	// approve itself resolves the acting contributor through the same fallback
	// chain the mutation above did.
	src := writeFeedbackDoc(t, "field-note.md", ownerArcFeedbackDoc)
	intakeOut, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	aid, _ := jsonData(t, intakeOut)["id"].(string)
	require.NotEmpty(t, aid, "intake mints an amendment id")

	_, stderr, code = execCLI(t, "amend", aid, "--add-epic", "reliability",
		"--title", "Reliability hardening",
		"--precondition", "key_absent=reliability",
		"--source", ".tiller/feedback/field-note.md#L3-L4", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	approveOut, _, code := execCLI(t, "approve", "amendment", aid)
	require.Equal(t, 0, code, approveOut)
	require.Contains(t, approveOut, "acting as jc (.tiller/identity.yml)",
		"a bare approve falls through to the identity file exactly like the incident's own approve did")
	require.Contains(t, approveOut, "1 epic(s) minted and owned by jc.",
		"the epic the batch minted is credited to the identity approve resolved, visible in the output "+
			"that used to stay silent about it")
	require.NotContains(t, approveOut, "minted nodes owned by")

	// --- an unregistered --as refuses, naming the roster and the fix ---
	unknownAsOut, _, code := execCLI(t, "join", "--as", "allen", "--json")
	require.Equal(t, 3, code, "an id absent from the roster is refused, never fabricated onto it")
	require.Contains(t, unknownAsOut, "contributor_unknown")
	require.Contains(t, unknownAsOut, "allen", "the refusal names the id that failed")
	require.Contains(t, unknownAsOut, "jc", "the refusal names the registered roster")
	require.Contains(t, unknownAsOut, "tiller add contributor allen", "the refusal points at registering the id")

	// --- an unregistered TILLER_CONTRIBUTOR refuses the same way on a bare join ---
	t.Setenv("TILLER_CONTRIBUTOR", "allen")
	envOut, _, code := execCLI(t, "join", "--json")
	require.Equal(t, 3, code, "an id supplied by the environment is looked up on the roster exactly like --as")
	require.Contains(t, envOut, "contributor_unknown")
	require.Contains(t, envOut, "allen")
	require.Contains(t, envOut, "jc", "the refusal still names the registered roster, non-empty this time")
	require.Contains(t, envOut, "tiller add contributor allen")
}
