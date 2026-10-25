package cli

// Link's dependency, surface, and attachment axes share one source
// transaction planned in core; no noun supports both surface and attach.

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// linkView is the SUCCESS payload of link: the node plus whichever of
// depends_on, surface, and attached_to this invocation recorded. Each list rides only when
// its own flag ran this call; a caller wanting the node's full state either
// way reads `tiller show`, which link's envelope was never meant to
// duplicate.
type linkView struct {
	Noun       string   `json:"noun"`
	ID         string   `json:"id"`
	Key        string   `json:"key,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
	Surface    []string `json:"surface,omitempty"`
	AttachedTo string   `json:"attached_to,omitempty"`
	Changed    bool     `json:"changed"`
}

func newLinkCmd() *cobra.Command {
	var dependsOn, surface, describes, from, blocks []string
	var attach string
	cmd := &cobra.Command{
		Use:   "link <noun> <node> [--depends-on <target>] [--surface <path>] [--attach <target>] [--describes <path>] [--from <path>] [--blocks <target>]",
		Short: "Record a dependency, code surface, attachment, design baseline link, or question hold.",
		Long: "link records structural facts a node can learn after add time, each\n" +
			"additively. Dependencies and surfaces commit together. --depends-on unions\n" +
			"targets into a node's depends_on, the graph every\n" +
			"sibling list is ordered by: for an epic or a feature a target names an\n" +
			"epic or a feature, for a task it names a sibling task under the same\n" +
			"feature, for a bug, chore, or tweak, other bugs, chores, and tweaks. --surface unions\n" +
			"repo-relative paths into an epic's or a feature's surface.paths, the\n" +
			"existing code it touches; surface.paths has no other writer. --attach\n" +
			"records the epic, feature, or task a bug, chore, or tweak was found in, pure\n" +
			"provenance. Dependencies may accompany either surface or attachment, and\n" +
			"every requested fact validates before any is written;\n" +
			"surface and attachment apply to different nouns. Targets and paths\n" +
			"are repeatable. Every surface must exist without symlink components\n" +
			"and must not overlap reference code, including its containing directories.\n" +
			"Repeating an existing legal relationship is an idempotent no-op success.\n" +
			"link design <doc> records literal, repeatable --describes code paths and --from source paths.\n" +
			"Design docs are relative to .tiller/design; --from records recoverable source blobs at project HEAD.\n" +
			"link question <Q-id> --blocks <target> widens what an open question holds: it unions\n" +
			"repeatable epic, feature, or task targets (id or key) into the question's blocks, every\n" +
			"target resolved before anything is written. --blocks is the only axis a question takes.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == store.NounDesign {
				return runDesignLink(cmd, "link", mutate.DesignLinkInput{Doc: args[1], Describes: describes, From: from})
			}
			if err := rejectQuestionAxisMix(cmd, "link", isQuestionNoun(args[0])); err != nil {
				return err
			}
			if isQuestionNoun(args[0]) {
				return runQuestionLink(cmd, args[1], blocks)
			}
			if err := rejectDesignOnlyFlags(cmd, "link"); err != nil {
				return err
			}
			return runLink(cmd, args[0], args[1], dependsOn, surface, attach)
		},
	}
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "the work depended on (epic or feature; a sibling task for a task; other bugs, chores, and tweaks for a bug, chore, or tweak; id or key, repeatable)")
	cmd.Flags().StringArrayVar(&surface, "surface", nil, "a repo-relative path of existing code touched (epic or feature only, repeatable)")
	cmd.Flags().StringVar(&attach, "attach", "", "bug|chore|tweak only: the epic, feature, or task this was found in (id or key; provenance only)")
	cmd.Flags().StringArrayVar(&describes, "describes", nil, "design only: an existing path under a declared code root (literal, repeatable)")
	cmd.Flags().StringArrayVar(&from, "from", nil, "design only: a source file to record at project HEAD (literal, repeatable)")
	cmd.Flags().StringArrayVar(&blocks, "blocks", nil, "question only: an epic, feature, or task this question holds (id or key, repeatable)")
	addContributorFlag(cmd)
	return cmd
}

// refuseBlankAttach rejects an explicitly passed blank --attach as the
// attach_target usage error. A blank target is malformed, not an absent flag:
// read as absent it would refuse with the wrong reason when alone and silently
// drop the attachment beside other axes. The core treats a whitespace target
// the same way. Omitting --attach is not refused here.
func refuseBlankAttach(cmd *cobra.Command, command, attach string) error {
	if cmd.Flags().Changed("attach") && strings.TrimSpace(attach) == "" {
		return emitFailure(cmd, command, &usageError{
			reason: "attach_target",
			msg:    "--attach needs an epic, feature, or task",
		})
	}
	return nil
}

func runLink(cmd *cobra.Command, noun, node string, dependsOn, surface []string, attach string) error {
	sing, err := validNoun(cmd, "link", noun, []string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}
	if err := refuseBlankAttach(cmd, "link", attach); err != nil {
		return err
	}
	// No flag is cobra-required now that any one is individually sufficient;
	// link still refuses doing nothing, the same misuse a single required
	// --depends-on used to catch.
	if len(dependsOn) == 0 && len(surface) == 0 && attach == "" {
		return emitFailure(cmd, "link", &usageError{
			reason: "link_target",
			msg:    "link needs --depends-on, --surface, --attach, or a combination",
		})
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "link", err)
	}
	// One owner resolution serves the whole call, so a corrupt identity file
	// refuses here before the combined mutation plans anything.
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "link", err)
	}
	if len(surface) > 0 && attach != "" {
		return emitFailure(cmd, "link", &usageError{
			reason: "surface_target",
			msg:    "--surface applies to epics and features; --attach applies to bugs, chores, and tweaks, so they cannot be combined",
		})
	}

	res, err := mutate.Link(tree, d, mutate.LinkInput{Noun: sing, Node: node, DependsOn: dependsOn, Surface: surface, Attach: attach})
	if err != nil {
		return reportDependencyChangeError(cmd, "link", err)
	}
	view := linkView{Noun: sing, ID: res.ID, Key: res.Key, Changed: res.Changed}
	ref := gen.RefParts(res.Key, res.ID)
	var lines []string
	if len(dependsOn) > 0 {
		view.DependsOn = nonNilStrings(res.DependsOn)
		depsList := strings.Join(refsByID(tree, res.DependsOn), ", ")
		if res.DependenciesChanged {
			lines = append(lines, fmt.Sprintf("%s %s now depends on %s.", res.Noun, ref, depsList))
		} else {
			lines = append(lines, fmt.Sprintf("%s %s already depends on %s (no change).", res.Noun, ref, depsList))
		}
	}
	if len(surface) > 0 {
		view.Surface = nonNilStrings(res.Surface)
		pathList := strings.Join(res.Surface, ", ")
		if res.SurfaceChanged {
			lines = append(lines, fmt.Sprintf("%s %s surface now includes %s.", res.Noun, ref, pathList))
		} else {
			lines = append(lines, fmt.Sprintf("%s %s surface already includes %s (no change).", res.Noun, ref, pathList))
		}
	}
	if attach != "" {
		view.AttachedTo = res.AttachedTo
		if res.AttachmentChanged {
			lines = append(lines, fmt.Sprintf("%s %s now attached to %s.", res.Noun, ref, gen.RefByID(tree, res.AttachedTo)))
		} else {
			lines = append(lines, fmt.Sprintf("%s %s already attached to %s (no change).", res.Noun, ref, gen.RefByID(tree, res.AttachedTo)))
		}
	}

	return emitSuccess(cmd, "link", view, strings.Join(lines, " "))
}
