package mutate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// --- fixtures ---

// stageFixture lays a one-epic, one-feature, one-task tree plus one freshly
// opened proposed amendment, the shape StageOperation's tests drive against: a
// live feature and task to target, a live epic to prove refused, and an open
// amendment to stage onto.
func stageFixture(t *testing.T) (root, amendmentID string) {
	t.Helper()
	root = writeOwnedRepo(t)
	_, err := mutate.AddEpic(load(t, root), testDeps(), mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(t, root), testDeps(), mutate.FeatureInput{
		Key: "form-engine/certification-form-loading", Title: "Certification form loading", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{
		Key: "form-engine/certification-form-loading/schema-transcription", Title: "Transcribe the schema"})
	require.NoError(t, err)
	res, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "Remove native iPad support from v1"})
	require.NoError(t, err)
	return root, res.ID
}

// eventsOfType filters an event list down to one type. stageFixture's own
// setup (AddEpic, AddFeature, AddTask, AddAmendment) already writes several
// events before a test's own call under test, so a bare require.Len on the
// full list is wrong; tests that care about "did this call emit one event"
// filter by type instead.
func eventsOfType(evs []events.Event, typ string) []events.Event {
	var out []events.Event
	for _, e := range evs {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func loadAmendment(t *testing.T, root, id string) *store.Amendment {
	t.Helper()
	a := load(t, root).Amendments[id]
	require.NotNil(t, a, "amendment %s must be in the loaded tree", id)
	return a
}

// setAmendmentStatus hand-lays a status onto an amendment's yml, the same way
// setTaskStatus stages a pre-guard task status: it proves the refusal bites on
// what is on disk, not only on what AddAmendment or WithdrawAmendment can
// themselves produce.
func setAmendmentStatus(t *testing.T, root, amendmentID, status string) {
	t.Helper()
	a := loadAmendment(t, root, amendmentID)
	path := filepath.Join(root, ".tiller", "amendments", a.ID+"-"+*a.Key+".yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: "+a.Status, "status: "+status, 1)
	require.NotEqual(t, string(data), out, "the replace must land or the fixture lies")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// setAmendmentSource hand-lays a source onto an amendment's yml, manufacturing
// the intake-minted shape (doc.Source != nil) StageOperation's per-op citation
// refusal keys on. Nothing in this build sets Amendment.Source yet (intake, a
// later task, is the one writer), so a test proving that refusal has to
// produce the shape by disk-level poke, the same discipline setAmendmentStatus
// uses one field over.
func setAmendmentSource(t *testing.T, root, amendmentID, path, revision string) {
	t.Helper()
	a := loadAmendment(t, root, amendmentID)
	ymlPath := filepath.Join(root, ".tiller", "amendments", a.ID+"-"+*a.Key+".yml")
	data, err := os.ReadFile(ymlPath)
	require.NoError(t, err)
	var doc store.Amendment
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Source = &store.AmendmentSource{Path: path, Revision: revision}
	out, err := yaml.Marshal(&doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(ymlPath, out, 0o644))
}

// --- AddAmendment ---

func TestAddAmendmentCreatesFilesAndEvent(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)

	res, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key:          "iphone-only",
		Title:        "Remove native iPad support from v1",
		Implements:   "ADR-v7b2d5",
		TriggeredBy:  []string{"design/04-platform-and-constraints.md"},
		AmendsDesign: []string{"design/01-overview.md"},
	})
	require.NoError(t, err)
	require.Equal(t, "A-aaaaaa", res.ID)
	require.Equal(t, "iphone-only", res.Key)
	require.Equal(t, store.AmendmentStatusDraft, res.Status)
	require.Equal(t, "ADR-v7b2d5", res.Implements)
	require.Equal(t, []string{"design/04-platform-and-constraints.md"}, res.TriggeredBy)
	require.Equal(t, []string{"design/01-overview.md"}, res.AmendsDesign)
	require.True(t, strings.HasPrefix(res.BaseRevision, "git:"), "base_revision must be the compact git:<sha> form")

	ymlPath := filepath.Join(root, ".tiller", "amendments", "A-aaaaaa-iphone-only.yml")
	mdPath := filepath.Join(root, ".tiller", "amendments", "A-aaaaaa-iphone-only.md")
	require.FileExists(t, ymlPath)
	require.FileExists(t, mdPath)

	reloaded := load(t, root)
	a := reloaded.Amendments["A-aaaaaa"]
	require.NotNil(t, a)
	require.Equal(t, store.AmendmentStatusDraft, a.Status)
	require.Equal(t, res.BaseRevision, a.BaseRevision)
	require.Empty(t, a.Operations)

	evs, err := events.List(reloaded)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	require.Equal(t, "amendment_opened", evs[0].Type)
	require.Equal(t, "jc", evs[0].Contributor)
	require.Contains(t, evs[0].Targets, "A-aaaaaa")

}

// TestAddAmendmentSeedsStubNarrative pins the .md stub shape: an H1 title, then
// three H2 sections (Why, What changes, What to watch) each carrying one
// placeholder sentence, in the style of the task.md stub `tiller add task`
// seeds (add.go's renderTaskMD).
func TestAddAmendmentSeedsStubNarrative(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "Remove native iPad support from v1",
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(root, ".tiller", "amendments", "A-aaaaaa-iphone-only.md"))
	require.NoError(t, err)
	md := string(raw)

	require.True(t, strings.HasPrefix(md, "# Remove native iPad support from v1\n"))
	require.Contains(t, md, "## Why\n")
	require.Contains(t, md, "## What changes\n")
	require.Contains(t, md, "## What to watch\n")

	// Each H2 carries a placeholder sentence rather than sitting bare, unlike
	// the task.md stub's empty section headers.
	sections := strings.Split(md, "## ")
	for _, heading := range []string{"Why", "What changes", "What to watch"} {
		found := false
		for _, s := range sections {
			if strings.HasPrefix(s, heading+"\n") {
				found = true
				require.NotEmpty(t, strings.TrimSpace(strings.TrimPrefix(s, heading+"\n")),
					"section %q must carry a placeholder sentence", heading)
			}
		}
		require.True(t, found, "stub must carry a %q section", heading)
	}
}

func TestAddAmendmentRefusesMalformedKey(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "Not A Key!", Title: "bad key",
	})
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "amendment_key_invalid", r.Reason())
	require.Empty(t, load(t, root).Amendments)
}

func TestAddAmendmentRefusesDuplicateProposedKey(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "First"})
	require.NoError(t, err)

	_, err = mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "Second"})
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "amendment_exists", r.Reason())
	require.Len(t, load(t, root).Amendments, 1, "the refused second open must not write")
}

// TestAddAmendmentAllowsKeyReuseAfterWithdrawn pins the ambiguity resolution:
// duplicate-key refusal applies only among open amendments, so a withdrawn
// amendment's key is free again.
func TestAddAmendmentAllowsKeyReuseAfterWithdrawn(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	first, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "First"})
	require.NoError(t, err)

	_, err = mutate.WithdrawAmendment(load(t, root), gitDeps(root), loadAmendment(t, root, first.ID), "superseded by a better plan")
	require.NoError(t, err)

	// A withdrawn amendment keeps its id (ids are never freed); only the key is
	// free again. fixedMint hands out the same "A-aaaaaa" deterministically, so
	// the second open needs a minter that actually offers a fresh token, the
	// same reason TestMintRetriesPastACollision (add_test.go) does.
	deps := gitDeps(root)
	deps.Mint = sequenceMinter("bbbbbb")
	second, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{
		Key: "iphone-only", Title: "Second"})
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	require.Len(t, load(t, root).Amendments, 2)
}

func TestAddAmendmentDedupsAmendsDesignPreservingOrder(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	res, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "First",
		AmendsDesign: []string{"design/a.md", "design/b.md", "design/a.md"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"design/a.md", "design/b.md"}, res.AmendsDesign)
}

// TestAddAmendmentSetsSourceWhenProvided pins intake's own write path: Source
// rides in on AmendmentInput, lands on the result, and round-trips through the
// written yml, the same way Implements/TriggeredBy/AmendsDesign already do one
// field over.
func TestAddAmendmentSetsSourceWhenProvided(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	src := &store.AmendmentSource{Path: ".tiller/feedback/review.md", Revision: "git:abc123"}
	res, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "First", Source: src,
	})
	require.NoError(t, err)
	require.Equal(t, src, res.Source)

	a := loadAmendment(t, root, res.ID)
	require.Equal(t, src, a.Source)
}

// TestAddAmendmentLeavesSourceNilForAHandAmendment pins the existing behavior
// unaffected: `tiller add amendment` never sets Source, so StageOperation's
// per-op citation refusal never fires on a hand-opened
// amendment.
func TestAddAmendmentLeavesSourceNilForAHandAmendment(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	res, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "First",
	})
	require.NoError(t, err)
	require.Nil(t, res.Source)
	require.Nil(t, loadAmendment(t, root, res.ID).Source)
}

// TestAddAmendmentEventCarriesSourcePathInAfter pins the brief's events rule:
// "the amendment-add event the existing AddAmendment emits, plus the artifact
// path in its After (... keep one event, not two)". Exactly one
// amendment_opened event fires, and its After names the artifact path.
func TestAddAmendmentEventCarriesSourcePathInAfter(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	src := &store.AmendmentSource{Path: ".tiller/feedback/review.md", Revision: "git:abc123"}
	res, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "First", Source: src,
	})
	require.NoError(t, err)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	opened := eventsOfType(evs, "amendment_opened")
	require.Len(t, opened, 1, "intake mints exactly one event, not two")
	require.Equal(t, res.ID, opened[0].Targets[0])
	require.Equal(t, src.Path, opened[0].After["source"])
}

// TestAddAmendmentEventOmitsSourceForAHandAmendment guards the other side of
// the same rule: a hand amendment's event After is byte-for-byte what it was
// before this field existed, carrying no "source" key at all.
func TestAddAmendmentEventOmitsSourceForAHandAmendment(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "First",
	})
	require.NoError(t, err)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	opened := eventsOfType(evs, "amendment_opened")
	require.Len(t, opened, 1)
	_, ok := opened[0].After["source"]
	require.False(t, ok, "a hand amendment's event must not carry a source key")
}

// --- PrecheckAddAmendment ---
//
// PrecheckAddAmendment is the seam a caller with a side effect to sequence
// BEFORE AddAmendment's own mint runs first (tiller intake's artifact copy,
// which must land before the amendment so its fingerprint can be computed).
// AddAmendment itself now calls this same function internally rather than
// duplicating its checks, so these tests also cover AddAmendment's own
// malformed-key/duplicate-key/HEAD-resolution behavior at the source.

func TestPrecheckAddAmendmentReturnsHeadWhenKeyIsFree(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	head, err := mutate.PrecheckAddAmendment(load(t, root), gitDeps(root), "iphone-only")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(head.Value, "git:"), "the returned head must be the compact git:<sha> form")
}

func TestPrecheckAddAmendmentRefusesMalformedKey(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.PrecheckAddAmendment(load(t, root), gitDeps(root), "Not A Key!")
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "amendment_key_invalid", r.Reason())
}

func TestPrecheckAddAmendmentRefusesDuplicateProposedKey(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{Key: "iphone-only", Title: "First"})
	require.NoError(t, err)

	_, err = mutate.PrecheckAddAmendment(load(t, root), gitDeps(root), "iphone-only")
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "amendment_exists", r.Reason())
}

// TestPrecheckAddAmendmentAllowsKeyOfAWithdrawnAmendment mirrors
// TestAddAmendmentAllowsKeyReuseAfterWithdrawn one grain down: the precheck
// alone (no mint) must agree that a withdrawn amendment's key is free.
func TestPrecheckAddAmendmentAllowsKeyOfAWithdrawnAmendment(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	first, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{Key: "iphone-only", Title: "First"})
	require.NoError(t, err)
	_, err = mutate.WithdrawAmendment(load(t, root), gitDeps(root), loadAmendment(t, root, first.ID), "dropped")
	require.NoError(t, err)

	_, err = mutate.PrecheckAddAmendment(load(t, root), gitDeps(root), "iphone-only")
	require.NoError(t, err)
}

func TestAddAmendmentOwnerUnsetIsEnvError(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	_, err := mutate.AddAmendment(load(t, root), mutate.Deps{}, mutate.AmendmentInput{Key: "iphone-only"})
	var ee *store.EnvError
	require.ErrorAs(t, err, &ee)
	require.Equal(t, 3, ee.ExitCode())
}

// TestAddAmendmentMintRetriesPastACollision is TestMintRetriesPastACollision's
// (add_test.go) counterpart for the "A" prefix: it pins that minting an
// amendment id checks tree.Amendments for an in-tree collision the same way it
// already checks Epics/Features/Tasks/Questions (treeHoldsID, add.go). The
// first amendment puts A-aaaaaa in the tree; the second is given a sequenced
// minter that offers that same token first, and must retry onto the next one
// rather than write the collision.
func TestAddAmendmentMintRetriesPastACollision(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	first, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{Key: "iphone-only", Title: "First"})
	require.NoError(t, err)
	require.Equal(t, "A-aaaaaa", first.ID, "the fixture must actually hold the id we collide with")

	deps := gitDeps(root)
	deps.Mint = sequenceMinter("aaaaaa", "bbbbbb")
	second, err := mutate.AddAmendment(load(t, root), deps, mutate.AmendmentInput{Key: "table-backed-forms", Title: "Second"})
	require.NoError(t, err)
	require.Equal(t, "A-bbbbbb", second.ID, "the mint must skip the id tree.Amendments already holds")
}

// --- StageOperation: happy paths ---

func TestStageOperationReviseHappyPath(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	tree := load(t, root)
	featureID := tree.Features["F-aaaaaa"].ID

	res, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:            store.AmendmentOpRevise,
		Target:        "form-engine/certification-form-loading",
		Preconditions: map[string]string{"spec_revision": "git:aaa", "eval_revision": "git:bbb"},
	})
	require.NoError(t, err)
	require.Equal(t, store.AmendmentOpRevise, res.Op.Op)
	require.Equal(t, featureID, res.Op.Target, "a key target resolves to its id")
	require.Equal(t, 0, res.Index)

	reloaded := loadAmendment(t, root, amendmentID)
	require.Len(t, reloaded.Operations, 1)
	require.Equal(t, featureID, reloaded.Operations[0].Target)
	require.Equal(t, map[string]string{"spec_revision": "git:aaa", "eval_revision": "git:bbb"}, reloaded.Operations[0].Preconditions)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsOfType(evs, "amendment_staged"), 1)
}

func TestStageOperationSupersedeHappyPathByExistingID(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	// A supersession is a lineage between TWO nodes, so this needs a second live
	// feature to name as the replacement; the shared fixture lays one, and the
	// fixed minter would hand a second the same id, so this one forces its own
	// (the idiom layTaskGrainGit uses for the same reason).
	deps := testDeps()
	deps.Mint = func(prefix string) string {
		if prefix == "F" {
			return "F-aaaaab"
		}
		return fixedMint(prefix)
	}
	replacement, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "form-engine/table-backed-forms", Title: "Table backed forms", ChangeType: "replace"})
	require.NoError(t, err)

	tree := load(t, root)
	featureID := tree.Features["F-aaaaaa"].ID

	res, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:            store.AmendmentOpSupersede,
		Target:        featureID,
		By:            replacement.ID,
		Preconditions: map[string]string{"target_status": "done"},
	})
	require.NoError(t, err)
	require.Equal(t, featureID, res.Op.Target)
	require.Equal(t, replacement.ID, res.Op.By)
}

func TestStageOperationSupersedeByStagedAddFeatureKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	tree := load(t, root)
	featureID := tree.Features["F-aaaaaa"].ID

	// Stage the add-feature first, so its key is available for --by to name.
	added, err := mutate.StageOperation(tree, seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:         store.AmendmentOpAddFeature,
		Key:        "form-engine/table-backed-forms",
		ChangeType: "replace",
	})
	require.NoError(t, err)

	res, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:     store.AmendmentOpSupersede,
		Target: featureID,
		By:     "form-engine/table-backed-forms",
	})
	require.NoError(t, err)
	require.Equal(t, added.Op.Node, res.Op.By,
		"a staged add is a live held draft, so --by records its id, which survives a rename")
}

func TestStageOperationCancelHappyPathWithSupersededByStagedAddTaskKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	tree := load(t, root)
	taskID := tree.Tasks["T-aaaaaa"].ID

	added, err := mutate.StageOperation(tree, seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:  store.AmendmentOpAddTask,
		Key: "form-engine/certification-form-loading/iphone-acceptance",
	})
	require.NoError(t, err)

	res, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:           store.AmendmentOpCancel,
		Target:       taskID,
		Reason:       "superseded under the iPhone-only decision",
		SupersededBy: "form-engine/certification-form-loading/iphone-acceptance",
	})
	require.NoError(t, err)
	require.Equal(t, taskID, res.Op.Target)
	require.Equal(t, "superseded under the iPhone-only decision", res.Op.Reason)
	require.Equal(t, added.Op.Node, res.Op.SupersededBy, "the held replacement resolves to its id")
}

func TestStageOperationAddFeatureHappyPath(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	res, err := mutate.StageOperation(load(t, root), seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:            store.AmendmentOpAddFeature,
		Key:           "form-engine/table-backed-forms",
		Title:         "Table-backed forms",
		ChangeType:    "replace",
		Preconditions: map[string]string{"key_absent": "form-engine/table-backed-forms"},
	})
	require.NoError(t, err)
	require.Equal(t, "form-engine/table-backed-forms", res.Op.Key)
	require.Equal(t, "replace", res.Op.ChangeType)
}

// TestStageOperationAddEpicHappyPath pins add-epic's own staging shape:
// a bare key and title, no parent to resolve, unlike
// add-feature and add-task.
func TestStageOperationAddEpicHappyPath(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	res, err := mutate.StageOperation(load(t, root), seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:    store.AmendmentOpAddEpic,
		Key:   "hardening",
		Title: "Hardening",
	})
	require.NoError(t, err)
	require.Equal(t, "hardening", res.Op.Key)
	require.Equal(t, "Hardening", res.Op.Title)

	reloaded := loadAmendment(t, root, amendmentID)
	require.Equal(t, store.AmendmentOpAddEpic, reloaded.Operations[0].Op)
}

// TestStageOperationAddEpicWithBlankKeyIsCaughtByValidateAmendment pins that a
// blank-key add-epic staged directly (bypassing the CLI's own charset check)
// is refused by store.ValidateAmendment's backstop, the same shape
// TestStageOperationUnrecognizedOpIsCaughtByValidateAmendment pins for an
// unrecognized op: add-epic has no parent segment to split, so unlike
// add-feature/add-task it has no bespoke invalid_key refusal of its own in
// this package.
func TestStageOperationAddEpicWithBlankKeyIsCaughtByValidateAmendment(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	_, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Title: "Hardening",
	})
	require.Error(t, err)
	var shapeErr *store.AmendmentShapeError
	require.ErrorAs(t, err, &shapeErr)
	require.Empty(t, loadAmendment(t, root, amendmentID).Operations, "a refused stage must not write")
}

func TestStageOperationAddTaskHappyPath(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	res, err := mutate.StageOperation(load(t, root), seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:    store.AmendmentOpAddTask,
		Key:   "form-engine/certification-form-loading/iphone-acceptance",
		Title: "iPhone-only acceptance",
	})
	require.NoError(t, err)
	require.Equal(t, "form-engine/certification-form-loading/iphone-acceptance", res.Op.Key)
	require.Equal(t, "iPhone-only acceptance", res.Op.Title)
}

// --- StageOperation: same-batch parenting ---

// TestStageOperationAddFeatureRescuedBySameBatchStagedEpic pins the
// same-batch parenting allowance at feature grain: an add-epic staged
// earlier on this same amendment parents a later add-feature naming it,
// because staging created the epic as a held draft.
func TestStageOperationAddFeatureRescuedBySameBatchStagedEpic(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	deps := seqDeps()

	_, err := mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})
	require.NoError(t, err)

	res, err := mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "hardening/csp-headers", Title: "CSP headers", ChangeType: "create",
	})
	require.NoError(t, err)
	require.Equal(t, "hardening/csp-headers", res.Op.Key)

	reloaded := loadAmendment(t, root, amendmentID)
	require.Len(t, reloaded.Operations, 2)
	require.Equal(t, store.AmendmentOpAddEpic, reloaded.Operations[0].Op)
	require.Equal(t, store.AmendmentOpAddFeature, reloaded.Operations[1].Op)
}

// TestStageOperationAddFeatureRefusesBeforeItsEpicIsStaged pins the ordering
// half of the same allowance: an add-feature naming an epic that is neither
// live nor staged EARLIER on this amendment finds nothing to rescue it and
// refuses by name, the message naming both misses (docs/spec/cli-surface.md: "no such epic in the tree, and none
// staged earlier on this amendment").
func TestStageOperationAddFeatureRefusesBeforeItsEpicIsStaged(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	_, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "hardening/csp-headers", Title: "CSP headers", ChangeType: "create",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "hardening")
	require.ErrorContains(t, err, "staged")

	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "not_found", r.Reason())
	require.Empty(t, loadAmendment(t, root, amendmentID).Operations, "a refused stage must not write")
}

// TestStageOperationAddTaskRefusesUnderSameBatchStagedFeature pins the
// deliberate asymmetry with add-feature above: a feature staged earlier on
// this same amendment never parents an add-task, because a new feature's task
// explosion is specify's act, run after the plan gate, not staging's (ruled
// 2026-08-12). The held draft is live, so the refusal is named
// (task_under_held_feature) rather than the resolver's not-found.
func TestStageOperationAddTaskRefusesUnderSameBatchStagedFeature(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	deps := seqDeps()

	_, err := mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/table-backed-forms", ChangeType: "replace",
	})
	require.NoError(t, err)

	_, err = mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: "form-engine/table-backed-forms/schema-validation",
	})
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "task_under_held_feature", r.Reason())
	require.ErrorContains(t, err, "specify")
	require.Len(t, load(t, root).Tasks, 1, "no task was created")

	reloaded := loadAmendment(t, root, amendmentID)
	require.Len(t, reloaded.Operations, 1, "the refused add-task must not write")
}

// TestStageOperationSupersedeByIgnoresSameBatchStagedEpicKey pins that a
// staged add-epic is no supersede replacement: an epic reshapes only through
// its features, and the held draft is a live epic, so naming it on --by
// refuses epic_target the way a long-standing epic does.
func TestStageOperationSupersedeByIgnoresSameBatchStagedEpicKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	tree := load(t, root)
	featureID := tree.Features["F-aaaaaa"].ID

	_, err := mutate.StageOperation(tree, seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})
	require.NoError(t, err)

	_, err = mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpSupersede, Target: featureID, By: "hardening",
	})
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "epic_target", r.Reason())
}

// TestStageOperationReviseTargetIgnoresSameBatchStagedEpicKey pins the same
// exclusion from the OTHER direction: a staged add-epic's key named as a
// revise target is a live held epic, so it refuses epic_target exactly as a
// long-standing epic key does ("epic target by key" in
// TestStageOperationRefusals).
func TestStageOperationReviseTargetIgnoresSameBatchStagedEpicKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	_, err := mutate.StageOperation(load(t, root), seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})
	require.NoError(t, err)

	_, err = mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: "hardening",
	})
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "epic_target", r.Reason())
}

// TestStageOperationAddQuestionHappyPath pins add-question's two-field staged
// shape and its YAML round trip: the short Title and full Statement remain
// independent, with no key and no required blocks.
func TestStageOperationAddQuestionHappyPath(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	res, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:        store.AmendmentOpAddQuestion,
		Title:     "Authentication provider",
		Statement: "Which auth provider?",
	})
	require.NoError(t, err)
	require.Equal(t, "Authentication provider", res.Op.Title)
	require.Equal(t, "Which auth provider?", res.Op.Statement)
	require.Empty(t, res.Op.Key, "a question carries no key")
	require.Empty(t, res.Op.Blocks)

	staged := loadAmendment(t, root, amendmentID).Operations[0]
	require.Equal(t, "Authentication provider", staged.Title)
	require.Equal(t, "Which auth provider?", staged.Statement)
}

func TestStageOperationAddQuestionRequiresTitleAndStatementWithoutWrites(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		in         mutate.StageInput
		wantReason string
	}{
		{
			name:       "missing title",
			in:         mutate.StageInput{Op: store.AmendmentOpAddQuestion, Statement: "Which auth provider?"},
			wantReason: "title_required",
		},
		{
			name:       "missing statement",
			in:         mutate.StageInput{Op: store.AmendmentOpAddQuestion, Title: "Authentication provider"},
			wantReason: "statement_required",
		},
		{
			name:       "line feed in title",
			in:         mutate.StageInput{Op: store.AmendmentOpAddQuestion, Title: "Authentication\nprovider", Statement: "Which auth provider?"},
			wantReason: "title_one_line",
		},
		{
			name:       "carriage return in title",
			in:         mutate.StageInput{Op: store.AmendmentOpAddQuestion, Title: "Authentication\rprovider", Statement: "Which auth provider?"},
			wantReason: "title_one_line",
		},
		{
			name:       "statement on another operation",
			in:         mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "hardening", Statement: "Which auth provider?"},
			wantReason: "statement_misplaced",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, amendmentID := stageFixture(t)
			_, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), tt.in)
			require.Error(t, err)
			var reasoned interface{ Reason() string }
			require.ErrorAs(t, err, &reasoned)
			require.Equal(t, tt.wantReason, reasoned.Reason())
			require.Empty(t, loadAmendment(t, root, amendmentID).Operations, "a refused stage must not write")
		})
	}
}

// TestStageOperationAddQuestionResolvesLiveBlocks pins the ordinary case: a
// blocks entry addressing an existing epic, feature, or task (by id or by
// key) resolves to the node's id at stage time, the same live-tree dispatch
// question.go's own resolveBlockTarget gives `tiller add question`.
func TestStageOperationAddQuestionResolvesLiveBlocks(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	tree := load(t, root)
	epicID := tree.Epics["E-aaaaaa"].ID
	featureID := tree.Features["F-aaaaaa"].ID

	res, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:        store.AmendmentOpAddQuestion,
		Title:     "Schema ownership",
		Statement: "Which schema owns this?",
		Blocks:    []string{epicID, "form-engine/certification-form-loading"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{epicID, featureID}, res.Op.Blocks, "both addresses resolved to ids")
}

// TestStageOperationAddQuestionResolvesStagedBlockKey pins the same-batch
// allowance: a blocks entry naming the key of an add-feature staged EARLIER
// on this same amendment names a live held draft, so it is recorded as the
// draft's id (decision 13), the same resolution a supersede's by gets
// (TestStageOperationSupersedeByStagedAddFeatureKey).
func TestStageOperationAddQuestionResolvesStagedBlockKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	const stagedKey = "form-engine/table-backed-forms"
	added, err := mutate.StageOperation(load(t, root), seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: stagedKey, ChangeType: "replace",
	})
	require.NoError(t, err)

	res, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Legacy schema", Statement: "Keep the legacy schema too?",
		Blocks: []string{stagedKey},
	})
	require.NoError(t, err)
	require.Equal(t, []string{added.Op.Node}, res.Op.Blocks,
		"a held draft is live, so its key resolves to its id at staging")
}

// TestStageOperationAddQuestionResolvesStagedEpicBlockKey mirrors
// TestStageOperationAddQuestionResolvesStagedBlockKey immediately above with
// the staged op swapped from add-feature to add-epic (#57's carried note: no
// pin exercised an add-question blocks target naming a staged EPIC key
// specifically). A held epic resolves like a held feature.
func TestStageOperationAddQuestionResolvesStagedEpicBlockKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	const stagedKey = "hardening"
	added, err := mutate.StageOperation(load(t, root), seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: stagedKey, Title: "Hardening",
	})
	require.NoError(t, err)

	res, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Hardening timing", Statement: "Land this epic now?",
		Blocks: []string{stagedKey},
	})
	require.NoError(t, err)
	require.Equal(t, []string{added.Op.Node}, res.Op.Blocks,
		"a held epic is live, so its key resolves to its id at staging")
}

// TestStageOperationAddQuestionRefusesAnUnresolvableBlock pins the refusal:
// an address naming neither a live node nor a same-batch staged key refuses
// unknown_target, the same reason a supersede's --by gets for the identical
// shape of failure.
func TestStageOperationAddQuestionRefusesAnUnresolvableBlock(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	_, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Missing blocker", Statement: "Blocked on nothing real?",
		Blocks: []string{"nowhere"},
	})
	require.Error(t, err)
	var usage interface{ Reason() string }
	require.ErrorAs(t, err, &usage)
	require.Equal(t, "unknown_target", usage.Reason())
}

// TestStageOperationAddQuestionRefusesANonBlockableTarget pins
// resolveStageBlockTarget's OTHER refusal branch: an address naming a
// non-blockable class (a question, by its own Q- prefix) is a definite
// refusal question.go's own resolveBlockTarget already built, not a
// not-found a staged key might rescue, so it propagates as-is rather than
// falling through to the staged-key search.
func TestStageOperationAddQuestionRefusesANonBlockableTarget(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	_, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddQuestion, Title: "Wrong blocker class", Statement: "Held by the wrong class?",
		Blocks: []string{"Q-aaaaaa"},
	})
	require.ErrorContains(t, err, "not blockable")
}

func TestStageOperationAppendsAcrossMultipleCalls(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	tree := load(t, root)
	featureID := tree.Features["F-aaaaaa"].ID
	taskID := tree.Tasks["T-aaaaaa"].ID

	_, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: featureID,
	})
	require.NoError(t, err)
	_, err = mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpCancel, Target: taskID, Reason: "no longer needed",
	})
	require.NoError(t, err)

	a := loadAmendment(t, root, amendmentID)
	require.Len(t, a.Operations, 2)
	require.Equal(t, store.AmendmentOpRevise, a.Operations[0].Op)
	require.Equal(t, store.AmendmentOpCancel, a.Operations[1].Op)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsOfType(evs, "amendment_staged"), 2, "each staged call records its own event")
}

// --- StageOperation: source ---

// TestStageOperationRoundTripsSource pins StageInput.Source:
// it lands on the staged op verbatim and survives a reload off disk, on a
// hand amendment that carries no Source of its own.
func TestStageOperationRoundTripsSource(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	tree := load(t, root)
	featureID := tree.Features["F-aaaaaa"].ID

	res, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:     store.AmendmentOpRevise,
		Target: featureID,
		Source: "docs/feedback/opus-review.md#L12-18",
	})
	require.NoError(t, err)
	require.Equal(t, "docs/feedback/opus-review.md#L12-18", res.Op.Source)

	reloaded := loadAmendment(t, root, amendmentID)
	require.Equal(t, "docs/feedback/opus-review.md#L12-18", reloaded.Operations[0].Source)
}

// TestStageOperationSourceOptionalWithoutAmendmentSource pins the acceptance
// criterion's second half: "a sourceless hand amendment
// stages exactly as before." stageFixture's own AddAmendment never sets
// Source, so a sourceless op stages clean.
func TestStageOperationSourceOptionalWithoutAmendmentSource(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	require.Nil(t, loadAmendment(t, root, amendmentID).Source)

	tree := load(t, root)
	res, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:     store.AmendmentOpRevise,
		Target: tree.Features["F-aaaaaa"].ID,
	})
	require.NoError(t, err)
	require.Empty(t, res.Op.Source)
}

// TestStageOperationRefusesSourcelessOpWhenAmendmentCarriesSource pins the
// staging rule's exact refusal: an intake-minted amendment
// (Source set) refuses an op with a blank Source, with the exact message text
// the brief pins verbatim.
func TestStageOperationRefusesSourcelessOpWhenAmendmentCarriesSource(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setAmendmentSource(t, root, amendmentID, ".tiller/feedback/opus-review.md", "git:9f2a1c0")

	tree := load(t, root)
	_, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:     store.AmendmentOpRevise,
		Target: tree.Features["F-aaaaaa"].ID,
	})
	require.Error(t, err)
	require.EqualError(t, err, "intake amendments trace every operation: pass --source citing the feedback document")
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "source_required", r.Reason())
	require.Empty(t, loadAmendment(t, root, amendmentID).Operations, "a refused stage must not write")
}

// TestStageOperationRefusesWhitespaceOnlySourceWhenAmendmentCarriesSource
// pins that the blank check trims: a source of pure whitespace is the same
// refusal as an empty one, not a citation that happens to pass a naive
// non-empty check.
func TestStageOperationRefusesWhitespaceOnlySourceWhenAmendmentCarriesSource(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setAmendmentSource(t, root, amendmentID, ".tiller/feedback/opus-review.md", "git:9f2a1c0")

	tree := load(t, root)
	_, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:     store.AmendmentOpRevise,
		Target: tree.Features["F-aaaaaa"].ID,
		Source: "   ",
	})
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "source_required", r.Reason())
}

// TestStageOperationAllowsSourcedOpWhenAmendmentCarriesSource pins the other
// side of the same rule: an intake-minted amendment stages an op that DOES
// carry a Source without complaint.
func TestStageOperationAllowsSourcedOpWhenAmendmentCarriesSource(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setAmendmentSource(t, root, amendmentID, ".tiller/feedback/opus-review.md", "git:9f2a1c0")

	tree := load(t, root)
	res, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op:     store.AmendmentOpRevise,
		Target: tree.Features["F-aaaaaa"].ID,
		Source: "docs/feedback/opus-review.md#L20-25",
	})
	require.NoError(t, err)
	require.Equal(t, "docs/feedback/opus-review.md#L20-25", res.Op.Source)
}

// --- StageOperation: refusals ---

func TestStageOperationRefusesWhenAmendmentNotProposed(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setAmendmentStatus(t, root, amendmentID, store.AmendmentStatusApproved)

	tree := load(t, root)
	_, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpRevise, Target: tree.Features["F-aaaaaa"].ID,
	})
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "amendment_not_open", r.Reason())
	require.Contains(t, err.Error(), store.AmendmentStatusApproved, "the refusal names the actual status")
}

func TestStageOperationRefusals(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	tree := load(t, root)
	featureID := tree.Features["F-aaaaaa"].ID
	epicID := tree.Epics["E-aaaaaa"].ID

	cases := []struct {
		name       string
		in         mutate.StageInput
		wantReason string
	}{
		{
			name:       "unknown target",
			in:         mutate.StageInput{Op: store.AmendmentOpRevise, Target: "no-such-target"},
			wantReason: "unknown_target",
		},
		{
			name:       "epic target",
			in:         mutate.StageInput{Op: store.AmendmentOpRevise, Target: epicID},
			wantReason: "epic_target",
		},
		{
			name:       "epic target by key",
			in:         mutate.StageInput{Op: store.AmendmentOpCancel, Target: "form-engine", Reason: "nope"},
			wantReason: "epic_target",
		},
		{
			name:       "superseded_by on revise",
			in:         mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID, SupersededBy: featureID},
			wantReason: "superseded_by_misplaced",
		},
		{
			name:       "superseded_by on supersede",
			in:         mutate.StageInput{Op: store.AmendmentOpSupersede, Target: featureID, By: featureID, SupersededBy: featureID},
			wantReason: "superseded_by_misplaced",
		},
		{
			name:       "supersede missing by",
			in:         mutate.StageInput{Op: store.AmendmentOpSupersede, Target: featureID},
			wantReason: "supersede_by_required",
		},
		{
			name:       "by on revise",
			in:         mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID, By: featureID},
			wantReason: "supersede_by_misplaced",
		},
		{
			name:       "by on cancel",
			in:         mutate.StageInput{Op: store.AmendmentOpCancel, Target: featureID, Reason: "x", By: featureID},
			wantReason: "supersede_by_misplaced",
		},
		{
			name:       "by on add-feature",
			in:         mutate.StageInput{Op: store.AmendmentOpAddFeature, Key: "form-engine/x", By: featureID},
			wantReason: "supersede_by_misplaced",
		},
		{
			name:       "superseded_by on add-task",
			in:         mutate.StageInput{Op: store.AmendmentOpAddTask, Key: "form-engine/certification-form-loading/x", SupersededBy: featureID},
			wantReason: "superseded_by_misplaced",
		},
		{
			name:       "by on add-epic",
			in:         mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "hardening", By: featureID},
			wantReason: "supersede_by_misplaced",
		},
		{
			name:       "superseded_by on add-epic",
			in:         mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "hardening", SupersededBy: featureID},
			wantReason: "superseded_by_misplaced",
		},
		{
			name:       "by on add-question",
			in:         mutate.StageInput{Op: store.AmendmentOpAddQuestion, Title: "Provider", Statement: "Which provider?", By: featureID},
			wantReason: "supersede_by_misplaced",
		},
		{
			name:       "superseded_by on add-question",
			in:         mutate.StageInput{Op: store.AmendmentOpAddQuestion, Title: "Provider", Statement: "Which provider?", SupersededBy: featureID},
			wantReason: "superseded_by_misplaced",
		},
		{
			name:       "blocks on revise",
			in:         mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID, Blocks: []string{featureID}},
			wantReason: "blocks_misplaced",
		},
		{
			name:       "blocks on supersede",
			in:         mutate.StageInput{Op: store.AmendmentOpSupersede, Target: featureID, By: featureID, Blocks: []string{featureID}},
			wantReason: "blocks_misplaced",
		},
		{
			name:       "blocks on cancel",
			in:         mutate.StageInput{Op: store.AmendmentOpCancel, Target: featureID, Reason: "x", Blocks: []string{featureID}},
			wantReason: "blocks_misplaced",
		},
		{
			name:       "blocks on add-epic",
			in:         mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "hardening", Blocks: []string{featureID}},
			wantReason: "blocks_misplaced",
		},
		{
			name:       "blocks on add-feature",
			in:         mutate.StageInput{Op: store.AmendmentOpAddFeature, Key: "form-engine/x", Blocks: []string{featureID}},
			wantReason: "blocks_misplaced",
		},
		{
			name:       "blocks on add-task",
			in:         mutate.StageInput{Op: store.AmendmentOpAddTask, Key: "form-engine/certification-form-loading/x", Blocks: []string{featureID}},
			wantReason: "blocks_misplaced",
		},
		{
			name:       "by names nothing resolvable",
			in:         mutate.StageInput{Op: store.AmendmentOpSupersede, Target: featureID, By: "nowhere"},
			wantReason: "unknown_target",
		},
		{
			name:       "supersede names its own target as the replacement",
			in:         mutate.StageInput{Op: store.AmendmentOpSupersede, Target: featureID, By: featureID},
			wantReason: "self_supersession",
		},
		{
			name:       "supersede names its own target by key",
			in:         mutate.StageInput{Op: store.AmendmentOpSupersede, Target: featureID, By: "form-engine/certification-form-loading"},
			wantReason: "self_supersession",
		},
		{
			name:       "cancel names its own target as the replacement",
			in:         mutate.StageInput{Op: store.AmendmentOpCancel, Target: featureID, Reason: "x", SupersededBy: featureID},
			wantReason: "self_supersession",
		},
		{
			name:       "superseded_by names nothing resolvable",
			in:         mutate.StageInput{Op: store.AmendmentOpCancel, Target: featureID, Reason: "x", SupersededBy: "nowhere"},
			wantReason: "unknown_target",
		},
		{
			name:       "unknown precondition name",
			in:         mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID, Preconditions: map[string]string{"bogus": "x"}},
			wantReason: "precondition_invalid",
		},
		{
			name:       "empty precondition value",
			in:         mutate.StageInput{Op: store.AmendmentOpRevise, Target: featureID, Preconditions: map[string]string{"spec_revision": "  "}},
			wantReason: "precondition_invalid",
		},
		{
			name:       "malformed add-feature key",
			in:         mutate.StageInput{Op: store.AmendmentOpAddFeature, Key: "not-canonical"},
			wantReason: "invalid_key",
		},
		{
			name:       "add-feature under unknown epic",
			in:         mutate.StageInput{Op: store.AmendmentOpAddFeature, Key: "no-such-epic/x"},
			wantReason: "not_found",
		},
		{
			name:       "malformed add-task key",
			in:         mutate.StageInput{Op: store.AmendmentOpAddTask, Key: "form-engine/certification-form-loading"},
			wantReason: "invalid_key",
		},
		{
			name:       "add-task under unknown feature",
			in:         mutate.StageInput{Op: store.AmendmentOpAddTask, Key: "form-engine/no-such-feature/x"},
			wantReason: "not_found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), tc.in)
			require.Error(t, err)

			if tc.wantReason == "not_found" {
				var nf *resolve.ErrNotFound
				require.ErrorAs(t, err, &nf)
				return
			}
			var r interface{ Reason() string }
			require.ErrorAs(t, err, &r)
			require.Equal(t, tc.wantReason, r.Reason())
			require.Empty(t, loadAmendment(t, root, amendmentID).Operations, "a refused stage must not write")
		})
	}
}

// TestStageOperationUnrecognizedOpIsCaughtByValidateAmendment pins that an
// op outside the five known kinds is not judged by a bespoke reason in this
// package: it rides through to store.ValidateAmendment (Task 1's own
// validator), which refuses the shape before it reaches disk.
func TestStageOperationUnrecognizedOpIsCaughtByValidateAmendment(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	_, err := mutate.StageOperation(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: "rewrite",
	})
	require.Error(t, err)
	var shapeErr *store.AmendmentShapeError
	require.ErrorAs(t, err, &shapeErr)
	require.Empty(t, loadAmendment(t, root, amendmentID).Operations)
}

// --- StageAmendsDesign ---

func TestStageAmendsDesignUnionsAndReportsNoChangeOnReAdd(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	res, err := mutate.StageAmendsDesign(load(t, root), testDeps(), loadAmendment(t, root, amendmentID),
		[]string{"design/a.md", "design/b.md"})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, []string{"design/a.md", "design/b.md"}, res.AmendsDesign)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsOfType(evs, "amendment_staged"), 1)

	// Re-adding the same path, plus one genuinely new one, unions in only the
	// new one and preserves first-seen order.
	res2, err := mutate.StageAmendsDesign(load(t, root), testDeps(), loadAmendment(t, root, amendmentID),
		[]string{"design/a.md", "design/c.md"})
	require.NoError(t, err)
	require.True(t, res2.Changed)
	require.Equal(t, []string{"design/a.md", "design/b.md", "design/c.md"}, res2.AmendsDesign)

	// A pure re-add of what is already there is a no-op: no write, no event.
	res3, err := mutate.StageAmendsDesign(load(t, root), testDeps(), loadAmendment(t, root, amendmentID),
		[]string{"design/a.md", "design/b.md"})
	require.NoError(t, err)
	require.False(t, res3.Changed)
	require.Equal(t, []string{"design/a.md", "design/b.md", "design/c.md"}, res3.AmendsDesign)

	evsAfter, err := events.List(load(t, root))
	require.NoError(t, err)
	require.Len(t, eventsOfType(evsAfter, "amendment_staged"), 2, "the no-op re-add records no new event")
}

func TestStageAmendsDesignRefusesWhenAmendmentNotProposed(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setAmendmentStatus(t, root, amendmentID, store.AmendmentStatusWithdrawn)

	_, err := mutate.StageAmendsDesign(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), []string{"design/a.md"})
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "amendment_not_open", r.Reason())
}

// --- WithdrawAmendment ---

func TestWithdrawAmendmentHappyPath(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	res, err := mutate.WithdrawAmendment(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), "superseded by a better plan")
	require.NoError(t, err)
	require.Equal(t, store.AmendmentStatusWithdrawn, res.Status)
	require.Equal(t, "superseded by a better plan", res.Reason)

	reloaded := loadAmendment(t, root, amendmentID)
	require.Equal(t, store.AmendmentStatusWithdrawn, reloaded.Status)

	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	withdrawn := eventsOfType(evs, "amendment_withdrawn")
	require.Len(t, withdrawn, 1)
	require.Equal(t, "superseded by a better plan", withdrawn[0].After["reason"])

}

func TestWithdrawAmendmentRequiresReason(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	_, err := mutate.WithdrawAmendment(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), "   ")
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "reason_required", r.Reason())
	require.Equal(t, store.AmendmentStatusDraft, loadAmendment(t, root, amendmentID).Status)
}

func TestWithdrawAmendmentRefusesWhenNotProposed(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setAmendmentStatus(t, root, amendmentID, store.AmendmentStatusApproved)

	_, err := mutate.WithdrawAmendment(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), "reconsidered")
	require.Error(t, err)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "amendment_not_open", r.Reason())
}

func TestAddAmendmentOpensAsDraft(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	res, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{
		Key: "iphone-only", Title: "Remove native iPad support from v1"})
	require.NoError(t, err)
	require.Equal(t, store.AmendmentStatusDraft, res.Status)

	tree := load(t, root)
	require.Equal(t, store.AmendmentStatusDraft, tree.Amendments[res.ID].Status)

	evs, err := events.List(tree)
	require.NoError(t, err)
	opened := eventsOfType(evs, "amendment_opened")
	require.Len(t, opened, 1)
	require.Equal(t, store.AmendmentStatusDraft, opened[0].After["status"])
}

func TestStageAndWithdrawWorkOnDraftAndProposed(t *testing.T) {
	t.Parallel()
	for _, status := range []string{store.AmendmentStatusDraft, store.AmendmentStatusProposed} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			root, amendmentID := stageFixture(t) // opens as draft
			if status != store.AmendmentStatusDraft {
				setAmendmentStatus(t, root, amendmentID, status)
			}

			tree := load(t, root)
			_, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
				Op: store.AmendmentOpRevise, Target: tree.Features["F-aaaaaa"].ID,
			})
			require.NoError(t, err)

			_, err = mutate.WithdrawAmendment(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), "changed course")
			require.NoError(t, err)
			require.Equal(t, store.AmendmentStatusWithdrawn, loadAmendment(t, root, amendmentID).Status)
		})
	}

	t.Run("approved is refused", func(t *testing.T) {
		t.Parallel()
		root, amendmentID := stageFixture(t)
		setAmendmentStatus(t, root, amendmentID, store.AmendmentStatusApproved)
		tree := load(t, root)
		_, err := mutate.StageOperation(tree, testDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: tree.Features["F-aaaaaa"].ID,
		})
		var r interface{ Reason() string }
		require.ErrorAs(t, err, &r)
		require.Equal(t, "amendment_not_open", r.Reason())
		_, err = mutate.WithdrawAmendment(load(t, root), testDeps(), loadAmendment(t, root, amendmentID), "late")
		require.ErrorAs(t, err, &r)
		require.Equal(t, "amendment_not_open", r.Reason())
	})
}

func TestOpenKeyCollisionCoversDraft(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	first, err := mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{Key: "iphone-only", Title: "First"})
	require.NoError(t, err)
	require.Equal(t, store.AmendmentStatusDraft, first.Status)

	_, err = mutate.AddAmendment(load(t, root), gitDeps(root), mutate.AmendmentInput{Key: "iphone-only", Title: "Second"})
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "amendment_exists", r.Reason())
	require.Len(t, load(t, root).Amendments, 1)
}

// --- StageOperation: held drafts ---

// stageHeld stages one add onto an open amendment with a sequential minter (the
// fixture's fixed minter collides with the fixture's own E-, F-, and T-aaaaaa
// once staging mints real nodes) and returns the result.
func stageHeld(t *testing.T, root string, deps mutate.Deps, amendmentID string, in mutate.StageInput) mutate.StageResult {
	t.Helper()
	res, err := mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, amendmentID), in)
	require.NoError(t, err)
	return res
}

// eventsFor filters an event list down to one type naming target first.
func eventsFor(t *testing.T, root, typ, target string) []events.Event {
	t.Helper()
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	var out []events.Event
	for _, e := range eventsOfType(evs, typ) {
		if len(e.Targets) > 0 && e.Targets[0] == target {
			out = append(out, e)
		}
	}
	return out
}

// requireHeldBy asserts the loaded tree's hold index and gen.AmendmentHold
// both name amendmentID as the holder of n.
func requireHeldBy(t *testing.T, tree *store.Tree, n store.Node, amendmentID string) {
	t.Helper()
	require.Equal(t, amendmentID, tree.AmendmentHolds[n.NodeID()])
	holder := gen.AmendmentHold(tree, n)
	require.NotNil(t, holder, "%s must be held", n.NodeID())
	require.Equal(t, amendmentID, holder.ID)
}

// TestStageAddEpicCreatesAHeldDraft: staging an add-epic creates the epic at
// once, in draft, records its id on the operation, and the tree knows the
// amendment holds it (decision 7). The node's own add.epic event carries the
// amendment and operation index, as approval wrote them before, and lands in
// the same commit as the amendment_staged event.
func TestStageAddEpicCreatesAHeldDraft(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	res := stageHeld(t, root, seqDeps(), amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})
	require.NotEmpty(t, res.Op.Node)
	require.FileExists(t, filepath.Join(root, ".tiller", "epics", "hardening", "epic.yml"))
	require.FileExists(t, filepath.Join(root, ".tiller", "epics", "hardening", "epic.md"))

	tree := load(t, root)
	epic := tree.Epics[res.Op.Node]
	require.NotNil(t, epic)
	require.Equal(t, "draft", epic.Status)
	require.Equal(t, "hardening", *epic.Key)
	require.Equal(t, res.Op.Node, tree.Amendments[amendmentID].Operations[0].Node)
	requireHeldBy(t, tree, epic, amendmentID)

	added := eventsFor(t, root, "add.epic", epic.ID)
	require.Len(t, added, 1)
	require.Equal(t, amendmentID, added[0].After["amendment"])
	require.EqualValues(t, 0, added[0].After["operation_index"])
	require.Contains(t, added[0].Targets, amendmentID)
	require.Len(t, eventsFor(t, root, "amendment_staged", amendmentID), 1)
}

// TestStageAddFeatureUnderAHeldEpic: a feature staged under an epic the same
// amendment added finds that epic live and lands as a second held draft.
func TestStageAddFeatureUnderAHeldEpic(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	deps := seqDeps()

	epic := stageHeld(t, root, deps, amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening",
	})
	res := stageHeld(t, root, deps, amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "hardening/csp-headers", Title: "CSP headers", ChangeType: "create",
	})

	tree := load(t, root)
	feat := tree.Features[res.Op.Node]
	require.NotNil(t, feat)
	require.Equal(t, "draft", feat.Status)
	require.Equal(t, "hardening", feat.EpicKey)
	requireHeldBy(t, tree, feat, amendmentID)
	requireHeldBy(t, tree, tree.Epics[epic.Op.Node], amendmentID)

	added := eventsFor(t, root, "add.feature", feat.ID)
	require.Len(t, added, 1)
	require.EqualValues(t, 1, added[0].After["operation_index"])
	require.Equal(t, epic.Op.Node, added[0].After["epic"])
}

// TestStageAddFeatureUnderAnApprovedEpic: a new feature under existing,
// approved work is a held draft too; the hold, not the parent, keeps it inert.
func TestStageAddFeatureUnderAnApprovedEpic(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setEpicStatus(t, root, "form-engine", "draft", "ready")

	res := stageHeld(t, root, seqDeps(), amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/table-backed-forms", Title: "Table-backed forms", ChangeType: "replace",
	})

	tree := load(t, root)
	feat := tree.Features[res.Op.Node]
	require.NotNil(t, feat)
	require.Equal(t, "draft", feat.Status)
	require.Equal(t, "replace", feat.ChangeType)
	requireHeldBy(t, tree, feat, amendmentID)
	require.Nil(t, gen.AmendmentHold(tree, tree.Epics["E-aaaaaa"]), "the approved parent is not held")
}

// TestStageAddTaskCreatesAHeldProposedTask: a task staged under an approved
// feature lands proposed, as `tiller add task` lands it, and held.
func TestStageAddTaskCreatesAHeldProposedTask(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setEpicStatus(t, root, "form-engine", "draft", "ready")
	setFeatureStatusOnDisk(t, root, "form-engine", "certification-form-loading", "ready")

	res := stageHeld(t, root, seqDeps(), amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddTask, Key: "form-engine/certification-form-loading/iphone-acceptance", Title: "iPhone-only acceptance",
	})

	tree := load(t, root)
	task := tree.Tasks[res.Op.Node]
	require.NotNil(t, task)
	require.Equal(t, "proposed", task.Status)
	require.Equal(t, "F-aaaaaa", task.FeatureID)
	require.FileExists(t, filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "tasks", "iphone-acceptance", "task.md"))
	requireHeldBy(t, tree, task, amendmentID)
	require.Len(t, eventsFor(t, root, "add.task", task.ID), 1)
}

// TestStageAddRefusesAnExistingKey: an amendment never re-adds a node that
// exists, so staging a key a live node holds refuses key_exists and writes
// nothing, not even the operation.
func TestStageAddRefusesAnExistingKey(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)

	_, err := mutate.StageOperation(load(t, root), seqDeps(), loadAmendment(t, root, amendmentID), mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/certification-form-loading", Title: "Again", ChangeType: "create",
	})
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "key_exists", r.Reason())
	require.Empty(t, loadAmendment(t, root, amendmentID).Operations, "a refused stage must not write")
	require.Empty(t, eventsFor(t, root, "amendment_staged", amendmentID))
	require.Len(t, load(t, root).Features, 1)
}

// TestStageAddRefusesAParentHeldElsewhere: an add under a draft a different
// open amendment holds is refused, naming that amendment, or approving or
// withdrawing one amendment would move or delete nodes the other proposes.
func TestStageAddRefusesAParentHeldElsewhere(t *testing.T) {
	t.Parallel()
	root, firstID := stageFixture(t)
	setEpicStatus(t, root, "form-engine", "draft", "ready")
	deps := seqDeps()
	stageHeld(t, root, deps, firstID, mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"})
	stageHeld(t, root, deps, firstID, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/table-backed-forms", Title: "Table-backed forms", ChangeType: "create",
	})
	secondDeps := gitDeps(root)
	secondDeps.Mint = deps.Mint
	second, err := mutate.AddAmendment(load(t, root), secondDeps, mutate.AmendmentInput{Key: "second", Title: "Second"})
	require.NoError(t, err)

	cases := []mutate.StageInput{
		{Op: store.AmendmentOpAddFeature, Key: "hardening/csp-headers", Title: "CSP headers", ChangeType: "create"},
		{Op: store.AmendmentOpAddTask, Key: "form-engine/table-backed-forms/schema", Title: "Schema"},
	}
	for _, in := range cases {
		_, err := mutate.StageOperation(load(t, root), deps, loadAmendment(t, root, second.ID), in)
		var r interface{ Reason() string }
		require.ErrorAs(t, err, &r, in.Key)
		require.Equal(t, "parent_held_elsewhere", r.Reason(), in.Key)
		require.ErrorContains(t, err, "is held by amendment")
		require.ErrorContains(t, err, firstID)
		require.ErrorContains(t, err, "stage this on that amendment")
	}
	require.Empty(t, loadAmendment(t, root, second.ID).Operations, "a refused stage must not write")
}

// TestProposeRefusesAMissingHeldNode: propose checks that every node the
// amendment holds still exists (decision 10), naming the operation whose node
// is gone, and leaves the amendment a draft.
func TestProposeRefusesAMissingHeldNode(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	stageHeld(t, root, seqDeps(), amendmentID, mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"})
	a := loadAmendment(t, root, amendmentID)
	ops := append([]store.AmendmentOp(nil), a.Operations...)
	ops[0].Node = "E-zzzzzz"
	a = writeAmendmentOps(t, root, a, ops)
	require.Empty(t, load(t, root).AmendmentHolds, "the index skips the missing node")

	_, err := mutate.ProposeAmendment(load(t, root), gitDeps(root), a)
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "held_node_missing", r.Reason())
	require.ErrorContains(t, err, "operation 0")
	require.Equal(t, store.AmendmentStatusDraft, loadAmendment(t, root, amendmentID).Status)
}

// TestStageSourceCitationReachesTheNode: an add's --source lands on the
// created node's own source, as approval carried it before (a task has no
// source field; its citation stays on the operation).
func TestStageSourceCitationReachesTheNode(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	deps := seqDeps()

	epic := stageHeld(t, root, deps, amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening", Source: "design/hardening.md#L1-L9",
	})
	feat := stageHeld(t, root, deps, amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "hardening/csp-headers", ChangeType: "create", Source: "design/hardening.md#L4-L6",
	})

	tree := load(t, root)
	require.Contains(t, tree.Epics, epic.Op.Node)
	require.Contains(t, tree.Features, feat.Op.Node)
	require.Equal(t, "design/hardening.md#L1-L9", tree.Epics[epic.Op.Node].Source)
	require.Equal(t, "design/hardening.md#L4-L6", tree.Features[feat.Op.Node].Source)
	require.Equal(t, "design/hardening.md#L4-L6", tree.Amendments[amendmentID].Operations[1].Source)
}

// TestAddFeatureUnderAHeldEpicIsRefused: a plain `tiller add feature` under a
// held epic would lay an unheld child beneath a proposal, which the
// amendment's approval and withdrawal would then mishandle. It is refused,
// naming the amendment, and nothing is written; the holding amendment's own
// staging path stays open.
func TestAddFeatureUnderAHeldEpicIsRefused(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	deps := heldDeps(root)
	epic := stageHeld(t, root, deps, amendmentID, mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"})

	_, err := mutate.AddFeature(load(t, root), deps, mutate.FeatureInput{
		Key: "hardening/csp-headers", Title: "CSP headers", ChangeType: "create"})
	var r interface{ Reason() string }
	require.ErrorAs(t, err, &r)
	require.Equal(t, "parent_held_elsewhere", r.Reason())
	require.ErrorContains(t, err, "hardening ("+epic.Op.Node+") is held by amendment iphone-only ("+amendmentID+"); stage this on that amendment")
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics", "hardening", "features", "csp-headers"))
	require.Len(t, load(t, root).Features, 1)

	res := stageHeld(t, root, deps, amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "hardening/csp-headers", Title: "CSP headers", ChangeType: "create"})
	require.NotEmpty(t, res.Op.Node, "the holding amendment still stages beneath its own epic")
}

// TestAddTaskUnderAHeldFeatureIsRefused: the same rule one grain down, for a
// task under a held feature and for a task under an unheld feature whose epic
// is held.
func TestAddTaskUnderAHeldFeatureIsRefused(t *testing.T) {
	t.Parallel()
	root, amendmentID := stageFixture(t)
	setEpicStatus(t, root, "form-engine", "draft", "ready")
	deps := heldDeps(root)
	stageHeld(t, root, deps, amendmentID, mutate.StageInput{
		Op: store.AmendmentOpAddFeature, Key: "form-engine/table-backed-forms", Title: "Table-backed forms", ChangeType: "create"})
	stageHeld(t, root, deps, amendmentID, mutate.StageInput{Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening"})
	// An unheld feature under the held epic, laid by hand the way a tree from
	// before this guard could carry one.
	featureDir := filepath.Join(root, ".tiller", "epics", "hardening", "features", "csp-headers")
	require.NoError(t, os.MkdirAll(featureDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "feature.yml"),
		[]byte("id: F-zzzzzz\nkey: hardening/csp-headers\ntitle: CSP headers\nstatus: draft\nchange_type: create\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "feature.md"), []byte("# CSP headers\n"), 0o644))
	tasksBefore := len(load(t, root).Tasks)

	for _, key := range []string{"form-engine/table-backed-forms/schema", "hardening/csp-headers/schema"} {
		_, err := mutate.AddTask(load(t, root), deps, mutate.TaskInput{Key: key, Title: "Schema"})
		var r interface{ Reason() string }
		require.ErrorAs(t, err, &r, key)
		require.Equal(t, "parent_held_elsewhere", r.Reason(), key)
		require.ErrorContains(t, err, "is held by amendment iphone-only ("+amendmentID+"); stage this on that amendment", key)
	}
	require.Len(t, load(t, root).Tasks, tasksBefore, "a refused add writes nothing")
	require.NoDirExists(t, filepath.Join(root, ".tiller", "epics", "form-engine", "features", "table-backed-forms", "tasks"))
}
