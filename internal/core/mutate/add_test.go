package mutate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// ownedManifest is a minimal manifest carrying one owner, enough for the add
// verbs whose only environment precondition is a resolvable owner.
const ownedManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

// fixedClock is a deterministic Clock so an event's dated path and timestamp are
// stable across runs.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// fixedMint keeps node ids stable while events remain immutable additions with
// distinct ids across the many mutations a fixture performs.
func fixedMint(prefix string) string {
	if prefix == "EV" {
		return id.Mint(prefix)
	}
	return prefix + "-aaaaaa"
}

func testDeps() mutate.Deps {
	return mutate.Deps{
		Contributor: "jc",
		Clock:       fixedClock{t: time.Date(2026, 6, 27, 14, 30, 0, 0, time.UTC)},
		Mint:        fixedMint,
	}
}

// writeOwnedRepo hands the test its own copy of a temp repo with a manifest that
// has one owner. It is a real git repo with one commit from the start: approve
// and sync both read HEAD (the commit anchor, artifact-schema.md "Accepted
// basis"), and HEAD cannot resolve before a first commit exists.
func writeOwnedRepo(t *testing.T) string {
	t.Helper()
	return ownedRepo.Copy(t, layOwnedRepo)
}

// ownedRepo is laid once per package and copied by every writeOwnedRepo.
var ownedRepo = testgit.Template{Name: "owned repo"}

// layOwnedRepo lays the owned repo into dir.
func layOwnedRepo(t testing.TB, dir string) {
	t.Helper()
	tdir := filepath.Join(dir, ".tiller")
	require.NoError(t, os.MkdirAll(tdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tdir, "tiller.yml"), []byte(ownedManifest), 0o644))
	gitInitM(t, dir)
	gitCommitAllM(t, dir, "init")
}

func load(t testing.TB, root string) *store.Tree {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	return tree
}

func TestAddEpicCreatesFilesAndEvent(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	tree := load(t, root)

	res, err := mutate.AddEpic(tree, testDeps(), mutate.EpicInput{
		Key:    "form-engine",
		Title:  "Form Engine",
		Source: "design/04-form-engine.md",
	})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, "E-aaaaaa", res.ID)
	require.Equal(t, "epic", res.Noun)
	require.Equal(t, "form-engine", res.Key)
	require.Equal(t, "draft", res.Status)
	require.Equal(t, "design/04-form-engine.md", res.Source)
	require.Equal(t, filepath.Join(".tiller", "epics", "form-engine"), res.Path)

	mdPath := filepath.Join(root, ".tiller", "epics", "form-engine", "epic.md")
	ymlPath := filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml")
	require.FileExists(t, mdPath)
	require.FileExists(t, ymlPath)

	// The state file round-trips through the store as a draft epic with the key.
	reloaded := load(t, root)
	e := reloaded.Epics["E-aaaaaa"]
	require.NotNil(t, e)
	require.Equal(t, "draft", e.Status)
	require.NotNil(t, e.Key)
	require.Equal(t, "form-engine", *e.Key)

	// Source provenance is persisted on the state file.
	raw, err := os.ReadFile(ymlPath)
	require.NoError(t, err)
	require.Contains(t, string(raw), "source: design/04-form-engine.md")

	// Exactly one event records the creation, attributed to the owner.
	evs, err := events.List(reloaded)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, "add.epic", evs[0].Type)
	require.Equal(t, "jc", evs[0].Contributor)
	require.Contains(t, evs[0].Targets, "E-aaaaaa")
}

func TestAddEpicRollsBackNodeFilesWhenTheEventCannotPersist(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	eventDir := filepath.Join(root, ".tiller", "events", "2026", "06", "27")
	require.NoError(t, os.MkdirAll(eventDir, 0o755))
	require.NoError(t, os.Chmod(eventDir, 0o555))
	t.Cleanup(func() { require.NoError(t, os.Chmod(eventDir, 0o755)) })

	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{
		Key: "atomic-epic", Title: "Atomic epic",
	})
	require.Error(t, err)

	dir := filepath.Join(root, ".tiller", "epics", "atomic-epic")
	require.NoFileExists(t, filepath.Join(dir, "epic.md"))
	require.NoFileExists(t, filepath.Join(dir, "epic.yml"))
	require.Empty(t, load(t, root).Epics)
	require.NoFileExists(t, filepath.Join(root, ".tiller", ".cache", "journal.yml"))
}

func TestAddEpicIdempotent(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)

	first, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/04-form-engine.md"})
	require.NoError(t, err)
	require.True(t, first.Created)

	// Re-adding the same key is a no-op success: same id, marked not created.
	second, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine", Source: "design/04-form-engine.md"})
	require.NoError(t, err)
	require.False(t, second.Created)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "design/04-form-engine.md", second.Source)

	// No second event was written.
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, evs, 1)
}

func TestAddFeatureNestsUnderEpic(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)

	res, err := mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{
		Key:        "form-engine/certification-form-loading",
		Title:      "Certification form loading",
		ChangeType: "create",
		Source:     "design/04-form-engine.md",
	})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, "F-aaaaaa", res.ID)
	require.Equal(t, "feature", res.Noun)
	require.Equal(t, "form-engine/certification-form-loading", res.Key)
	require.Equal(t, "draft", res.Status)
	require.Equal(t, "create", res.ChangeType)

	ymlPath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
	mdPath := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.md")
	require.FileExists(t, ymlPath)
	require.FileExists(t, mdPath)

	reloaded := load(t, root)
	f := reloaded.Features["F-aaaaaa"]
	require.NotNil(t, f)
	require.Equal(t, "draft", f.Status)
	require.NotNil(t, f.Key)
	require.Equal(t, "form-engine/certification-form-loading", *f.Key)
	require.Equal(t, "form-engine", f.EpicKey)
}

func TestAddFeatureUnknownEpicNotFound(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)

	_, err := mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{
		Key:        "no-such-epic/some-feature",
		ChangeType: "create",
	})
	var nf *resolve.ErrNotFound
	require.ErrorAs(t, err, &nf)
}

func TestAddTaskLaysDownAProposedFolder(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", ChangeType: "create"})
	require.NoError(t, err)

	res, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/schema-transcription",
		Title: "Transcribe CERT-LTI-02-A into JSON Schema",
	})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, "T-aaaaaa", res.ID)
	require.Equal(t, "task", res.Noun)
	require.Equal(t, "proposed", res.Status)
	require.Equal(t, "F-aaaaaa", res.Feature)
	require.Equal(t, "form-engine/certification-form-loading/schema-transcription", res.Key)
	require.Equal(t, "Transcribe CERT-LTI-02-A into JSON Schema", res.Title)

	// Both files land in the task's own folder: the implementation spec beside
	// the state (artifact-schema.md, Tasks).
	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "tasks", "schema-transcription")
	require.FileExists(t, filepath.Join(dir, "task.md"))
	require.FileExists(t, filepath.Join(dir, "task.yml"))

	reloaded := load(t, root)
	require.Len(t, reloaded.Tasks, 1)
	task := reloaded.Tasks["T-aaaaaa"]
	require.NotNil(t, task)
	require.Equal(t, "proposed", task.Status)
	require.Equal(t, "jc", task.ProposedBy)
	require.Equal(t, "F-aaaaaa", task.FeatureID)
	require.NotNil(t, task.Key)
	require.Equal(t, "form-engine/certification-form-loading/schema-transcription", *task.Key)
}

// The stub seeds a Reuses heading between Approach and Surfaces, the place
// specify records what the task calls instead of rewriting it. The stub marker
// stays, because spec feature reads it to tell an unwritten task.md from one a
// human has started.
func TestAddTaskStubCarriesReuses(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key:   "form-engine/certification-form-loading/schema-transcription",
		Title: "Transcribe CERT-LTI-02-A into JSON Schema",
	})
	require.NoError(t, err)

	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "tasks", "schema-transcription")
	data, err := os.ReadFile(filepath.Join(dir, "task.md"))
	require.NoError(t, err)
	stub := string(data)
	require.Contains(t, stub, "## Approach\n\n## Reuses\n\n## Surfaces\n")
	require.Contains(t, stub, "Proposed task created by `tiller add task`")
}

// A task key that is not the canonical three segments is misuse, refused before
// anything is written (artifact-schema.md, Keys and paths).
func TestAddTaskRefusesANonCanonicalKey(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", ChangeType: "create"})
	require.NoError(t, err)

	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/certification-form-loading", Title: "Two segments is a feature",
	})
	require.Error(t, err)
	require.Empty(t, load(t, root).Tasks)
}

// The parent feature is resolved from the key's first two segments, so a task
// under an unknown feature is the resolver's typed not-found.
func TestAddTaskRefusesAnUnknownFeature(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine"})
	require.NoError(t, err)

	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/no-such-feature/some-task", Title: "Nowhere to land",
	})
	var nf *resolve.ErrNotFound
	require.ErrorAs(t, err, &nf)
}

func TestAddTaskIdempotentByKey(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{Key: "form-engine/certification-form-loading", ChangeType: "create"})
	require.NoError(t, err)

	in := mutate.TaskInput{Key: "form-engine/certification-form-loading/schema-transcription", Title: "Transcribe CERT-LTI-02-A into JSON Schema"}
	first, err := mutate.AddTask(load(t, root), testDeps(), in)
	require.NoError(t, err)
	require.True(t, first.Created)

	second, err := mutate.AddTask(load(t, root), testDeps(), in)
	require.NoError(t, err)
	require.False(t, second.Created)
	require.Equal(t, first.ID, second.ID)

	require.Len(t, load(t, root).Tasks, 1)
}

// TestAddEpicSourceSurvivesApproveRewrite pins the fix for the dropped-source
// bug: mutate reads and writes store.Epic directly (no epicDoc wrapper), so a
// hand-set Source must still be there after a real lifecycle rewrite of
// epic.yml, not just on the freshly-added file.
func TestAddEpicSourceSurvivesApproveRewrite(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{
		Key:    "form-engine",
		Title:  "Form Engine",
		Source: "design/04-form-engine.md",
	})
	require.NoError(t, err)

	res := driveTo(t, root, store.NounEpic, "form-engine", mutate.TransitionInput{Verb: "approve"})
	require.Equal(t, "ready", res.Status)

	got := load(t, root).Epics["E-aaaaaa"]
	require.NotNil(t, got)
	require.Equal(t, "design/04-form-engine.md", got.Source,
		"a transition rewrite must preserve the hand-set source provenance")
}

// TestAddFeatureSourceSurvivesSpecApproveRewrite is the feature counterpart:
// Source must survive both the spec and approve rewrites of feature.yml.
func TestAddFeatureSourceSurvivesSpecApproveRewrite(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{
		Key:        "form-engine/certification-form-loading",
		Title:      "Certification form loading",
		ChangeType: "create",
		Source:     "design/04-form-engine.md",
	})
	require.NoError(t, err)
	writeStage2Output(t, root)

	driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "spec"})
	res := driveTo(t, root, store.NounFeature, "form-engine/certification-form-loading", mutate.TransitionInput{Verb: "approve"})
	require.Equal(t, "ready", res.Status)

	got := load(t, root).Features["F-aaaaaa"]
	require.NotNil(t, got)
	require.Equal(t, "design/04-form-engine.md", got.Source,
		"a transition rewrite must preserve the hand-set source provenance")
}

func TestAddOwnerUnsetIsEnvError(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)

	_, err := mutate.AddEpic(load(t, root), mutate.Deps{Contributor: ""}, mutate.EpicInput{Key: "form-engine"})
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	require.Equal(t, 3, ee.ExitCode())
	require.Equal(t, store.ReasonContributorUnset, ee.Reason)
}

func TestAddOwnerUnknownIsEnvError(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)

	_, err := mutate.AddEpic(load(t, root), mutate.Deps{Contributor: "nobody"}, mutate.EpicInput{Key: "form-engine"})
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	require.Equal(t, store.ReasonContributorUnknown, ee.Reason)
}

// sequenceMinter returns its tokens in order, prefixed and hyphenated like a
// real id, so a test can hand the first one a value already in the tree and
// prove the retry reaches the second rather than writing the collision. Once
// exhausted it falls back to a fixed token, which no test here should reach.
func sequenceMinter(tokens ...string) func(string) string {
	i := 0
	return func(prefix string) string {
		if i < len(tokens) {
			t := tokens[i]
			i++
			return prefix + "-" + t
		}
		return prefix + "-zzzzzz"
	}
}

// TestMintRetriesPastACollision pins the retry half of the schema's promise
// (artifact-schema.md, IDs): a mint that lands on an id the loaded tree
// already carries must not write the duplicate, it must try again.
func TestMintRetriesPastACollision(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)

	// The fixed test minter always hands out "-aaaaaa" first, so this feature
	// is what puts F-aaaaaa in the tree for the next add to collide with.
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{
		Key: "form-engine/first-feature", Title: "First feature", ChangeType: "create",
	})
	require.NoError(t, err)

	tree := load(t, root)
	require.Contains(t, tree.Features, "F-aaaaaa", "the fixture must actually hold the id we collide with")

	deps := testDeps()
	deps.Mint = sequenceMinter("aaaaaa", "bbbbbb")

	res, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{
		Key: "form-engine/second-feature", Title: "Second feature", ChangeType: "create",
	})
	require.NoError(t, err)
	require.Equal(t, "F-bbbbbb", res.ID, "the mint must skip the id the tree already holds")
}

// TestMintGivesUpRatherThanSpinning pins the bound on the retry: a minter that
// never offers a free id must error rather than loop forever or write the
// collision it kept drawing.
func TestMintGivesUpRatherThanSpinning(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{
		Key: "form-engine/first-feature", Title: "First feature", ChangeType: "create",
	})
	require.NoError(t, err)

	tree := load(t, root)
	require.Contains(t, tree.Features, "F-aaaaaa", "the fixture must actually hold the id we collide with")

	deps := testDeps()
	// A minter that only ever returns a token already in the tree. Without a
	// bound this hangs the suite rather than failing it.
	deps.Mint = func(prefix string) string { return prefix + "-aaaaaa" }

	_, err = mutate.AddFeature(tree, deps, mutate.FeatureInput{
		Key: "form-engine/second-feature", Title: "Second feature", ChangeType: "create",
	})
	require.Error(t, err, "an exhausted minter must error, never loop forever")
	require.Contains(t, err.Error(), "id")
}

func TestReAddRefusesEscapingStateAfterTreeLoad(t *testing.T) {
	t.Parallel()
	for _, component := range []string{"state-file", "node-directory"} {
		t.Run(component, func(t *testing.T) {
			root := writeOwnedRepo(t)
			_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "bounded"})
			require.NoError(t, err)
			tree := load(t, root)
			outside := t.TempDir()
			external := filepath.Join(outside, "epic.yml")
			private := []byte("id: E-private\nkey: bounded\nstatus: draft\ntitle: confidential\n")
			require.NoError(t, os.WriteFile(external, private, 0o644))
			dir := filepath.Join(root, ".tiller", "epics", "bounded")
			if component == "state-file" {
				require.NoError(t, os.Remove(filepath.Join(dir, "epic.yml")))
				require.NoError(t, os.Symlink(external, filepath.Join(dir, "epic.yml")))
			} else {
				require.NoError(t, os.RemoveAll(dir))
				require.NoError(t, os.Symlink(outside, dir))
			}

			result, err := mutate.AddEpic(tree, testDeps(), mutate.EpicInput{Key: "bounded"})
			require.Error(t, err)
			require.Empty(t, result.ID, "re-add must not return an outside state identity")
			after, err := os.ReadFile(external)
			require.NoError(t, err)
			require.Equal(t, private, after)
		})
	}
}
