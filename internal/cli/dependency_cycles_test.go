package cli

import (
	"encoding/json"
	"testing"

	"github.com/madeintandem/tiller/internal/core/readiness"
	"github.com/stretchr/testify/require"
)

// layCycleEpic lays the epic foundation with features skeleton and spike,
// the shape whose same-epic prerequisite used to read as a deadlock.
func layCycleEpic(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)
	for _, args := range [][]string{
		{"add", "epic", "foundation", "--title", "Foundation"},
		{"add", "feature", "foundation/skeleton", "--title", "Skeleton"},
		{"add", "feature", "foundation/spike", "--title", "Spike"},
	} {
		out, stderr, code := execCLI(t, append(args, "--contributor", "jc")...)
		require.Equal(t, 0, code, out+stderr)
	}
}

// cyclesIn decodes the dependency_cycles a command's --json envelope carries.
func cyclesIn(t *testing.T, out string) []readiness.Gap {
	t.Helper()
	var answer struct {
		OK   bool `json:"ok"`
		Data struct {
			DependencyCycles []readiness.Gap `json:"dependency_cycles"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &answer), out)
	require.True(t, answer.OK, out)
	return answer.Data.DependencyCycles
}

// blockingCycleMessage is the diagnosis `check readiness` reports for the
// tree on disk, the text a creating command must repeat.
func blockingCycleMessage(t *testing.T) string {
	t.Helper()
	gaps := readiness.DependencyDiagnostics(loadTestTree(t)).Gaps
	require.Len(t, gaps, 1)
	require.Equal(t, readiness.ReasonDependencyCycle, gaps[0].Reason)
	return gaps[0].Message
}

// A prerequisite between two features riding one epic aggregate is ordinary
// ordering inside the review, so neither the commands that record it nor
// readiness call it a cycle.
func TestSameEpicFeaturePrerequisiteUnderEpicGrainIsNotACycle(t *testing.T) {
	layCycleEpic(t)
	out, stderr, code := execCLI(t, "link", "feature", "foundation/spike", "--depends-on", "foundation/skeleton", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	out, stderr, code = execCLI(t, "configure", "epic", "foundation", "--review-grain", "epic", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	require.NotContains(t, out, "dependency cycle")
	out, stderr, code = execCLI(t, "add", "feature", "foundation/spike2", "--title", "Spike2", "--depends-on", "foundation/skeleton", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	require.Empty(t, cyclesIn(t, out))
	require.Empty(t, readiness.DependencyDiagnostics(loadTestTree(t)).Gaps)
}

// A dependent that takes its own review still deadlocks against a
// prerequisite riding the epic aggregate: the configure that closes the loop
// prints readiness's diagnosis and still succeeds.
func TestConfigureReviewGrainPrintsTheCycleItCreates(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		layCycleEpic(t)
		runAll(t,
			[]string{"link", "feature", "foundation/spike", "--depends-on", "foundation/skeleton"},
			[]string{"configure", "feature", "foundation/spike", "--review-grain", "feature"},
		)
		requireCycleDiagnosis(t, jsonOut, "review grain: epic recorded on",
			"configure", "epic", "foundation", "--review-grain", "epic")

		// The cycle is the earlier command's, so a later one does not repeat it.
		runAll(t, []string{"configure", "project", "--review-grain", "epic"})
	}
}

// requireCycleDiagnosis runs the command that closes a loop, in human or
// --json form, and requires it to succeed carrying readiness's own diagnosis.
func requireCycleDiagnosis(t *testing.T, jsonOut bool, confirmation string, args ...string) {
	t.Helper()
	args = append(args, "--contributor", "jc")
	if jsonOut {
		args = append(args, "--json")
	}
	out, stderr, code := execCLI(t, args...)
	require.Equal(t, 0, code, out+stderr)
	message := blockingCycleMessage(t)
	if jsonOut {
		require.Equal(t, []readiness.Gap{{Reason: readiness.ReasonDependencyCycle, Message: message}}, cyclesIn(t, out))
		return
	}
	require.Contains(t, out, confirmation)
	require.Contains(t, out, "Blocking dependency cycle: "+message)
}

func runAll(t *testing.T, commands ...[]string) {
	t.Helper()
	for _, args := range commands {
		out, stderr, code := execCLI(t, append(args, "--contributor", "jc")...)
		require.Equal(t, 0, code, out+stderr)
		require.NotContains(t, out, "dependency cycle")
	}
}

func TestLinkDependsOnPrintsTheCycleItCreates(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		layCycleEpic(t)
		runAll(t,
			[]string{"configure", "epic", "foundation", "--review-grain", "epic"},
			[]string{"configure", "feature", "foundation/spike", "--review-grain", "feature"},
		)
		requireCycleDiagnosis(t, jsonOut, "now depends on",
			"link", "feature", "foundation/spike", "--depends-on", "foundation/skeleton")
	}
}

// A feature added to an epic-grain epic joins its aggregate, so a
// prerequisite on work that waits for a sibling rider's completion closes a
// loop through the shared merge.
func TestAddDependsOnPrintsTheCycleItCreates(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		layCycleEpic(t)
		runAll(t,
			[]string{"configure", "epic", "foundation", "--review-grain", "epic"},
			[]string{"add", "epic", "other", "--title", "Other"},
			[]string{"add", "feature", "other/consumer", "--title", "Consumer", "--depends-on", "foundation/skeleton"},
		)
		requireCycleDiagnosis(t, jsonOut, "Created feature",
			"add", "feature", "foundation/late", "--title", "Late", "--depends-on", "other/consumer")
	}
}

func TestApproveReviewGrainPrintsTheCycleItCreates(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		layCycleEpic(t)
		runAll(t,
			[]string{"link", "feature", "foundation/spike", "--depends-on", "foundation/skeleton"},
			[]string{"configure", "feature", "foundation/spike", "--review-grain", "feature"},
		)
		requireCycleDiagnosis(t, jsonOut, "foundation",
			"approve", "epic", "foundation", "--review-grain", "epic")
	}
}
