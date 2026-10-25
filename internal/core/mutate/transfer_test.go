package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func transferWrite(t *testing.T, root, rel string, value any) {
	t.Helper()
	data, err := yaml.Marshal(value)
	require.NoError(t, err)
	transferBytes(t, root, rel, data)
}

func transferBytes(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

const transferFeatureDir = ".tiller/epics/demo/features/work"
const transferTaskDir = transferFeatureDir + "/tasks/alpha"

func transferFixture(t *testing.T, noun string) (string, mutate.Deps, string, string) {
	t.Helper()
	root := t.TempDir()
	transferBytes(t, root, ".tiller/tiller.yml", []byte(fmt.Sprintf("schema_version: %d\nproject: {key: test, name: Test}\ncontributors:\n  - {id: jc, name: JC}\n  - {id: sam, name: Sam}\n  - {id: reviewer, name: Reviewer}\ngit: {integration_branch: main, branch_pattern: 'tiller/{key}'}\n", store.CurrentSchemaVersion)))
	epicKey, featureKey, taskKey := "demo", "demo/work", "demo/work/alpha"
	epic := &store.Epic{ID: "E-aaaaaa", Key: &epicKey, Status: "ready"}
	feature := &store.Feature{ID: "F-aaaaaa", Key: &featureKey, Status: "active", ChangeType: "create"}
	transferWrite(t, root, ".tiller/epics/demo/epic.yml", epic)
	transferBytes(t, root, ".tiller/epics/demo/epic.md", []byte("# Demo\n"))
	transferWrite(t, root, transferFeatureDir+"/feature.yml", feature)
	transferBytes(t, root, transferFeatureDir+"/feature.md", []byte("# Approved scope\n\nPreserve every input.\n"))
	transferBytes(t, root, transferFeatureDir+"/feature.eval.md", []byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: Inputs survive\n    polarity: introduce\n    binding: {type: manual}\n    required: true\n---\n"))
	prefix, err := id.PrefixFor(noun)
	require.NoError(t, err)
	nid, key, dir := prefix+"-aaaaaa", taskKey, transferTaskDir
	if noun != store.NounTask {
		key, dir = store.ShelfCanonicalKey(noun, "alpha"), store.ShelfRelDir(noun, "alpha")
	}
	transferBytes(t, root, filepath.Join(dir, noun+".md"), []byte("# Original approved scope\n\nDo exactly this work; retain T-aaaaaa as an authored historical citation.\n"))
	gitInitM(t, root)
	gitCommitAllM(t, root, "initial contract")
	gitRunM(t, root, "branch", "-M", "main")
	gitRunM(t, root, "checkout", "-b", "tiller/"+key)
	commitFile(t, root, "implementation.txt", "original implementation\n")
	gitRunM(t, root, "checkout", "main")
	rev := vcs.New(root)
	pin := func(rel string) store.RevisionRef {
		ref, err := mutate.WorktreeBlobRef(rev, root, rel)
		require.NoError(t, err)
		return ref
	}
	commit, err := rev.CurrentCommit()
	require.NoError(t, err)
	feature.AcceptedAgainst = store.BasisRefs{Spec: pin(transferFeatureDir + "/feature.md"), Eval: pin(transferFeatureDir + "/feature.eval.md"), Commit: commit}
	feature.Gates = map[string]store.Gate{"evals": {Status: "pass", Criteria: map[string]store.Criterion{"C1": {Status: "pass"}}}}
	transferWrite(t, root, transferFeatureDir+"/feature.yml", feature)
	basis := &store.BasisRefs{Spec: pin(filepath.Join(dir, noun+".md")), Commit: commit}
	pr := 41
	gates := store.GatesBlock{Gates: map[string]store.Gate{"tests": {Status: "pass", Evidence: []store.Evidence{{Kind: "test", Ref: "historical"}}}}, Attestations: []store.Attestation{{ID: "AT-aaaaaa", By: "jc", Status: "pass"}}}
	if noun == store.NounTask {
		transferWrite(t, root, dir+"/task.yml", &store.Task{ID: nid, Key: &key, Title: "Original", Status: "in_progress", Started: true, Owner: "jc", ProposedBy: "jc", Branch: "tiller/" + key, PR: &pr, AcceptedAgainst: basis, Evidence: []store.Evidence{{Kind: "diff", Ref: "original"}}, GatesBlock: gates})
	} else {
		w, err := store.NewWorkNode(noun)
		require.NoError(t, err)
		*w.Body() = store.WorkItem{ID: nid, Key: &key, Title: "Original", Status: "in_progress", Started: true, Owner: "jc", ProposedBy: "jc", AttachedTo: feature.ID, Source: "feedback/original", Branch: "tiller/" + key, PR: &pr, AcceptedAgainst: basis, Evidence: []store.Evidence{{Kind: "diff", Ref: "original"}}, GatesBlock: gates}
		transferWrite(t, root, filepath.Join(dir, noun+".yml"), w)
	}
	return root, mutate.Deps{Contributor: "reviewer", Mint: id.Mint, Rev: rev}, nid, dir
}

func TestTransferPreservesHistoryAndCreatesFreshApprovedWork(t *testing.T) {
	t.Parallel()
	for _, noun := range []string{store.NounTask, store.NounBug, store.NounChore, store.NounTweak} {
		t.Run(noun, func(t *testing.T) {
			root, deps, nid, dir := transferFixture(t, noun)
			prose, err := os.ReadFile(filepath.Join(root, dir, noun+".md"))
			require.NoError(t, err)
			res, err := mutate.Transfer(load(t, root), deps, mutate.TransferInput{Noun: noun, Node: nid, To: "sam", Reason: "Human directed handoff"})
			require.NoError(t, err)
			fresh := load(t, root)
			var originalOwner, proposer string
			var successor *string
			var replacementStarted bool
			var basis *store.BasisRefs
			var supersedes []string
			var replacementBranch string
			var replacementPR *int
			var replacementEvidence []store.Evidence
			var replacementGates store.GatesBlock
			if noun == store.NounTask {
				old, next := fresh.Tasks[nid], fresh.Tasks[res.Replacement.ID]
				require.Equal(t, "cancelled", old.Status)
				require.True(t, old.Started)
				require.Equal(t, "ready", next.Status)
				require.Equal(t, "sam", next.Owner)
				require.Equal(t, 41, *old.PR)
				require.NotEmpty(t, old.Evidence)
				originalOwner, proposer, successor = old.Owner, next.ProposedBy, old.SupersededBy
				replacementStarted, basis, supersedes, replacementBranch, replacementPR, replacementEvidence, replacementGates = next.Started, next.AcceptedAgainst, next.Supersedes, next.Branch, next.PR, next.Evidence, next.GatesBlock
			} else {
				oldNode, _ := fresh.WorkNode(nid)
				nextNode, _ := fresh.WorkNode(res.Replacement.ID)
				old, next := oldNode.Body(), nextNode.Body()
				require.Equal(t, "cancelled", old.Status)
				require.Equal(t, "transferred", old.Resolution)
				require.Empty(t, old.DuplicateOf)
				require.True(t, old.Started)
				require.Equal(t, "ready", next.Status)
				require.Equal(t, "sam", next.Owner)
				require.Equal(t, old.AttachedTo, next.AttachedTo)
				require.Equal(t, old.Source, next.Source)
				require.Equal(t, 41, *old.PR)
				originalOwner, proposer, successor = old.Owner, next.ProposedBy, old.SupersededBy
				replacementStarted, basis, supersedes, replacementBranch, replacementPR, replacementEvidence, replacementGates = next.Started, next.AcceptedAgainst, next.Supersedes, next.Branch, next.PR, next.Evidence, next.GatesBlock
			}
			require.Equal(t, "jc", originalOwner)
			require.Equal(t, "jc", proposer)
			require.Equal(t, res.Replacement.ID, *successor)
			require.Equal(t, []string{nid}, supersedes)
			require.False(t, replacementStarted)
			require.Empty(t, replacementBranch)
			require.Nil(t, replacementPR)
			require.Empty(t, replacementEvidence)
			require.Empty(t, replacementGates.Gates)
			require.Empty(t, replacementGates.Attestations)
			require.NotEqual(t, filepath.Join(dir, noun+".md"), basis.Spec.Path)
			copied, err := os.ReadFile(filepath.Join(root, basis.Spec.Path))
			require.NoError(t, err)
			require.Equal(t, prose, copied)
			evs, err := events.List(fresh)
			require.NoError(t, err)
			require.Len(t, evs, 1)
			require.Equal(t, "reviewer", evs[0].Contributor)
			require.Equal(t, []string{nid, res.Replacement.ID}, evs[0].Targets)
			before := snapshotDir(t, root)
			_, err = mutate.Transfer(fresh, deps, mutate.TransferInput{Noun: noun, Node: nid, To: "sam", Reason: "repeat"})
			var blocked *mutate.TransferBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, res.Replacement.ID, blocked.Successor)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestTransferPreservesTaskConstraintsAndRedirectsOnlyLiveRelationships(t *testing.T) {
	t.Parallel()
	root, deps, nid, _ := transferFixture(t, store.NounTask)
	tree := load(t, root)
	original := tree.Tasks[nid]
	original.Blocked = true
	original.DependsOn = []string{"T-cccccc"}
	transferWrite(t, root, transferTaskDir+"/task.yml", original)
	for _, node := range []struct {
		key, id, status string
		depends         []string
	}{
		{"prerequisite", "T-cccccc", "done", nil},
		{"dependent", "T-dddddd", "ready", []string{nid, "demo/work/alpha"}},
		{"historical", "T-eeeeee", "cancelled", []string{nid}},
	} {
		key := "demo/work/" + node.key
		tk := &store.Task{ID: node.id, Key: &key, Status: node.status, DependsOn: node.depends, Owner: "sam"}
		if node.status == "done" {
			tk.Started = true
		}
		transferWrite(t, root, transferFeatureDir+"/tasks/"+node.key+"/task.yml", tk)
	}
	feature := tree.Features["F-aaaaaa"]
	feature.Order = []string{"T-cccccc", nid, "T-dddddd", "T-eeeeee"}
	feature.ReviewGrain, feature.Branch = store.ReviewGrainFeature, "tiller/reviews/features/demo/work"
	gitRunM(t, root, "branch", feature.Branch, original.Branch)
	pr := 99
	feature.PR = &pr
	transferWrite(t, root, transferFeatureDir+"/feature.yml", feature)
	q := &store.Question{ID: "Q-aaaaaa", Status: "open", Blocks: []string{"demo/work/alpha"}, Owner: "reviewer"}
	transferWrite(t, root, ".tiller/decisions/open-questions.yml", &store.QuestionsFile{Questions: []*store.Question{q}})
	carriedKey := "bugs/carried"
	transferWrite(t, root, ".tiller/bugs/carried/bug.yml", &store.Bug{WorkItem: store.WorkItem{ID: "B-aaaaaa", Key: &carriedKey, Status: "ready", ResolvedBy: nid, AttachedTo: nid}})
	res, err := mutate.Transfer(load(t, root), deps, mutate.TransferInput{Noun: store.NounTask, Node: nid, To: "sam", Reason: "human handoff"})
	require.NoError(t, err)
	fresh := load(t, root)
	next := fresh.Tasks[res.Replacement.ID]
	require.True(t, next.Blocked)
	require.Equal(t, []string{"T-cccccc"}, next.DependsOn)
	require.Equal(t, []string{res.Replacement.ID}, fresh.Tasks["T-dddddd"].DependsOn)
	require.Equal(t, []string{nid}, fresh.Tasks["T-eeeeee"].DependsOn)
	require.Equal(t, []string{"demo/work/alpha", res.Replacement.ID}, fresh.Questions[q.ID].Blocks)
	require.Equal(t, res.Replacement.ID, fresh.Bugs["B-aaaaaa"].ResolvedBy)
	require.Equal(t, nid, fresh.Bugs["B-aaaaaa"].AttachedTo)
	require.Equal(t, "carried", gen.WorkActivity(fresh, fresh.Bugs["B-aaaaaa"]))
	require.Equal(t, []string{"T-cccccc", res.Replacement.ID, "T-dddddd", "T-eeeeee"}, fresh.Features[feature.ID].Order)
	require.Equal(t, 99, *fresh.Features[feature.ID].PR)
	require.Equal(t, feature.Branch, fresh.Features[feature.ID].Branch)
	require.Equal(t, "stale", fresh.Features[feature.ID].Gates["evals"].Criteria["C1"].Status)
	require.Contains(t, strings.Join(res.FollowThrough, "\n"), "tiller submit feature F-aaaaaa")
}

func TestTransferRefusesUnsafeStateWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"same_owner", "unstarted", "terminal", "scope_drift", "parent_drift", "started_dependent", "missing_owner"} {
		t.Run(scenario, func(t *testing.T) {
			root, deps, nid, _ := transferFixture(t, store.NounTask)
			tree := load(t, root)
			task := tree.Tasks[nid]
			to := "sam"
			switch scenario {
			case "same_owner":
				to = "jc"
			case "unstarted":
				task.Status, task.Started, task.Branch, task.PR = "ready", false, "", nil
			case "terminal":
				task.Status = "cancelled"
			case "scope_drift":
				transferBytes(t, root, transferTaskDir+"/task.md", []byte("changed requirements\n"))
			case "parent_drift":
				transferBytes(t, root, transferFeatureDir+"/feature.md", []byte("changed parent\n"))
			case "started_dependent":
				key := "demo/work/dependent"
				transferWrite(t, root, transferFeatureDir+"/tasks/dependent/task.yml", &store.Task{ID: "T-bbbbbb", Key: &key, Status: "in_progress", Started: true, Owner: "sam", Branch: "tiller/" + key, DependsOn: []string{nid}})
			case "missing_owner":
				to = "not-registered"
			}
			transferWrite(t, root, transferTaskDir+"/task.yml", task)
			before := snapshotDir(t, root)
			_, err := mutate.Transfer(load(t, root), deps, mutate.TransferInput{Noun: store.NounTask, Node: nid, To: to, Reason: "human handoff"})
			require.Error(t, err)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}

func TestTransferRacingRelationshipsRefuseBeforeCancellation(t *testing.T) {
	t.Parallel()
	for _, race := range []string{"dependent", "question", "carrier", "pause", "roster", "shelf_dependent"} {
		t.Run(race, func(t *testing.T) {
			noun := store.NounTask
			if race == "shelf_dependent" {
				noun = store.NounBug
			}
			root, deps, nid, dir := transferFixture(t, noun)
			loaded := load(t, root)
			var raced map[string]string
			deps.Mint = func(prefix string) string {
				if prefix != "EV" && raced == nil {
					switch race {
					case "dependent":
						key := "demo/work/new-dependent"
						transferWrite(t, root, transferFeatureDir+"/tasks/new-dependent/task.yml", &store.Task{ID: "T-cccccc", Key: &key, Status: "ready", DependsOn: []string{nid}})
					case "question":
						transferWrite(t, root, ".tiller/decisions/open-questions.yml", &store.QuestionsFile{Questions: []*store.Question{{ID: "Q-aaaaaa", Status: "open", Blocks: []string{nid}}}})
					case "carrier":
						key := "bugs/carried"
						transferWrite(t, root, ".tiller/bugs/carried/bug.yml", &store.Bug{WorkItem: store.WorkItem{ID: "B-cccccc", Key: &key, Status: "ready", ResolvedBy: nid}})
					case "pause":
						task := *loaded.Tasks[nid]
						task.Blocked = true
						transferWrite(t, root, dir+"/task.yml", &task)
					case "roster":
						manifest := *loaded.Manifest
						manifest.Contributors = manifest.Contributors[:1]
						transferWrite(t, root, ".tiller/tiller.yml", &manifest)
					case "shelf_dependent":
						key := "chores/new-dependent"
						transferWrite(t, root, ".tiller/chores/new-dependent/chore.yml", &store.Chore{WorkItem: store.WorkItem{ID: "CH-cccccc", Key: &key, Status: "ready", DependsOn: []string{nid}}})
					}
					raced = snapshotDir(t, root)
				}
				return id.Mint(prefix)
			}
			_, err := mutate.Transfer(loaded, deps, mutate.TransferInput{Noun: noun, Node: nid, To: "sam", Reason: "human handoff"})
			var stale *store.EnvError
			require.ErrorAs(t, err, &stale)
			require.Equal(t, store.ReasonMutationStale, stale.Reason)
			requireNoWrites(t, raced, snapshotDir(t, root))
		})
	}
}

func TestTransferRejectsIntegratedReviewEvenAfterBranchDeletion(t *testing.T) {
	t.Parallel()
	root, deps, nid, dir := transferFixture(t, store.NounBug)
	gitRunM(t, root, "checkout", "tiller/bugs/alpha")
	commitFile(t, root, "implementation.txt", "implemented\n")
	head, err := deps.Rev.CurrentCommit()
	require.NoError(t, err)
	gitRunM(t, root, "checkout", "main")
	gitRunM(t, root, "merge", "--no-ff", "tiller/bugs/alpha", "-m", "human merge")
	gitRunM(t, root, "branch", "-d", "tiller/bugs/alpha")
	bug := load(t, root).Bugs[nid]
	bug.ReviewRef = &store.ReviewRef{Branch: bug.Branch, Head: head}
	bug.PR = nil
	transferWrite(t, root, filepath.Join(dir, "bug.yml"), bug)
	before := snapshotDir(t, root)
	_, err = mutate.Transfer(load(t, root), deps, mutate.TransferInput{Noun: store.NounBug, Node: nid, To: "sam", Reason: "too late"})
	var blocked *mutate.TransferBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "transfer_review_merged", blocked.Code)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestTransferRejectsPRWithUnobservableDeletedBranch(t *testing.T) {
	t.Parallel()
	root, deps, nid, _ := transferFixture(t, store.NounChore)
	gitCommitAllM(t, root, "record active source")
	gitRunM(t, root, "merge", "--no-ff", "tiller/chores/alpha", "-m", "human merge")
	gitRunM(t, root, "branch", "-d", "tiller/chores/alpha")
	before := snapshotDir(t, root)
	_, err := mutate.Transfer(load(t, root), deps, mutate.TransferInput{Noun: store.NounChore, Node: nid, To: "sam", Reason: "too late"})
	var blocked *mutate.TransferBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "transfer_review_unobservable", blocked.Code)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestTransferShelfPreservesCarrierPrerequisiteAndCrossShelfDependents(t *testing.T) {
	t.Parallel()
	root, deps, nid, dir := transferFixture(t, store.NounBug)
	bug := load(t, root).Bugs[nid]
	bug.ResolvedBy, bug.Blocked = "F-aaaaaa", true
	bug.DependsOn = []string{"CH-cccccc"}
	transferWrite(t, root, filepath.Join(dir, "bug.yml"), bug)
	for _, item := range []struct {
		key, id, status string
		depends         []string
	}{
		{"prerequisite", "CH-cccccc", "done", nil},
		{"dependent", "CH-dddddd", "ready", []string{nid, nid}},
		{"historical", "CH-eeeeee", "cancelled", []string{nid}},
	} {
		key := "chores/" + item.key
		transferWrite(t, root, ".tiller/chores/"+item.key+"/chore.yml", &store.Chore{WorkItem: store.WorkItem{ID: item.id, Key: &key, Status: item.status, Started: item.status == "done", Owner: "sam", DependsOn: item.depends}})
	}
	res, err := mutate.Transfer(load(t, root), deps, mutate.TransferInput{Noun: store.NounBug, Node: nid, To: "sam", Reason: "human handoff"})
	require.NoError(t, err)
	tree := load(t, root)
	next := tree.Bugs[res.Replacement.ID]
	require.Equal(t, "F-aaaaaa", next.ResolvedBy)
	require.True(t, next.Blocked)
	require.Equal(t, "carried", gen.WorkActivity(tree, next))
	require.Equal(t, []string{"CH-cccccc"}, next.DependsOn)
	require.Equal(t, []string{res.Replacement.ID}, tree.Chores["CH-dddddd"].DependsOn)
	require.Equal(t, []string{nid}, tree.Chores["CH-eeeeee"].DependsOn)
	require.Equal(t, []string{res.Replacement.ID}, tree.BugOrder.Order)
}

func TestTransferUsesDistinctDeterministicKeyAndPreservesUnlistedPosition(t *testing.T) {
	t.Parallel()
	root, deps, nid, _ := transferFixture(t, store.NounTask)
	for _, item := range []struct{ key, id string }{
		{"alpha-transfer-2", "T-bbbbbb"}, {"alpha-other", "T-cccccc"},
	} {
		key := "demo/work/" + item.key
		transferWrite(t, root, transferFeatureDir+"/tasks/"+item.key+"/task.yml", &store.Task{ID: item.id, Key: &key, Status: "proposed"})
	}
	tree := load(t, root)
	before := gen.OrderTasks(tree, tree.FeatureTasks["F-aaaaaa"], nil)
	res, err := mutate.Transfer(tree, deps, mutate.TransferInput{Noun: store.NounTask, Node: nid, To: "sam", Reason: "human handoff"})
	require.NoError(t, err)
	require.Equal(t, "demo/work/alpha-transfer-3", res.Replacement.Key)
	expected := make([]string, len(before))
	for i, task := range before {
		expected[i] = task.ID
		if task.ID == nid {
			expected[i] = res.Replacement.ID
		}
	}
	require.Equal(t, expected, load(t, root).Features["F-aaaaaa"].Order)
}

func TestTransferEventCollisionCannotPartiallyCancelOrCreate(t *testing.T) {
	t.Parallel()
	root, deps, nid, _ := transferFixture(t, store.NounChore)
	clock := fixedClock{t: time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)}
	deps.Clock = clock
	ev := events.NewEvent(clock, "historical.chore", "jc", "historical event", nid)
	ev.ID = "EV-cccccc"
	path, data, err := events.File(load(t, root), ev)
	require.NoError(t, err)
	rel, err := filepath.Rel(root, path)
	require.NoError(t, err)
	transferBytes(t, root, rel, data)
	deps.Mint = func(prefix string) string {
		if prefix == "EV" {
			return ev.ID
		}
		return id.Mint(prefix)
	}
	before := snapshotDir(t, root)
	_, err = mutate.Transfer(load(t, root), deps, mutate.TransferInput{Noun: store.NounChore, Node: nid, To: "sam", Reason: "human handoff"})
	require.Error(t, err)
	requireNoWrites(t, before, snapshotDir(t, root))
}

func TestTransferRejectsSquashedImmutableReviewAfterBranchChanges(t *testing.T) {
	t.Parallel()
	for _, branchChange := range []string{"deleted", "advanced"} {
		t.Run(branchChange, func(t *testing.T) {
			root, deps, nid, dir := transferFixture(t, store.NounBug)
			gitCommitAllM(t, root, "record active source")
			gitRunM(t, root, "checkout", "tiller/bugs/alpha")
			head, err := deps.Rev.CurrentCommit()
			require.NoError(t, err)
			gitRunM(t, root, "checkout", "main")
			gitRunM(t, root, "merge", "--squash", "tiller/bugs/alpha")
			gitCommitAllM(t, root, "human squash merge")
			if branchChange == "deleted" {
				gitRunM(t, root, "branch", "-D", "tiller/bugs/alpha")
			} else {
				gitRunM(t, root, "checkout", "tiller/bugs/alpha")
				commitFile(t, root, "implementation.txt", "later unreviewed changes\n")
				gitRunM(t, root, "checkout", "main")
			}
			bug := load(t, root).Bugs[nid]
			bug.ReviewRef = &store.ReviewRef{Branch: bug.Branch, Head: head}
			bug.PR = nil
			transferWrite(t, root, filepath.Join(dir, "bug.yml"), bug)
			before := snapshotDir(t, root)
			_, err = mutate.Transfer(load(t, root), deps, mutate.TransferInput{Noun: store.NounBug, Node: nid, To: "sam", Reason: "too late"})
			var blocked *mutate.TransferBlockedError
			require.ErrorAs(t, err, &blocked)
			require.Equal(t, "transfer_review_merged", blocked.Code)
			requireNoWrites(t, before, snapshotDir(t, root))
		})
	}
}
