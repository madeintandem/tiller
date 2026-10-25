package drift_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func baselineWrite(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func baselineFixture(t *testing.T, described string, roots ...store.CodeRoot) (*store.Tree, mutate.Deps) {
	t.Helper()
	root := scratchProjectGit(t)
	gitRun(t, root, "config", "core.abbrev", "7")
	baselineWrite(t, root, "app/main.go", "package app\n")
	baselineWrite(t, root, ".tiller/design/as-is/model.md", "# Model\n")
	manifest, err := store.LoadManifest(root)
	require.NoError(t, err)
	if len(roots) == 0 {
		roots = []store.CodeRoot{{Root: ".", Mode: "surface"}}
	}
	manifest.Inputs.Code = roots
	manifest.Drift.Checks = []string{drift.MarkerStaleDesign, drift.MarkerShiftedDesign}
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	baselineWrite(t, root, ".tiller/tiller.yml", string(data))
	gitCommitAll(t, root, "baseline inputs")
	deps := seqDeps()
	_, err = mutate.LinkDesign(loadTree(t, root), deps, mutate.DesignLinkInput{Doc: "as-is/model.md", Describes: []string{described}})
	require.NoError(t, err)
	_, err = mutate.ApproveDesign(loadTree(t, root), deps, []string{"as-is/model.md"})
	require.NoError(t, err)
	return loadTree(t, root), deps
}

func baselineFeature(tree *store.Tree, id string, crossing store.RevisionRef, surfaces ...string) {
	key := "epic/" + strings.TrimPrefix(id, "F-")
	tree.Features[id] = &store.Feature{ID: id, Key: &key, EpicKey: "epic", Status: "done", Surface: store.Surface{Paths: surfaces}, VerifiedAgainst: store.VerifiedAgainst{Commit: crossing}}
}

func baselineCommit(t *testing.T, tree *store.Tree, name string) store.RevisionRef {
	t.Helper()
	baselineWrite(t, tree.Root, name, name+"\n")
	gitCommitAll(t, tree.Root, name)
	ref, err := vcs.New(tree.Root).CurrentCommit()
	require.NoError(t, err)
	ref.ObservedAt = fixedInstant.Add(time.Hour)
	return ref
}

func TestDesignDriftJoinsEveryCrossingAndCountsDistinctDocs(t *testing.T) {
	tree, _ := baselineFixture(t, "app")
	before, err := os.ReadFile(filepath.Join(tree.Root, store.BaselineRelPath))
	require.NoError(t, err)
	first := baselineCommit(t, tree, "app/first.go")
	rev := vcs.New(tree.Root)
	rev.Clock = driftFixedClock{t: fixedInstant}
	second := baselineCommit(t, tree, "app/second.go")
	baselineFeature(tree, "F-first1", first, "app/main.go")
	baselineFeature(tree, "F-last01", second, ".")
	baselineFeature(tree, "F-other1", second, "apple")
	rep, err := drift.Run(tree, rev)
	require.NoError(t, err)
	require.Empty(t, rep.Uncovered)
	require.Equal(t, 0, rep.Compared)
	require.Equal(t, 1, rep.ComparedDesign)
	require.Len(t, rep.Candidates, 2)
	stale := rep.Candidates[0]
	require.Equal(t, "design", stale.Noun)
	require.Empty(t, stale.ID)
	require.Equal(t, "as-is/model.md", stale.Key)
	require.Equal(t, []string{"app"}, stale.Files)
	require.Equal(t, tree.Baseline.Docs[0].Confirmed.Commit, *stale.Since)
	require.Equal(t, second, *stale.Until)
	require.Equal(t, []drift.DesignFeature{
		{ID: "F-first1", Key: "epic/first1", Spec: ".tiller/epics/epic/features/first1/feature.md", Commit: first},
		{ID: "F-last01", Key: "epic/last01", Spec: ".tiller/epics/epic/features/last01/feature.md", Commit: second},
	}, stale.Features)
	candidates, uncovered, err := drift.ForDesign(tree, rev, tree.Baseline.Docs[0])
	require.NoError(t, err)
	require.Equal(t, rep.Candidates, candidates)
	require.Equal(t, rep.Uncovered, uncovered)
	after, err := os.ReadFile(filepath.Join(tree.Root, store.BaselineRelPath))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestStaleDesignEqualAnchorOrdersCrossingAndReconfirmation(t *testing.T) {
	tree, deps := baselineFixture(t, "app")
	tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
	crossing := tree.Baseline.Docs[0].Confirmed.Commit
	baselineFeature(tree, "F-equal1", crossing, "app")
	rep, err := drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Empty(t, rep.Candidates, "equal commit and equal observation is not a later crossing")
	crossing.ObservedAt = crossing.ObservedAt.Add(time.Hour)
	tree.Features["F-equal1"].VerifiedAgainst.Commit = crossing
	rep, err = drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Len(t, rep.Candidates, 1)
	deps.Clock = driftFixedClock{t: crossing.ObservedAt.Add(time.Hour)}
	result, err := mutate.ApproveDesign(tree, deps, []string{"as-is/model.md"})
	require.NoError(t, err)
	require.True(t, result.Docs[0].Changed, "review must clear the crossing without another code commit")
	tree.Baseline.Docs[0] = result.Docs[0].Doc
	rep, err = drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Empty(t, rep.Candidates)
	require.Empty(t, rep.Uncovered)
	require.Equal(t, 1, rep.ComparedDesign)
}

func TestStaleDesignHistoryFailureNeverBecomesCleanOrFabricatesRange(t *testing.T) {
	t.Run("missing crossing", func(t *testing.T) {
		tree, _ := baselineFixture(t, "app")
		tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
		baselineFeature(tree, "F-miss01", store.RevisionRef{}, "app")
		rep, err := drift.Run(tree, vcs.New(tree.Root))
		require.NoError(t, err)
		require.Empty(t, rep.Candidates)
		require.Equal(t, "history_unavailable", rep.Uncovered[0].Reason)
		require.Zero(t, rep.ComparedDesign)
	})
	t.Run("diverged crossing", func(t *testing.T) {
		tree, _ := baselineFixture(t, "app")
		tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
		gitRun(t, tree.Root, "checkout", "-b", "other", "HEAD~1")
		other := baselineCommit(t, tree, "other.txt")
		gitRun(t, tree.Root, "checkout", "-")
		baselineFeature(tree, "F-div001", other, "app")
		rep, err := drift.Run(tree, vcs.New(tree.Root))
		require.NoError(t, err)
		require.Empty(t, rep.Candidates)
		require.Equal(t, "history_diverged", rep.Uncovered[0].Reason)
		require.Zero(t, rep.ComparedDesign)
	})
	t.Run("unavailable confirmation with known missing path", func(t *testing.T) {
		tree, _ := baselineFixture(t, "app")
		tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
		tree.Baseline.Docs[0].Confirmed.Commit.Value = "git:0000000000000000000000000000000000000000"
		require.NoError(t, os.RemoveAll(filepath.Join(tree.Root, "app")))
		rep, err := drift.Run(tree, vcs.New(tree.Root))
		require.NoError(t, err)
		require.Len(t, rep.Candidates, 1)
		require.Nil(t, rep.Candidates[0].Until)
		require.Equal(t, "history_unavailable", rep.Uncovered[0].Reason)
	})
}

type failedDesignAncestry struct{ vcs.Revisions }

func (failedDesignAncestry) IsMergedInto(string, string) (bool, error) {
	return false, fmt.Errorf("ancestry unavailable")
}

func TestStaleDesignAncestryErrorIsUncovered(t *testing.T) {
	tree, _ := baselineFixture(t, "app")
	tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
	crossing := baselineCommit(t, tree, "app/later.go")
	baselineFeature(tree, "F-error1", crossing, "app")
	rep, err := drift.Run(tree, failedDesignAncestry{vcs.New(tree.Root)})
	require.NoError(t, err)
	require.Empty(t, rep.Candidates)
	require.Equal(t, "history_unavailable", rep.Uncovered[0].Reason)
	require.Zero(t, rep.ComparedDesign)
}

func TestDesignDriftMissingUnsafeUnreadableAndUnconfirmed(t *testing.T) {
	for _, condition := range []string{"missing", "symlink", "unreadable", "unconfirmed"} {
		t.Run(condition, func(t *testing.T) {
			tree, _ := baselineFixture(t, "app/main.go")
			full := filepath.Join(tree.Root, "app/main.go")
			switch condition {
			case "missing":
				require.NoError(t, os.Remove(full))
			case "symlink":
				require.NoError(t, os.Remove(full))
				require.NoError(t, os.Symlink("absent.go", full))
			case "unreadable":
				if os.Geteuid() == 0 {
					t.Skip("root can read mode-000 files")
				}
				require.NoError(t, os.Chmod(full, 0))
				t.Cleanup(func() { _ = os.Chmod(full, 0o644) })
			case "unconfirmed":
				tree.Baseline.Docs[0].Confirmed = nil
			}
			rep, err := drift.Run(tree, vcs.New(tree.Root))
			require.NoError(t, err)
			if condition == "missing" {
				require.Len(t, rep.Candidates, 2)
				require.Empty(t, rep.Uncovered)
				require.Equal(t, 1, rep.ComparedDesign)
			} else {
				require.Empty(t, rep.Candidates)
				require.Len(t, rep.Uncovered, 2)
				reason := "surface_unreadable"
				if condition == "unconfirmed" {
					reason = "unconfirmed"
				}
				for _, u := range rep.Uncovered {
					require.Equal(t, reason, u.Reason)
				}
				require.Zero(t, rep.ComparedDesign)
			}
		})
	}
}

func TestShiftedDesignUsesDirtyWorkingTreeAndExactInventory(t *testing.T) {
	for _, change := range []string{"bytes", "addition", "removal", "mode", "ignored", "metadata"} {
		t.Run(change, func(t *testing.T) {
			tree, _ := baselineFixture(t, "app")
			tree.Manifest.Drift.Checks = []string{drift.MarkerShiftedDesign}
			switch change {
			case "bytes":
				baselineWrite(t, tree.Root, "app/main.go", "package changed\n")
			case "addition":
				baselineWrite(t, tree.Root, "app/new.go", "package new\n")
			case "removal":
				require.NoError(t, os.Remove(filepath.Join(tree.Root, "app/main.go")))
			case "mode":
				require.NoError(t, os.Chmod(filepath.Join(tree.Root, "app/main.go"), 0o755))
			case "ignored":
				baselineWrite(t, tree.Root, ".git/info/exclude", "app/cache\n")
				baselineWrite(t, tree.Root, "app/cache", "ignored\n")
			case "metadata":
				baselineWrite(t, tree.Root, "app/.tiller/cache", "metadata\n")
			}
			rep, err := drift.Run(tree, vcs.New(tree.Root))
			require.NoError(t, err)
			require.Empty(t, rep.Uncovered)
			if change == "ignored" || change == "metadata" {
				require.Empty(t, rep.Candidates)
			} else {
				require.Len(t, rep.Candidates, 1)
				require.Equal(t, []string{"app"}, rep.Candidates[0].Files)
				require.Equal(t, rep.Candidates[0].Since.Value, rep.Candidates[0].Until.Value, "dirty drift needs no code commit")
			}
		})
	}
}

func TestDesignDriftExternalReferenceAndRootBoundary(t *testing.T) {
	external := t.TempDir()
	baselineWrite(t, external, "main.go", "old code\n")
	tree, _ := baselineFixture(t, external, store.CodeRoot{Root: external, Mode: "reference"}, store.CodeRoot{Root: ".", Mode: "surface"})
	crossing := baselineCommit(t, tree, "app/later.go")
	baselineFeature(tree, "F-local1", crossing, ".")
	baselineWrite(t, external, "new.go", "external addition\n")
	rep, err := drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Empty(t, rep.Uncovered)
	require.Len(t, rep.Candidates, 1)
	require.Equal(t, drift.MarkerShiftedDesign, rep.Candidates[0].Marker)
	require.Equal(t, tree.Baseline.Docs[0].Describes, rep.Candidates[0].Files)
}

func TestStaleDesignCanonicalDeclarationAliasMatchesRecordedCoordinates(t *testing.T) {
	tree, _ := baselineFixture(t, "app")
	require.NoError(t, os.Symlink("app", filepath.Join(tree.Root, "alias")))
	tree.Manifest.Inputs.Code = append(tree.Manifest.Inputs.Code, store.CodeRoot{Root: "alias", Mode: "surface"})
	tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
	crossing := baselineCommit(t, tree, "app/later.go")
	baselineFeature(tree, "F-alias1", crossing, "alias/../alias/main.go")
	rep, err := drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Empty(t, rep.Uncovered)
	require.Len(t, rep.Candidates, 1)
	require.Equal(t, []string{"app"}, rep.Candidates[0].Files)
}

func TestStaleDesignKeepsKnownWorkWithoutBoundingUnknownCrossing(t *testing.T) {
	tree, _ := baselineFixture(t, "app")
	tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
	crossing := baselineCommit(t, tree, "app/later.go")
	baselineFeature(tree, "F-known1", crossing, "app")
	baselineFeature(tree, "F-miss01", store.RevisionRef{}, "app")
	rep, err := drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Len(t, rep.Candidates, 1)
	require.Len(t, rep.Candidates[0].Features, 1)
	require.Equal(t, "F-known1", rep.Candidates[0].Features[0].ID)
	require.Nil(t, rep.Candidates[0].Until)
	require.Equal(t, "history_unavailable", rep.Uncovered[0].Reason)
	require.Zero(t, rep.ComparedDesign)
}

func TestStaleDesignReconfirmationIncludesAncestorCrossings(t *testing.T) {
	tree, _ := baselineFixture(t, "app")
	tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
	crossing := baselineCommit(t, tree, "app/landed.go")
	baselineFeature(tree, "F-before", crossing, "app")
	confirmation := baselineCommit(t, tree, "review.txt")
	tree.Baseline.Docs[0].Confirmed.Commit = confirmation
	rep, err := drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Empty(t, rep.Candidates)
	require.Empty(t, rep.Uncovered)
	require.Equal(t, 1, rep.ComparedDesign)
}

func TestShiftedDesignGitInventoryFailureIsNotClean(t *testing.T) {
	tree, _ := baselineFixture(t, "app")
	tree.Manifest.Drift.Checks = []string{drift.MarkerShiftedDesign}
	t.Setenv("GIT_INDEX_FILE", t.TempDir())
	rep, err := drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Empty(t, rep.Candidates)
	require.NotEmpty(t, rep.Uncovered)
	require.Equal(t, "surface_unreadable", rep.Uncovered[0].Reason)
	require.Zero(t, rep.ComparedDesign)
}

func TestStaleDesignRejectsDeclarationRetargetedToMetadata(t *testing.T) {
	tree, _ := baselineFixture(t, "app", store.CodeRoot{Root: "app", Mode: "surface"})
	tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
	require.NoError(t, os.RemoveAll(filepath.Join(tree.Root, "app")))
	require.NoError(t, os.Symlink(".git", filepath.Join(tree.Root, "app")))
	rep, err := drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Empty(t, rep.Candidates)
	require.Equal(t, "surface_unreadable", rep.Uncovered[0].Reason)
	require.Zero(t, rep.ComparedDesign)
}

func TestStaleDesignReconfirmationUsesCommitIdentityAcrossAbbreviations(t *testing.T) {
	tree, deps := baselineFixture(t, "app")
	tree.Manifest.Drift.Checks = []string{drift.MarkerStaleDesign}
	var crossing store.RevisionRef
	t.Run("observe crossing with longer abbreviation", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_COUNT", "1")
		t.Setenv("GIT_CONFIG_KEY_0", "core.abbrev")
		t.Setenv("GIT_CONFIG_VALUE_0", "40")
		var err error
		crossing, err = vcs.New(tree.Root).CurrentCommit()
		require.NoError(t, err)
	})
	require.NotEqual(t, tree.Baseline.Docs[0].Confirmed.Commit.Value, crossing.Value)
	crossing.ObservedAt = fixedInstant.Add(time.Hour)
	baselineFeature(tree, "F-abbrev", crossing, "app")
	rep, err := drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Len(t, rep.Candidates, 1)
	deps.Clock = driftFixedClock{t: fixedInstant.Add(2 * time.Hour)}
	result, err := mutate.ApproveDesign(tree, deps, []string{"as-is/model.md"})
	require.NoError(t, err)
	require.True(t, result.Docs[0].Changed)
	tree.Baseline.Docs[0] = result.Docs[0].Doc
	rep, err = drift.Run(tree, vcs.New(tree.Root))
	require.NoError(t, err)
	require.Empty(t, rep.Candidates)
	require.Empty(t, rep.Uncovered)
}
