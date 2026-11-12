package serve_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type identityClock struct{ at time.Time }

func (c *identityClock) Now() time.Time { c.at = c.at.Add(time.Second); return c.at }

func identityRepo(t *testing.T) (string, mutate.Deps) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))
	testgit.Init(t, root)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "init")
	return root, mutate.Deps{Contributor: "jc", Clock: &identityClock{at: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}, Mint: sequentialMint()}
}

func identityTree(t *testing.T, root string) *store.Tree {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	return tree
}

func identityStage(t *testing.T, root string, deps mutate.Deps, amendmentID string, ops ...mutate.StageInput) {
	t.Helper()
	for _, op := range ops {
		tree := identityTree(t, root)
		_, err := mutate.StageOperation(tree, deps, tree.Amendments[amendmentID], op)
		require.NoError(t, err)
	}
}

// legacyStage stages one add-feature and then rewrites what staging left into
// the shape an amendment staged before schema 15 carries: the operation
// keeps no node, the draft staging created is gone, and so is its
// add.feature event. Only the amendment_staged record remains.
func legacyStage(t *testing.T, root string, deps mutate.Deps, amendmentID string, op mutate.StageInput) {
	t.Helper()
	require.Equal(t, store.AmendmentOpAddFeature, op.Op)
	identityStage(t, root, deps, amendmentID, op)
	tree := identityTree(t, root)
	doc := *tree.Amendments[amendmentID]
	last := len(doc.Operations) - 1
	node := doc.Operations[last].Node
	require.NotEmpty(t, node)
	evs, err := events.List(tree)
	require.NoError(t, err)
	for _, ev := range evs {
		if ev.Type == "add.feature" && ev.Targets[0] == node {
			path, _, err := events.File(tree, ev)
			require.NoError(t, err)
			require.NoError(t, os.Remove(path))
		}
	}
	feature := tree.Features[node]
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".tiller", "epics", feature.EpicKey, "features", filepath.Base(*feature.Key))))
	doc.Operations = append([]store.AmendmentOp(nil), doc.Operations...)
	doc.Operations[last].Node = ""
	data, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(amendmentDocPath(t, root, amendmentID), data, 0o644))
}

// approveFeatureOnDisk marks a feature ready, the approved state an amendment
// needs before it may add tasks to it.
func approveFeatureOnDisk(t *testing.T, root, featureKey string) {
	t.Helper()
	epicKey, local, _ := strings.Cut(featureKey, "/")
	path := filepath.Join(root, ".tiller", "epics", epicKey, "features", local, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: draft", "status: ready", 1)
	require.NotEqual(t, string(data), out)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// returnTask moves a task the amendment's approval promoted back to proposed,
// the narrow reverse that makes its key and its removal legal again.
func returnTask(t *testing.T, root string, deps mutate.Deps, taskID string) {
	t.Helper()
	tree := identityTree(t, root)
	_, err := mutate.Transition(tree, deps, tree.Tasks[taskID], mutate.TransitionInput{Verb: "return", Reason: "Revisit"})
	require.NoError(t, err)
}

func identityPage(t *testing.T, srv *httptest.Server, noun, nodeID string) views.NodePage {
	t.Helper()
	var page views.NodePage
	require.Equal(t, 200, getJSON(t, srv, "/api/node/"+noun+"/"+nodeID, &page))
	return page
}

func TestAmendmentNavigationSurvivesApplicationRenameRemoveAndKeyReuse(t *testing.T) {
	root, deps := identityRepo(t)
	ep, err := mutate.AddEpic(identityTree(t, root), deps, mutate.EpicInput{Key: "engine", Title: "Engine"})
	require.NoError(t, err)
	feature, err := mutate.AddFeature(identityTree(t, root), deps, mutate.FeatureInput{Key: "engine/parse", Title: "Parse", ChangeType: "create"})
	require.NoError(t, err)
	old, err := mutate.AddTask(identityTree(t, root), deps, mutate.TaskInput{Key: "engine/parse/old", Title: "Old parser"})
	require.NoError(t, err)
	sibling, err := mutate.AddTask(identityTree(t, root), deps, mutate.TaskInput{Key: "engine/parse/unchanged", Title: "Unchanged"})
	require.NoError(t, err)
	approveFeatureOnDisk(t, root, "engine/parse")
	a, err := mutate.AddAmendment(identityTree(t, root), deps, mutate.AmendmentInput{Key: "reshape", Title: "Replace parser"})
	require.NoError(t, err)
	proposeAmendment(t, root, deps, a.ID)
	identityStage(t, root, deps, a.ID,
		mutate.StageInput{Op: "add-task", Key: "engine/parse/new", Title: "New parser"},
		mutate.StageInput{Op: "add-task", Key: "engine/parse/extra", Title: "Extra parser"},
		mutate.StageInput{Op: "cancel", Target: old.ID, SupersededBy: "engine/parse/new", Reason: "Replaced"},
		mutate.StageInput{Op: "add-question", Title: "Parser policy", Statement: "Which parser policy?", Blocks: []string{"engine/parse/new", "engine/parse/extra"}},
	)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	proposed := identityPage(t, srv, "amendment", a.ID)
	heldNew := identityTree(t, root).Amendments[a.ID].Operations[0].Node
	// Staging creates the held draft and resolves the replacement to its id,
	// so both links are live before approval.
	require.Equal(t, heldNew, proposed.Amendment.Operations[0].Created.ID)
	require.Equal(t, heldNew, proposed.Amendment.Operations[2].SupersededBy.ID, "the replacement is the held draft's id")
	require.Equal(t, feature.ID, proposed.Amendment.Operations[0].Parent.ID)
	tree := identityTree(t, root)
	approved, err := mutate.ApproveAmendment(tree, deps, tree.Amendments[a.ID])
	require.NoError(t, err)
	newID, extraID := approved.Applied[0].NodeID, approved.Applied[1].NodeID
	require.Equal(t, heldNew, newID)
	// Approval promoted both tasks to ready; returning them makes the rename
	// and the removal below legal again.
	returnTask(t, root, deps, newID)
	returnTask(t, root, deps, extraID)
	_, err = mutate.RenameKey(identityTree(t, root), deps, mutate.RenameKeyInput{Noun: "task", Node: newID, Key: "engine/parse/renamed"})
	require.NoError(t, err)
	reused, err := mutate.AddTask(identityTree(t, root), deps, mutate.TaskInput{Key: "engine/parse/new", Title: "Unrelated reuse"})
	require.NoError(t, err)
	_, err = mutate.RemoveDraft(identityTree(t, root), deps, mutate.RemoveInput{Noun: "task", Node: extraID, Reason: "Drop proposal"})
	require.NoError(t, err)
	reusedExtra, err := mutate.AddTask(identityTree(t, root), deps, mutate.TaskInput{Key: "engine/parse/extra", Title: "Unrelated extra"})
	require.NoError(t, err)
	forward := identityPage(t, srv, "amendment", a.ID).Amendment
	require.Equal(t, newID, forward.Operations[0].Created.ID)
	require.Equal(t, "engine/parse/renamed", forward.Operations[0].Created.Key)
	require.Equal(t, newID, forward.Operations[2].SupersededBy.ID, "cancel's replacement lives in event After, not its Targets")
	require.Equal(t, extraID, forward.Operations[1].Created.ID, "removed identities remain historical IDs, not today's key holder")
	require.Empty(t, forward.Operations[1].Created.Noun, "deleted identity is text, not a live-node navigation target")
	require.NotEmpty(t, forward.Operations[1].IdentityNotes)
	require.Equal(t, []string{newID, extraID}, []string{forward.Operations[3].Blocks[0].ID, forward.Operations[3].Blocks[1].ID})
	require.Equal(t, approved.Applied[3].NodeID, forward.Operations[3].Created.ID)
	for _, unchanged := range []string{sibling.ID, reused.ID, reusedExtra.ID} {
		require.Empty(t, identityPage(t, srv, "task", unchanged).Relationships.Amendments)
	}
	for _, ancestor := range []struct{ noun, id string }{{"feature", feature.ID}, {"epic", ep.ID}} {
		rels := identityPage(t, srv, ancestor.noun, ancestor.id).Relationships.Amendments
		require.Len(t, rels, 1, "multiple descendants count as one amendment")
		require.Equal(t, "Replace parser", rels[0].Amendment.Title)
		var descendantIDs []string
		for _, effect := range rels[0].Effects {
			if effect.Scope == "descendant" && effect.Role == "created" {
				descendantIDs = append(descendantIDs, effect.Node.ID)
			}
		}
		require.Contains(t, descendantIDs, newID)
		require.NotContains(t, descendantIDs, reused.ID)
	}
	taskRels := identityPage(t, srv, "task", newID).Relationships.Amendments
	require.Len(t, taskRels, 1)
	for _, effect := range taskRels[0].Effects {
		require.Equal(t, "direct", effect.Scope)
	}
	require.NotEmpty(t, taskRels[0].ProposedAt)
	require.NotEmpty(t, taskRels[0].ApprovedAt)

	// Terminal amendment keys are reusable, but links and row membership are IDs.
	withdrawn, err := mutate.AddAmendment(identityTree(t, root), deps, mutate.AmendmentInput{Key: "reshape", Title: "Withdrawn alternative"})
	require.NoError(t, err)
	proposeAmendment(t, root, deps, withdrawn.ID)
	identityStage(t, root, deps, withdrawn.ID, mutate.StageInput{Op: "cancel", Target: newID, Reason: "Alternative"})
	tree = identityTree(t, root)
	_, err = mutate.WithdrawAmendment(tree, deps, tree.Amendments[withdrawn.ID], "Not selected")
	require.NoError(t, err)
	pending, err := mutate.AddAmendment(identityTree(t, root), deps, mutate.AmendmentInput{Key: "reshape", Title: "Pending alternative"})
	require.NoError(t, err)
	proposeAmendment(t, root, deps, pending.ID)
	identityStage(t, root, deps, pending.ID, mutate.StageInput{Op: "cancel", Target: newID, Reason: "Consider removal"})
	rows := identityPage(t, srv, "task", newID).Relationships.Amendments
	require.Len(t, rows, 3)
	require.Equal(t, pending.ID, rows[0].Amendment.ID)
	terminal := []string{a.ID, withdrawn.ID}
	sort.Strings(terminal)
	require.Equal(t, terminal, []string{rows[1].Amendment.ID, rows[2].Amendment.ID})
	listServer := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(listServer.Close)
	for _, noun := range []string{"amendment", "amendments"} {
		var list views.ListResult
		require.Equal(t, 200, getJSON(t, listServer, "/api/ls?noun="+noun, &list))
		require.Len(t, list.Nodes, 3)
		require.Equal(t, pending.ID, list.Nodes[0].ID)
		require.Equal(t, terminal, []string{list.Nodes[1].ID, list.Nodes[2].ID})
	}
}

func TestAmendmentSameBatchParentAndHistoricalEpicDoNotRetarget(t *testing.T) {
	root, deps := identityRepo(t)
	a, err := mutate.AddAmendment(identityTree(t, root), deps, mutate.AmendmentInput{Key: "new-area", Title: "New area"})
	require.NoError(t, err)
	proposeAmendment(t, root, deps, a.ID)
	identityStage(t, root, deps, a.ID,
		mutate.StageInput{Op: "add-epic", Key: "area", Title: "Area"},
		mutate.StageInput{Op: "add-feature", Key: "area/feature", Title: "Feature", ChangeType: "create"},
	)
	// An approved epic's key is locked, so the epic moves while it is still a
	// held draft, the window rename is legal in.
	epicID := identityTree(t, root).Amendments[a.ID].Operations[0].Node
	_, err = mutate.RenameKey(identityTree(t, root), deps, mutate.RenameKeyInput{Noun: "epic", Node: epicID, Key: "moved"})
	require.NoError(t, err)
	tree := identityTree(t, root)
	approved, err := mutate.ApproveAmendment(tree, deps, tree.Amendments[a.ID])
	require.NoError(t, err)
	require.Equal(t, epicID, approved.Applied[0].NodeID)
	featureID := approved.Applied[1].NodeID
	newEpic, err := mutate.AddEpic(identityTree(t, root), deps, mutate.EpicInput{Key: "area", Title: "Different area"})
	require.NoError(t, err)
	_, err = mutate.RemoveDraft(identityTree(t, root), deps, mutate.RemoveInput{Noun: "feature", Node: featureID, Reason: "Not needed"})
	require.NoError(t, err)
	newFeature, err := mutate.AddFeature(identityTree(t, root), deps, mutate.FeatureInput{Key: "area/feature", Title: "Different feature", ChangeType: "create"})
	require.NoError(t, err)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	forward := identityPage(t, srv, "amendment", a.ID).Amendment
	require.Equal(t, featureID, forward.Operations[1].Created.ID)
	require.Equal(t, epicID, forward.Operations[1].Parent.ID)
	original := identityPage(t, srv, "epic", epicID).Relationships.Amendments
	require.Len(t, original, 1)
	var historical bool
	for _, effect := range original[0].Effects {
		if effect.Role == "addition_parent" {
			historical = effect.Scope == "historical_parent"
			require.Equal(t, 2, *effect.OperationIndex)
		}
	}
	require.True(t, historical, "original parent association survives removal without claiming current descendants")
	require.Empty(t, identityPage(t, srv, "epic", newEpic.ID).Relationships.Amendments)
	require.Empty(t, identityPage(t, srv, "feature", newFeature.ID).Relationships.Amendments)
}

func TestAmendmentQuestionFoldsAndPartialApplicationRemainHonest(t *testing.T) {
	root, deps := identityRepo(t)
	ep, err := mutate.AddEpic(identityTree(t, root), deps, mutate.EpicInput{Key: "area", Title: "Area"})
	require.NoError(t, err)
	q, err := mutate.AddQuestion(identityTree(t, root), deps, mutate.QuestionInput{Title: "Existing policy", Statement: "Which policy?"})
	require.NoError(t, err)
	a, err := mutate.AddAmendment(identityTree(t, root), deps, mutate.AmendmentInput{Key: "questions", Title: "Question update"})
	require.NoError(t, err)
	proposeAmendment(t, root, deps, a.ID)
	identityStage(t, root, deps, a.ID,
		mutate.StageInput{Op: "add-feature", Key: "area/new", Title: "New feature", ChangeType: "create"},
		mutate.StageInput{Op: "add-question", Title: "Existing policy", Statement: "Which policy?", Blocks: []string{ep.ID}},
		mutate.StageInput{Op: "add-question", Title: "Existing policy", Statement: "Which policy?", Blocks: []string{ep.ID}},
		mutate.StageInput{Op: "add-question", Title: "New policy", Statement: "Which new policy?", Blocks: []string{"area/new"}},
	)
	tree := identityTree(t, root)
	res, err := mutate.ApproveAmendment(tree, deps, tree.Amendments[a.ID])
	require.NoError(t, err)
	require.Equal(t, q.ID, res.Applied[1].NodeID)
	require.Equal(t, mutate.AppliedUpdated, res.Applied[1].Outcome)
	require.Equal(t, mutate.AppliedNoChange, res.Applied[2].Outcome)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	forward := identityPage(t, srv, "amendment", a.ID).Amendment
	require.Nil(t, forward.Operations[1].Created, "an add.question event can be a fold, not creation")
	require.Nil(t, forward.Operations[2].Created, "a no-op has no creation event")
	require.Equal(t, res.Applied[3].NodeID, forward.Operations[3].Created.ID)
	require.Equal(t, res.Applied[0].NodeID, forward.Operations[3].Blocks[0].ID)
	blocks := identityPage(t, srv, "epic", ep.ID).Relationships.Amendments
	require.Len(t, blocks, 1)
	questionEffects := 0
	for _, effect := range blocks[0].Effects {
		if effect.Role == "question_block" {
			questionEffects++
			require.NotEmpty(t, effect.Note)
		}
	}
	require.Equal(t, 3, questionEffects)

	// Model the documented interrupted boundary: application events/nodes landed,
	// but the amendment status write and approval event have not landed.
	tree = identityTree(t, root)
	doc := tree.Amendments[a.ID]
	doc.Status = store.AmendmentStatusProposed
	data, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(amendmentDocPath(t, root, a.ID), data, 0o644))
	evs, err := events.List(tree)
	require.NoError(t, err)
	for _, ev := range evs {
		if ev.Type != "amendment_approved" {
			continue
		}
		path, _, err := events.File(tree, ev)
		require.NoError(t, err)
		require.NoError(t, os.Remove(path))
	}
	partial := identityPage(t, srv, "feature", res.Applied[0].NodeID).Relationships.Amendments
	require.Len(t, partial, 1)
	require.Equal(t, "proposed", partial[0].Amendment.Status)
	require.Empty(t, partial[0].ApprovedAt)
	require.Equal(t, res.Applied[0].NodeID, identityPage(t, srv, "amendment", a.ID).Amendment.Operations[0].Created.ID)
}

func TestAmendmentHistoryReadFailureKeepsKnownDirectLinks(t *testing.T) {
	root, amendmentID, featureID := amendmentFixture(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	path := filepath.Join(root, ".tiller", "events", "broken.yml")
	require.NoError(t, os.WriteFile(path, []byte("after: [unterminated"), 0o644))
	page := identityPage(t, srv, "feature", featureID)
	require.Equal(t, featureID, page.Node.ID)
	require.NotEmpty(t, page.Relationships.AmendmentHistoryWarning)
	require.Len(t, page.Relationships.Amendments, 1)
	require.Equal(t, amendmentID, page.Relationships.Amendments[0].Amendment.ID)
	require.Equal(t, "target", page.Relationships.Amendments[0].Effects[0].Role)
	require.Empty(t, page.History)
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".tiller", "events")))
	page = identityPage(t, srv, "feature", featureID)
	require.NotEmpty(t, page.Relationships.AmendmentHistoryWarning, "a missing history directory is not proof of zero relationships")
	require.Len(t, page.Relationships.Amendments, 1)
}

func TestLegacyAmendmentParentUsesBoundedIdentityEvidence(t *testing.T) {
	root, deps := identityRepo(t)
	epic, err := mutate.AddEpic(identityTree(t, root), deps, mutate.EpicInput{Key: "original", Title: "Original"})
	require.NoError(t, err)
	a, err := mutate.AddAmendment(identityTree(t, root), deps, mutate.AmendmentInput{Key: "proposal", Title: "Proposal"})
	require.NoError(t, err)
	proposeAmendment(t, root, deps, a.ID)
	legacyStage(t, root, deps, a.ID, mutate.StageInput{Op: "add-feature", Key: "original/feature", Title: "Feature", ChangeType: "create"})
	tree := identityTree(t, root)
	_, err = mutate.WithdrawAmendment(tree, deps, tree.Amendments[a.ID], "Deferred")
	require.NoError(t, err)
	_, err = mutate.RenameKey(identityTree(t, root), deps, mutate.RenameKeyInput{Noun: "epic", Node: epic.ID, Key: "renamed"})
	require.NoError(t, err)
	reused, err := mutate.AddEpic(identityTree(t, root), deps, mutate.EpicInput{Key: "original", Title: "Unrelated"})
	require.NoError(t, err)
	tree = identityTree(t, root)
	evs, err := events.List(tree)
	require.NoError(t, err)
	var stagedAt time.Time
	// Legacy staging records carry only their original index/op metadata.
	for i := range evs {
		if evs[i].Type != "amendment_staged" {
			continue
		}
		stagedAt = evs[i].At
		evs[i].After = map[string]any{"op": "add-feature", "index": 0}
	}
	forward := views.NodePageOf(tree, evs, tree.Amendments[a.ID], nil)
	require.Equal(t, epic.ID, forward.Amendment.Operations[0].Parent.ID)
	require.Empty(t, views.NodePageOf(tree, evs, tree.Epics[reused.ID], nil).Relationships.Amendments)
	require.Len(t, views.NodePageOf(tree, evs, tree.Epics[epic.ID], nil).Relationships.Amendments, 1)
	t.Run("missing origin", func(t *testing.T) {
		var partial []events.Event
		for _, ev := range evs {
			if ev.Type == "add.epic" && ev.Targets[0] == epic.ID {
				continue
			}
			partial = append(partial, ev)
		}
		page := views.NodePageOf(tree, partial, tree.Amendments[a.ID], nil)
		require.Empty(t, page.Amendment.Operations[0].Parent.ID)
		require.NotEmpty(t, page.Amendment.Operations[0].IdentityNotes)
		require.NotEmpty(t, page.Relationships.AmendmentHistoryWarning)
		require.Empty(t, views.NodePageOf(tree, partial, tree.Epics[reused.ID], nil).Relationships.Amendments)
	})
	t.Run("same instant ownership", func(t *testing.T) {
		ambiguous := append([]events.Event(nil), evs...)
		for i := range ambiguous {
			if ambiguous[i].Type == "rename.key.epic" {
				ambiguous[i].At = stagedAt
			}
		}
		page := views.NodePageOf(tree, ambiguous, tree.Amendments[a.ID], nil)
		require.Empty(t, page.Amendment.Operations[0].Parent.ID, "EV ordering cannot determine whether the rename preceded staging")
		require.NotEmpty(t, page.Relationships.AmendmentHistoryWarning)
	})
	t.Run("raw key edit", func(t *testing.T) {
		fresh := identityTree(t, root)
		key := "unrecorded"
		fresh.Epics[epic.ID].Key = &key
		page := views.NodePageOf(fresh, evs, fresh.Amendments[a.ID], nil)
		require.Empty(t, page.Amendment.Operations[0].Parent.ID)
		require.NotEmpty(t, page.Relationships.AmendmentHistoryWarning)
	})
	_, err = mutate.RemoveDraft(tree, deps, mutate.RemoveInput{Noun: "epic", Node: epic.ID, Reason: "Not needed"})
	require.NoError(t, err)
	tree = identityTree(t, root)
	removedEvents, err := events.List(tree)
	require.NoError(t, err)
	for i := range removedEvents {
		if removedEvents[i].Type == "amendment_staged" {
			removedEvents[i].After = map[string]any{"op": "add-feature", "index": 0}
		}
	}
	removed := views.NodePageOf(tree, removedEvents, tree.Amendments[a.ID], nil)
	require.Equal(t, epic.ID, removed.Amendment.Operations[0].Parent.ID, "remove metadata proves the old identity without requiring a live node")
	require.NotEmpty(t, removed.Amendment.Operations[0].IdentityNotes)
	require.Empty(t, views.NodePageOf(tree, removedEvents, tree.Epics[reused.ID], nil).Relationships.Amendments)
}

func TestLegacyReplacementEventSurvivesMissingAddAndAmbiguousOperationJoin(t *testing.T) {
	root, deps := identityRepo(t)
	_, err := mutate.AddEpic(identityTree(t, root), deps, mutate.EpicInput{Key: "area", Title: "Area"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(identityTree(t, root), deps, mutate.FeatureInput{Key: "area/feature", Title: "Feature", ChangeType: "create"})
	require.NoError(t, err)
	old, err := mutate.AddTask(identityTree(t, root), deps, mutate.TaskInput{Key: "area/feature/old", Title: "Old"})
	require.NoError(t, err)
	approveFeatureOnDisk(t, root, "area/feature")
	a, err := mutate.AddAmendment(identityTree(t, root), deps, mutate.AmendmentInput{Key: "replace", Title: "Replace"})
	require.NoError(t, err)
	proposeAmendment(t, root, deps, a.ID)
	identityStage(t, root, deps, a.ID,
		mutate.StageInput{Op: "add-task", Key: "area/feature/new", Title: "New"},
		mutate.StageInput{Op: "cancel", Target: old.ID, SupersededBy: "area/feature/new", Reason: "Replaced"},
	)
	tree := identityTree(t, root)
	applied, err := mutate.ApproveAmendment(tree, deps, tree.Amendments[a.ID])
	require.NoError(t, err)
	newID := applied.Applied[0].NodeID
	returnTask(t, root, deps, newID)
	_, err = mutate.RenameKey(identityTree(t, root), deps, mutate.RenameKeyInput{Noun: "task", Node: newID, Key: "area/feature/renamed"})
	require.NoError(t, err)
	reused, err := mutate.AddTask(identityTree(t, root), deps, mutate.TaskInput{Key: "area/feature/new", Title: "Unrelated"})
	require.NoError(t, err)
	tree = identityTree(t, root)
	// The record an amendment staged before schema 15 keeps: its add carries
	// no node, its cancel names the replacement by the staged key, and its
	// approval wrote no approve.task promotion.
	legacy := tree.Amendments[a.ID]
	legacy.Operations[0].Node = ""
	legacy.Operations[1].SupersededBy = "area/feature/new"
	evs, err := events.List(tree)
	require.NoError(t, err)
	var partial []events.Event
	for _, ev := range evs {
		if (ev.Type == "add.task" || ev.Type == "approve.task") && ev.Targets[0] == newID {
			continue
		}
		delete(ev.After, "operation_index")
		partial = append(partial, ev)
	}
	page := views.NodePageOf(tree, partial, tree.Amendments[a.ID], nil)
	require.Nil(t, page.Amendment.Operations[0].Created)
	require.Equal(t, newID, page.Amendment.Operations[1].SupersededBy.ID)
	require.NotEmpty(t, page.Relationships.AmendmentHistoryWarning)
	require.Empty(t, views.NodePageOf(tree, partial, tree.Tasks[reused.ID], nil).Relationships.Amendments)
	known := views.NodePageOf(tree, partial, tree.Tasks[newID], nil).Relationships.Amendments
	require.Len(t, known, 1)
	require.Equal(t, "replacement", known[0].Effects[0].Role)
	require.Equal(t, 2, *known[0].Effects[0].OperationIndex)

	// Without a recorded index, duplicate retained operations are not an exact
	// event join. The original event still contributes its proven replacement.
	tree.Amendments[a.ID].Operations = append(tree.Amendments[a.ID].Operations, tree.Amendments[a.ID].Operations[1])
	unknownJoin := views.NodePageOf(tree, partial, tree.Tasks[newID], nil).Relationships.Amendments
	require.Len(t, unknownJoin, 1)
	require.Len(t, unknownJoin[0].Effects, 1)
	require.Equal(t, "replacement", unknownJoin[0].Effects[0].Role)
	require.Nil(t, unknownJoin[0].Effects[0].OperationIndex)
}

// proposeAmendment hands a draft amendment over, as the agent does, so the
// tests exercise proposed amendments.
func proposeAmendment(t *testing.T, root string, deps mutate.Deps, amendmentID string) {
	t.Helper()
	tree := identityTree(t, root)
	a := tree.Amendments[amendmentID]
	require.NotNil(t, a)
	_, err := mutate.ProposeAmendment(tree, deps, a)
	require.NoError(t, err)
}

// heldDraftAmendment lays an approved feature and a proposed amendment that
// stages two held tasks under it, returning the amendment id and the held ids.
func heldDraftAmendment(t *testing.T) (root string, deps mutate.Deps, amendmentID, keptID, droppedID string) {
	t.Helper()
	root, deps = identityRepo(t)
	_, err := mutate.AddEpic(identityTree(t, root), deps, mutate.EpicInput{Key: "engine", Title: "Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(identityTree(t, root), deps, mutate.FeatureInput{Key: "engine/parse", Title: "Parse", ChangeType: "create"})
	require.NoError(t, err)
	approveFeatureOnDisk(t, root, "engine/parse")
	a, err := mutate.AddAmendment(identityTree(t, root), deps, mutate.AmendmentInput{Key: "reshape", Title: "Reshape parsing"})
	require.NoError(t, err)
	identityStage(t, root, deps, a.ID,
		mutate.StageInput{Op: "add-task", Key: "engine/parse/kept", Title: "Kept"},
		mutate.StageInput{Op: "add-task", Key: "engine/parse/dropped", Title: "Dropped"},
	)
	ops := identityTree(t, root).Amendments[a.ID].Operations
	return root, deps, a.ID, ops[0].Node, ops[1].Node
}

// TestAmendmentPageLinksHeldDraftsBeforeApproval: an open amendment's page
// links each held draft from the operation's own node, never reads the add as
// applied, and carries a removed operation's reason.
func TestAmendmentPageLinksHeldDraftsBeforeApproval(t *testing.T) {
	root, deps, amendmentID, keptID, droppedID := heldDraftAmendment(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	// Still a draft: the page already links both held drafts.
	page := identityPage(t, srv, "amendment", amendmentID).Amendment
	require.Equal(t, keptID, page.Operations[0].Created.ID)
	require.Equal(t, "engine/parse/kept", page.Operations[0].Created.Key)
	require.Equal(t, "task", page.Operations[0].Created.Noun, "the link is live, openable")
	require.Equal(t, droppedID, page.Operations[1].Created.ID)
	require.Empty(t, page.Operations[0].Removed)
	require.Empty(t, page.Operations[0].IdentityNotes)

	// The held draft's own page lists the amendment, its effect proposed.
	rels := identityPage(t, srv, "task", keptID).Relationships.Amendments
	require.Len(t, rels, 1)
	require.Equal(t, amendmentID, rels[0].Amendment.ID)
	hold := identityPage(t, srv, "task", keptID).Node.AmendmentHold
	require.NotNil(t, hold, "the held draft's own page names its amendment")
	require.Equal(t, amendmentID, hold.ID)

	// Hand removal marks the operation removed, with the reason, and the
	// operation keeps its place and the draft's historical id.
	proposeAmendment(t, root, deps, amendmentID)
	_, err := mutate.RemoveDraft(identityTree(t, root), deps, mutate.RemoveInput{Noun: "task", Node: droppedID, Reason: "Not this quarter"})
	require.NoError(t, err)
	page = identityPage(t, srv, "amendment", amendmentID).Amendment
	require.Len(t, page.Operations, 2)
	require.Equal(t, "Not this quarter", page.Operations[1].Removed)
	require.Equal(t, droppedID, page.Operations[1].Created.ID)
	require.Empty(t, page.Operations[1].Created.Noun, "the removed draft is text, not a link")
	require.Empty(t, page.Operations[0].Removed)
}

func historyTypes(page views.NodePage) []string {
	var out []string
	for _, h := range page.History {
		out = append(out, h.Type)
	}
	return out
}

// TestAmendmentHistoryShowsHandRemovalAndWithdraw: a hand removal of a held
// draft targets the amendment whose operation it marked, so it shows on the
// amendment page's history; a withdraw's removals are the amendment_withdrawn
// event's, on the same page, with every operation still carrying the removed
// draft's id.
func TestAmendmentHistoryShowsHandRemovalAndWithdraw(t *testing.T) {
	root, deps, amendmentID, keptID, droppedID := heldDraftAmendment(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	require.NotContains(t, historyTypes(identityPage(t, srv, "amendment", amendmentID)), "remove.proposed.task")

	_, err := mutate.RemoveDraft(identityTree(t, root), deps, mutate.RemoveInput{Noun: "task", Node: droppedID, Reason: "Not this quarter"})
	require.NoError(t, err)
	require.Contains(t, historyTypes(identityPage(t, srv, "amendment", amendmentID)), "remove.proposed.task",
		"the hand removal shows beside the operation it marked")

	tree := identityTree(t, root)
	res, err := mutate.WithdrawAmendment(tree, deps, tree.Amendments[amendmentID], "Superseded by a rewrite")
	require.NoError(t, err)
	require.Equal(t, []string{keptID}, res.RemovedIDs)
	page := identityPage(t, srv, "amendment", amendmentID)
	require.Contains(t, historyTypes(page), "amendment_withdrawn")
	require.Equal(t, keptID, page.Amendment.Operations[0].Created.ID, "the withdrawn amendment still names what it held")
	require.Equal(t, "Not this quarter", page.Amendment.Operations[1].Removed)
}
