package readiness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBaselineConfirmationIsAnIndependentExistingCodeGate(t *testing.T) {
	for _, kind := range []string{store.KindGreenfield, store.KindBrownfield, store.KindLegacyMigration} {
		t.Run(kind, func(t *testing.T) {
			m := loadRepo(t, completeManifest, true)
			m.Project.Kind = kind
			if kind == store.KindLegacyMigration {
				m.Inputs.Code = []store.CodeRoot{{Root: ".", Mode: store.CodeModeReference}}
			}
			tree := &store.Tree{Manifest: m}
			report := Check(m, tree)
			if kind == store.KindGreenfield {
				require.True(t, report.Ready)
			} else {
				require.Equal(t, []string{ReasonNoConfirmedBaseline}, baselineReasons(report))
			}
			tree.Baseline.Docs = []store.BaselineDoc{{Path: "00-index.md", Describes: []string{"vanished/code"}}}
			require.Equal(t, report, Check(m, tree), "a link is not a confirmation; described-path drift is not readiness")
			tree.Baseline.Docs[0].Confirmed = &store.BaselineConfirmation{By: "retired-owner"}
			tree.Baseline.Docs = append(tree.Baseline.Docs, store.BaselineDoc{Path: "pending.md", Describes: []string{"."}})
			require.NoError(t, store.WriteRepositoryFileAtomic(m.Root(), filepath.Join(".tiller/design/pending.md"), []byte("pending\n")))
			require.True(t, Check(m, tree).Ready, "one confirmation releases the gate; remaining docs and retired confirming owners do not block")
			require.True(t, Check(m, nil).Ready, "nil tree retains skip-tree-check semantics")
		})
	}
}

func TestMissingBaselineProseIsIndependentOfKindAndConfirmation(t *testing.T) {
	for _, kind := range []string{store.KindGreenfield, store.KindBrownfield, store.KindLegacyMigration} {
		t.Run(kind, func(t *testing.T) {
			m := loadRepo(t, completeManifest, true)
			m.Project.Kind = kind
			if kind == store.KindLegacyMigration {
				m.Inputs.Code = []store.CodeRoot{{Root: ".", Mode: store.CodeModeReference}}
			}
			tree := &store.Tree{Manifest: m, Baseline: store.BaselineLedger{Docs: []store.BaselineDoc{
				{Path: "z-missing.md", Describes: []string{"."}},
				{Path: "a-missing.md", Describes: []string{"."}, Confirmed: &store.BaselineConfirmation{By: "jc"}},
			}}}
			report := Check(m, tree)
			require.False(t, report.Ready)
			require.Equal(t, []string{ReasonMissingDesignDoc, ReasonMissingDesignDoc}, baselineReasons(report))
			require.Contains(t, report.Gaps[0].Message, "a-missing.md")
			require.Contains(t, report.Gaps[1].Message, "z-missing.md")
			tree.Baseline.Docs[1].Confirmed = nil
			report = Check(m, tree)
			want := []string{ReasonMissingDesignDoc, ReasonMissingDesignDoc}
			if kind != store.KindGreenfield {
				want = append([]string{ReasonNoConfirmedBaseline}, want...)
			}
			require.Equal(t, want, baselineReasons(report))
		})
	}
}

func TestSymlinkedBaselineProseNeedsRepairWithoutReopeningStartupGate(t *testing.T) {
	for _, intermediate := range []bool{false, true} {
		name := "leaf"
		if intermediate {
			name = "intermediate"
		}
		t.Run(name, func(t *testing.T) {
			m := loadRepo(t, completeManifest, true)
			m.Project.Kind = store.KindBrownfield
			external := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(external, "accepted.md"), []byte("external prose\n"), 0o644))
			doc := "accepted.md"
			target := filepath.Join(external, doc)
			link := filepath.Join(m.Root(), ".tiller", "design", doc)
			if intermediate {
				doc = "as-is/accepted.md"
				target = external
				link = filepath.Join(m.Root(), ".tiller", "design", "as-is")
			}
			require.NoError(t, os.Symlink(target, link))
			tree := &store.Tree{Manifest: m, Baseline: store.BaselineLedger{Docs: []store.BaselineDoc{
				{Path: doc, Describes: []string{"."}, Confirmed: &store.BaselineConfirmation{By: "jc"}},
			}}}
			report := Check(m, tree)
			require.False(t, report.Ready)
			require.Equal(t, []string{ReasonMissingDesignDoc}, baselineReasons(report))
			require.Contains(t, report.Gaps[0].Message, doc)
		})
	}
}

func TestBaselineDoesNotMaskOtherReadinessGaps(t *testing.T) {
	m := loadRepo(t, noOwnersManifest, false)
	m.Project.Kind = store.KindLegacyMigration
	m.Inputs.Code = []store.CodeRoot{{Root: "missing-code", Mode: "invalid"}}
	require.NoError(t, store.WriteRepositoryFileAtomic(m.Root(), filepath.Join(store.BaselineRelPath), []byte("docs: []\n")))
	require.NoError(t, store.WriteRepositoryFileAtomic(m.Root(), filepath.Join(".tiller/orientation.md"), []byte(restampedOrientation(t, 1))))
	tree := &store.Tree{Manifest: m, Baseline: store.BaselineLedger{Docs: []store.BaselineDoc{{Path: "missing.md", Describes: []string{"."}}}}}
	require.Equal(t, []string{store.ReasonMissingContributors, store.ReasonMissingDesign, store.ReasonInvalidCodeRootMode, store.ReasonMissingCodeRoot, store.ReasonNoReferenceRoot, ReasonStaleOrientation, ReasonNoConfirmedBaseline, ReasonMissingDesignDoc}, baselineReasons(Check(m, tree)))
	// The ledger alone is not a design input, without changing the existing
	// shallow directory check into a recursive prose-validation policy.
	require.NoError(t, store.WriteRepositoryFileAtomic(m.Root(), filepath.Join(".tiller/design/notes.txt"), []byte("input\n")))
	require.NotContains(t, baselineReasons(Check(m, nil)), store.ReasonMissingDesign)
}

func baselineReasons(report Report) []string {
	reasons := make([]string, 0, len(report.Gaps))
	for _, gap := range report.Gaps {
		reasons = append(reasons, gap.Reason)
	}
	return reasons
}
