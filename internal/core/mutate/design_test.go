package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func designFixture(t *testing.T) string {
	t.Helper()
	root := writeOwnedRepo(t)
	designWrite(t, root, ".tiller/tiller.yml", ownedManifest+"inputs:\n  code:\n    - {root: app, mode: surface}\n")
	designWrite(t, root, ".tiller/design/api.md", "# API\n\nHuman-authored baseline prose.\n")
	designWrite(t, root, ".tiller/design/data.md", "# Data\n")
	designWrite(t, root, "app/main.go", "package app\n")
	designWrite(t, root, "app/extra.go", "package app\n// extra\n")
	return root
}

func designWrite(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func designBytes(t *testing.T, root, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	require.NoError(t, err)
	return data
}

func linkDesign(t *testing.T, root, doc string, paths ...string) mutate.DesignResult {
	t.Helper()
	result, err := mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: doc, Describes: paths})
	require.NoError(t, err)
	return result
}

func TestDesignDescribeUnionInvalidatesOnlyRealCoverageChanges(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	manifest := designBytes(t, root, ".tiller/tiller.yml")
	prose := designBytes(t, root, ".tiller/design/api.md")
	linkDesign(t, root, "api.md", "app/main.go")
	approved, err := mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
	require.NoError(t, err)
	stamp := approved.Docs[0].Doc.Confirmed
	require.NotNil(t, stamp)
	before := designBytes(t, root, store.BaselineRelPath)
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	retry := linkDesign(t, root, "api.md", "app/main.go", "app/main.go")
	require.False(t, retry.Changed)
	require.Equal(t, before, designBytes(t, root, store.BaselineRelPath))
	afterEvents, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, afterEvents, len(evs))
	changed := linkDesign(t, root, "api.md", "app/extra.go", "app/main.go")
	require.True(t, changed.Changed)
	require.Nil(t, changed.Doc.Confirmed)
	require.Equal(t, []string{"app/main.go", "app/extra.go"}, changed.Doc.Describes)
	require.Equal(t, manifest, designBytes(t, root, ".tiller/tiller.yml"))
	require.Equal(t, prose, designBytes(t, root, ".tiller/design/api.md"))
}

func TestDesignProvenanceUsesRecoverablePinnedHEADAndUpserts(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	designWrite(t, root, "docs/old.md", "committed source\n")
	designWrite(t, root, "docs/other.md", "other source\n")
	gitRunM(t, root, "add", "docs/old.md", "docs/other.md")
	gitRunM(t, root, "commit", "-m", "record translation inputs")
	oldBlob, err := vcs.New(root).BlobHash("docs/old.md")
	require.NoError(t, err)
	linkDesign(t, root, "api.md", "app")
	_, err = mutate.ApproveDesign(load(t, root), testDeps(), []string{"api.md"})
	require.NoError(t, err)
	designWrite(t, root, "docs/old.md", "uncommitted edit must not be provenance\n")
	input := mutate.DesignLinkInput{Doc: "api.md", From: []string{"docs/old.md", "docs/other.md"}}
	result, err := mutate.LinkDesign(load(t, root), testDeps(), input)
	require.NoError(t, err)
	require.Equal(t, "git:"+oldBlob, result.Doc.DerivedFrom[0].Value)
	require.NotNil(t, result.Doc.Confirmed, "provenance does not revoke code confirmation")
	before := designBytes(t, root, store.BaselineRelPath)
	require.NoError(t, os.Remove(filepath.Join(root, "docs", "old.md")))
	result, err = mutate.LinkDesign(load(t, root), testDeps(), input)
	require.NoError(t, err)
	require.False(t, result.Changed, "retired source is still recoverable at HEAD")
	require.Equal(t, before, designBytes(t, root, store.BaselineRelPath))
	designWrite(t, root, "docs/old.md", "new committed source\n")
	gitRunM(t, root, "add", "docs/old.md")
	gitRunM(t, root, "commit", "-m", "revise translation input")
	result, err = mutate.LinkDesign(load(t, root), testDeps(), input)
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Len(t, result.Doc.DerivedFrom, 2)
	require.NotEqual(t, "git:"+oldBlob, result.Doc.DerivedFrom[0].Value)
	require.Equal(t, "docs/other.md", result.Doc.DerivedFrom[1].Path)
	require.NotNil(t, result.Doc.Confirmed)
	ev := findEvent(t, load(t, root), "link.design")
	require.Equal(t, []string{".tiller/design/api.md"}, ev.Targets)
}

func TestDesignUnlinkSurvivesMissingProseCodeSourcesAndGit(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	designWrite(t, root, "docs/source.md", "source\n")
	gitRunM(t, root, "add", "docs/source.md")
	gitRunM(t, root, "commit", "-m", "source")
	_, err := mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", Describes: []string{"app"}, From: []string{"docs/source.md"}})
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "design", "api.md")))
	require.NoError(t, os.RemoveAll(filepath.Join(root, "app")))
	require.NoError(t, os.RemoveAll(filepath.Join(root, "docs")))
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))
	result, err := mutate.UnlinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", From: []string{"docs/source.md"}})
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Empty(t, result.Doc.DerivedFrom)
	result, err = mutate.UnlinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", Describes: []string{"app"}})
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Empty(t, load(t, root).Baseline.Docs)
	result, err = mutate.UnlinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", Describes: []string{"app"}})
	require.NoError(t, err)
	require.False(t, result.Changed)
}

func TestDesignFreshSnapshotsPreserveNewLedgerAndManifestState(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	tree := load(t, root)
	ledger := store.BaselineLedger{Docs: []store.BaselineDoc{{Path: "data.md", Describes: []string{"app"}}}}
	data, err := yaml.Marshal(ledger)
	require.NoError(t, err)
	designWrite(t, root, store.BaselineRelPath, string(data))
	manifest := strings.Replace(ownedManifest, "id: jc", "id: fresh", 1) + "inputs:\n  code:\n    - {root: app, mode: reference}\n"
	designWrite(t, root, ".tiller/tiller.yml", manifest)
	deps := testDeps()
	deps.Contributor = "fresh"
	_, err = mutate.LinkDesign(tree, deps, mutate.DesignLinkInput{Doc: "api.md", Describes: []string{"app"}})
	require.NoError(t, err)
	require.Equal(t, []string{"data.md", "api.md"}, []string{load(t, root).Baseline.Docs[0].Path, load(t, root).Baseline.Docs[1].Path})
	require.Equal(t, manifest, string(designBytes(t, root, ".tiller/tiller.yml")))
	ev := findEvent(t, load(t, root), "link.design")
	require.Equal(t, "fresh", ev.Contributor)
}

func TestDesignLinkDoesNotRequireGitButFromDoes(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))
	linkDesign(t, root, "api.md", "app")
	_, err := mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "data.md", From: []string{"docs/source.md"}})
	require.Error(t, err)
	_, err = mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", From: []string{"docs/source.md"}})
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, vcs.ReasonGitUnavailable, env.Reason)
}

func TestDesignRejectsUnrecoverableAndNonregularSources(t *testing.T) {
	t.Parallel()
	root := designFixture(t)
	linkDesign(t, root, "api.md", "app")
	designWrite(t, root, "docs/untracked.md", "not recoverable\n")
	require.NoError(t, os.Symlink("untracked.md", filepath.Join(root, "docs", "alias.md")))
	gitRunM(t, root, "add", "docs/alias.md")
	gitRunM(t, root, "commit", "-m", "source symlink")
	before := designBytes(t, root, store.BaselineRelPath)
	for _, source := range []string{"docs/untracked.md", "docs/alias.md", "../external.md", "docs"} {
		_, err := mutate.LinkDesign(load(t, root), testDeps(), mutate.DesignLinkInput{Doc: "api.md", From: []string{source}})
		require.Error(t, err, source)
		require.Equal(t, before, designBytes(t, root, store.BaselineRelPath))
	}
}
