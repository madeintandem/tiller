package mutate

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// landedCommits is the git window the scan reads, canned: the commits on the
// integration branch that change the project's code, newest first.
type landedCommits struct {
	messages []vcs.CommitMessage
	calls    int
	ref      string
	limit    int
}

func (l *landedCommits) CodeCommitMessages(ref string, limit int) ([]vcs.CommitMessage, error) {
	l.calls++
	l.ref, l.limit = ref, limit
	return l.messages, nil
}

func landed(messages ...string) *landedCommits {
	l := &landedCommits{}
	for i, m := range messages {
		subject := m
		for j, r := range m {
			if r == '\n' {
				subject = m[:j]
				break
			}
		}
		l.messages = append(l.messages, vcs.CommitMessage{
			Commit: string(rune('a'+i)) + "000000000000000000000000000000000000000", Subject: subject, Message: m})
	}
	return l
}

func shelfTree(bugStatus string) *store.Tree {
	key := "alpha"
	return &store.Tree{
		Manifest: &store.Manifest{},
		Bugs:     map[string]*store.Bug{"B-aaaaaa": {WorkItem: store.WorkItem{ID: "B-aaaaaa", Key: &key, Status: bugStatus}}},
		Chores:   map[string]*store.Chore{},
		Tweaks:   map[string]*store.Tweak{},
		Features: map[string]*store.Feature{},
		Tasks:    map[string]*store.Task{},
		Epics:    map[string]*store.Epic{},
	}
}

func TestUnrecordedShelfMergesWarnsOnlyBeforeSubmission(t *testing.T) {
	t.Parallel()
	for status, want := range map[string]bool{
		"proposed":    false,
		"ready":       true,
		"in_progress": true,
		"check":       true,
		"in_review":   false,
		"done":        false,
		"cancelled":   false,
	} {
		t.Run(status, func(t *testing.T) {
			rev := landed("B-aaaaaa Preserve capture order")
			got, err := unrecordedShelfMerges(shelfTree(status), rev, "main", "tip")
			require.NoError(t, err)
			if !want {
				require.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			require.Equal(t, UnrecordedMerge{
				Noun: store.NounBug, ID: "B-aaaaaa", Key: "alpha", Status: status,
				Commits: []NamingCommit{{Commit: rev.messages[0].Commit, Subject: "B-aaaaaa Preserve capture order"}},
			}, got[0])
		})
	}
}

func TestUnrecordedShelfMergesReadsOneBoundedWindowOfCodeCommits(t *testing.T) {
	t.Parallel()
	rev := landed()
	_, err := unrecordedShelfMerges(shelfTree("ready"), rev, "main", "tipcommit")
	require.NoError(t, err)
	require.Equal(t, 1, rev.calls)
	require.Equal(t, "tipcommit", rev.ref, "the scan reads the one integration commit the run resolved")
	require.Equal(t, unrecordedMergeScanLimit, rev.limit)
}

func TestUnrecordedShelfMergesAsksGitNothingWithNoOpenShelfWork(t *testing.T) {
	t.Parallel()
	rev := landed("B-aaaaaa fix")
	got, err := unrecordedShelfMerges(shelfTree("done"), rev, "main", "tip")
	require.NoError(t, err)
	require.Empty(t, got)
	require.Zero(t, rev.calls)
}

func TestUnrecordedShelfMergesMatchesWholeIDsInSubjectAndBody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		message string
		want    bool
	}{
		{"subject prefix", "B-aaaaaa Preserve capture order", true},
		{"subject with punctuation", "Fix capture order (B-aaaaaa).", true},
		{"body line among other ids", "Fix the inbox\n\nB-zzzzzz B-aaaaaa B-yyyyyy", true},
		{"trailing colon", "B-aaaaaa: fix", true},
		{"longer token", "B-aaaaaab fix", false},
		{"prefixed token", "XB-aaaaaa fix", false},
		{"another shelf's prefix", "TW-aaaaaa fix", false},
		{"lower case", "b-aaaaaa fix", false},
		{"unknown id", "B-bbbbbb fix", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := unrecordedShelfMerges(shelfTree("ready"), landed(tc.message), "main", "tip")
			require.NoError(t, err)
			require.Equal(t, tc.want, len(got) == 1)
		})
	}
}

func TestUnrecordedShelfMergesListsEveryNamingCommitNewestFirst(t *testing.T) {
	t.Parallel()
	rev := landed("B-aaaaaa follow-up", "unrelated", "B-aaaaaa first fix")
	got, err := unrecordedShelfMerges(shelfTree("in_progress"), rev, "main", "tip")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []NamingCommit{
		{Commit: rev.messages[0].Commit, Subject: "B-aaaaaa follow-up"},
		{Commit: rev.messages[2].Commit, Subject: "B-aaaaaa first fix"},
	}, got[0].Commits)
}

func TestUnrecordedShelfMergesCoversEveryShelfInShelfOrder(t *testing.T) {
	t.Parallel()
	tree := shelfTree("ready")
	choreKey, tweakKey := "tidy", "nudge"
	tree.Chores["CH-bbbbbb"] = &store.Chore{WorkItem: store.WorkItem{ID: "CH-bbbbbb", Key: &choreKey, Status: "check"}}
	tree.Tweaks["TW-cccccc"] = &store.Tweak{WorkItem: store.WorkItem{ID: "TW-cccccc", Key: &tweakKey, Status: "in_progress"}}
	got, err := unrecordedShelfMerges(tree, landed("TW-cccccc CH-bbbbbb B-aaaaaa one sweep"), "main", "tip")
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, []string{"B-aaaaaa", "CH-bbbbbb", "TW-cccccc"}, []string{got[0].ID, got[1].ID, got[2].ID})
	require.Equal(t, []string{"bug", "chore", "tweak"}, []string{got[0].Noun, got[1].Noun, got[2].Noun})
}

// A carried node never submits: its fix rides its carrier's merge, so commits
// naming it are the carrier's work landing, not an unrecorded merge. Once the
// carrier can no longer carry it, the node answers for itself again.
func TestUnrecordedShelfMergesLeavesCarriedNodesToTheirCarrier(t *testing.T) {
	t.Parallel()
	tree := shelfTree("ready")
	tree.Bugs["B-aaaaaa"].ResolvedBy = "T-carrier"
	tree.Tasks["T-carrier"] = &store.Task{ID: "T-carrier", Status: "in_progress"}
	got, err := unrecordedShelfMerges(tree, landed("B-aaaaaa fix"), "main", "tip")
	require.NoError(t, err)
	require.Empty(t, got)

	tree.Tasks["T-carrier"].Status = "cancelled"
	got, err = unrecordedShelfMerges(tree, landed("B-aaaaaa fix"), "main", "tip")
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// A branch contains its own commits, so reading a node's working branch as the
// integration branch would report the work in progress as a merge.
func TestUnrecordedShelfMergesSkipsANodeObservedOnItsOwnBranch(t *testing.T) {
	t.Parallel()
	tree := shelfTree("in_progress")
	tree.Bugs["B-aaaaaa"].Branch = "tiller/bugs/alpha"
	got, err := unrecordedShelfMerges(tree, landed("B-aaaaaa fix"), "tiller/bugs/alpha", "tip")
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = unrecordedShelfMerges(tree, landed("B-aaaaaa fix"), "main", "tip")
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestUnrecordedMergeNextStepIsTheNodesOwnOracle(t *testing.T) {
	t.Parallel()
	m := UnrecordedMerge{Noun: store.NounTweak, ID: "TW-cccccc", Key: "nudge", Status: "ready"}
	require.Equal(t, "tiller status tweak TW-cccccc", m.NextStep())
}

func TestUnrecordedMergeWarningNamesNodeStatusCommitAndNextStep(t *testing.T) {
	t.Parallel()
	m := UnrecordedMerge{Noun: store.NounBug, ID: "B-aaaaaa", Key: "alpha", Status: "ready", Commits: []NamingCommit{
		{Commit: "1234567890abcdef1234567890abcdef12345678", Subject: "B-aaaaaa Preserve capture order"},
		{Commit: "abcdef1234567890abcdef1234567890abcdef12", Subject: "B-aaaaaa first fix"},
	}}
	require.Equal(t,
		`bug alpha (B-aaaaaa) is ready with no recorded submission, but commit 1234567 "B-aaaaaa Preserve capture order" and 1 more name it; next: tiller status bug B-aaaaaa`,
		m.Warning())
}
