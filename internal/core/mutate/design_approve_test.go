package mutate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

func TestDesignApprovalBatchIsAtomicAndRetriesAreNoOps(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	linkDesign(t, root, "api.md", "app/main.go")
	linkDesign(t, root, "data.md", "app/extra.go")
	before := designBytes(t, root, store.BaselineRelPath)
	beforeEvents, err := events.List(load(t, root))
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, "app", "extra.go")))
	_, err = mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md", "data.md"})
	require.Error(t, err)
	require.Equal(t, before, designBytes(t, root, store.BaselineRelPath))
	afterEvents, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, afterEvents, len(beforeEvents))
	designWrite(t, root, "app/extra.go", "package app\n// extra\n")
	result, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md", "data.md", "api.md"})
	require.NoError(t, err)
	require.Len(t, result.Docs, 2)
	require.True(t, result.Docs[0].Changed)
	require.True(t, result.Docs[1].Changed)
	a, b := result.Docs[0].Doc.Confirmed, result.Docs[1].Doc.Confirmed
	require.Equal(t, a.Commit.Value, b.Commit.Value)
	require.True(t, a.Commit.ObservedAt.Equal(b.Commit.ObservedAt))
	afterEvents, err = events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, afterEvents, len(beforeEvents)+2)
	before = designBytes(t, root, store.BaselineRelPath)
	deps := testDeps()
	deps.Clock = fixedClock{t: a.Commit.ObservedAt.Add(time.Hour)}
	result, err = mutate.ApproveDesign(load(t, root), deps, []string{"api.md", "data.md"})
	require.NoError(t, err)
	require.False(t, result.Docs[0].Changed)
	require.False(t, result.Docs[1].Changed)
	require.Equal(t, before, designBytes(t, root, store.BaselineRelPath))
	designWrite(t, root, "app/main.go", "package app\n// dirty code\n")
	result, err = mutate.ApproveDesign(load(t, root), deps, []string{"api.md", "data.md"})
	require.NoError(t, err)
	require.True(t, result.Docs[0].Changed)
	require.False(t, result.Docs[1].Changed)
	require.NotEqual(t, a.Surfaces[0].Value, result.Docs[0].Doc.Confirmed.Surfaces[0].Value)
}

func TestDesignApprovalRejectsStaleGenerationAndGuardedInputs(t *testing.T) {
	t.Parallel()
	for _, changedPath := range []string{"generation", store.BaselineRelPath, ".tiller/tiller.yml", ".tiller/design/api.md"} {
		t.Run(changedPath, func(t *testing.T) {
			root := designFixture(t)
			linkDesign(t, root, "api.md", "app")
			tree := load(t, root)
			before := designBytes(t, root, store.BaselineRelPath)
			deps := testDeps()
			if changedPath == "generation" {
				linkDesign(t, root, "data.md", "app")
			} else {
				deps.Mint = func(prefix string) string {
					data := designBytes(t, root, changedPath)
					designWrite(t, root, changedPath, string(data)+"\n# concurrently edited\n")
					return id.Mint(prefix)
				}
			}
			_, err := mutate.ApproveDesign(tree, deps, []string{"api.md"})
			var env *store.EnvError
			require.ErrorAs(t, err, &env)
			require.Equal(t, store.ReasonMutationStale, env.Reason)
			require.Nil(t, load(t, root).Baseline.Docs[0].Confirmed)
			if changedPath != store.BaselineRelPath && changedPath != "generation" {
				require.Equal(t, before, designBytes(t, root, store.BaselineRelPath))
			}
			evs, err := events.List(load(t, root))
			require.NoError(t, err)
			for _, ev := range evs {
				require.NotEqual(t, "approve.design", ev.Type)
			}
		})
	}
}

func TestDesignApprovalIgnoresRetiredBoardObstruction(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	linkDesign(t, root, "api.md", "app")
	board := filepath.Join(root, ".tiller", "board.md")
	require.NoFileExists(t, board)
	require.NoError(t, os.Mkdir(board, 0o755))
	result, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
	require.NoError(t, err)
	require.True(t, result.Docs[0].Changed)
	require.True(t, load(t, root).Baseline.HasConfirmedDocs())
	require.True(t, hasEvent(t, load(t, root), "approve.design"))
	require.DirExists(t, board)
}

func TestDesignApprovalRequiresProjectHEADWithoutAdoptionFallback(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	linkDesign(t, root, "api.md", "app")
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))
	for _, unborn := range []bool{false, true} {
		if unborn {
			gitInitM(t, root)
		}
		_, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
		var env *store.EnvError
		require.ErrorAs(t, err, &env)
		require.Equal(t, vcs.ReasonGitUnavailable, env.Reason)
		require.False(t, load(t, root).Baseline.HasConfirmedDocs())
	}
}

func TestDesignApprovalUsesProjectHEADForExternalReference(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	external := t.TempDir()
	filenames := []string{"main.go", "literal\\name.go", "line\nbreak.go", "carriage\rreturn.go", "tab\tname.go", "control\x01.go"}
	for _, filename := range filenames {
		designWrite(t, external, filepath.Join("module", filename), "package legacy\n")
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	canonicalExternal, err := filepath.EvalSymlinks(external)
	require.NoError(t, err)
	manifest := ownedManifest + "inputs:\n  code:\n    - {root: " + external + ", mode: reference}\n"
	designWrite(t, root, ".tiller/tiller.yml", manifest)
	recorded, err := filepath.Rel(canonicalRoot, filepath.Join(canonicalExternal, "module"))
	require.NoError(t, err)
	result := linkDesign(t, root, "api.md", filepath.Join(external, "module"))
	require.Equal(t, []string{filepath.ToSlash(recorded)}, result.Doc.Describes)
	approved, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
	require.NoError(t, err)
	head, err := vcs.New(root).CurrentCommit()
	require.NoError(t, err)
	require.Equal(t, head.Value, approved.Docs[0].Doc.Confirmed.Commit.Value)
	inventory := make([]string, 0, len(filenames))
	for _, filename := range filenames {
		inventory = append(inventory, filepath.ToSlash(filepath.Join(recorded, filename)))
	}
	require.ElementsMatch(t, inventory, approved.Docs[0].Doc.Confirmed.Surfaces[0].Inventory)
	require.Equal(t, approved.Docs[0].Doc, load(t, root).Baseline.Docs[0], "generated filesystem inventories must load without changing literal filenames")
	_, err = mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", Describes: []string{filepath.Dir(external)}})
	require.Error(t, err, "containing a declared root is not membership")
	escape := filepath.Join(external, "module", "escape")
	require.NoError(t, os.Symlink(t.TempDir(), escape))
	_, err = mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", Describes: []string{escape}})
	require.Error(t, err)
}

func TestDesignDocumentSafetyDoesNotFollowSymlinks(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	external := filepath.Join(t.TempDir(), "secret.md")
	require.NoError(t, os.WriteFile(external, []byte("outside"), 0o644))
	require.NoError(t, os.Symlink(external, filepath.Join(root, ".tiller", "design", "alias.md")))
	for _, doc := range []string{"../secret.md", "baseline.yml", "alias.md"} {
		_, err := mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: doc, Describes: []string{"app"}})
		require.Error(t, err, doc)
	}
	require.Empty(t, load(t, root).Baseline.Docs)
}

func TestDesignDeclaredAliasAndFileRootsStayAnchored(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	projectAlias := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.Symlink(root, projectAlias))
	root = projectAlias
	external := t.TempDir()
	designWrite(t, external, "module/source.go", "package legacy\n")
	require.NoError(t, os.Symlink(external, filepath.Join(root, "legacy")))
	designWrite(t, root, ".tiller/tiller.yml", ownedManifest+"inputs:\n  code:\n    - {root: ., mode: surface}\n    - {root: legacy, mode: reference}\n")
	linked := linkDesign(t, root, "api.md", "legacy/module/source.go")
	canonicalRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	canonicalExternal, err := filepath.EvalSymlinks(external)
	require.NoError(t, err)
	recorded, err := filepath.Rel(canonicalRoot, filepath.Join(canonicalExternal, "module", "source.go"))
	require.NoError(t, err)
	require.Equal(t, []string{filepath.ToSlash(recorded)}, linked.Doc.Describes)
	relinked := linkDesign(t, root, "api.md", linked.Doc.Describes...)
	require.False(t, relinked.Changed, "printed canonical coordinates must name the same coverage through a project alias")
	approved, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
	require.NoError(t, err)
	require.Equal(t, store.KindGitBlob, approved.Docs[0].Doc.Confirmed.Surfaces[0].Kind)
	require.Equal(t, linked.Doc.Describes, approved.Docs[0].Doc.Confirmed.Surfaces[0].Inventory)
	require.Equal(t, approved.Docs[0].Doc, load(t, root).Baseline.Docs[0])
	require.NoError(t, os.Remove(filepath.Join(external, "module", "source.go")))
	unlinked, err := mutate.UnlinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", Describes: []string{"legacy/module/source.go"}})
	require.NoError(t, err)
	require.True(t, unlinked.Changed)
	require.Empty(t, load(t, root).Baseline.Docs)
	designWrite(t, external, "module/source.go", "package legacy\n")
	designWrite(t, root, ".tiller/tiller.yml", ownedManifest+"inputs:\n  code:\n    - {root: legacy/module/source.go, mode: reference}\n")
	fileRoot := linkDesign(t, root, "api.md", "legacy/module/source.go")
	require.Equal(t, linked.Doc.Describes, fileRoot.Doc.Describes)
	_, err = mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
	require.NoError(t, err)
	designWrite(t, external, "module/undeclared.go", "package legacy\n")
	for _, raw := range []string{"legacy/module", "legacy/module/undeclared.go"} {
		_, err := mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", Describes: []string{raw}})
		require.Error(t, err, "a file-root declaration does not authorize its parent or siblings")
	}
}

func TestDesignApprovalRejectsSymlinkedDescendantsOfDeclaredAlias(t *testing.T) {
	t.Parallel()
	for _, replaced := range []string{"module", "module/source.go"} {
		t.Run(replaced, func(t *testing.T) {
			root := designFixture(t)
			external := t.TempDir()
			designWrite(t, external, "module/source.go", "package legacy\n")
			require.NoError(t, os.Symlink(external, filepath.Join(root, "legacy")))
			require.NoError(t, os.Symlink(external, filepath.Join(root, "undeclared")))
			designWrite(t, root, ".tiller/tiller.yml", ownedManifest+"inputs:\n  code:\n    - {root: legacy, mode: reference}\n")
			_, err := mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", Describes: []string{"undeclared/module/source.go"}})
			require.Error(t, err, "resolving an undeclared alias must not grant membership")
			linkDesign(t, root, "api.md", "legacy/module/source.go")
			before := designBytes(t, root, store.BaselineRelPath)
			original := filepath.Join(external, replaced)
			moved := filepath.Join(external, "moved")
			require.NoError(t, os.Rename(original, moved))
			require.NoError(t, os.Symlink(moved, original))
			_, err = mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
			require.Error(t, err, "even in-root symlinks below a declaration anchor must refuse")
			require.Equal(t, before, designBytes(t, root, store.BaselineRelPath))
			require.Nil(t, load(t, root).Baseline.Docs[0].Confirmed)
		})
	}
}

// crossedFeature seeds one done feature in memory whose completion record
// crossed the described surface at crossing. ApproveDesign reads the loaded
// tree, so no feature files are needed to exercise the reapproval scan.
func crossedFeature(tree *store.Tree, featureID string, crossing store.RevisionRef, surfaces ...string) {
	key := "epic/" + featureID
	tree.Features[featureID] = &store.Feature{ID: featureID, Key: &key, EpicKey: "epic", Status: "done", Surface: store.Surface{Paths: surfaces}, VerifiedAgainst: store.VerifiedAgainst{Commit: crossing}}
}

// Issue #111: the reapproval scan ranged a Go map and returned on the first
// same-HEAD crossing or the first Git error, so identical input succeeded or
// aborted depending on iteration order. The scan must decide from the whole
// set, in a deterministic order, and an unavailable historical comparison must
// never veto a valid current-state approval.
func TestDesignReapprovalIsIndependentOfCrossingOrderAndUnavailableHistory(t *testing.T) {
	t.Parallel()
	unavailable := store.RevisionRef{Kind: store.KindGitCommit, Value: "git:0000000000000000000000000000000000000000"}
	// Both orders: the unavailable crossing sorts before the known one, and after it.
	for _, ids := range [][2]string{{"F-aaaaa0", "F-zzzzz1"}, {"F-zzzzz0", "F-aaaaa1"}} {
		unavailableID, knownID := ids[0], ids[1]
		t.Run(unavailableID+"-then-"+knownID, func(t *testing.T) {
			root := designFixture(t)
			linkDesign(t, root, "api.md", "app/main.go")
			linkDesign(t, root, "data.md", "app/extra.go")
			first, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
			require.NoError(t, err)
			confirmed := first.Docs[0].Doc.Confirmed.Commit
			later := confirmed
			later.ObservedAt = confirmed.ObservedAt.Add(time.Hour)
			unavailableLater := unavailable
			unavailableLater.ObservedAt = later.ObservedAt
			deps := testDeps()
			deps.Clock = fixedClock{t: later.ObservedAt.Add(time.Hour)}
			ledgerBefore := designBytes(t, root, store.BaselineRelPath)
			for attempt := 0; attempt < 8; attempt++ {
				require.NoError(t, os.WriteFile(filepath.Join(root, store.BaselineRelPath), ledgerBefore, 0o644))
				tree := load(t, root)
				crossedFeature(tree, unavailableID, unavailableLater, "app")
				crossedFeature(tree, knownID, later, "app")
				result, err := mutate.ApproveDesign(tree, deps, []string{"api.md", "data.md"})
				require.NoError(t, err, "attempt %d", attempt)
				require.Len(t, result.Docs, 2)
				require.True(t, result.Docs[0].Changed, "known same-HEAD crossing must reapprove")
				require.True(t, result.Docs[1].Changed, "the unconfirmed sibling approves in the same batch")
				require.True(t, result.Docs[0].Doc.Confirmed.Commit.ObservedAt.After(later.ObservedAt))
				reloaded := load(t, root)
				require.NotNil(t, reloaded.Baseline.Docs[1].Confirmed, "the batch applied atomically")
			}
		})
	}
}

func TestDesignReapprovalRecordsFreshConfirmationWhenOnlyUnavailableHistoryExists(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	linkDesign(t, root, "api.md", "app/main.go")
	first, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
	require.NoError(t, err)
	confirmed := first.Docs[0].Doc.Confirmed.Commit
	unavailable := store.RevisionRef{Kind: store.KindGitCommit, Value: "git:0000000000000000000000000000000000000000", ObservedAt: confirmed.ObservedAt.Add(time.Hour)}
	deps := testDeps()
	deps.Clock = fixedClock{t: unavailable.ObservedAt.Add(time.Hour)}
	tree := load(t, root)
	crossedFeature(tree, "F-aaaaaa", unavailable, "app")
	result, err := mutate.ApproveDesign(tree, deps, []string{"api.md"})
	require.NoError(t, err, "unavailable history is not a reason to refuse the current state")
	require.True(t, result.Docs[0].Changed)
	require.True(t, result.Docs[0].Doc.Confirmed.Commit.ObservedAt.After(unavailable.ObservedAt))
}

func TestDesignReapprovalStaysNoOpWithoutLaterCrossing(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	linkDesign(t, root, "api.md", "app/main.go")
	first, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
	require.NoError(t, err)
	confirmed := first.Docs[0].Doc.Confirmed.Commit
	deps := testDeps()
	deps.Clock = fixedClock{t: confirmed.ObservedAt.Add(2 * time.Hour)}
	before := designBytes(t, root, store.BaselineRelPath)
	tree := load(t, root)
	// A crossing observed before the confirmation is outside the reapproval
	// question, so an unchanged confirmation stays a no-op.
	earlier := confirmed
	earlier.ObservedAt = confirmed.ObservedAt.Add(-time.Hour)
	crossedFeature(tree, "F-aaaaaa", earlier, "app")
	result, err := mutate.ApproveDesign(tree, deps, []string{"api.md"})
	require.NoError(t, err)
	require.False(t, result.Docs[0].Changed)
	require.Equal(t, before, designBytes(t, root, store.BaselineRelPath))
}
