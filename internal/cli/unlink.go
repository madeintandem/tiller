package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// unlinkView is the success payload. Dependency references remain stored ids in
// JSON; only the human sentence resolves them back to display references.
type unlinkView struct {
	Noun      string   `json:"noun"`
	ID        string   `json:"id"`
	Key       string   `json:"key,omitempty"`
	DependsOn []string `json:"depends_on"`
	Removed   []string `json:"removed"`
	// Surface and SurfaceRemoved ride only when the call named --surface, and
	// then always, as [] when nothing remains or nothing was removed.
	Surface        *[]string `json:"surface,omitempty"`
	SurfaceRemoved *[]string `json:"surface_removed,omitempty"`
	Changed        bool      `json:"changed"`
}

type dependencyChangeLockedView struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

func newUnlinkCmd() *cobra.Command {
	var dependsOn, surface, describes, from, blocks []string
	var detach bool
	cmd := &cobra.Command{
		Use:   "unlink <noun> <node> [--depends-on <target>] [--surface <path>] [--detach] [--describes <path>] [--from <path>] [--blocks <target>]",
		Short: "Remove dependencies, code surfaces, attachments, design baseline links, or question holds.",
		Long: "unlink removes resolved ids from a node's depends_on while preserving the order of every surviving edge.\n" +
			"Targets follow link's exact address and scope rules: epics and features name epics or features, a task names sibling tasks, and a bug, chore, or tweak names other bugs, chores, and tweaks.\n" +
			"Every target resolves before anything is written. An edge already absent is an idempotent no-op success.\n" +
			"--detach (bug, chore, or tweak only) clears attached_to, legal at any state since attachment is provenance, not a lifecycle fact.\n" +
			"--surface (epic or feature only) removes literal, repeatable paths from surface.paths, even when they no longer exist, so work that moved\n" +
			"or deleted its surface can say so before done. A done feature or epic, or an archived epic, keeps its surface; a specified replace or\n" +
			"migrate feature cannot drop its last path, so link the code's new location first.\n" +
			"--depends-on combines with --surface or --detach: every axis validates before any is written, so a refusal on one leaves the others in place.\n" +
			"unlink design <doc> removes literal, repeatable --describes and --from paths, even when those files no longer exist.\n" +
			"Removing the last described path removes the ledger entry, never the design prose.\n" +
			"unlink question <Q-id> --blocks <target> narrows what an open question holds, preserving the order of the rest;\n" +
			"a target already absent is a no-op success and removing every target is legal. --blocks is the only axis a question takes.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == store.NounDesign {
				return runDesignLink(cmd, "unlink", mutate.DesignLinkInput{Doc: args[1], Describes: describes, From: from})
			}
			if err := rejectQuestionAxisMix(cmd, "unlink", isQuestionNoun(args[0])); err != nil {
				return err
			}
			if isQuestionNoun(args[0]) {
				return runQuestionUnlink(cmd, args[1], blocks)
			}
			if err := rejectDesignOnlyFlags(cmd, "unlink"); err != nil {
				return err
			}
			return runUnlink(cmd, args[0], args[1], dependsOn, surface, detach)
		},
	}
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "the dependency to remove (id or key, repeatable)")
	cmd.Flags().StringArrayVar(&surface, "surface", nil, "epic or feature only: a recorded surface path to remove (literal, repeatable)")
	cmd.Flags().BoolVar(&detach, "detach", false, "bug|chore|tweak only: clear the recorded attachment")
	cmd.Flags().StringArrayVar(&describes, "describes", nil, "design only: a described code path to remove (literal, repeatable)")
	cmd.Flags().StringArrayVar(&from, "from", nil, "design only: a source provenance path to remove (literal, repeatable)")
	cmd.Flags().StringArrayVar(&blocks, "blocks", nil, "question only: an epic, feature, or task this question holds (id or key, repeatable)")
	addContributorFlag(cmd)
	return cmd
}

func runUnlink(cmd *cobra.Command, noun, node string, dependsOn, surface []string, detach bool) error {
	sing, err := validNoun(cmd, "unlink", noun, []string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}
	if len(dependsOn) == 0 && len(surface) == 0 && !detach {
		return emitFailure(cmd, "unlink", &usageError{
			reason: "unlink_target",
			msg:    "unlink needs --depends-on, --surface, --detach, or a combination",
		})
	}
	if len(surface) > 0 && detach {
		return emitFailure(cmd, "unlink", &usageError{
			reason: "surface_target",
			msg:    "--surface applies to epics and features; --detach applies to bugs, chores, and tweaks, so they cannot be combined",
		})
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "unlink", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "unlink", err)
	}

	res, err := mutate.Unlink(tree, d, mutate.UnlinkInput{Noun: sing, Node: node, DependsOn: dependsOn, Surface: surface, Detach: detach})
	if err != nil {
		return reportDependencyChangeError(cmd, "unlink", err)
	}

	view := unlinkView{
		Noun: res.Noun, ID: res.ID, Key: res.Key,
		DependsOn: nonNilStrings(res.DependsOn), Removed: nonNilStrings(res.Removed), Changed: res.Changed,
	}
	ref := gen.RefParts(res.Key, res.ID)
	var lines []string
	if len(dependsOn) > 0 {
		remaining := dependencyCount(len(res.DependsOn))
		if len(res.Removed) > 0 {
			lines = append(lines, fmt.Sprintf("Removed %s from %s %s; %s remains.",
				strings.Join(refsByID(tree, res.Removed), ", "), res.Noun, ref, remaining))
		} else {
			lines = append(lines, fmt.Sprintf("No requested dependencies were present on %s %s; %s remains (no change).",
				res.Noun, ref, remaining))
		}
	}
	if len(surface) > 0 {
		surface, removed := nonNilStrings(res.Surface), nonNilStrings(res.SurfaceRemoved)
		view.Surface, view.SurfaceRemoved = &surface, &removed
		remaining := "no surface remains"
		if len(res.Surface) > 0 {
			remaining = "surface is now " + strings.Join(res.Surface, ", ")
		}
		if res.SurfaceChanged {
			lines = append(lines, fmt.Sprintf("Removed %s from %s %s surface; %s.",
				strings.Join(res.SurfaceRemoved, ", "), res.Noun, ref, remaining))
		} else {
			lines = append(lines, fmt.Sprintf("No requested surface paths were recorded on %s %s; %s (no change).",
				res.Noun, ref, remaining))
		}
	}
	if detach {
		if res.AttachmentChanged {
			lines = append(lines, fmt.Sprintf("%s %s detached from %s.", res.Noun, ref, gen.RefByID(tree, res.DetachedFrom)))
		} else {
			lines = append(lines, fmt.Sprintf("%s %s was not attached (no change).", res.Noun, ref))
		}
	}
	return emitSuccess(cmd, "unlink", view, strings.Join(lines, " "))
}

func dependencyCount(count int) string {
	if count == 1 {
		return "1 dependency"
	}
	return fmt.Sprintf("%d dependencies", count)
}

// reportDependencyChangeError makes the shared Link/Unlink lifecycle refusal a
// blocked business answer. All unrelated failures keep their ordinary error
// envelope and exit classification.
func reportDependencyChangeError(cmd *cobra.Command, command string, err error) error {
	var locked *mutate.DependencyChangeLockedError
	if !errors.As(err, &locked) {
		return emitFailure(cmd, command, err)
	}
	detail := fmt.Sprintf("dependencies on %s %s cannot change in %s. Only an added edge from a started task to a done task is allowed; amendments cannot change dependencies.",
		locked.Noun, gen.RefParts(locked.Key, locked.ID), locked.Status)
	view := dependencyChangeLockedView{
		Noun: locked.Noun, ID: locked.ID, Key: locked.Key, Status: locked.Status,
		Reason: locked.Reason, Detail: detail,
	}
	human := "Blocked: " + detail
	return emitReport(cmd, command, false, view, human, 1)
}
