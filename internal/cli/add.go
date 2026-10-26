package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// This file implements `tiller add epic|feature|task`, the structural verbs the
// decompose skill uses to lay a draft tree down (cli-surface.md, Decomposition),
// plus `tiller add bug|chore|tweak`, which proposes a shelf node instead
// (docs/spec/cli-surface.md). It is a thin shell: it validates the address
// shape, resolves the acting contributor, hands a clean request to
// internal/core/mutate, and renders the new-node envelope. All reasoning,
// minting, file writing, and event recording live in the core.

// validChangeTypes is the set the create/attach grammar accepts for a feature
// (artifact-schema.md, change_type). Greenfield only ever uses create; the
// others are validated here and gated against the archetype in a later task.
var validChangeTypes = map[string]bool{
	"create":  true,
	"modify":  true,
	"replace": true,
	"migrate": true,
}

// nodeResult is the --json payload of a successful add: the resulting node state
// the agent reads back, plus whether this call created it. It is the stable
// new-node envelope shape later read commands echo. Fields that do not apply to
// a noun are omitted (a task has no change_type, an epic no feature).
type nodeResult struct {
	Noun       string   `json:"noun"`
	ID         string   `json:"id"`
	Key        string   `json:"key,omitempty"`
	Title      string   `json:"title,omitempty"`
	Status     string   `json:"status"`
	ChangeType string   `json:"change_type,omitempty"`
	Source     string   `json:"source,omitempty"`
	Feature    string   `json:"feature,omitempty"`
	AttachedTo string   `json:"attached_to,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
	Path       string   `json:"path"`
	Created    bool     `json:"created"`
}

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register a contributor or create a work node.",
		Long: "add contributor registers a person on the project's roster, on explicit human instruction.\n" +
			"Other add commands lay a node down in its initial state: an epic or feature in draft\n" +
			"with source provenance, a task proposed in its own folder under a\n" +
			"feature, an open question on the decisions ledger, or an amendment\n" +
			"opened as a proposed staging document for a reshape.\n" +
			"The agent proposes; the human approves later. Re-adding the same epic,\n" +
			"feature, task, or question is an idempotent no-op success; an amendment\n" +
			"is the one exception, since a second open proposal under one key is not\n" +
			"the same proposal as the first (see `tiller add amendment --help`).",
		// With no subcommand, print help and exit cleanly.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newContributorAddCmd(), newAddEpicCmd(), newAddFeatureCmd(), newAddTaskCmd(), newAddQuestionCmd(), newAddAmendmentCmd(), newAddBugCmd(), newAddChoreCmd(), newAddTweakCmd())
	return cmd
}

func newAddEpicCmd() *cobra.Command {
	var title, source string
	var dependsOn []string
	cmd := &cobra.Command{
		Use:   "epic <key>",
		Short: "Create an epic in draft.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddEpic(cmd, args[0], title, source, dependsOn)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "human-readable epic title")
	cmd.Flags().StringVar(&source, "source", "", "provenance: the input doc this epic was drawn from")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "work this epic depends on (epic or feature, id or key, repeatable)")
	addContributorFlag(cmd)
	return cmd
}

func newAddFeatureCmd() *cobra.Command {
	var title, changeType, source string
	var dependsOn []string
	cmd := &cobra.Command{
		Use:   "feature <epic-key>/<feature-key>",
		Short: "Create a feature in draft, nested under its epic.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddFeature(cmd, args[0], title, changeType, source, dependsOn)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "human-readable feature title")
	cmd.Flags().StringVar(&changeType, "change-type", "create", "create | modify | replace | migrate")
	cmd.Flags().StringVar(&source, "source", "", "provenance: the input doc this feature was drawn from")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "work this feature depends on (epic or feature, id or key, repeatable)")
	addContributorFlag(cmd)
	return cmd
}

func newAddTaskCmd() *cobra.Command {
	var title string
	var dependsOn []string
	cmd := &cobra.Command{
		Use:   "task <epic-key>/<feature-key>/<task-key>",
		Short: "Propose a task in its own folder under a feature.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddTask(cmd, args[0], title, dependsOn)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "what the task accomplishes (required)")
	_ = cmd.MarkFlagRequired("title")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "sibling tasks this one follows (id or canonical key, repeatable)")
	addContributorFlag(cmd)
	return cmd
}

func runAddEpic(cmd *cobra.Command, key, title, source string, dependsOn []string) error {
	if !keyRe.MatchString(key) {
		return emitFailure(cmd, "add", &usageError{
			reason: "invalid_key",
			msg:    fmt.Sprintf("epic key %q must match [a-z0-9]+(-[a-z0-9]+)*", key),
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
	res, err := mutate.AddEpic(tree, d, mutate.EpicInput{Key: key, Title: title, Source: source, DependsOn: dependsOn})
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	return emitSuccess(cmd, "add", payload(res), humanLine(res, title, gen.RefParts(res.Key, res.ID), "", refsByID(tree, res.DependsOn)))
}

func runAddFeature(cmd *cobra.Command, key, title, changeType, source string, dependsOn []string) error {
	if err := validateFeatureKey(key); err != nil {
		return emitFailure(cmd, "add", err)
	}
	if !validChangeTypes[changeType] {
		return emitFailure(cmd, "add", &usageError{
			reason: "invalid_change_type",
			msg:    fmt.Sprintf("change-type %q must be one of create, modify, replace, migrate", changeType),
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
	res, err := mutate.AddFeature(tree, d, mutate.FeatureInput{
		Key: key, Title: title, ChangeType: changeType, Source: source, DependsOn: dependsOn,
	})
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	return emitSuccess(cmd, "add", payload(res), humanLine(res, title, gen.RefParts(res.Key, res.ID), "", refsByID(tree, res.DependsOn)))
}

func runAddTask(cmd *cobra.Command, key, title string, dependsOn []string) error {
	if err := validateTaskKey(key); err != nil {
		return emitFailure(cmd, "add", err)
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	res, err := mutate.AddTask(tree, d, mutate.TaskInput{Key: key, Title: title, DependsOn: dependsOn})
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	return emitSuccess(cmd, "add", payload(res), humanLine(res, title, gen.RefParts(res.Key, res.ID), "", refsByID(tree, res.DependsOn)))
}

func newAddBugCmd() *cobra.Command   { return newAddWorkItemCmd(store.NounBug) }
func newAddChoreCmd() *cobra.Command { return newAddWorkItemCmd(store.NounChore) }
func newAddTweakCmd() *cobra.Command { return newAddWorkItemCmd(store.NounTweak) }

// newAddWorkItemCmd builds `add bug <key>` and `add chore <key>`: a proposed
// shelf node with its contract stub. The key is the local segment alone; the
// shelf prefix (bugs/, chores/) is implied by the noun. --attach names the
// epic, feature, or task it was found in, provenance only; --source is the
// free-text citation amend already uses; --depends-on names bugs, chores, and tweaks.
func newAddWorkItemCmd(noun string) *cobra.Command {
	var title, attach, source string
	var dependsOn []string
	cmd := &cobra.Command{
		Use:     noun + " <key>",
		Aliases: []string{store.ShelfDir(noun)},
		Short:   fmt.Sprintf("Propose a %s on its shelf (.tiller/%s/<key>/).", noun, store.ShelfDir(noun)),
		Long: fmt.Sprintf("add %s creates .tiller/%s/<key>/ holding %s (the contract to write\n"+
			"before a human triages it with approve) and %s (state). The key is one\n"+
			"local segment; the canonical key is %s/<key>. --attach records where it\n"+
			"was found (an epic, feature, or task; id or key); it is provenance and\n"+
			"never gates. --depends-on names other bugs, chores, or tweaks only.\n"+
			"Who may run: both; the agent proposes, the human approves.",
			noun, store.ShelfDir(noun), store.ShelfProseFile(noun), store.ShelfStateFile(noun), store.ShelfDir(noun)),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddWorkItem(cmd, noun, args[0], title, attach, source, dependsOn)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "display title (required)")
	_ = cmd.MarkFlagRequired("title")
	cmd.Flags().StringVar(&attach, "attach", "", "the epic, feature, or task this was found in (id or key; provenance only)")
	cmd.Flags().StringVar(&source, "source", "", "citation to the report or document this came from (path#Lstart-Lend by convention)")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "bugs, chores, or tweaks this one follows (id or shelf key, repeatable)")
	addContributorFlag(cmd)
	return cmd
}

func runAddWorkItem(cmd *cobra.Command, noun, key, title, attach, source string, dependsOn []string) error {
	if strings.Contains(key, "/") || !keyRe.MatchString(key) {
		return emitFailure(cmd, "add", &usageError{reason: "invalid_key",
			msg: fmt.Sprintf("a %s key is one local segment (%s); the %s/ prefix is implied", noun, keyRe.String(), store.ShelfDir(noun))})
	}
	if err := refuseBlankAttach(cmd, "add", attach); err != nil {
		return err
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	res, err := mutate.AddWorkItem(tree, d, mutate.WorkItemInput{
		Noun: noun, Key: key, Title: title, AttachedTo: attach, Source: source, DependsOn: dependsOn,
	})
	if err != nil {
		return emitFailure(cmd, "add", err)
	}
	ref := gen.RefParts(res.Key, res.ID)
	return emitSuccess(cmd, "add", payload(res), humanLine(res, title, ref, gen.RefByID(tree, res.AttachedTo), refsByID(tree, res.DependsOn)))
}

// validateFeatureKey enforces the canonical <epic-key>/<feature-key> shape so a
// malformed address fails as misuse before any work.
func validateFeatureKey(raw string) error {
	parts := strings.Split(raw, "/")
	if len(parts) != 2 || !keyRe.MatchString(parts[0]) || !keyRe.MatchString(parts[1]) {
		return &usageError{
			reason: "invalid_key",
			msg:    fmt.Sprintf("feature key %q must be <epic-key>/<feature-key>", raw),
		}
	}
	return nil
}

// validateTaskKey enforces the canonical <epic-key>/<feature-key>/<task-key>
// shape so a malformed address fails as misuse before any work.
func validateTaskKey(raw string) error {
	parts := strings.Split(raw, "/")
	if len(parts) != 3 || !keyRe.MatchString(parts[0]) || !keyRe.MatchString(parts[1]) || !keyRe.MatchString(parts[2]) {
		return &usageError{
			reason: "invalid_key",
			msg:    fmt.Sprintf("task key %q must be <epic-key>/<feature-key>/<task-key>", raw),
		}
	}
	return nil
}

// payload maps a core Result onto the stable JSON envelope payload.
func payload(r mutate.Result) nodeResult {
	return nodeResult{
		Noun:       r.Noun,
		ID:         r.ID,
		Key:        r.Key,
		Title:      r.Title,
		Status:     r.Status,
		ChangeType: r.ChangeType,
		Source:     r.Source,
		Feature:    r.Feature,
		AttachedTo: r.AttachedTo,
		DependsOn:  r.DependsOn,
		Path:       r.Path,
		Created:    r.Created,
	}
}

// humanLine is the plain-text confirmation for a terminal reader. The node is
// named by the shared reference convention; ref is the caller-computed
// "<key-path> (<id>)". A no-op whose requested nonblank title differs from the
// stored title points to the explicit display-title mutation without changing
// add's JSON result. A recorded dependency graph rides the ordinary one-line
// confirmation, and a task's or a shelf node's line echoes its stored title; a
// bug, chore, or tweak that carries an attachment also names it, through
// attachedRef, the caller-computed reference for it (gen.RefByID), so the
// line reads "attached to form-engine/loading (F-79dh3q)" like every other
// human reference rather than printing the bare stored id.
func humanLine(r mutate.Result, requestedTitle, ref, attachedRef string, depsRefs []string) string {
	if !r.Created && strings.TrimSpace(requestedTitle) != "" && requestedTitle != r.Title {
		return fmt.Sprintf("Found existing %s %s in %s; unchanged.\n"+
			"To change its display title:\n  tiller rename %s %s --title %s",
			r.Noun, ref, r.Status, r.Noun, r.ID, quoteShellArg(requestedTitle))
	}
	verb := "Created"
	if !r.Created {
		verb = "Found existing"
	}
	line := fmt.Sprintf("%s %s %s in %s", verb, r.Noun, ref, r.Status)
	// A task's title closes the identity clause, so the dependency clause
	// follows it rather than splitting the node from its own title. A line that
	// ENDS on the quoted title takes no period, which is the shape a task
	// confirmation has always had; every other line is a sentence and ends
	// like one.
	if (r.Noun == store.NounTask || store.IsWorkNoun(r.Noun)) && r.Title != "" {
		line += fmt.Sprintf(": %q", r.Title)
		if attachedRef != "" {
			line += fmt.Sprintf(" (attached to %s)", attachedRef)
		}
		if len(depsRefs) == 0 {
			return line
		}
	}
	if len(depsRefs) > 0 {
		return line + "; depends on " + strings.Join(depsRefs, ", ") + "."
	}
	return line + "."
}

// quoteShellArg renders one literal POSIX shell argument. Single quotes stop
// command, backtick, variable, glob, and whitespace evaluation; an embedded
// apostrophe closes the quote, emits one quoted apostrophe, and reopens it.
func quoteShellArg(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

// refsByID renders a list of node ids as human references through the loaded
// tree.
func refsByID(tree *store.Tree, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	refs := make([]string, len(ids))
	for i, id := range ids {
		refs[i] = gen.RefByID(tree, id)
	}
	return refs
}

// loadTree finds the repo root and loads the tree, returning the canonical
// environment error (exit 3) when the working directory is not a Tiller repo.
func loadTree(cmd *cobra.Command) (*store.Tree, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	return store.Load(root)
}

// addContributorFlag wires the --contributor flag every mutating command shares. The acting
// owner falls back to TILLER_CONTRIBUTOR, then to the checkout's .tiller/identity.yml,
// and the core refuses (exit 3) if none resolves to a project owner.
func addContributorFlag(cmd *cobra.Command) {
	cmd.Flags().String("contributor", "", "acting contributor id (falls back to TILLER_CONTRIBUTOR, then .tiller/identity.yml)")
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations["acting-contributor"] = "true"
}

// Acting-owner attribution sources, the exact strings design doc's "The
// fallback speaks" names. contributorSourceFlag never itself appears in a printed
// echo (actorEcho, envelope.go): an explicit --contributor is already on the
// command line the user typed, so only the two fallbacks are ever spoken.
const (
	contributorSourceFlag     = "--contributor"
	contributorSourceEnv      = "TILLER_CONTRIBUTOR"
	contributorSourceIdentity = ".tiller/identity.yml"
)

// resolveContributorFallback owns the env-then-identity fallback tail every acting-
// owner chain shares once its own leading source is empty (the --contributor flag
// on a mutation, --as on join): TILLER_CONTRIBUTOR, then this checkout's
// .tiller/identity.yml. It returns the id the fallback resolved (empty when
// neither source has one) and which of the two sources supplied it.
// store.ReadLocalIdentity returns a real error for a present-but-malformed
// file (its own doc comment: "a real error the caller surfaces"); this
// function surfaces that error too rather than swallowing it, so each of its
// callers can decide for itself whether a corrupt file refuses the command
// (a mutation, join) or is quietly degraded (a read surface) instead of
// masquerading as an absent one.
//
// The env branch returns the trimmed id, not the raw environment value:
// resolveJoinContributor (join.go) already trims its own copy of this
// same TILLER_CONTRIBUTOR check, and store.ResolveContributor trims again before it ever
// looks the id up on the roster, so a padded TILLER_CONTRIBUTOR was never stamped
// wrong. It was only ever echoed wrong: the actor line and the JSON actor id
// (envelope.go's resolveActor) carried the raw padded value because this was
// the one place in the chain that checked TrimSpace but returned v instead of
// it. Trimming here means every consumer of this tail, join and every
// mutation alike, agrees on the same id.
func resolveContributorFallback(root string) (id, source string, err error) {
	if v := strings.TrimSpace(os.Getenv("TILLER_CONTRIBUTOR")); v != "" {
		return v, contributorSourceEnv, nil
	}
	id, err = store.ReadLocalIdentity(root)
	if err != nil {
		return "", contributorSourceIdentity, err
	}
	return id, contributorSourceIdentity, nil
}

// resolveActingContributorWithSource is resolveActingContributor's sibling: the identical
// precedence chain, plus which of the three sources actually won. A mutation
// itself stays plumbed through the single id resolveActingContributor returns
// (mutate.Deps carries no source field; attribution is a display truth, not
// tree state, design doc's "What this deliberately does not do"). This is
// what the envelope layer (envelope.go's resolveActor) calls to speak a
// fallback resolution aloud, and what guide's route and readout's actor line
// call directly: both are read surfaces, so a present-but-corrupt identity
// file degrades to "no id" here exactly as an absent one does, rather than
// refusing a command that has nothing to mutate. resolveActingContributorOrRefuse
// below is the sibling that a mutation calls instead, precisely because it
// must not degrade the same way.
func resolveActingContributorWithSource(cmd *cobra.Command, root string) (id, source string) {
	if v, _ := cmd.Flags().GetString("contributor"); strings.TrimSpace(v) != "" {
		return v, contributorSourceFlag
	}
	id, source, _ = resolveContributorFallback(root)
	return id, source
}

// resolveActingContributor resolves who a mutation attributes to, self-contained in the
// repo: the --contributor flag, then TILLER_CONTRIBUTOR, then the gitignored
// .tiller/identity.yml that `tiller join` wrote on this checkout. The flag and env
// still override the file, so CI and one-off attribution keep working. It returns
// "" when none resolves, which store.ResolveContributor turns into the refusal that
// points at `tiller join`. It is the read-surface caller (guide's routing,
// readout, a gate's own post-mutation readout block and human line): each of
// those already runs after the actual mutation resolved its owner through
// resolveActingContributorOrRefuse and either refused or succeeded, so a corrupt
// file reaching here degrades to "" rather than refusing a second time.
func resolveActingContributor(cmd *cobra.Command, root string) string {
	id, _ := resolveActingContributorWithSource(cmd, root)
	return id
}

// resolveActingContributorOrRefuse is deps' own resolution: the same --contributor,
// TILLER_CONTRIBUTOR, identity-file precedence resolveActingContributor walks, except a
// present-but-corrupt .tiller/identity.yml is not swallowed into "no id"
// here. Explicit --contributor or TILLER_CONTRIBUTOR never consult the file at all, so a
// corrupt one is irrelevant on either of those paths; only once both are
// empty does the file's own read error surface, as the named
// identity_unreadable refusal (exit 3) rather than falling through to
// contributor_unset and masquerading corruption as an un-joined checkout (issue
// #52).
func resolveActingContributorOrRefuse(cmd *cobra.Command, root string) (string, error) {
	if v, _ := cmd.Flags().GetString("contributor"); strings.TrimSpace(v) != "" {
		return v, nil
	}
	id, _, err := resolveContributorFallback(root)
	if err != nil {
		return "", identityUnreadableError(err)
	}
	return id, nil
}

// identityUnreadableError is the exit-3 refusal every mutating command and
// bare `tiller join` raise when the flag and env are both empty and this
// checkout's .tiller/identity.yml is present but cannot be read: a corrupt
// file is a real problem, not an absent one, and running the same command
// again on the same corrupt bytes cannot fix it, so the message names the
// file and points at the one command that rewrites it. cause is
// store.ReadLocalIdentity's own error (a parse failure, but also a
// permission-denied or a directory sitting at the path), which already
// wraps the absolute path; the message here names the file by its short,
// canonical reference (contributorSourceIdentity, the same string the fallback
// echo uses) rather than repeating the path a second time. A legacy owner key
// already names its own remedy, the in-place rename, so it gets no second one.
func identityUnreadableError(cause error) error {
	remedy := ": run `tiller join --as <id>` to rewrite it"
	var refusal *store.EnvError
	if errors.As(cause, &refusal) && refusal.Reason == store.ReasonObsoleteIdentity {
		remedy = ""
	}
	return &store.EnvError{
		Code:   3,
		Reason: store.ReasonIdentityUnreadable,
		Msg:    fmt.Sprintf("%s cannot be read (%v)%s", contributorSourceIdentity, cause, remedy),
	}
}

// deps builds the core mutation dependencies from the command: the acting contributor
// resolved against the repo root, with the system clock and id minter defaulted
// by the core. It is the sole point every mutating command resolves its owner
// through, so it is also the sole point that refuses a present-but-corrupt
// identity file (resolveActingContributorOrRefuse) rather than any of the twenty-odd
// call sites doing so independently.
func deps(cmd *cobra.Command, root string) (mutate.Deps, error) {
	id, err := resolveActingContributorOrRefuse(cmd, root)
	if err != nil {
		return mutate.Deps{}, err
	}
	return mutate.Deps{Contributor: id}, nil
}
