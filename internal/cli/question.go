package cli

// This file wires the question ledger's write half (issue #19,
// cli-surface.md "The verb set"): `tiller add question "<statement>"
// --title "<short title>"` records an owned, date-gated open question on the
// decisions ledger. `tiller answer question <Q-id> --answer "<text>"` records
// its answer. Both are thin shells over internal/core/mutate; the blocks
// enforcement these feed
// (start/check/submit refusing a question-blocked node) already exists. The
// add subcommand itself is registered from add.go beside epic/feature/task.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// questionView is the SUCCESS payload of add question, answer question, and
// cancel question: the ledger entry's state the caller reads back.
type questionView struct {
	Noun      string   `json:"noun"`
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Statement string   `json:"statement"`
	Status    string   `json:"status"`
	Blocks    []string `json:"blocks,omitempty"`
	Answer    string   `json:"answer,omitempty"`
	// CancelReason and AssumedBy ride only on a cancelled question; Reasks
	// only on an add that re-asked a cancelled one.
	CancelReason string   `json:"cancel_reason,omitempty"`
	AssumedBy    []string `json:"assumed_by,omitempty"`
	Reasks       string   `json:"reasks,omitempty"`
	Path         string   `json:"path"`
	Created      bool     `json:"created"`
	Changed      bool     `json:"changed"`
}

// newAddQuestionCmd builds `add question <statement>`, registered onto the add
// command beside epic, feature, and task.
func newAddQuestionCmd() *cobra.Command {
	var blocks []string
	var title, due, dueHint, source string
	cmd := &cobra.Command{
		Use:   "question <statement>",
		Short: "Record an open question on the decisions ledger.",
		Long: "add question records an owned open question in\n" +
			".tiller/decisions/open-questions.yml, where readiness and blocks\n" +
			"enforcement can see it. --blocks names the nodes the question holds:\n" +
			"start, check, and submit refuse a held node until the question is\n" +
			"answered or cancelled. A held feature may instead proceed under a\n" +
			"recorded assumption with check feature or approve feature\n" +
			"--assume-question <Q-id> --reason <text>; tasks cannot assume one.\n" +
			"Re-adding the same statement is an idempotent no-op success; re-adding\n" +
			"a cancelled question's statement asks it again under a new id.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddQuestion(cmd, title, args[0], blocks, store.QuestionDue{Date: due, Hint: dueHint}, source)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "required one-line title for scanning the question")
	cmd.Flags().StringSliceVar(&blocks, "blocks", nil, "nodes this question holds (id or key, repeatable)")
	cmd.Flags().StringVar(&due, "due", "", "date the answer is due (YYYY-MM-DD)")
	cmd.Flags().StringVar(&dueHint, "due-hint", "", "free-form due hint")
	cmd.Flags().StringVar(&source, "source", "", "provenance: where the question surfaced")
	addContributorFlag(cmd)
	return cmd
}

func runAddQuestion(cmd *cobra.Command, title, statement string, blocks []string, due store.QuestionDue, source string) error {
	if strings.TrimSpace(title) == "" {
		return emitFailure(cmd, "add", &usageError{
			reason: "title_required",
			msg:    "a question needs --title with a one-line scanning title",
		})
	}
	if strings.ContainsAny(title, "\r\n") {
		return emitFailure(cmd, "add", &usageError{
			reason: "title_one_line",
			msg:    "a question title must be one line",
		})
	}
	if strings.TrimSpace(statement) == "" {
		return emitFailure(cmd, "add", &usageError{
			reason: "statement_required",
			msg:    "a question needs a statement",
		})
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	res, err := mutate.AddQuestion(tree, d, mutate.QuestionInput{
		Title: title, Statement: statement, Blocks: blocks, Due: due, Source: source,
	})
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	return emitSuccess(cmd, "add", questionPayload(res), humanQuestion(tree, res))
}

// newAnswerCmd builds `answer question <node> --answer <text>`, the human gate
// that closes an open question (cli-surface.md, the answer verb).
func newAnswerCmd() *cobra.Command {
	var answer string
	cmd := &cobra.Command{
		Use:   "answer question <node> --answer <text>",
		Short: "Record the answer to an open question (open -> answered).",
		Long: "answer records the answer to an open question and the revision it was\n" +
			"recorded at, clearing the question's computed blockers (start, check,\n" +
			"and submit only count open questions). Re-answering with the same text\n" +
			"is an idempotent no-op; a different text is refused, because a recorded\n" +
			"answer does not silently change.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAnswer(cmd, args[0], args[1], answer)
		},
	}
	cmd.Flags().StringVar(&answer, "answer", "", "the answer text (required)")
	_ = cmd.MarkFlagRequired("answer")
	addContributorFlag(cmd)
	return cmd
}

func runAnswer(cmd *cobra.Command, noun, node, answer string) error {
	if _, err := validNoun(cmd, "answer", noun, []string{store.NounQuestion}); err != nil {
		return err
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "answer", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "answer", err)
	}
	res, err := mutate.AnswerQuestion(tree, d, vcs.New(tree.Root), mutate.AnswerInput{Node: node, Answer: answer})
	if err != nil {
		var already *mutate.AlreadyAnsweredError
		if errors.As(err, &already) {
			data := map[string]any{
				"noun":   store.NounQuestion,
				"id":     already.QuestionID,
				"reason": "already_answered",
			}
			human := fmt.Sprintf("Blocked: question %s is already answered; a recorded answer does not silently change.", already.QuestionID)
			return emitReport(cmd, "answer", false, data, human, 1)
		}
		return reportQuestionError(cmd, "answer", err)
	}
	// The confirmation carries the id, short title, and full statement as
	// separate parts. answer question is one of the four human gates
	// (gateMomentFor, readout_gate.go), so both outcomes carry the readout
	// after the confirmation, including the idempotent no-change case: it is
	// still a completed `answer` invocation with ok true.
	// act.Answered.Blocks is res.Blocks, the exact node ids THIS question
	// held, so answerContent (internal/views/readout.go) can check each
	// against the fresh tree for "what it unblocked" rather than a tally of
	// everything still open.
	act := &views.GateAct{Answered: &views.GateActAnswer{Blocks: res.Blocks}}
	if !res.Changed {
		return emitReportWithReadout(cmd, "answer", true, questionPayload(res),
			fmt.Sprintf("question %s (%q): %q is already answered (no change).", res.ID, res.Title, res.Statement), 0, gateMomentFor("answer"), tree.Root, act)
	}
	return emitReportWithReadout(cmd, "answer", true, questionPayload(res),
		fmt.Sprintf("question %s (%q): %q is now answered.", res.ID, res.Title, res.Statement), 0, gateMomentFor("answer"), tree.Root, act)
}

// questionPayload maps a core result onto the stable JSON envelope payload.
func questionPayload(res mutate.QuestionResult) questionView {
	return questionView{
		Noun:      res.Noun,
		ID:        res.ID,
		Title:     res.Title,
		Statement: res.Statement,
		Status:    res.Status,
		Blocks:    res.Blocks,
		Answer:    res.Answer,
		Path:      res.Path,
		Created:   res.Created,
		Changed:   res.Changed,

		CancelReason: res.CancelReason,
		AssumedBy:    res.AssumedBy,
		Reasks:       res.Reasks,
	}
}

// reportQuestionError renders a refused move on a closed question (answer,
// cancel, link, or unlink) as a blocked business answer naming the actual
// status; every other error keeps its ordinary failure envelope.
func reportQuestionError(cmd *cobra.Command, command string, err error) error {
	var closed *mutate.QuestionClosedError
	if !errors.As(err, &closed) {
		return emitFailure(cmd, command, err)
	}
	data := map[string]any{
		"noun":          store.NounQuestion,
		"id":            closed.QuestionID,
		"reason":        "question_closed",
		"current_state": closed.Status,
	}
	return emitReport(cmd, command, false, data, "Blocked: "+closed.Error()+".", 1)
}

// humanQuestion is the plain-text confirmation for a recorded question. It
// keeps the short scanning title distinct from the full statement, while
// blocked nodes use the shared reference convention so the human sees exactly
// what the question now holds.
func humanQuestion(tree *store.Tree, res mutate.QuestionResult) string {
	verb := "Created"
	if !res.Created {
		verb = "Found existing"
	}
	line := fmt.Sprintf("%s question %s (%q): %q", verb, res.ID, res.Title, res.Statement)
	if len(res.Blocks) > 0 {
		refs := make([]string, len(res.Blocks))
		for i, id := range res.Blocks {
			refs[i] = gen.RefByID(tree, id)
		}
		line += fmt.Sprintf("; blocks %s", strings.Join(refs, ", "))
	}
	if res.Reasks != "" {
		line += "; re-asks cancelled " + res.Reasks
	}
	return line + "."
}
