package mutate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// layAttachTarget lays one epic and one feature so a bug has something to
// attach to.
//
// deps.Mint is a prefix-keyed closure rather than sequenceMinter: sequenceMinter
// draws its next token on every call regardless of prefix, and each add call
// here mints twice (the node's own id, then its EV event id), so a plain
// sequence would hand the feature's node id the token this fixture meant for
// its event and land the feature on "F-zzzzzz" instead of the id
// TestAddBugCreatesShelfFolderAndEvent asserts against.
//
// deps.Clock is stamped one minute earlier than testDeps()'s own instant, so
// this fixture's two events sort strictly BEFORE whatever the caller adds
// next by timestamp alone. events.Sort ties on event id, drawn here from
// crypto/rand (fixedMint's own "EV" branch), when two events share an exact
// instant; TestAddBugCreatesShelfFolderAndEvent reads the tree's LAST event
// expecting its own bug's, and leaving this fixture on the shared instant
// left that assertion racing a random string compare against this fixture's
// own epic and feature events.
func layAttachTarget(t *testing.T) string {
	t.Helper()
	root := writeOwnedRepo(t)
	deps := testDeps()
	deps.Clock = fixedClock{t: time.Date(2026, 6, 27, 14, 29, 0, 0, time.UTC)}
	deps.Mint = func(prefix string) string {
		switch prefix {
		case "E":
			return "E-aaaaaa"
		case "F":
			return "F-aaaaaa"
		default:
			return fixedMint(prefix)
		}
	}
	_, err := mutate.AddEpic(load(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{Key: "form-engine/loading", Title: "Loading", ChangeType: "create"})
	require.NoError(t, err)
	return root
}

func TestAddBugCreatesShelfFolderAndEvent(t *testing.T) {
	t.Parallel()
	root := layAttachTarget(t)
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "B" {
			return "B-aaaaaa"
		}
		return fixedMint(prefix)
	}
	res, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{
		Noun: store.NounBug, Key: "blank-on-reload", Title: "Blank on reload",
		AttachedTo: "form-engine/loading", Source: "docs/feedback/reload.md#L1-9",
	})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, "bug", res.Noun)
	require.Equal(t, "B-aaaaaa", res.ID)
	require.Equal(t, "bugs/blank-on-reload", res.Key)
	require.Equal(t, "proposed", res.Status)
	require.Equal(t, "F-aaaaaa", res.AttachedTo)
	require.Equal(t, filepath.Join(".tiller", "bugs", "blank-on-reload"), res.Path)

	require.FileExists(t, filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.md"))
	raw, err := os.ReadFile(filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.yml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "attached_to: F-aaaaaa")
	require.Contains(t, string(raw), "source: docs/feedback/reload.md#L1-9")
	require.Contains(t, string(raw), "proposed_by: jc")

	md, err := os.ReadFile(filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.md"))
	require.NoError(t, err)
	require.Contains(t, string(md), "## Reproduction")
	require.Contains(t, string(md), "## Fixed when")

	reloaded := load(t, root)
	require.Equal(t, "proposed", reloaded.Bugs["B-aaaaaa"].Status)
	evs, err := events.List(reloaded)
	require.NoError(t, err)
	last := evs[len(evs)-1]
	require.Equal(t, "add.bug", last.Type)
	require.Equal(t, "F-aaaaaa", last.After["attached_to"])
}

func TestAddChoreTemplateAndIdempotentReAdd(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	deps := testDeps()
	deps.Mint = func(prefix string) string { return "CH-aaaaaa" }
	in := mutate.WorkItemInput{Noun: store.NounChore, Key: "bump-rails", Title: "Bump Rails"}
	res, err := mutate.AddWorkItem(load(t, root), deps, in)
	require.NoError(t, err)
	require.Equal(t, "chores/bump-rails", res.Key)
	md, err := os.ReadFile(filepath.Join(root, ".tiller", "chores", "bump-rails", "chore.md"))
	require.NoError(t, err)
	require.Contains(t, string(md), "## Done when")
	again, err := mutate.AddWorkItem(load(t, root), deps, in)
	require.NoError(t, err)
	require.False(t, again.Created)
	require.Equal(t, "CH-aaaaaa", again.ID)
}

func TestAddWorkItemRefusals(t *testing.T) {
	t.Parallel()
	root := layAttachTarget(t)
	cases := []struct {
		name string
		in   mutate.WorkItemInput
		want string
	}{
		{"canonical key", mutate.WorkItemInput{Noun: store.NounBug, Key: "bugs/x", Title: "x"}, "local key"},
		{"attach to a bug", mutate.WorkItemInput{Noun: store.NounBug, Key: "x", Title: "x", AttachedTo: "B-zzzzzz"}, "epic, feature, or task"},
		{"wrong noun", mutate.WorkItemInput{Noun: store.NounTask, Key: "x", Title: "x"}, "shelf noun"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := mutate.AddWorkItem(load(t, root), testDeps(), c.in)
			require.Error(t, err)
			require.Contains(t, err.Error(), c.want)
		})
	}
}

func TestAddBugDependsOnChoreByIDAndShelfKey(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	deps := testDeps()
	// Prefix-keyed rather than sequenceMinter for the same reason
	// layAttachTarget is: each add call also mints an EV id through the same
	// closure, which would otherwise consume tokens meant for the B- ids
	// below.
	bTokens := []string{"aaaaaa", "bbbbbb"}
	bCalls := 0
	deps.Mint = func(prefix string) string {
		switch prefix {
		case "CH":
			return "CH-aaaaaa"
		case "B":
			tok := bTokens[bCalls]
			bCalls++
			return "B-" + tok
		default:
			return fixedMint(prefix)
		}
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounChore, Key: "bump", Title: "Bump"})
	require.NoError(t, err)
	res, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "alpha", Title: "A", DependsOn: []string{"CH-aaaaaa"}})
	require.NoError(t, err)
	require.Equal(t, []string{"CH-aaaaaa"}, res.DependsOn)
	res, err = mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: store.NounBug, Key: "bravo", Title: "B", DependsOn: []string{"chores/bump", "bugs/alpha"}})
	require.NoError(t, err)
	require.Equal(t, []string{"CH-aaaaaa", "B-aaaaaa"}, res.DependsOn)
}

func TestAddEpicRefusesReservedShelfKeys(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	for _, key := range []string{"bugs", "chores"} {
		_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: key, Title: key})
		require.Error(t, err)
		require.Equal(t, "reserved_key", reasonOf(t, err))
	}
}
