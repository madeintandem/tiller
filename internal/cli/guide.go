package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/skills"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// This file implements `tiller guide [<skill>]`, the harness-agnostic backstop
// that surfaces a Tiller skill straight from the CLI (cli-surface.md, "tiller
// guide"; skill-set.md, "Format, packaging, and portability"). With a skill name
// it prints that embedded skill in full. With no argument it reads project state,
// prints the ranked next list, and names the skill for the highest-ranked agent
// move, the same list `.tiller/orientation.md` describes. It is read-only.

// GuideView is the --json payload of `tiller guide <skill>`: the named skill and
// its full text, so a harness that prefers structured output can load the body
// without scraping stdout.
type GuideView struct {
	Skill string `json:"skill"`
	Text  string `json:"text"`
}

// SuggestView is the --json payload of bare `tiller guide`: the skill that fits
// the current state and the one-line reason it was chosen, so the agent (or a
// harness) can route without re-deriving the state.
type SuggestView struct {
	Skill  string `json:"skill"`
	Reason string `json:"reason"`
	Queue  string `json:"queue"`
	// Next is the ranked list bare guide prints, cut to the shortlist the way
	// `tiller next --json` cuts it.
	Next views.Next `json:"next"`
}

func newGuideCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "guide [<skill>]",
		Short: "Print a Tiller skill, or list your next moves and name the skill that fits.",
		Long: "guide prints the text of a Tiller skill so any harness reaches the same\n" +
			"guidance a richer one auto-loads. With no argument it prints the ranked next\n" +
			"moves and names the skill for the highest-ranked agent move. It is read-only.\n\n" +
			"Skills: " + strings.Join(skills.Names(), ", ") + ".",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return runGuideSkill(cmd, args[0])
			}
			return runGuideSuggest(cmd)
		},
	}
}

// runGuideSkill prints one named skill in full. An unknown skill is misuse (exit
// 2) and lists the valid names so the caller can correct the call.
func runGuideSkill(cmd *cobra.Command, name string) error {
	text, ok := skills.Lookup(name)
	if !ok {
		return emitFailure(cmd, "guide", &usageError{
			reason: "unknown_skill",
			msg:    fmt.Sprintf("unknown skill %q (want one of %s)", name, strings.Join(skills.Names(), ", ")),
		})
	}
	return emitSuccess(cmd, "guide", GuideView{Skill: name, Text: text}, text)
}

// runGuideSuggest prints the ranked next list and names the skill for its
// highest-ranked agent move, since the top item is often the human's.
// A skill-less harness gets the same list
// `tiller next` prints and the one skill that fits beneath it.
func runGuideSuggest(cmd *cobra.Command) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "guide", err)
	}
	// The acting contributor ranks the list and scopes the execute arm for both
	// tasks and shelf work. guide carries no --contributor flag, so this
	// resolves through TILLER_CONTRIBUTOR and then the checkout's identity file;
	// resolveActingContributor reads a flag that is not registered as "" and
	// falls through, so the same helper serves both.
	next := rankedNext(tree, resolveActingContributor(cmd, tree.Root))
	skill, reason := suggestFromNext(tree, next)
	queue := views.ReadyShelfQueueLine(tree)
	suggestion := fmt.Sprintf("Suggested skill: %s\n%s\n%s\nRun `tiller guide %s` to load it.", skill, reason, queue, skill)
	payload := next
	payload.Candidates = next.Shortlist()
	return emitSuccess(cmd, "guide", SuggestView{Skill: skill, Reason: reason, Queue: queue, Next: payload},
		renderNextText(next, false)+"\n\n"+suggestion)
}

// rankedNext ranks the legal moves for reader against the current clock.
func rankedNext(t *store.Tree, reader string) views.Next {
	return views.NextOf(t, readinput.ForNext(t, reader, time.Now()))
}

// suggestSkill names the skill that fits reader's top agent move. Surfaces that
// close on guide's suggestion, such as check readiness, call it.
func suggestSkill(t *store.Tree, reader string) (skill, reason string) {
	next := rankedNext(t, reader)
	if next.CaughtUp && next.Hint != "" {
		// No list sits above this reason, so it carries the hint that
		// explains held work or how work enters.
		return "orient", views.Sentence(next.Hint)
	}
	if _, ok := next.TopAgent(); !ok && !next.CaughtUp {
		// No list sits above this reason, so it points at the one that
		// holds the human's moves and keeps how work enters.
		reason = "No agent move is legal right now; `tiller next` lists the moves that are yours."
		if next.Hint != "" {
			reason += " " + views.Sentence(next.Hint)
		}
		return "orient", reason
	}
	return suggestFromNext(t, next)
}

// suggestFromNext maps the highest-ranked agent move onto the imperative
// wording appropriate to guide; seam-specific instructions stay local. A
// caught-up list is an orient result; the list above already prints its hint,
// so the reason does not repeat it. A list of only human moves is orient too:
// no skill fits a wait on a person.
func suggestFromNext(t *store.Tree, next views.Next) (skill, reason string) {
	if next.CaughtUp {
		return "orient", "No move is legal right now."
	}
	top, ok := next.TopAgent()
	if !ok {
		return "orient", "No agent move is legal right now; the moves above are yours."
	}
	switch top.Skill {
	case "derive-baseline":
		return top.Skill, views.Sentence(top.Reason)
	case "decompose":
		return top.Skill, "The tree is undecomposed; lay a draft epic and feature tree for the human to approve. " + views.Sentence(top.Reason)
	case "specify":
		return top.Skill, "A feature is in draft; write its spec, its acceptance contract, and its tasks, then run tiller spec feature."
	}
	return "execute", executeReason(t, top)
}

// executeReason is the execute arm's wording for a move: work the reader holds,
// a claim, or the coarse review finish line.
func executeReason(t *store.Tree, top views.NextCandidate) string {
	// The reader's own task observed in flight is continued where it runs,
	// never claimed or started again. The branch names the task.
	if top.Branch != "" {
		return "Your task is in flight on " + top.Branch + "; continue it there."
	}
	// Claimable work needs its claim before the execute loop can begin.
	if top.Kind == "claim_task" {
		return "An approved task is unclaimed and startable; claim it, then start its work loop. " + nextMove(top)
	}
	reason := "Continue the resolved review surface through its remaining checks, publication, and recorded submission."
	if top.Node != nil {
		switch top.Node.Noun {
		case store.NounTask, store.NounBug, store.NounChore, store.NounTweak:
			if status := nodeStatusByID(t, top.Node.ID); status == "check" {
				reason = fmt.Sprintf("You hold a %s in check; continue execute without starting or resuming it again.", top.Node.Noun)
			} else {
				reason = fmt.Sprintf("You hold a %s in %s; run its work loop through publication and recorded submission.", top.Node.Noun, status)
			}
		}
		// Name the move's node, so a feature or epic's coarse review finish
		// line reads as the selected review surface.
		reason += " " + nextMove(top)
	}
	if detail := views.Sentence(top.Reason); detail != "" {
		reason += " " + detail
	}
	return reason
}

// nextMove names a move's action and node; the move must have a node.
func nextMove(c views.NextCandidate) string {
	return "Next move: " + c.Action + " " + c.Node.Ref + "."
}

// nodeStatusByID is the status of a task or shelf node, "" when it is gone.
func nodeStatusByID(t *store.Tree, id string) string {
	if task, ok := t.Tasks[id]; ok {
		return task.NodeStatus()
	}
	if work, ok := t.WorkNode(id); ok {
		return work.NodeStatus()
	}
	return ""
}
