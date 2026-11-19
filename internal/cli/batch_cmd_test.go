package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// Serial: every test here chdirs into its own scratch repository.

// sevenBugKeys are the Headway report's shape: seven proposed bugs the
// developer wants approved and fixed under one pull request.
var sevenBugKeys = []string{"one", "two", "three", "four", "five", "six", "seven"}

// laySevenBugs lays seven proposed bugs with written contracts on main.
func laySevenBugs(t *testing.T) string {
	t.Helper()
	root := initGatedGitRepo(t, "true")
	gitRun(t, root, "branch", "-M", "main")
	declareIntegrationBranchCLI(t, root)
	for _, key := range sevenBugKeys {
		mustRun(t, "add", "bug", key, "--title", "Bug "+key, "--severity", "minor", "--contributor", "jc")
		contract := "# Bug " + key + "\n\n## Reproduction\n\nx\n\n## Expected\n\ny\n\n## Actual\n\nz\n\n## Fixed when\n\nfixed\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "bugs", key, "bug.md"), []byte(contract), 0o644))
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "capture seven bugs")
	return root
}

func bugRefs(keys []string) []string {
	refs := make([]string, len(keys))
	for i, key := range keys {
		refs[i] = "bugs/" + key
	}
	return refs
}

// batchRefusal runs a command expected to be a blocked batch answer and
// returns its data block.
func batchRefusal(t *testing.T, args ...string) BatchRefusalData {
	t.Helper()
	out, stderr, code := execCLI(t, append([]string{"--json"}, args...)...)
	require.Equalf(t, 1, code, "%v: %s%s", args, out, stderr)
	var envelope struct {
		OK   bool             `json:"ok"`
		Data BatchRefusalData `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope), out)
	require.False(t, envelope.OK)
	return envelope.Data
}

// The instruction the Headway agent could not follow lawfully: "There are 7
// proposed bugs. Approve them all. Fix them all under one PR." Driven through
// the real commands: one batch, one branch, one check, one submission, one
// merge, and one sync that crosses the batch and all seven bugs.
func TestSevenBugsUnderOneReviewCLI(t *testing.T) {
	root := laySevenBugs(t)

	// The batch may be created ahead of triage, but it will not start there.
	out, stderr, code := execCLI(t, append([]string{"add", "batch", "core-model", "--title", "Core model fixes", "--contributor", "jc"}, bugRefs(sevenBugKeys)...)...)
	require.Equal(t, 0, code, out+stderr)
	require.Contains(t, out, `Created batch batches/core-model (`)
	require.Contains(t, out, "reviewing 7 members")
	require.Contains(t, out, "start batch waits on approval of")
	mustRun(t, "claim", "batch", "core-model", "--contributor", "jc")
	unready := batchRefusal(t, "start", "batch", "core-model", "--contributor", "jc")
	require.Equal(t, "batch_member_unready", unready.Reason)
	require.NotEmpty(t, unready.Member, "the refusal names the member")

	mustRun(t, append([]string{"approve", "bug", "--contributor", "jc"}, sevenBugKeys...)...)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "approve seven bugs and batch them")

	// A member is not an execute target on its own; each refusal names the batch.
	for verb, args := range map[string][]string{
		"claim":  {"claim", "bug", "one", "--contributor", "jc"},
		"start":  {"start", "bug", "one", "--contributor", "jc"},
		"check":  {"check", "bug", "one", "--contributor", "jc"},
		"submit": {"submit", "bug", "one", "--contributor", "jc"},
		"resume": {"resume", "bug", "one", "--reason", "x", "--contributor", "jc"},
	} {
		refusal := batchRefusal(t, args...)
		require.Equalf(t, "work_batched", refusal.Reason, "%s on a member", verb)
		require.Equal(t, "batches/core-model", refusal.BatchKey)
		require.Equal(t, fmt.Sprintf("tiller %s batch core-model", verb), refusal.Command)
	}

	out, _, code = execCLI(t, "start", "batch", "core-model", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Work happens on branch tiller/batches/core-model")
	gitRun(t, root, "checkout", "-b", "tiller/batches/core-model")
	tree := loadTestTree(t)
	var batchID string
	for id := range tree.Batches {
		batchID = id
	}
	for _, bug := range tree.Bugs {
		require.NoError(t, os.WriteFile(filepath.Join(root, bug.ID+".txt"), []byte("fix\n"), 0o644))
		gitRun(t, root, "add", bug.ID+".txt")
		gitRun(t, root, "commit", "-m", bug.ID+" fix and regression test")
	}

	// The queue, the ranked list, and the readout hold the batch as one unit.
	out, _, code = execCLI(t, "next", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "continue batches/core-model ("+batchID+")")
	require.NotContains(t, out, "bugs/one")
	out, _, code = execCLI(t, "readout", "execute")
	require.Equal(t, 0, code)
	require.Contains(t, out, "batches/core-model")
	require.NotContains(t, out, "bugs/one")

	mustRun(t, "check", "batch", "core-model", "--contributor", "jc")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", batchID+" record gate evidence")
	mustRun(t, "submit", "batch", "core-model", "--contributor", "jc")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", batchID+" in review")

	// Both node reads name the batch on a member and the members on the batch.
	out, _, code = execCLI(t, "status", "bug", "one")
	require.Equal(t, 0, code)
	require.Contains(t, out, "state: ready (batched:")
	require.Contains(t, out, "batch: batches/core-model ("+batchID+") is in_review, branch tiller/batches/core-model, review ref git:")
	out, _, code = execCLI(t, "show", "bug", "one")
	require.Equal(t, 0, code)
	require.Contains(t, out, "batch: batches/core-model ("+batchID+") is in_review, branch tiller/batches/core-model")
	for _, read := range []string{"status", "show"} {
		out, _, code = execCLI(t, read, "batch", "core-model")
		require.Equal(t, 0, code)
		require.Contains(t, out, "members: 7")
		for _, key := range sevenBugKeys {
			require.Contains(t, out, "bug bugs/"+key+" (")
		}
	}
	out, _, code = execCLI(t, "ls", "bugs")
	require.Equal(t, 0, code)
	require.Contains(t, out, "(batched: "+batchID+")")

	// Membership is closed while the reviewer reads it.
	closed := batchRefusal(t, "batch", "core-model", "--remove", "bugs/seven", "--contributor", "jc")
	require.Equal(t, "batch_in_review", closed.Reason)

	// The human merges; one sync on the integration branch crosses everything.
	gitRun(t, root, "checkout", "main")
	gitRun(t, root, "merge", "--no-ff", "-m", "Merge the core model fixes", "tiller/batches/core-model")
	out, stderr, code = execCLI(t, "--json", "sync", "--contributor", "jc")
	require.Equal(t, 0, code, out+stderr)
	var envelope struct {
		Data SyncView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.Len(t, envelope.Data.Crossings, 8)
	require.Equal(t, "batch", envelope.Data.Crossings[0].Noun)
	require.Equal(t, "ancestry", envelope.Data.Crossings[0].Basis)
	for _, crossing := range envelope.Data.Crossings[1:] {
		require.Equal(t, "bug", crossing.Noun)
		require.Equal(t, "batched", crossing.Basis)
		require.Equal(t, batchID, crossing.Batch)
	}
	require.Empty(t, envelope.Data.Awaiting)
	require.Empty(t, envelope.Data.UnrecordedMerges)

	tree = loadTestTree(t)
	require.Equal(t, "done", tree.Batches[batchID].Status)
	for _, bug := range tree.Bugs {
		require.Equal(t, "done", bug.Status)
		require.Equal(t, "batched", bug.DoneBasis)
	}
	out, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "no merged branches to record", "a second sync finds nothing left")
}

func TestSyncNamesTheBatchOnEachMemberCrossing(t *testing.T) {
	root := laySevenBugs(t)
	mustRun(t, append([]string{"approve", "bug", "--contributor", "jc"}, sevenBugKeys...)...)
	mustRun(t, "add", "batch", "pair", "--title", "Pair", "bugs/one", "bugs/two", "--contributor", "jc")
	mustRun(t, "claim", "batch", "pair", "--contributor", "jc")
	mustRun(t, "start", "batch", "pair", "--contributor", "jc")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "start the pair")
	gitRun(t, root, "checkout", "-b", "tiller/batches/pair")
	mustRun(t, "check", "batch", "pair", "--contributor", "jc")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "pair gate evidence")
	mustRun(t, "submit", "batch", "pair", "--contributor", "jc")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "pair in review")

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "batch batches/pair")
	require.NotContains(t, out, "bugs/one", "a member awaits no merge of its own")

	gitRun(t, root, "checkout", "main")
	gitRun(t, root, "merge", "--no-ff", "-m", "merge the pair", "tiller/batches/pair")
	out, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Sync: recorded 3 done crossing(s):")
	require.Contains(t, out, "- batch batches/pair (")
	require.Contains(t, out, "-> done (batched: batches/pair (")
}

func TestBatchEditAndCancelCLI(t *testing.T) {
	laySevenBugs(t)
	mustRun(t, append([]string{"approve", "bug", "--contributor", "jc"}, sevenBugKeys...)...)

	_, stderr, code := execCLI(t, "add", "batch", "solo", "--title", "Solo", "bugs/one", "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stderr, "two or more")

	mustRun(t, "add", "batch", "pair", "--title", "Pair", "bugs/one", "bugs/two", "--contributor", "jc")
	taken := batchRefusal(t, "add", "batch", "other", "--title", "Other", "bugs/two", "bugs/three", "--contributor", "jc")
	require.Equal(t, "batch_member_in_batch", taken.Reason)

	out, _, code := execCLI(t, "batch", "pair", "--add", "bugs/three", "--remove", "bugs/two", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Added to batch batches/pair")
	require.Contains(t, out, "Removed from batch batches/pair")
	require.Contains(t, out, "are yours to revert")

	out, _, code = execCLI(t, "queue", "--noun", "batch")
	require.Equal(t, 0, code, "batch is a queue noun")
	_ = out

	// Cancelling a member takes it out; cancelling the batch releases the rest.
	mustRun(t, "cancel", "bug", "three", "--reason", "not a bug", "--resolution", "invalid", "--contributor", "jc")
	require.Empty(t, bugByKey(t, "three").BatchID)
	_, _, code = execCLI(t, "cancel", "batch", "pair", "--reason", "apart after all", "--resolution", "wontfix", "--contributor", "jc")
	require.Equal(t, 2, code, "a batch takes no resolution")
	out, _, code = execCLI(t, "cancel", "batch", "pair", "--reason", "apart after all", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Equal(t, "ready", bugByKey(t, "one").Status)
	require.Empty(t, bugByKey(t, "one").BatchID)
	out, _, code = execCLI(t, "ls", "batches", "--status", "cancelled")
	require.Equal(t, 0, code)
	require.Contains(t, out, "batches/pair")

	// A released member starts on its own again.
	mustRun(t, "claim", "bug", "one", "--contributor", "jc")
	mustRun(t, "start", "bug", "one", "--contributor", "jc")
}

func TestSubmitBatchRecoveryRefusesCLI(t *testing.T) {
	laySevenBugs(t)
	mustRun(t, "add", "batch", "pair", "--title", "Pair", "bugs/one", "bugs/two", "--contributor", "jc")
	_, stderr, code := execCLI(t, "submit", "batch", "pair", "--recover-merged",
		"--head", "1111111111111111111111111111111111111111", "--merge-commit", "2222222222222222222222222222222222222222",
		"--reason", "merged early", "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stderr, "does not apply to a batch")
}

// bugByKey reads one bug back off disk by its local key.
func bugByKey(t *testing.T, key string) *store.Bug {
	t.Helper()
	for _, bug := range loadTestTree(t).Bugs {
		if bug.Key != nil && *bug.Key == "bugs/"+key {
			return bug
		}
	}
	require.Failf(t, "no bug", "bugs/%s", key)
	return nil
}

// layPairInReview drives two approved bugs through one batch to in_review on
// its branch. commitMembers says whether the start commit carries the member
// state files, the step whose omission strands a batch.
func layPairInReview(t *testing.T, commitMembers bool) string {
	t.Helper()
	root := laySevenBugs(t)
	mustRun(t, append([]string{"approve", "bug", "--contributor", "jc"}, sevenBugKeys...)...)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "approve")
	mustRun(t, "add", "batch", "pair", "--title", "Pair", "bugs/one", "bugs/two", "--contributor", "jc")
	mustRun(t, "claim", "batch", "pair", "--contributor", "jc")
	mustRun(t, "start", "batch", "pair", "--contributor", "jc")
	gitRun(t, root, "checkout", "-b", "tiller/batches/pair")
	if commitMembers {
		gitRun(t, root, "add", "-A")
	} else {
		gitRun(t, root, "add", "--", ".tiller/batches", ".tiller/events")
	}
	gitRun(t, root, "commit", "-m", "start batch")
	return root
}

// Following the choreography with the member state files left out of the
// batch branch: check and submit refuse and name the paths.
func TestBatchCheckAndSubmitRefuseUncommittedMemberStateCLI(t *testing.T) {
	root := layPairInReview(t, false)
	refusal := batchRefusal(t, "check", "batch", "pair", "--contributor", "jc")
	require.Equal(t, "batch_state_uncommitted", refusal.Reason)
	require.Equal(t, []string{".tiller/bugs/one/bug.yml", ".tiller/bugs/two/bug.yml"}, refusal.Paths)
	require.Equal(t, "git add -- .tiller/bugs/one/bug.yml .tiller/bugs/two/bug.yml", refusal.Command)

	out, stderr, code := execCLI(t, "check", "batch", "pair", "--contributor", "jc")
	require.Equal(t, 1, code)
	require.Contains(t, out+stderr, "cannot be checked while its state is uncommitted: .tiller/bugs/one/bug.yml, .tiller/bugs/two/bug.yml. Commit these on branch tiller/batches/pair with the events beside them")

	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "member pointers")
	mustRun(t, "check", "batch", "pair", "--contributor", "jc")
	refusal = batchRefusal(t, "submit", "batch", "pair", "--contributor", "jc")
	require.Equal(t, "batch_state_uncommitted", refusal.Reason)
	require.Equal(t, []string{".tiller/batches/pair/batch.yml"}, refusal.Paths)
}

// The integration branch receives the batch record and no member pointer.
// Sync refuses to close the batch over bugs it cannot see, and says what to do.
func TestSyncLeavesAMemberlessBatchInReviewCLI(t *testing.T) {
	root := layPairInReview(t, true)
	mustRun(t, "check", "batch", "pair", "--contributor", "jc")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "gate evidence")
	mustRun(t, "submit", "batch", "pair", "--contributor", "jc")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "in review")
	gitRun(t, root, "checkout", "main")
	gitRun(t, root, "merge", "--no-ff", "-m", "merge the pair", "tiller/batches/pair")
	// What integration holds when the member files never rode the branch.
	for _, key := range []string{"one", "two"} {
		path := filepath.Join(root, ".tiller", "bugs", key, "bug.yml")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var kept []string
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "batch: ") {
				kept = append(kept, line)
			}
		}
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644))
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "integration without member pointers")

	out, _, code := execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Warning: merged into main but not crossed (1):")
	require.Contains(t, out, "merged with no members on this branch, so it was not crossed: the state files that point its bugs, chores, or tweaks at it were never committed on tiller/batches/pair.")
	require.Contains(t, out, `Cancel it (tiller cancel batch pair --reason "<why>"), then record each one that merged with tiller submit <bug|chore|tweak> <node> --recover-merged`)
	require.NotContains(t, out, "-> done")
	require.NotContains(t, out, "no merged branches to record")

	out, _, code = execCLI(t, "--json", "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	var envelope struct {
		Data SyncView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.Empty(t, envelope.Data.Crossings)
	require.Len(t, envelope.Data.MemberlessBatches, 1)
	require.Equal(t, `tiller cancel batch pair --reason "<why>"`, envelope.Data.MemberlessBatches[0].Next)
	for _, batch := range loadTestTree(t).Batches {
		require.Equal(t, "in_review", batch.Status)
	}
}

// A member's History is its own story: it joined a batch and left it. The
// batch's verbs and statuses never appear on it.
func TestMemberHistoryNeverShowsTheBatchsVerbOrStatus(t *testing.T) {
	laySevenBugs(t)
	mustRun(t, "approve", "bug", "one", "two", "--contributor", "jc")
	mustRun(t, "add", "batch", "trio", "--title", "Trio", "bugs/one", "bugs/two", "bugs/three", "--contributor", "jc")
	mustRun(t, "batch", "trio", "--remove", "bugs/two", "--contributor", "jc")
	mustRun(t, "cancel", "batch", "trio", "--reason", "apart after all", "--contributor", "jc")

	tree := loadTestTree(t)
	evs, err := events.List(tree)
	require.NoError(t, err)
	history := func(key string) (types, outcomes []string) {
		for _, entry := range views.HistoryOf(evs, bugByKey(t, key).ID) {
			types = append(types, entry.Type)
			if entry.Outcome != "" {
				outcomes = append(outcomes, entry.Outcome)
			}
		}
		return
	}
	// three stayed proposed throughout: added once, joined, released.
	types, outcomes := history("three")
	require.ElementsMatch(t, []string{"add.bug", "batch.bug", "batch.bug"}, types)
	require.Equal(t, []string{"proposed"}, outcomes, "no Ready chip from add.batch and no Cancelled from cancel.batch")
	// two was removed by an edit; one was released by the cancel.
	for _, key := range []string{"one", "two"} {
		types, outcomes = history(key)
		require.ElementsMatch(t, []string{"add.bug", "approve.bug", "batch.bug", "batch.bug"}, types, key)
		require.NotContains(t, outcomes, "cancelled", key)
	}
	// The batch's own history still names every member it held.
	var batchID string
	for id := range tree.Batches {
		batchID = id
	}
	var batchTypes []string
	for _, entry := range views.HistoryOf(evs, batchID) {
		batchTypes = append(batchTypes, entry.Type)
	}
	require.ElementsMatch(t, []string{"add.batch", "edit.batch", "cancel.batch"}, batchTypes)
	for _, ev := range evs {
		if ev.Type == "add.batch" {
			require.Len(t, ev.After["members"], 3, "members stay discoverable from the batch's event")
		}
	}
}

func TestBatchReadsAndRefusalsTellTheTruthCLI(t *testing.T) {
	root := laySevenBugs(t)
	mustRun(t, "approve", "bug", "one", "--contributor", "jc")
	mustRun(t, "add", "batch", "pair", "--title", "Paired fixes", "bugs/one", "bugs/two", "--contributor", "jc")
	mustRun(t, "claim", "batch", "pair", "--contributor", "jc")

	// An unstartable batch is not the agent's move; the triage of its
	// proposed member is the human's.
	out, _, code := execCLI(t, "next", "--all", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.NotContains(t, out, "continue batches/pair")
	require.Contains(t, out, "triage bugs/two")
	require.NotContains(t, out, "1 batch ready")

	// Approving a member reports the moves that are really open on it.
	out, _, code = execCLI(t, "approve", "bug", "two", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "Now legal on the node(s) just approved: cancel, return")
	require.NotContains(t, out, "cancel, return, start")
	out, _, code = execCLI(t, "next", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "continue batches/pair")

	// Contradictory edit input.
	_, stderr, code := execCLI(t, "batch", "pair", "--add", "bugs/two", "--remove", "bugs/two", "--contributor", "jc")
	require.Equal(t, 2, code)
	require.Contains(t, stderr, "--add and --remove both name")

	// Search and query find the batch.
	var batchID string
	for id := range loadTestTree(t).Batches {
		batchID = id
	}
	for _, q := range []string{batchID, "batches/pair", "Paired"} {
		out, _, code = execCLI(t, "search", q)
		require.Equal(t, 0, code, q)
		require.Contains(t, out, "batches/pair ("+batchID+")", q)
	}
	out, _, code = execCLI(t, "query", "--from", batchID, "--follow", "batches")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "bugs/one")
	require.Contains(t, out, "bugs/two")
	out, _, code = execCLI(t, "query", "--from", "bugs/one", "--follow", "batched_in")
	require.Equal(t, 0, code)
	require.Contains(t, out, "batches/pair ("+batchID+")")

	// A removed contributor's remedy for a batch is not transfer.
	mustRun(t, "add", "contributor", "pat", "--name", "Pat Nguyen")
	out, stderr, code = execCLI(t, "remove", "contributor", "jc", "--contributor", "pat")
	out += stderr
	require.Equal(t, 1, code)
	require.Contains(t, out, "1 live batch(es)")
	require.Contains(t, out, "release or reassign an unstarted batch, and finish a started one or cancel it")

	// A merged fix for a member of an unsubmitted batch: the advice names the
	// batch's own repair, since neither it nor a member can be recovered.
	mustRun(t, "start", "batch", "pair", "--contributor", "jc")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "start the pair")
	require.NoError(t, os.WriteFile(filepath.Join(root, "fix.txt"), []byte("fix\n"), 0o644))
	gitRun(t, root, "add", "fix.txt")
	gitRun(t, root, "commit", "-m", bugByKey(t, "one").ID+" fix on the wrong branch")
	out, _, code = execCLI(t, "sync", "--contributor", "jc")
	require.Equal(t, 0, code)
	require.Contains(t, out, "batch batches/pair ("+batchID+") is in_progress with no recorded submission")
	require.Contains(t, out, "it observes a bug, chore, tweak, or batch only once it is in review")
	require.Contains(t, out, `A batch has no recovery of its own: cancel it with tiller cancel batch <batch> --reason "<why>", which releases its members, then recover each member that merged.`)

	// The fast-forward assertion on a member names its batch.
	out, stderr, code = execCLI(t, "sync", "--accept-fast-forward", "bugs/one", "--contributor", "jc")
	out += stderr
	require.Equal(t, 1, code)
	require.Contains(t, out, "reviews it, so the merge to assert is the batch's: tiller sync --accept-fast-forward "+batchID)
}

// While a batch is done and a member has yet to cross, the member's status
// offers no move that would refuse.
func TestStatusOfAMemberWaitingOnADoneBatchCLI(t *testing.T) {
	root := layPairInReview(t, true)
	batchPath := filepath.Join(root, ".tiller", "batches", "pair", "batch.yml")
	data, err := os.ReadFile(batchPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(batchPath, []byte(strings.Replace(string(data), "status: in_progress", "status: done\ndone_basis: ancestry", 1)), 0o644))
	out, _, code := execCLI(t, "status", "bug", "one")
	require.Equal(t, 0, code)
	require.Contains(t, out, "state: ready (batched:")
	require.Contains(t, out, "next:  (none of its own: tiller sync crosses it with its batch)")
	require.NotContains(t, out, "cancel")
}
