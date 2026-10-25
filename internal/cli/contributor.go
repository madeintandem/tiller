package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// Registration and retirement are deliberate human-gated acts. Join selects
// among the registered contributors; claims say who currently holds work.
// An agent changes the roster only on explicit human instruction naming the
// id and, for registration, the display name.

// contributorAddResult reports the registered contributor and resulting roster.
type contributorAddResult struct {
	Contributor ContributorInfo   `json:"contributor"`
	Roster      []ContributorInfo `json:"roster"`
	Changed     bool              `json:"changed"`
}

// contributorRemoveResult reports the retired contributor and remaining roster.
type contributorRemoveResult struct {
	Removed ContributorInfo   `json:"removed"`
	Roster  []ContributorInfo `json:"roster"`
}

// ContributorRemoveSelfData is the BLOCKED data block for remove contributor's own
// no-self-lockout refusal (mutate.RemoveContributorSelfError).
type ContributorRemoveSelfData struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ContributorRemoveHoldingsData is the BLOCKED data block for remove contributor's
// census refusal: unfinished execution assignments and open questions.
type ContributorRemoveHoldingsData struct {
	ID        string `json:"id"`
	Reason    string `json:"reason"`
	Tasks     int    `json:"tasks"`
	Bugs      int    `json:"bugs"`
	Chores    int    `json:"chores"`
	Tweaks    int    `json:"tweaks"`
	Questions int    `json:"questions"`
}

func newContributorAddCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "contributor <id>",
		Short: "Register a person onto the contributors roster.",
		Long: "add contributor puts a person on this project's committed contributors roster\n" +
			"(tiller.yml). It is the only command that does: `tiller join` only\n" +
			"selects among the contributors already registered here. It is the human\n" +
			"registration gate: an agent runs it only on an explicit human\n" +
			"instruction naming both the id and the display name, echoed back, the\n" +
			"same delegation contract `approve` carries.\n" +
			"Re-adding an id with the same --name is a no-op that says so; a\n" +
			"different --name updates the display name. Both <id> and --name are\n" +
			"required and must be non-blank.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContributorAdd(cmd, args[0], name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "the person's display name (required)")
	return cmd
}

func runContributorAdd(cmd *cobra.Command, id, name string) error {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" {
		return emitFailure(cmd, "add contributor", &usageError{
			reason: "blank_contributor_id",
			msg:    "tiller add contributor needs a non-blank <id>",
		})
	}
	if name == "" {
		return emitFailure(cmd, "add contributor", &usageError{
			reason: "blank_contributor_name",
			msg:    "tiller add contributor needs a non-blank --name \"<display>\"",
		})
	}

	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "add contributor", err)
	}

	res, err := mutate.AddContributor(tree, mutate.Deps{}, store.Contributor{ID: id, Name: name})
	if err != nil {
		return emitFailure(cmd, "add contributor", err)
	}

	result := contributorAddResult{
		Contributor: ContributorInfo{ID: id, Name: name},
		Roster:      contributorInfos(res.Roster),
		Changed:     res.Outcome != mutate.ContributorAddNoChange,
	}
	return emitSuccess(cmd, "add contributor", result, renderContributorAddHuman(result, res.Outcome))
}

// renderContributorAddHuman is add contributor's summary, in the CLI-wide output grammar
// (see tty.go): a headline naming what happened (registered, updated, or the
// no-op told apart from a real write), then the roster it echoes so the human
// reading it sees the result, not just the delta.
func renderContributorAddHuman(r contributorAddResult, outcome mutate.ContributorAddOutcome) string {
	var b strings.Builder
	switch outcome {
	case mutate.ContributorAddRegistered:
		fmt.Fprintf(&b, "Registered %s.\n", contributorLabel(r.Contributor))
	case mutate.ContributorAddNameUpdated:
		fmt.Fprintf(&b, "Updated %s's name to %q.\n", r.Contributor.ID, r.Contributor.Name)
	default:
		fmt.Fprintf(&b, "%s is already on the roster as %q; no change.\n", r.Contributor.ID, r.Contributor.Name)
	}
	fmt.Fprintf(&b, "Roster (%d contributor%s): ", len(r.Roster), plural(len(r.Roster)))
	labels := make([]string, len(r.Roster))
	for i, o := range r.Roster {
		labels[i] = contributorLabel(o)
	}
	b.WriteString(strings.Join(labels, ", "))
	return b.String()
}

func newContributorRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contributor <id>",
		Short: "Retire a person from the contributors roster.",
		Long: "remove contributor retires id from this project's committed contributors\n" +
			"roster. Historical references (events, gate results, attestations,\n" +
			"done nodes) keep the id forever; only the roster row goes. It\n" +
			"refuses removing the acting contributor (no self-lockout: another\n" +
			"registered contributor removes you, or you join as someone else first),\n" +
			"and it refuses an id that owns an unfinished task, bug, chore,\n" +
			"or open question. Release or reassign reservations before start.\n" +
			"Started work must finish or be explicitly transferred; its original\n" +
			"owner remains in history. Answer open questions before retirement.\n" +
			"Who may run: human, the same delegation contract as add contributor. An\n" +
			"agent runs this only on an explicit human instruction naming the id.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContributorRemove(cmd, args[0])
		},
	}
	addContributorFlag(cmd)
	return cmd
}

func runContributorRemove(cmd *cobra.Command, id string) error {
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "remove contributor", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "remove contributor", err)
	}

	res, err := mutate.RemoveContributor(tree, d, id)
	if err != nil {
		return reportContributorRemoveError(cmd, err)
	}

	result := contributorRemoveResult{
		Removed: ContributorInfo{ID: res.Removed.ID, Name: res.Removed.Name},
		Roster:  contributorInfos(res.Roster),
	}
	return emitSuccess(cmd, "remove contributor", result, renderContributorRemoveHuman(result))
}

// reportContributorRemoveError maps remove contributor's two typed blocked answers onto
// exit-1 ok:false data blocks, the same stable-reason-token-plus-fields
// idiom transitionBlockedView keeps for the claim family (lifecycle_cmds.go)
// and every other blocked answer in this package. Kept local to contributor.go
// rather than folded into transitionBlockedView because these two errors
// have nothing to do with a node transition and reach the CLI through no
// other path; anything transitionBlockedView does not recognize (a bad
// contributor, a write fault) falls through to emitFailure exactly as
// reportTransitionError's own callers already expect.
func reportContributorRemoveError(cmd *cobra.Command, err error) error {
	var self *mutate.RemoveContributorSelfError
	if errors.As(err, &self) {
		d := ContributorRemoveSelfData{ID: self.ID, Reason: self.Reason}
		return emitReport(cmd, "remove contributor", false, d,
			fmt.Sprintf("Blocked: %s cannot remove themselves from the roster; another registered contributor removes them, or they join as someone else first.", self.ID),
			1)
	}
	var held *mutate.RemoveContributorHoldingsError
	if errors.As(err, &held) {
		h := held.Holdings
		d := ContributorRemoveHoldingsData{ID: held.ID, Reason: held.Reason, Tasks: h.Tasks,
			Bugs: h.Bugs, Chores: h.Chores, Tweaks: h.Tweaks, Questions: h.Questions}
		return emitReport(cmd, "remove contributor", false, d,
			fmt.Sprintf("Blocked: %s still holds %d live task(s), %d live bug(s), %d live chore(s), %d live tweak(s), and %d open question(s); "+
				"release or reassign unstarted reservations, finish or explicitly transfer started work "+
				"(`tiller transfer task|bug|chore|tweak <node> --to <owner> --reason \"<why>\"`), "+
				"and answer open questions (`tiller answer question <Q-id> --answer \"<answer>\"`).",
				held.ID, h.Tasks, h.Bugs, h.Chores, h.Tweaks, h.Questions),
			1)
	}
	return emitFailure(cmd, "remove contributor", err)
}

// renderContributorRemoveHuman is remove contributor's summary: who was retired, then the
// roster that remains, the same shape renderContributorAddHuman already keeps.
func renderContributorRemoveHuman(r contributorRemoveResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Removed %s.\n", contributorLabel(r.Removed))
	fmt.Fprintf(&b, "Roster (%d contributor%s): ", len(r.Roster), plural(len(r.Roster)))
	labels := make([]string, len(r.Roster))
	for i, o := range r.Roster {
		labels[i] = contributorLabel(o)
	}
	b.WriteString(strings.Join(labels, ", "))
	return b.String()
}
