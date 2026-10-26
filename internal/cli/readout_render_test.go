package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

const needsAttentionHeading = "**Needs attention**"

// requireNeedsYouSection accepts the one exact counts-list heading or the
// unchanged all-clear sentence. A per-kind bold header no longer counts as a
// needs-attention section.
func requireNeedsYouSection(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "Nothing needs you right now.") {
		require.NotContains(t, out, needsAttentionHeading, "empty and nonempty attention shapes are exclusive")
		return
	}
	require.Contains(t, out, needsAttentionHeading)
	require.Equal(t, 1, strings.Count(out, needsAttentionHeading))
}

// splitAtNext cuts a rendered block at its **Next:** label: what the block
// says above the ranked list, and the list itself.
func splitAtNext(t *testing.T, block string) (head, next string) {
	t.Helper()
	head, next, found := strings.Cut(block, "**Next:**\n")
	require.True(t, found, "the block closes on the ranked list: %s", block)
	return head, next
}

// requireNoReadoutBlock asserts out carries neither legal attention shape.
func requireNoReadoutBlock(t *testing.T, out string) {
	t.Helper()
	require.NotContains(t, out, "Nothing needs you right now.")
	require.NotContains(t, out, needsAttentionHeading)
}

// TestRenderReadoutMatchesItsGolden pins decompose's rendered block byte for
// byte, including the single Needs attention list and its inline queue seam.
func TestRenderReadoutMatchesItsGolden(t *testing.T) {
	r := views.Readout{
		Moment:   views.MomentDecompose,
		Headline: "Decomposition laid down",
		Artifact: "f70abe3",
		Entities: []views.ReadoutEntity{{Ref: "`platform-foundation` (E-1wmkgv)", Note: "7 features"}},
		Withheld: 9,
		Checks:   []views.ReadoutCheck{{Label: "Readiness", Result: "passed"}},
		NeedsYou: []views.ReadoutAction{{Text: "1 epic awaits your approval", Command: "tiller queue --kind approve_epic"}},
		Next: views.Next{Reader: "jc", Candidates: []views.NextCandidate{
			approveEpicMove("platform-foundation", "E-1wmkgv"),
			specifyMove("form-engine/certification-form-loading", "F-79dh3q"),
		}},
	}
	got := renderReadout(r)
	golden := filepath.Join("testdata", "readout_decompose.golden.md")
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

func TestRenderReadoutAlwaysCarriesTheShelfQueueLine(t *testing.T) {
	got := renderReadout(views.Readout{
		Moment:   views.MomentOrient,
		Headline: "Project oriented",
		Queue:    "queue: 2 bugs, 1 chore ready",
		Next:     views.Next{Reader: "jc", CaughtUp: true},
	})
	require.Contains(t, got, "\nqueue: 2 bugs, 1 chore ready\n")
	require.Less(t, strings.Index(got, "queue: 2 bugs"), strings.Index(got, "**Next:**"))
}

// TestRenderReadoutEmitsNoTableAndNoHeading holds the two hard format rules
// (task-4-brief.md, Step 1): the block sits inside the agent's own chat
// message, so a heading would fight its structure, and a table renders
// inconsistently across harnesses.
func TestRenderReadoutEmitsNoTableAndNoHeading(t *testing.T) {
	got := renderReadout(views.Readout{Moment: views.MomentSync, Headline: "1 crossing recorded"})
	require.NotContains(t, got, "|", "tables render inconsistently across harnesses")
	require.NotContains(t, got, "\n#", "headings would fight the harness's own message structure")
}

// approveEpicMove and specifyMove are the two moves a decomposition hands
// off, shaped the way views.NextOf words them.
func approveEpicMove(key, id string) views.NextCandidate {
	ref := key + " (" + id + ")"
	return views.NextCandidate{
		Actor: "human", Kind: "approve_epic", Action: "approve epic", Tier: 5,
		Node:    &views.NextNode{Noun: "epic", ID: id, Key: key, Ref: ref},
		Command: "tiller approve epic " + key, Reason: "its decomposition awaits your approval",
	}
}

func specifyMove(key, id string) views.NextCandidate {
	ref := key + " (" + id + ")"
	return views.NextCandidate{
		Actor: "agent", Kind: "specify", Action: "specify", Tier: 5,
		Node:  &views.NextNode{Noun: "feature", ID: id, Key: key, Ref: ref},
		Skill: "specify", Reason: "draft, with every dependency met",
	}
}

// The block ends on the ranked list: the **Next:** label on its own line,
// the shortlist numbered under it, the count of moves left out, and then any
// grain notes.
func TestRenderReadoutEndsOnTheRankedList(t *testing.T) {
	var moves []views.NextCandidate
	for _, id := range []string{"F-aaaaa1", "F-aaaaa2", "F-aaaaa3", "F-aaaaa4", "F-aaaaa5", "F-aaaaa6", "F-aaaaa7"} {
		moves = append(moves, specifyMove("epic/"+id, id))
	}
	got := renderReadout(views.Readout{
		Moment:     views.MomentSpecify,
		Headline:   "Specs written",
		Queue:      "queue: 0 bugs ready",
		Next:       views.Next{Reader: "jc", Candidates: moves},
		GrainNotes: []string{"a/b/c (T-aaaaaa) rides task grain; its PR must merge before submit feature a/b (F-aaaaaa)."},
	})

	want := "\n**Next:**\n" +
		"1. Agent: specify epic/F-aaaaa1 (F-aaaaa1). Draft, with every dependency met. `tiller guide specify`\n" +
		"2. Agent: specify epic/F-aaaaa2 (F-aaaaa2). Draft, with every dependency met. `tiller guide specify`\n" +
		"3. Agent: specify epic/F-aaaaa3 (F-aaaaa3). Draft, with every dependency met. `tiller guide specify`\n" +
		"4. Agent: specify epic/F-aaaaa4 (F-aaaaa4). Draft, with every dependency met. `tiller guide specify`\n" +
		"5. Agent: specify epic/F-aaaaa5 (F-aaaaa5). Draft, with every dependency met. `tiller guide specify`\n" +
		"2 more legal moves; `tiller next --all` lists them.\n" +
		"\n- a/b/c (T-aaaaaa) rides task grain; its PR must merge before submit feature a/b (F-aaaaaa).\n"
	require.True(t, strings.HasSuffix(got, want), got)
	require.Less(t, strings.Index(got, "queue: 0 bugs ready"), strings.Index(got, "**Next:**"))
	require.Equal(t, 1, strings.Count(got, "**Next"), "one Next label heads the whole list")
	require.NotContains(t, got, "F-aaaaa6", "the list shows the shortlist, not every move")
}

// A caught-up checkout still closes on Next, with the caught-up sentence and
// its hint rather than a dangling label. The hint renders as a sentence, so a
// lowercase hint ("no work is planned yet") opens with a capital.
func TestRenderReadoutClosesACaughtUpCheckoutOnNext(t *testing.T) {
	got := renderReadout(views.Readout{
		Moment:   views.MomentSync,
		Headline: "Crossings recorded",
		Next:     views.Next{Reader: "jc", CaughtUp: true, Hint: "held work waits"},
	})
	require.Equal(t, "**Crossings recorded**\n\nNothing needs you right now.\n\n**Next:**\nCaught up: nothing is legal to do right now.\nHeld work waits.\n", got)
}

func TestRenderReadoutUsesOneExactCountsOnlyNeedsAttentionList(t *testing.T) {
	got := renderReadout(views.Readout{
		Moment:   views.MomentDecompose,
		Headline: "Decomposition laid down",
		NeedsYou: []views.ReadoutAction{
			{Text: "3 questions await your answer", Command: "tiller queue --kind answer_question"},
			{Text: "2 task review gates await your attestation", Command: "tiller queue --kind attest_gate"},
		},
	})

	want := "**Decomposition laid down**\n\n**Needs attention**\n\n" +
		"- 3 questions await your answer · `tiller queue --kind answer_question`\n" +
		"- 2 task review gates await your attestation · `tiller queue --kind attest_gate`\n" +
		"\n**Next:**\n" + views.CaughtUpText + "\n"
	require.Equal(t, want, got)
	require.Equal(t, 1, strings.Count(got, needsAttentionHeading), "one umbrella heading, never one header per kind")
	require.NotContains(t, got, "\n\n- 2 task", "the bullets are one contiguous Markdown list")
	for _, concrete := range []string{
		"Q-private", "PRIVATE QUESTION STATEMENT", "private_gate_alpha", "PRIVATE_CRITERION",
		"tiller answer question", "tiller attest task", "```",
	} {
		require.NotContainsf(t, got, concrete, "the count list must reject concrete item detail %q", concrete)
	}
}

func TestRenderReadoutKeepsTheEmptyAttentionState(t *testing.T) {
	got := renderReadout(views.Readout{Moment: views.MomentSync, Headline: "Crossings recorded"})
	require.Equal(t, "**Crossings recorded**\n\nNothing needs you right now.\n\n**Next:**\n"+views.CaughtUpText+"\n", got)
	require.NotContains(t, got, needsAttentionHeading, "the empty state stays a sentence, not an empty list")
}

// TestRenderReadoutPassesNotEvaluatedThroughUnmangled pins the renderer's
// treatment of a check's "not evaluated" result: an ordinary result string, no
// different from "passed" or "9 of 12", not a sentinel the renderer detects
// and rewrites. specifyContent and epicSpecChecks (internal/views/readout.go)
// render this literal string when their caller supplied no data for a check,
// deliberately distinct from a real zero (task-3-report.md finding 1); the
// renderer must reproduce it verbatim rather than, say, dropping the line or
// swapping in its own "unknown" wording.
func TestRenderReadoutPassesNotEvaluatedThroughUnmangled(t *testing.T) {
	got := renderReadout(views.Readout{
		Moment:   views.MomentSpecify,
		Headline: "Specs written",
		Checks:   []views.ReadoutCheck{{Label: "Contracts valid", Result: "not evaluated"}},
	})
	require.Contains(t, got, "- Contracts valid: not evaluated\n")
}

// An unpushed warning belongs under its own task, not under its sibling.
func TestRenderReadoutRendersTheUnpushedWarningDirectlyUnderItsNode(t *testing.T) {
	got := renderReadout(views.Readout{
		Moment:   views.MomentExecute,
		Headline: "Execution status",
		Entities: []views.ReadoutEntity{
			{Ref: "form-engine/certification-form-loading/transcribe (T-aaaaaa)", Note: "PR #41",
				Unpushed: &views.UnpushedReviewView{Commit: "abc1234", Branch: "tiller/form-engine/certification-form-loading/transcribe", Upstream: "origin/tiller/form-engine/certification-form-loading/transcribe"}},
			{Ref: "form-engine/certification-form-loading/validate (T-bbbbbb)", Note: "PR #42"},
		},
	})

	first := strings.Index(got, "transcribe (T-aaaaaa)")
	warning := strings.Index(got, "abc1234")
	sibling := strings.Index(got, "validate (T-bbbbbb)")
	require.Greater(t, warning, first)
	require.Greater(t, sibling, warning)
	require.Equal(t, 1, strings.Count(got, "abc1234"))
}

func TestStableReadoutPrefersGenerationChangeAfterProjection(t *testing.T) {
	root := t.TempDir()
	tiller := filepath.Join(root, ".tiller")
	require.NoError(t, os.MkdirAll(tiller, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tiller, "tiller.yml"), []byte(`schema_version: 12
schema_epoch: initial
project:
  name: Readout stability
  key: readout-stability
  kind: greenfield
contributors: []
`), 0o644))
	tree, err := store.Load(root)
	require.NoError(t, err)

	_, err = stableReadout(tree, func() (string, error) {
		generation := filepath.Join(tiller, ".cache", "generation")
		require.NoError(t, os.MkdirAll(filepath.Dir(generation), 0o755))
		require.NoError(t, os.WriteFile(generation, []byte("next-generation"), 0o600))
		return "", os.ErrNotExist
	})
	require.Error(t, err)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)
}

// correctionFixture is the block issue #94 asked for, as views hands it to
// the renderer: the headline, both shas, two Changed lines, and the full
// block's own queue and Next lines.
func correctionFixture() views.Correction {
	return views.Correction{
		Moment:   views.MomentDecompose,
		Headline: "Decomposition corrected",
		Artifact: "a244b05",
		Since:    "33fd465",
		Changed: []string{
			"added technician-application/admin-landing-and-search (F-vbcm4r)",
			"linked 2 edges (1 cross-epic) on technician-application/admin-landing-and-search (F-vbcm4r)",
		},
		Queue: "queue: 0 bugs, 0 chores, 0 tweaks ready",
		Next: views.Next{Reader: "jc", Candidates: []views.NextCandidate{
			specifyMove("foundation/repo-and-local-dev", "F-jny6ol"),
			specifyMove("technician-application/admin-landing-and-search", "F-vbcm4r"),
		}},
	}
}

// TestRenderCorrectionMatchesItsGolden pins the correction block byte for
// byte: markdown with no outer fence, the same small vocabulary as the full
// block, and no Produced, Verification, or Needs attention section.
func TestRenderCorrectionMatchesItsGolden(t *testing.T) {
	got := renderCorrection(correctionFixture())
	want, err := os.ReadFile(filepath.Join("testdata", "readout_correction.golden.md"))
	require.NoError(t, err)
	require.Equal(t, string(want), got)
	for _, absent := range []string{"**Produced**", "**Verification**", needsAttentionHeading, "Nothing needs you right now."} {
		require.NotContains(t, got, absent, "the correction block omits the full block's %s", absent)
	}
}

// TestRenderCorrectionWithNothingRecordedSaysSo pins the empty case: the
// Changed section still renders, with one line naming the since sha, so a
// developer reading the block learns that the correction recorded nothing
// rather than wondering whether the section was dropped.
func TestRenderCorrectionWithNothingRecordedSaysSo(t *testing.T) {
	c := correctionFixture()
	c.Changed = []string{}
	got := renderCorrection(c)
	require.Contains(t, got, "**Changed**\n\n- nothing recorded since `33fd465`\n")
}

// TestRenderCorrectionWithoutACommitStillNamesSince pins the dirty-tree
// headline: no committed-at clause, the same rule the full block follows,
// while the since sha stays on the line because it is the block's subject.
func TestRenderCorrectionWithoutACommitStillNamesSince(t *testing.T) {
	c := correctionFixture()
	c.Artifact = ""
	got := renderCorrection(c)
	require.True(t, strings.HasPrefix(got, "**Decomposition corrected** (since `33fd465`)\n"), got)
	require.NotContains(t, got, "committed at")
}

// The holding question and the legal task are both moves; the held task is
// not.
func TestReadoutsListTheHoldingQuestionAndTheLegalTask(t *testing.T) {
	epic := &store.Epic{ID: "E-route0", Key: new("notifications"), Status: "ready"}
	feature := &store.Feature{ID: "F-route0", Key: new("notifications/history"), EpicKey: "notifications", Status: "ready"}
	held := &store.Task{ID: "T-held00", Key: new("notifications/history/preferred"), FeatureID: feature.ID, Status: "ready"}
	legal := &store.Task{ID: "T-legal0", Key: new("notifications/history/alternative"), FeatureID: feature.ID, Status: "ready"}
	feature.Order = []string{held.ID, legal.ID}
	question := &store.Question{ID: "Q-hold00", Title: "Delivery retention", Status: "open", Blocks: []string{held.ID}}
	tree := &store.Tree{
		Manifest:     &store.Manifest{},
		Epics:        map[string]*store.Epic{epic.ID: epic},
		Features:     map[string]*store.Feature{feature.ID: feature},
		Tasks:        map[string]*store.Task{held.ID: held, legal.ID: legal},
		FeatureTasks: map[string][]*store.Task{feature.ID: {held, legal}},
		Questions:    map[string]*store.Question{question.ID: question},
	}
	for _, moment := range []views.Moment{views.MomentOrient, views.MomentExecute, views.MomentSpecify} {
		t.Run(string(moment), func(t *testing.T) {
			full := renderReadout(views.ReadoutOfInput(tree, moment, views.ReadoutInput{}))
			correction := renderCorrection(views.CorrectionOfInput(tree, moment, views.CorrectionInput{Since: "previous"}))
			for _, output := range []string{full, correction} {
				_, next := splitAtNext(t, output)
				require.Contains(t, next, "You: answer "+question.ID+".")
				require.Contains(t, next, "`tiller claim task "+*legal.Key+"`")
				require.NotContains(t, next, held.ID, "a question-held task is not a move")
			}
		})
	}
}
