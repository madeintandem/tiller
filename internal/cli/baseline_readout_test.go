package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBaselineReadoutHumanJSONAndCorrectionPreserveCompleteReview(t *testing.T) {
	initOwnedRepo(t)
	root, err := os.Getwd()
	require.NoError(t, err)
	writeFixture(t, root, ".tiller/tiller.yml", strings.Replace(ownedManifest, "kind: greenfield", "kind: brownfield", 1))
	ledger := store.BaselineLedger{}
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	for i := range 5 {
		doc := store.BaselineDoc{Path: fmt.Sprintf("module-%d.md", i), Describes: []string{fmt.Sprintf("src/module-%d", i)}, DerivedFrom: []store.RevisionRef{
			{Path: "docs/retired-source.md", Kind: store.KindGitBlob, Value: []string{"git:abcdef0", "git:1234567"}[i%2], ObservedAt: at},
		}}
		ledger.Docs = append(ledger.Docs, doc)
		if i != 4 {
			writeFixture(t, root, ".tiller/design/"+doc.Path, "baseline prose\n")
		}
	}
	data, err := yaml.Marshal(ledger)
	require.NoError(t, err)
	writeFixture(t, root, store.BaselineRelPath, string(data))
	out, stderr, code := execCLI(t, "readout", "derive-baseline")
	require.Equal(t, 0, code, stderr)
	structured, stderr, code := execCLI(t, "readout", "derive-baseline", "--json")
	require.Equal(t, 0, code, stderr)
	var envelope struct {
		Data readoutData `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(structured), &envelope))
	require.Equal(t, "derive-baseline", envelope.Data.Moment)
	require.Equal(t, strings.TrimSpace(out), strings.TrimSpace(envelope.Data.Readout))
	for _, doc := range ledger.Docs {
		require.Contains(t, out, doc.Path)
	}
	for _, blob := range []string{"abcdef0", "1234567"} {
		require.Contains(t, out, "git show "+blob)
	}
	require.Contains(t, out, "module-4.md` · describes: src/module-4 · MISSING PROSE")
	require.Contains(t, out, "tiller unlink design")
	require.Contains(t, out, "tiller approve design")
	require.NotContains(t, out, "Nothing needs you right now")
	require.NotContains(t, out, "approve_design")
	require.NotContains(t, strings.ToLower(out), "dropped")
	require.NotContains(t, out, "Questions opened")
	require.Equal(t, 1, strings.Count(out, "**Next:**"))
	require.Contains(t, out, "**Next:**\n1. You: confirm design doc module-0.md.")
	correction, stderr, code := execCLI(t, "readout", "derive-baseline", "--since", "HEAD")
	require.Equal(t, 0, code, stderr)
	for _, doc := range ledger.Docs {
		require.Contains(t, correction, doc.Path)
	}
	require.Equal(t, out[strings.Index(out, "**Next:**"):], correction[strings.Index(correction, "**Next:**"):])
}

func TestBaselineApprovalReleasesGuideAndReportsOnlyActualConfirmations(t *testing.T) {
	for _, kind := range []string{store.KindBrownfield, store.KindLegacyMigration} {
		t.Run(kind, func(t *testing.T) {
			initOwnedRepo(t)
			root, err := os.Getwd()
			require.NoError(t, err)
			mode := store.CodeModeSurface
			if kind == store.KindLegacyMigration {
				mode = store.CodeModeReference
			}
			manifest := strings.Replace(ownedManifest, "kind: greenfield", "kind: "+kind, 1) + "inputs:\n  code:\n    - {root: src, mode: " + mode + "}\n"
			writeFixture(t, root, ".tiller/tiller.yml", manifest)
			writeFixture(t, root, "src/source.go", "package source\n")
			out, stderr, code := execCLI(t, "guide", "--json")
			require.Equal(t, 0, code, stderr)
			var suggestion struct {
				Data SuggestView `json:"data"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &suggestion))
			require.Equal(t, "derive-baseline", suggestion.Data.Skill)
			for _, doc := range []string{"-owner's guide.md", "pending.md"} {
				writeFixture(t, root, ".tiller/design/"+doc, "reviewed prose\n")
				_, stderr, code = execCLI(t, "link", "design", "--describes", "src", "--contributor", "jc", "--", doc)
				require.Equal(t, 0, code, stderr)
			}
			out, stderr, code = execCLI(t, "guide", "--json")
			require.Equal(t, 0, code, stderr)
			require.NoError(t, json.Unmarshal([]byte(out), &suggestion))
			// Registered docs await the human's confirmation: no agent move is
			// legal, so the suggestion is orient and the list names the review.
			require.Equal(t, "orient", suggestion.Data.Skill)
			require.Contains(t, suggestion.Data.Reason, "the moves above are yours")
			require.Len(t, suggestion.Data.Next.Candidates, 2)
			require.Equal(t, "confirm_design", suggestion.Data.Next.Candidates[0].Kind)
			require.Contains(t, suggestion.Data.Next.Candidates[0].Reason, "review")
			out, stderr, code = execCLI(t, "approve", "design", "--contributor", "jc", "--", "-owner's guide.md")
			require.Equal(t, 0, code, stderr)
			require.Contains(t, out, "Design docs confirmed: 1")
			require.NotContains(t, out, "Moved to ready")
			require.Contains(t, out, "**Next:**\n1. You: confirm design doc pending.md.",
				"the doc still pending stays a move once startup releases")
			require.Contains(t, out, "\nNo work is planned yet")
			require.NotContains(t, out, "Nothing needs you right now", "pending sibling still needs review without blocking startup")
			out, stderr, code = execCLI(t, "approve", "design", "--contributor", "jc", "--", "-owner's guide.md")
			require.Equal(t, 0, code, stderr)
			require.Contains(t, out, "Design docs confirmed: 0")
			require.Contains(t, out, "Confirmations unchanged: 1")
			out, stderr, code = execCLI(t, "guide", "--json")
			require.Equal(t, 0, code, stderr)
			require.NoError(t, json.Unmarshal([]byte(out), &suggestion))
			require.Equal(t, "orient", suggestion.Data.Skill)
			require.Contains(t, suggestion.Data.Next.Hint, "no work is planned yet")
			out, stderr, code = execCLI(t, "check", "readiness")
			require.Equal(t, 0, code, stderr)
			require.Contains(t, out, "No work is planned yet")
		})
	}
}

func TestBaselineReviewDoesNotHideWorkQueueAttention(t *testing.T) {
	got := renderReadout(views.Readout{
		Baseline: &views.BaselineReadout{Pending: 1, Review: &views.ReadoutAction{Owner: "human"}},
		NeedsYou: []views.ReadoutAction{{Text: "2 open questions await your answer", Command: "tiller queue --kind answer_question"}},
	})
	require.Contains(t, got, "tiller queue --kind answer_question")
	require.Contains(t, got, "Baseline review")
	require.NotContains(t, got, "Nothing needs you right now")
}

func TestReadoutDesignDocPresenceRefusesIntermediateSymlinks(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	writeFixture(t, external, "accepted.md", "external prose\n")
	writeFixture(t, root, ".tiller/design/local.md", "local prose\n")
	require.NoError(t, os.Symlink(external, filepath.Join(root, ".tiller/design/as-is")))
	tree := &store.Tree{Root: root, Baseline: store.BaselineLedger{Docs: []store.BaselineDoc{
		{Path: "as-is/accepted.md", Describes: []string{"."}},
		{Path: "local.md", Describes: []string{"."}},
		{Path: "missing.md", Describes: []string{"."}},
	}}}
	require.Equal(t, map[string]bool{
		"as-is/accepted.md": false,
		"local.md":          true,
		"missing.md":        false,
	}, readoutDesignDocPresence(tree))
}

func TestMissingConfirmedBaselineReadoutNeverClaimsAllClear(t *testing.T) {
	initOwnedRepo(t)
	root, err := os.Getwd()
	require.NoError(t, err)
	manifest := strings.Replace(ownedManifest, "kind: greenfield", "kind: brownfield", 1) + "inputs:\n  code:\n    - {root: src, mode: surface}\n"
	writeFixture(t, root, ".tiller/tiller.yml", manifest)
	writeFixture(t, root, "src/source.go", "package source\n")
	writeFixture(t, root, ".tiller/design/accepted.md", "reviewed prose\n")
	_, stderr, code := execCLI(t, "link", "design", "accepted.md", "--describes", "src", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = execCLI(t, "approve", "design", "accepted.md", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller/design/accepted.md")))

	out, stderr, code := execCLI(t, "readout", "derive-baseline")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "confirmed: 1 · unconfirmed: 0")
	require.Contains(t, out, "MISSING PROSE")
	require.Contains(t, out, "Baseline repair")
	require.Contains(t, out, "tiller unlink design")
	require.NotContains(t, out, "Nothing needs you right now")
	require.NotContains(t, out, "await human confirmation")
	require.Contains(t, out, "**Next:**\n"+views.CaughtUpText+"\nNo work is planned yet")
	structured, stderr, code := execCLI(t, "readout", "derive-baseline", "--json")
	require.Equal(t, 0, code, stderr)
	var envelope struct {
		Data readoutData `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(structured), &envelope))
	require.Equal(t, strings.TrimSpace(out), strings.TrimSpace(envelope.Data.Readout))
	correction, stderr, code := execCLI(t, "readout", "derive-baseline", "--since", "HEAD")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, correction, "Baseline repair")
	require.NotContains(t, correction, "Nothing needs you right now")
}
