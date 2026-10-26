package cli

// link and unlink on a question widen or narrow what an open question blocks
// (docs/spec/cli-surface.md, "Proposal correction commands", the Questions
// paragraphs). The only axis a question has is --blocks, and --blocks belongs
// to no other noun.

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// questionLinkView is the SUCCESS payload of link and unlink on a question:
// its blocks after the call, always present, [] when it blocks nothing.
type questionLinkView struct {
	Noun    string   `json:"noun"`
	ID      string   `json:"id"`
	Blocks  []string `json:"blocks"`
	Changed bool     `json:"changed"`
}

// isQuestionNoun reports whether a command's noun argument names a question,
// in either the singular or the plural form.
func isQuestionNoun(noun string) bool {
	sing, _ := singularNoun(noun)
	return sing == store.NounQuestion
}

// rejectQuestionAxisMix refuses --blocks on any noun but a question, and
// every flag but --blocks on a question, before anything is read.
func rejectQuestionAxisMix(cmd *cobra.Command, command string, question bool) error {
	if !question {
		if cmd.Flags().Changed("blocks") {
			return emitFailure(cmd, command, &usageError{
				reason: "bad_args", msg: fmt.Sprintf("--blocks applies only to %s question", command),
			})
		}
		return nil
	}
	var rejected []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "blocks", "contributor", "json":
		default:
			rejected = append(rejected, "--"+f.Name)
		}
	})
	if len(rejected) > 0 {
		return emitFailure(cmd, command, &usageError{
			reason: "bad_args",
			msg:    fmt.Sprintf("%s question accepts only --blocks, not %s", command, strings.Join(rejected, ", ")),
		})
	}
	return nil
}

func runQuestionLink(cmd *cobra.Command, node string, blocks []string) error {
	return runQuestionBlocks(cmd, "link", node, blocks)
}

func runQuestionUnlink(cmd *cobra.Command, node string, blocks []string) error {
	return runQuestionBlocks(cmd, "unlink", node, blocks)
}

func runQuestionBlocks(cmd *cobra.Command, command, node string, blocks []string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, command, err)
	}
	in := mutate.QuestionBlocksInput{Node: node, Blocks: blocks}
	var res mutate.QuestionBlocksResult
	if command == "link" {
		res, err = mutate.LinkQuestionBlocks(tree, d, in)
	} else {
		res, err = mutate.UnlinkQuestionBlocks(tree, d, in)
	}
	if err != nil {
		return reportQuestionError(cmd, command, err)
	}
	view := questionLinkView{Noun: store.NounQuestion, ID: res.ID, Blocks: nonNilStrings(res.Blocks), Changed: res.Changed}
	refs := func(ids []string) string { return strings.Join(refsByID(tree, ids), ", ") }
	var human string
	switch {
	case command == "link" && res.Changed:
		human = fmt.Sprintf("question %s now blocks %s.", res.ID, refs(res.Blocks))
	case command == "link":
		human = fmt.Sprintf("question %s already blocks %s (no change).", res.ID, refs(res.Blocks))
	case res.Changed:
		remaining := "nothing"
		if len(res.Blocks) > 0 {
			remaining = refs(res.Blocks)
		}
		human = fmt.Sprintf("Removed %s from question %s; it blocks %s.", refs(res.Removed), res.ID, remaining)
	default:
		human = fmt.Sprintf("question %s did not block %s (no change).", res.ID, refs(res.Targets))
	}
	return emitSuccess(cmd, command, view, human)
}
