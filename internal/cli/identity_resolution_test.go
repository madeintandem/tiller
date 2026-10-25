package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// corruptIdentityFile overwrites root's .tiller/identity.yml with bytes that
// fail to parse as YAML: an unterminated flow sequence, chosen because it is
// unambiguously malformed rather than merely an unexpected shape.
func corruptIdentityFile(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "identity.yml"), []byte("contributor: [unterminated\n"), 0o644))
}

// TestMutationResolvesOwnerFromIdentityFile confirms the self-contained
// resolution chain: after `tiller join`, a mutation with no --contributor and no
// TILLER_CONTRIBUTOR attributes to the owner recorded in .tiller/identity.yml.
func TestMutationResolvesOwnerFromIdentityFile(t *testing.T) {
	initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	registerContributor(t, "jc", "JC Grubbs")

	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--json")
	require.Equal(t, 0, code, "the mutation resolves jc from .tiller/identity.yml with no flag or env")
	require.Contains(t, out, "\"created\": true")
}

// TestMutationBeforeJoinRefusesPointingAtJoin confirms a mutation in a freshly
// inited, un-joined repo (empty roster, no identity file) refuses with the
// environment error that points at `tiller join`.
func TestMutationBeforeJoinRefusesPointingAtJoin(t *testing.T) {
	initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--json")
	require.Equal(t, 3, code, "no owner resolvable before join is an environment refusal")
	require.Contains(t, out, store.ReasonContributorUnset)
	require.Contains(t, out, "tiller join", "the refusal points at join")
}

// TestFlagAndEnvOverrideIdentityFile confirms --contributor and TILLER_CONTRIBUTOR still
// override the identity file. With jc recorded as the local identity, an explicit
// owner that is absent from the roster is consulted (and refused as unknown)
// rather than silently falling back to the valid local jc.
func TestFlagAndEnvOverrideIdentityFile(t *testing.T) {
	initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	registerContributor(t, "jc", "JC Grubbs")

	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	// --contributor overrides the local jc identity: a non-roster owner is refused as
	// unknown, which it could only be if the flag beat the identity file.
	out, _, code := execCLI(t, "add", "epic", "e-flag", "--contributor", "nobody", "--json")
	require.Equal(t, 3, code)
	require.Contains(t, out, store.ReasonContributorUnknown, "--contributor overrides the identity file")

	// TILLER_CONTRIBUTOR overrides it the same way.
	t.Setenv("TILLER_CONTRIBUTOR", "nobody")
	out, _, code = execCLI(t, "add", "epic", "e-env", "--json")
	require.Equal(t, 3, code)
	require.Contains(t, out, store.ReasonContributorUnknown, "TILLER_CONTRIBUTOR overrides the identity file")
}

// TestMutationEchoesTheIdentityFileFallback confirms the human surface speaks
// a fallback resolution aloud (design doc, "The fallback speaks"): a bare
// mutation that falls through to .tiller/identity.yml prints "acting as <id>
// (<source>)" as part of its own report.
func TestMutationEchoesTheIdentityFileFallback(t *testing.T) {
	initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	registerContributor(t, "jc", "JC Grubbs")
	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "acting as jc (.tiller/identity.yml)")
}

// TestMutationEchoesTheEnvFallback confirms TILLER_CONTRIBUTOR is spoken the same
// way: it is still a fallback past the explicit flag, so the human output
// names it.
func TestMutationEchoesTheEnvFallback(t *testing.T) {
	initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "acting as jc (TILLER_CONTRIBUTOR)")
}

// TestMutationEchoesTrimmedEnvFallback confirms issue #58's trim symmetry: a
// padded TILLER_CONTRIBUTOR resolves and echoes the trimmed id, not the padded raw
// value. resolveJoinContributor (join.go) already trimmed its own copy of this
// fallback; resolveContributorFallback (add.go), the tail every mutation shares,
// did not, so a padded env var rendered an untrimmed "acting as  jc " echo
// and JSON actor id even though the stamped owner itself was always the
// trimmed id (store.ResolveContributor trims internally).
func TestMutationEchoesTrimmedEnvFallback(t *testing.T) {
	initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")
	t.Setenv("TILLER_CONTRIBUTOR", " jc ")

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "acting as jc (TILLER_CONTRIBUTOR)", "the echo names the trimmed id, not the padded env value")

	jsonOut, _, code := execCLI(t, "add", "feature", "form-engine/x", "--title", "X",
		"--change-type", "create", "--json")
	require.Equal(t, 0, code, jsonOut)
	data := jsonData(t, jsonOut)
	actorField, ok := data["actor"].(map[string]any)
	require.True(t, ok, "actor rides the data block additively")
	require.Equal(t, "jc", actorField["id"], "the JSON actor id is trimmed too")
}

// TestExplicitOwnerFlagPrintsNoEchoButJSONCarriesTheSource confirms an
// explicit --contributor stays silent on the human surface (design doc: it is
// "already on the command line the user typed") while the --json envelope's
// actor field still names "--contributor" as the source that won, always present
// regardless of which source resolved.
func TestExplicitOwnerFlagPrintsNoEchoButJSONCarriesTheSource(t *testing.T) {
	initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	registerContributor(t, "jc", "JC Grubbs")

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "acting as", "an explicit --contributor is already on the command line the user typed")

	jsonOut, _, code := execCLI(t, "add", "feature", "form-engine/x", "--title", "X",
		"--change-type", "create", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, jsonOut)
	data := jsonData(t, jsonOut)
	actorField, ok := data["actor"].(map[string]any)
	require.True(t, ok, "actor rides the data block additively")
	require.Equal(t, "jc", actorField["id"])
	require.Equal(t, "--contributor", actorField["source"])
}

// TestMutationRefusesCorruptIdentityFile confirms issue #52: a present but
// unparseable .tiller/identity.yml is no longer swallowed into contributor_unset.
// With no --contributor and no TILLER_CONTRIBUTOR, a mutation refuses by name
// (identity_unreadable, exit 3), naming the file and pointing at `tiller
// join --as <id>` to rewrite it, instead of masquerading as an un-joined
// checkout.
func TestMutationRefusesCorruptIdentityFile(t *testing.T) {
	dir := initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	corruptIdentityFile(t, dir)

	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--json")
	require.Equal(t, 3, code, "a corrupt identity file refuses rather than masquerading as absent")
	require.Contains(t, out, store.ReasonIdentityUnreadable)
	require.Contains(t, out, filepath.Join(dir, ".tiller", "identity.yml"), "the refusal names the file")
	require.Contains(t, out, "tiller join --as", "the refusal points at the fix")
	require.NotContains(t, out, store.ReasonContributorUnset, "corruption is never reported as absence")
}

// TestFlagAndEnvBypassACorruptIdentityFile confirms --contributor and TILLER_CONTRIBUTOR
// never consult .tiller/identity.yml at all, so a corrupt file is irrelevant
// on either path: neither refuses identity_unreadable, unlike the bare
// fallback in TestMutationRefusesCorruptIdentityFile above.
func TestFlagAndEnvBypassACorruptIdentityFile(t *testing.T) {
	dir := initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	registerContributor(t, "jc", "JC Grubbs")
	corruptIdentityFile(t, dir)

	out, _, code := execCLI(t, "add", "epic", "e-flag", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, store.ReasonIdentityUnreadable, "--contributor never consults the corrupt file")

	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	out, _, code = execCLI(t, "add", "epic", "e-env", "--json")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, store.ReasonIdentityUnreadable, "TILLER_CONTRIBUTOR never consults the corrupt file")
}

func TestLegacyOwnerEnvironmentDoesNotSelectAnActor(t *testing.T) {
	initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	t.Setenv("TILLER_OWNER", "jc")
	registerContributor(t, "jc", "JC")
	out, _, code := execCLI(t, "add", "epic", "example", "--json")
	require.Equal(t, 3, code, out)
	require.Contains(t, out, store.ReasonContributorUnset)
}

// Ordinary commands refuse a legacy owner identity and name the refresh that
// renames it. A bare join selects the legacy id (still checked against the
// roster) and rewrites the file with the contributor key.
func TestLegacyIdentityIsRefusedOutsideMigrationAndJoin(t *testing.T) {
	root := initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	registerContributor(t, "jc", "JC")
	identityPath := filepath.Join(root, ".tiller", "identity.yml")
	require.NoError(t, os.WriteFile(identityPath, []byte("owner: jc\n"), 0o644))
	out, _, code := execCLI(t, "add", "epic", "example", "--json")
	require.Equal(t, 3, code, out)
	require.Contains(t, out, store.ReasonIdentityUnreadable)
	require.Contains(t, out, "tiller upgrade --project-only")

	require.NoError(t, os.WriteFile(identityPath, []byte("owner: missing\n"), 0o644))
	out, _, code = execCLI(t, "join", "--json")
	require.Equal(t, 3, code, out)
	require.Contains(t, out, store.ReasonContributorUnknown)

	require.NoError(t, os.WriteFile(identityPath, []byte("owner: jc\ncontributor: jc\n"), 0o644))
	out, _, code = execCLI(t, "join", "--json")
	require.Equal(t, 3, code, out)
	require.Contains(t, out, store.ReasonIdentityUnreadable)

	require.NoError(t, os.WriteFile(identityPath, []byte("owner: jc\n"), 0o644))
	out, _, code = execCLI(t, "join", "--json")
	require.Equal(t, 0, code, out)
	id, err := store.ReadLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "jc", id)
	out, _, code = execCLI(t, "add", "epic", "example", "--json")
	require.Equal(t, 0, code, out)
}

// A legacy owner identity carries its own remedy, the in-place rename, so the
// refusal states that one remedy and not join's rewrite as a second.
func TestLegacyIdentityRefusalNamesOneRemedy(t *testing.T) {
	root := initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	registerContributor(t, "jc", "JC")
	identityPath := filepath.Join(root, ".tiller", "identity.yml")
	require.NoError(t, os.WriteFile(identityPath, []byte("owner: jc\n"), 0o644))
	_, stderr, code := execCLI(t, "add", "epic", "example")
	require.Equal(t, 3, code, stderr)
	require.Equal(t, "tiller: .tiller/identity.yml cannot be read ("+identityPath+
		" contains legacy owner; schema 9 uses contributor identity and contributors roster; "+
		"run `tiller upgrade --project-only` (or a bare `tiller join`) to rename the key in place)\n", stderr)
}
