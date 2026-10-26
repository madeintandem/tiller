package mutate_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/stretchr/testify/require"
)

const blocksTaskKey = "form-engine/certification-form-loading/render-the-form"

// layQuestionBlocking lays the question repo with a task (T-aaaaaa) beside the
// epic (E-aaaaaa) and feature (F-bbbbbb), and an open question blocking blocks.
func layQuestionBlocking(t *testing.T, blocks ...string) (string, string) {
	t.Helper()
	root := writeQuestionRepo(t)
	_, err := mutate.AddTask(load(t, root), testDeps(), mutate.TaskInput{Key: blocksTaskKey, Title: "Render the form"})
	require.NoError(t, err)
	res, err := mutate.AddQuestion(load(t, root), testDeps(), mutate.QuestionInput{
		Title: "Which store", Statement: "Which store holds the forms?", Blocks: blocks,
	})
	require.NoError(t, err)
	return root, res.ID
}

func countEvents(t *testing.T, root, kind string) int {
	t.Helper()
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	n := 0
	for _, ev := range evs {
		if ev.Type == kind {
			n++
		}
	}
	return n
}

func TestLinkQuestionBlocksUnions(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "T-aaaaaa")

	res, err := mutate.LinkQuestionBlocks(load(t, root), testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"F-bbbbbb", "T-aaaaaa"}})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, []string{"T-aaaaaa", "F-bbbbbb"}, res.Blocks)
	require.Equal(t, []string{"F-bbbbbb"}, res.Added)
	require.Equal(t, []string{"T-aaaaaa", "F-bbbbbb"}, load(t, root).Questions[qid].Blocks)
	require.Equal(t, 1, countEvents(t, root, "link.question"))
}

func TestLinkQuestionBlocksByKey(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "T-aaaaaa")

	res, err := mutate.LinkQuestionBlocks(load(t, root), testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"form-engine/certification-form-loading"}})
	require.NoError(t, err)
	require.Equal(t, []string{"F-bbbbbb"}, res.Added)
}

func TestLinkQuestionBlocksNoChange(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "T-aaaaaa")
	before := ledgerBytes(t, root)

	res, err := mutate.LinkQuestionBlocks(load(t, root), testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"T-aaaaaa"}})
	require.NoError(t, err)
	require.False(t, res.Changed)
	require.Empty(t, res.Added)
	require.Equal(t, before, ledgerBytes(t, root))
	require.Zero(t, countEvents(t, root, "link.question"))
}

func TestUnlinkQuestionBlocksPreservesOrder(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "E-aaaaaa", "F-bbbbbb", "T-aaaaaa")

	res, err := mutate.UnlinkQuestionBlocks(load(t, root), testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"F-bbbbbb"}})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, []string{"E-aaaaaa", "T-aaaaaa"}, res.Blocks)
	require.Equal(t, []string{"F-bbbbbb"}, res.Removed)
	require.Equal(t, []string{"E-aaaaaa", "T-aaaaaa"}, load(t, root).Questions[qid].Blocks)
	require.Equal(t, 1, countEvents(t, root, "unlink.question"))
}

func TestUnlinkQuestionBlocksAbsentTarget(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "T-aaaaaa")
	before := ledgerBytes(t, root)

	res, err := mutate.UnlinkQuestionBlocks(load(t, root), testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"F-bbbbbb"}})
	require.NoError(t, err)
	require.False(t, res.Changed)
	require.Empty(t, res.Removed)
	require.Equal(t, []string{"F-bbbbbb"}, res.Targets, "the resolved request names what was absent")
	require.Equal(t, before, ledgerBytes(t, root))
	require.Zero(t, countEvents(t, root, "unlink.question"))
}

func TestUnlinkQuestionBlocksLastTarget(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "T-aaaaaa")

	res, err := mutate.UnlinkQuestionBlocks(load(t, root), testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"T-aaaaaa"}})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Empty(t, res.Blocks)
	q := load(t, root).Questions[qid]
	require.Empty(t, q.Blocks)
	require.Contains(t, []string{"open", ""}, q.Status)
}

func TestUnlinkQuestionBlocksBadTargetWritesNothing(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "T-aaaaaa")
	before := ledgerBytes(t, root)

	_, err := mutate.UnlinkQuestionBlocks(load(t, root), testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"T-aaaaaa", "nope"}})
	require.Error(t, err)
	require.Equal(t, before, ledgerBytes(t, root))
	require.Zero(t, countEvents(t, root, "unlink.question"))
}

func TestQuestionBlocksClosedRefuses(t *testing.T) {
	t.Parallel()
	for _, closing := range []string{"answered", "cancelled"} {
		root, qid := layQuestionBlocking(t, "T-aaaaaa")
		var err error
		if closing == "answered" {
			_, err = mutate.AnswerQuestion(load(t, root), testDeps(), stubRev(root), mutate.AnswerInput{Node: qid, Answer: "Postgres."})
		} else {
			_, err = mutate.CancelQuestion(load(t, root), testDeps(), mutate.CancelInput{Node: qid, Reason: "moot"})
		}
		require.NoError(t, err)
		before := ledgerBytes(t, root)
		in := mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"F-bbbbbb"}}

		_, err = mutate.LinkQuestionBlocks(load(t, root), testDeps(), in)
		var closed *mutate.QuestionClosedError
		require.ErrorAs(t, err, &closed, closing)
		require.Equal(t, "link", closed.Verb)
		require.Equal(t, closing, closed.Status)

		_, err = mutate.UnlinkQuestionBlocks(load(t, root), testDeps(), in)
		closed = nil
		require.ErrorAs(t, err, &closed, closing)
		require.Equal(t, "unlink", closed.Verb)
		require.Equal(t, before, ledgerBytes(t, root))
	}
}

func TestQuestionBlocksNeedTarget(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "T-aaaaaa")
	in := mutate.QuestionBlocksInput{Node: qid}

	_, err := mutate.LinkQuestionBlocks(load(t, root), testDeps(), in)
	require.Error(t, err)
	_, err = mutate.UnlinkQuestionBlocks(load(t, root), testDeps(), in)
	require.Error(t, err)
}

func TestLinkAndUnlinkStaleTreeRefuseAnswered(t *testing.T) {
	t.Parallel()
	root, qid := layQuestionBlocking(t, "F-bbbbbb")
	stale := load(t, root)
	answerOnDisk(t, root, qid)
	before := ledgerBytes(t, root)

	_, err := mutate.LinkQuestionBlocks(stale, testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"T-aaaaaa"}})
	var closed *mutate.QuestionClosedError
	require.ErrorAs(t, err, &closed)
	require.Equal(t, "link", closed.Verb)
	require.Equal(t, "answered", closed.Status)

	_, err = mutate.UnlinkQuestionBlocks(stale, testDeps(), mutate.QuestionBlocksInput{Node: qid, Blocks: []string{"F-bbbbbb"}})
	require.ErrorAs(t, err, &closed)
	require.Equal(t, "unlink", closed.Verb)
	require.Equal(t, before, ledgerBytes(t, root))
}
