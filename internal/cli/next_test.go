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
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

const nextManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
  - { id: bob, name: Bob }
`

// initNextRepo lays a committed repo whose roster names jc and bob, with
// no identity file and no ambient TILLER_CONTRIBUTOR.
func initNextRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("TILLER_CONTRIBUTOR", "")
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "tiller.yml"), []byte(nextManifest), 0o644))
	testgit.Init(t, dir)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")
	return dir
}

// layNextEpics adds n draft epics, each an approve_epic move for a human.
func layNextEpics(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		mustRun(t, "add", "epic", fmt.Sprintf("epic-%d", i), "--title", fmt.Sprintf("Epic %d", i), "--contributor", "jc")
	}
}

// layBobsBug adds an approved bug claimed by bob: bob's tier 2 Continue move.
func layBobsBug(t *testing.T, dir string) {
	t.Helper()
	mustRun(t, "add", "bug", "alpha", "--title", "Alpha", "--contributor", "jc")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".tiller", "bugs", "alpha", "bug.md"), []byte("# Alpha\n\n## Reproduction\n\nx\n"), 0o644))
	mustRun(t, "approve", "bug", "alpha", "--contributor", "jc")
	mustRun(t, "claim", "bug", "alpha", "--contributor", "bob")
}

// decodeNext reads a --json envelope's data block back into the Next payload.
func decodeNext(t *testing.T, raw string) views.Next {
	t.Helper()
	b, err := json.Marshal(jsonData(t, raw))
	require.NoError(t, err)
	var next views.Next
	require.NoError(t, json.Unmarshal(b, &next))
	return next
}

// numberedLines are the output lines that start a ranked entry.
func numberedLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 2 && line[0] >= '1' && line[0] <= '9' && line[1] == '.' {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestNextPrintsTheShortlistForTheIdentity(t *testing.T) {
	dir := initNextRepo(t)
	layNextEpics(t, 7)
	require.NoError(t, store.WriteLocalIdentity(dir, "alice"))

	out, stderr, code := execCLI(t, "next")
	require.Equal(t, 0, code, stderr)
	require.True(t, strings.HasPrefix(out, "Up next for alice:\n"), out)
	lines := numberedLines(out)
	require.Len(t, lines, 5, out)
	require.Contains(t, lines[0], "1. You: ")
	require.Contains(t, lines[0], "`tiller approve epic ")
	require.Contains(t, out, "2 more legal moves; `tiller next --all` lists them.")
	require.NotContains(t, out, "acting as", "next is a read, so it speaks no actor line")
}

func TestNextAllPrintsEveryCandidate(t *testing.T) {
	dir := initNextRepo(t)
	layNextEpics(t, 7)
	require.NoError(t, store.WriteLocalIdentity(dir, "alice"))

	out, stderr, code := execCLI(t, "next", "--all")
	require.Equal(t, 0, code, stderr)
	require.Len(t, numberedLines(out), 7, out)
	require.NotContains(t, out, "more legal move")
}

func TestNextContributorFlagRanksForAnother(t *testing.T) {
	dir := initNextRepo(t)
	layNextEpics(t, 2)
	layBobsBug(t, dir)
	require.NoError(t, store.WriteLocalIdentity(dir, "jc"))

	out, stderr, code := execCLI(t, "next", "--contributor", "bob", "--json")
	require.Equal(t, 0, code, stderr)
	next := decodeNext(t, out)
	require.Equal(t, "bob", next.Reader)
	var found bool
	for _, c := range next.Candidates {
		if c.Kind == "continue" {
			found = true
			require.Equal(t, 2, c.Tier)
			require.Equal(t, "bugs/alpha", c.Node.Key)
		}
	}
	require.True(t, found, "bob's own claimed bug is a Continue move: %s", out)

	human, stderr, code := execCLI(t, "next", "--contributor", "bob")
	require.Equal(t, 0, code, stderr)
	require.True(t, strings.HasPrefix(human, "Up next for bob:\n"), human)
	require.Contains(t, human, "Agent: continue bugs/alpha")
	require.Contains(t, human, "`tiller guide execute`")
}

func TestNextWithoutIdentitySaysNotPersonalized(t *testing.T) {
	initNextRepo(t)
	layNextEpics(t, 2)

	out, stderr, code := execCLI(t, "next")
	require.Equal(t, 0, code, stderr)
	require.True(t, strings.HasPrefix(out, "Up next (not personalized; run `tiller join`):\n"), out)
	require.Len(t, numberedLines(out), 2, out)
}

func TestNextJSONMatchesTheNextPayload(t *testing.T) {
	dir := initNextRepo(t)
	layNextEpics(t, 7)
	require.NoError(t, store.WriteLocalIdentity(dir, "alice"))

	out, stderr, code := execCLI(t, "next", "--json")
	require.Equal(t, 0, code, stderr)
	next := decodeNext(t, out)
	require.Equal(t, "alice", next.Reader)
	require.Len(t, next.Candidates, 5)
	require.False(t, next.CaughtUp)
	require.NotContains(t, jsonData(t, out), "actor", "a read carries no actor attribution")

	out, stderr, code = execCLI(t, "next", "--all", "--json")
	require.Equal(t, 0, code, stderr)
	require.Len(t, decodeNext(t, out).Candidates, 7)
}

func TestNextCaughtUpAndUnplanned(t *testing.T) {
	initNextRepo(t)

	out, stderr, code := execCLI(t, "next")
	require.Equal(t, 0, code, stderr)
	require.True(t, strings.HasPrefix(out, "Caught up: nothing is legal to do right now.\n"), out)
	require.Empty(t, numberedLines(out))
	jsonOut, stderr, code := execCLI(t, "next", "--json")
	require.Equal(t, 0, code, stderr)
	next := decodeNext(t, jsonOut)
	require.True(t, next.CaughtUp)
	require.True(t, next.Unplanned)
	require.NotEmpty(t, next.Hint)
	require.Contains(t, out, "\n"+views.Sentence(next.Hint)+"\n", "the zero-day hint prints on its own line, as a sentence")
}

func TestRenderNextLinesCopy(t *testing.T) {
	feature := &views.NextNode{Noun: "feature", ID: "F-ab12", Key: "checkout/alpha", Ref: "checkout/alpha (F-ab12)"}
	task := &views.NextNode{Noun: "task", ID: "T-ef56", Key: "checkout/alpha/t0", Ref: "checkout/alpha/t0 (T-ef56)"}
	n := views.Next{Reader: "jc", Candidates: []views.NextCandidate{
		{Actor: "human", Action: "approve the spec for", Node: feature, Command: "tiller approve feature checkout/alpha",
			Reason: "holds checkout/alpha/t1 (T-cd34) in your current feature"},
		{Actor: "agent", Action: "continue", Node: task, Skill: "execute", Reason: "your task is in progress"},
		{Actor: "agent", Action: "continue", Node: task, Branch: "task/t0", Reason: "your task is in flight"},
		{Actor: "human", Action: "confirm design doc docs/a.md", Command: "tiller approve design docs/a.md", Reason: "the baseline awaits review."},
		{Actor: "agent", Action: "derive the baseline for existing code", Skill: "derive-baseline", Reason: "no baseline yet"},
	}}
	require.Equal(t, strings.Join([]string{
		"1. You: approve the spec for checkout/alpha (F-ab12). Holds checkout/alpha/t1 (T-cd34) in your current feature. `tiller approve feature checkout/alpha`",
		"2. Agent: continue checkout/alpha/t0 (T-ef56). Your task is in progress. `tiller guide execute`",
		"3. Agent: continue checkout/alpha/t0 (T-ef56). Your task is in flight. Continue it on task/t0.",
		"4. You: confirm design doc docs/a.md. The baseline awaits review. `tiller approve design docs/a.md`",
		"5. Agent: derive the baseline for existing code. No baseline yet. `tiller guide derive-baseline`",
	}, "\n"), renderNextLines(n, false))
	require.Equal(t, "Up next for jc:", renderNextHeading(n))
	require.Equal(t, "Up next (not personalized; run `tiller join`):", renderNextHeading(views.Next{}))
}

// A reason that opens with a node key keeps its case: keys are case-sensitive.
func TestRenderNextLinesNeverAltersAKeyedReason(t *testing.T) {
	task := &views.NextNode{Noun: "task", ID: "T-x", Key: "checkout/alpha/t1", Ref: "checkout/alpha/t1 (T-x)"}
	n := views.Next{Reader: "jc", Candidates: []views.NextCandidate{
		{Actor: "agent", Action: "claim and start", Node: task, Skill: "execute",
			Reason: "checkout/alpha/t1 (T-x) is ready and unclaimed"},
		{Actor: "agent", Action: "claim and start", Node: task, Skill: "execute", Reason: "T-x is ready"},
		{Actor: "agent", Action: "claim and start", Node: task, Skill: "execute", Reason: "bugs/alpha is next"},
	}}
	lines := strings.Split(renderNextLines(n, false), "\n")
	require.Equal(t, "1. Agent: claim and start checkout/alpha/t1 (T-x). checkout/alpha/t1 (T-x) is ready and unclaimed. `tiller guide execute`", lines[0])
	require.Equal(t, "2. Agent: claim and start checkout/alpha/t1 (T-x). T-x is ready. `tiller guide execute`", lines[1])
	require.Equal(t, "3. Agent: claim and start checkout/alpha/t1 (T-x). bugs/alpha is next. `tiller guide execute`", lines[2])
}

func TestRenderNextLinesMoreLineCounts(t *testing.T) {
	n := views.Next{Reader: "jc"}
	for i := 0; i < 6; i++ {
		n.Candidates = append(n.Candidates, views.NextCandidate{Actor: "human", Action: "do", Reason: "r"})
	}
	require.Contains(t, renderNextLines(n, false), "1 more legal move; `tiller next --all` lists it.")
	n.Candidates = append(n.Candidates, n.Candidates[0])
	require.Contains(t, renderNextLines(n, false), "2 more legal moves; `tiller next --all` lists them.")
	require.NotContains(t, renderNextLines(n, true), "more legal move")
}

// An unplanned hint with candidates prints after the list, not in place of it.
func TestRenderNextLinesKeepsTheHintBesideCandidates(t *testing.T) {
	n := views.Next{
		Reader:     "jc",
		Candidates: []views.NextCandidate{{Actor: "human", Action: "answer", Reason: "a question is open"}},
		Unplanned:  true,
		Hint:       "No work is planned yet.",
	}
	got := renderNextLines(n, false)
	require.Equal(t, "1. You: answer. A question is open.\nNo work is planned yet.", got)
}

func TestRenderNextCaughtUpWithAHint(t *testing.T) {
	n := views.Next{Reader: "jc", CaughtUp: true, Hint: "Held work waits."}
	require.Equal(t, "Caught up: nothing is legal to do right now.\nHeld work waits.", renderNextLines(n, false))
	require.Equal(t, "Caught up: nothing is legal to do right now.", renderNextLines(views.Next{CaughtUp: true}, false))
}
