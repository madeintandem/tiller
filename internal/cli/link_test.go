package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinkAttachAndUnlinkDetach(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--contributor", "jc")
	out, stderr, code := execCLI(t, "link", "bug", "alpha", "--attach", "form-engine", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "now attached to form-engine (E-")
	out, stderr, code = execCLI(t, "unlink", "bug", "alpha", "--detach", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "detached")
	out, _, code = execCLI(t, "link", "feature", "form-engine/x", "--attach", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "attach_noun"`)
}

// layCombinedShelfCLI lays an epic and three bugs: alpha is the node under
// test, beta and gamma are the shelf work it may depend on.
func layCombinedShelfCLI(t *testing.T) string {
	t.Helper()
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--contributor", "jc")
	mustRun(t, "add", "bug", "beta", "--title", "Beta", "--contributor", "jc")
	mustRun(t, "add", "bug", "gamma", "--title", "Gamma", "--contributor", "jc")
	root, err := repoRoot()
	require.NoError(t, err)
	return filepath.Join(root, ".tiller")
}

func showBug(t *testing.T, key string) map[string]any {
	t.Helper()
	out, stderr, code := execCLI(t, "show", "bug", key, "--json")
	require.Equal(t, 0, code, stderr)
	return jsonData(t, out)
}

// A combined link whose attachment cannot resolve must not leave the valid
// dependency half committed: no state, no event.
func TestCombinedLinkRefusalWritesNothing(t *testing.T) {
	tillerDir := layCombinedShelfCLI(t)
	for _, attach := range []string{"missing", "bugs/beta"} {
		before := snapshotTree(t, tillerDir)
		out, _, code := execCLI(t, "link", "bug", "alpha", "--depends-on", "bugs/beta", "--attach", attach, "--contributor", "jc", "--json")
		require.Equal(t, 2, code, out)
		require.Equal(t, before, snapshotTree(t, tillerDir), "a refused combined link (--attach %s) must write nothing", attach)
		require.Nil(t, showBug(t, "alpha")["depends_on"])
	}
}

// A blank --attach is the attach_target usage error, alone or beside a valid
// dependency, never read as "no attachment requested": the combined call writes
// nothing.
func TestLinkBlankAttachIsAttachTargetMisuse(t *testing.T) {
	tillerDir := layCombinedShelfCLI(t)
	for _, args := range [][]string{
		{"--attach", ""},
		{"--attach", "  "},
		{"--depends-on", "bugs/beta", "--attach", ""},
	} {
		before := snapshotTree(t, tillerDir)
		call := append([]string{"link", "bug", "alpha"}, args...)
		out, _, code := execCLI(t, append(call, "--contributor", "jc", "--json")...)
		require.Equal(t, 2, code, out)
		require.Contains(t, out, `"reason": "attach_target"`, "args %q", args)
		require.Equal(t, before, snapshotTree(t, tillerDir), "args %q must write nothing", args)
	}
}

func TestCombinedLinkRecordsBothFactsAndReportsThem(t *testing.T) {
	layCombinedShelfCLI(t)
	beta := showBug(t, "beta")["id"]
	epic := func() any {
		out, _, _ := execCLI(t, "show", "epic", "form-engine", "--json")
		return jsonData(t, out)["id"]
	}()

	out, stderr, code := execCLI(t, "link", "bug", "alpha", "--depends-on", "bugs/beta", "--attach", "form-engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, true, data["changed"])
	require.Equal(t, []any{beta}, data["depends_on"])
	require.Equal(t, epic, data["attached_to"])

	shown := showBug(t, "alpha")
	require.Equal(t, []any{beta}, shown["depends_on"])
	require.Equal(t, epic, shown["attached_to"])
}

// unlink --depends-on with --detach removes both facts, and a repeat is a
// truthful no-op naming the surviving dependencies.
func TestCombinedUnlinkRemovesBothFacts(t *testing.T) {
	layCombinedShelfCLI(t)
	beta := showBug(t, "beta")["id"]
	gamma := showBug(t, "gamma")["id"]
	mustRun(t, "link", "bug", "alpha", "--depends-on", "bugs/beta", "--depends-on", "bugs/gamma", "--attach", "form-engine", "--contributor", "jc")

	out, stderr, code := execCLI(t, "unlink", "bug", "alpha", "--depends-on", "bugs/beta", "--detach", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, true, data["changed"])
	require.Equal(t, []any{beta}, data["removed"])
	require.Equal(t, []any{gamma}, data["depends_on"])

	shown := showBug(t, "alpha")
	require.Equal(t, []any{gamma}, shown["depends_on"])
	require.Nil(t, shown["attached_to"])

	out, stderr, code = execCLI(t, "unlink", "bug", "alpha", "--depends-on", "bugs/beta", "--detach", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data = jsonData(t, out)
	require.Equal(t, false, data["changed"])
	require.Equal(t, []any{}, data["removed"])
	require.Equal(t, []any{gamma}, data["depends_on"])
}

// An invalid dependency request leaves the attachment in place.
func TestCombinedUnlinkInvalidDependencyKeepsAttachment(t *testing.T) {
	tillerDir := layCombinedShelfCLI(t)
	mustRun(t, "link", "bug", "alpha", "--depends-on", "bugs/beta", "--attach", "form-engine", "--contributor", "jc")
	before := snapshotTree(t, tillerDir)
	out, _, code := execCLI(t, "unlink", "bug", "alpha", "--depends-on", "bugs/missing", "--detach", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Equal(t, before, snapshotTree(t, tillerDir))
	require.NotNil(t, showBug(t, "alpha")["attached_to"])
}

// add for a shelf noun treats an explicitly blank --attach like link does:
// attach_target misuse with no write. Omitting --attach still adds unattached.
func TestAddShelfBlankAttachIsAttachTargetMisuse(t *testing.T) {
	for _, noun := range []string{"bug", "chore", "tweak"} {
		for _, blank := range []string{"", "  "} {
			tillerDir := layCombinedShelfCLI(t)
			before := snapshotTree(t, tillerDir)
			out, _, code := execCLI(t, "add", noun, "gamma", "--title", "G", "--attach", blank, "--contributor", "jc", "--json")
			require.Equal(t, 2, code, out)
			require.Contains(t, out, `"reason": "attach_target"`, "%s %q", noun, blank)
			require.Equal(t, before, snapshotTree(t, tillerDir), "%s %q must write nothing", noun, blank)
		}
		layCombinedShelfCLI(t)
		out, _, code := execCLI(t, "add", noun, "gamma", "--title", "G", "--contributor", "jc", "--json")
		require.Equal(t, 0, code, out)
	}
}
