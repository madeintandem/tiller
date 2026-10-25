package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveCommand(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/loading", "--title", "Loading", "--contributor", "jc")
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--contributor", "jc")
	out, stderr, code := execCLI(t, "resolve", "bug", "alpha", "--by", "form-engine/loading", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Regexp(t, `^F-[a-z0-9]{6}$`, data["resolved_by"])
	out, stderr, code = execCLI(t, "show", "bug", "alpha")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "carried by: form-engine/loading (F-")
	require.Contains(t, out, "carried")
	_, stderr, code = execCLI(t, "resolve", "bug", "alpha", "--clear", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	out, _, code = execCLI(t, "resolve", "bug", "alpha", "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "resolve_by_required"`)
}

// TestResolvedByCancelledCarrierRendersThePhase pins the read surfaces for the
// recompute: once the carrier is cancelled, both the full node view and the
// status oracle say so beside the node's own status, the same parenthetical
// idiom the feature rollup phases use, so a reader is never told the bug is
// carried by work that will never merge.
func TestResolvedByCancelledCarrierRendersThePhase(t *testing.T) {
	initOwnedRepo(t)
	mustRun(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	mustRun(t, "add", "feature", "form-engine/loading", "--title", "Loading", "--contributor", "jc")
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--contributor", "jc")
	mustRun(t, "resolve", "bug", "alpha", "--by", "form-engine/loading", "--contributor", "jc")
	mustRun(t, "cancel", "feature", "form-engine/loading", "--reason", "dropped", "--contributor", "jc")

	out, stderr, code := execCLI(t, "show", "bug", "alpha")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "status: proposed (carrier cancelled)")
	require.Contains(t, out, "carried by: form-engine/loading (F-",
		"the pointer stays on the record for the human to clear or re-point")

	out, stderr, code = execCLI(t, "status", "bug", "alpha")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "state: proposed (carrier cancelled: it will never merge; resolve --clear or --by re-points it)")

	out, stderr, code = execCLI(t, "show", "bug", "alpha", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "carrier_cancelled", jsonData(t, out)["activity"])
}
