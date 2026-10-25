package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// layExtraEpic adds a draft epic so the ranked list always has a move to print
// after the act under test (an approve_epic move for jc).
func layExtraEpic(t *testing.T) {
	t.Helper()
	mustRun(t, "add", "epic", "extra-epic", "--title", "Extra epic", "--contributor", "jc")
}

// decodeEmbeddedNext reads the "next" field of a --json envelope's data block.
func decodeEmbeddedNext(t *testing.T, raw string) views.Next {
	t.Helper()
	b, err := json.Marshal(jsonData(t, raw)["next"])
	require.NoError(t, err)
	var next views.Next
	require.NoError(t, json.Unmarshal(b, &next))
	return next
}

func TestSubmitPrintsUpNext(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := walkTaskToCheck(t)
	layExtraEpic(t)

	out, stderr, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	head, block, found := strings.Cut(out, "\n\n**Up next:**\n")
	require.True(t, found, "submit appends an Up next block: %s", out)
	require.Contains(t, head, "in_review", "the confirmation stays first")
	require.NotEmpty(t, numberedLines(block), block)
	require.Contains(t, block, "`tiller approve epic ", "the list is the live ranking after the submit")
}

func TestCheckPassPrintsUpNext(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := driveToStartedTask(t)
	layExtraEpic(t)

	out, stderr, code := execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	head, block, found := strings.Cut(out, "\n\n**Up next:**\n")
	require.True(t, found, "a passing check appends an Up next block: %s", out)
	require.Contains(t, head, "all required gates green")
	require.NotEmpty(t, numberedLines(block), block)
}

func TestCheckFailurePrintsNoUpNext(t *testing.T) {
	initGatedGitRepo(t, "false")
	tid := driveToStartedTask(t)
	layExtraEpic(t)

	out, stderr, code := execCLI(t, "check", "task", tid, "--contributor", "jc")
	require.Equal(t, 1, code)
	require.NotContains(t, out+stderr, "Up next")

	jsonOut, _, code := execCLI(t, "check", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 1, code)
	require.NotContains(t, jsonData(t, jsonOut), "next")
}

func TestStartPrintsNoUpNext(t *testing.T) {
	initGatedGitRepo(t, "true")
	walkFeatureToActive(t)
	mustRun(t, "add", "task", cliFeatureKey+"/transcribe-the-schema", "--title", "Transcribe the schema", "--contributor", "jc")
	tid := soleTaskID(t)
	mustRun(t, "approve", "task", tid, "--contributor", "jc")
	layExtraEpic(t)

	out, stderr, code := execCLI(t, "start", "task", tid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.NotContains(t, out, "Up next", "beginning commands print no list (decision 9)")
	require.NotContains(t, jsonData(t, out), "next", out)
}

func TestSubmitJSONCarriesNext(t *testing.T) {
	initGatedGitRepo(t, "true")
	tid := walkTaskToCheck(t)
	layExtraEpic(t)

	out, stderr, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "in_review", data["status"], "the existing shape is kept")
	require.Equal(t, tid, data["id"])
	next := decodeEmbeddedNext(t, out)
	require.Equal(t, "jc", next.Reader)
	require.NotEmpty(t, next.Candidates)
	require.LessOrEqual(t, len(next.Candidates), 5, "the JSON next is the shortlist, as in `tiller next --json`")
}

// A coarse-grain feature submit is a completing command too: it ends on the
// list, and its JSON keeps the review-surface fields beside "next".
func TestSubmitFeaturePrintsUpNext(t *testing.T) {
	mergeReadyFeatureFixture(t)
	layExtraEpic(t)

	out, stderr, code := execCLI(t, "submit", "feature", cliFeatureKey, "--pr", "99", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	head, block, found := strings.Cut(out, "\n\n**Up next:**\n")
	require.True(t, found, out)
	require.Contains(t, head, "pr #99")
	require.NotEmpty(t, numberedLines(block), block)

	jsonOut, stderr, code := execCLI(t, "submit", "feature", cliFeatureKey, "--pr", "99", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.EqualValues(t, 99, jsonData(t, jsonOut)["pr"])
	require.NotEmpty(t, decodeEmbeddedNext(t, jsonOut).Candidates)
}
