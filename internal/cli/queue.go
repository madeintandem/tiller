package cli

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/spf13/cobra"
)

// queueFlags narrow the existing action queue. Validation belongs to this CLI
// boundary; views.FilterQueue remains a pure filter over an admitted Queue.
type queueFlags struct {
	kind string
	noun string
}

var queueNounOrder = []string{
	store.NounAmendment, store.NounEpic, store.NounFeature, store.NounTask,
	store.NounBug, store.NounChore, store.NounTweak, store.NounQuestion,
}

func newQueueCmd() *cobra.Command {
	var flags queueFlags
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "List every current action-queue entry.",
		Long: "queue prints the current read-only action queue in its established order.\n" +
			"--kind and --noun narrow those existing entries; they do not change which\n" +
			"work is eligible for the queue.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runQueue(cmd, flags)
		},
	}
	cmd.Flags().StringVar(&flags.kind, "kind", "", "show only entries of this queue kind")
	cmd.Flags().StringVar(&flags.noun, "noun", "", "show only entries for this noun")
	return cmd
}

func runQueue(cmd *cobra.Command, flags queueFlags) error {
	if flags.kind != "" && !queueFilterValue(views.QueueKinds, flags.kind) {
		return emitFailure(cmd, "queue", &usageError{
			reason: "unknown_queue_kind",
			msg:    fmt.Sprintf("unknown queue kind %q (want one of %s)", flags.kind, strings.Join(views.QueueKinds, ", ")),
		})
	}
	if flags.noun != "" && !queueFilterValue(queueNounOrder, flags.noun) {
		return emitFailure(cmd, "queue", &usageError{
			reason: "unknown_queue_noun",
			msg:    fmt.Sprintf("unknown queue noun %q (want one of %s)", flags.noun, strings.Join(queueNounOrder, ", ")),
		})
	}

	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "queue", err)
	}
	queue := views.QueueOfInput(tree, views.QueueInput{Workflow: readinput.Workflow(tree)})
	queue = views.FilterQueue(queue, views.QueueFilter{Kind: flags.kind, Noun: flags.noun})
	return emitSuccess(cmd, "queue", queue, renderQueueHuman(queue))
}

func queueFilterValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// renderQueueHuman keeps the granular queue projection legible without
// inventing another model. Each row carries its category, reference, available
// review context, and exact mutation command; multiple gates on one node remain
// distinct because their gate names remain distinct rows.
func renderQueueHuman(queue views.Queue) string {
	if len(queue.Entries) == 0 {
		return "Nothing needs you right now."
	}

	var out strings.Builder
	for i, entry := range queue.Entries {
		if i > 0 {
			out.WriteByte('\n')
		}
		parts := []string{entry.Kind, entry.Ref}
		if entry.Prose != "" {
			parts = append(parts, entry.Prose)
		}
		if entry.Due != "" {
			parts = append(parts, "due "+entry.Due)
		}
		if entry.Gate != "" {
			gate := "gate " + entry.Gate
			if entry.GateStatus != "" {
				gate += " (" + entry.GateStatus + ")"
			}
			parts = append(parts, gate)
		}
		fmt.Fprintf(&out, "- %s · `%s`", strings.Join(parts, " · "), entry.Command)
	}
	return out.String()
}
