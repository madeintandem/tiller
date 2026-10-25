package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/skills"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

// Join explicitly selects checkout identity, then uses the same transactional
// artifact refresh as maintenance. Init remains a narrower project scaffold.
type joinResult struct {
	Harness               string            `json:"harness"`
	Contributor           ContributorInfo   `json:"contributor"`
	Roster                []ContributorInfo `json:"roster"`
	IdentityFile          string            `json:"identity_file"`
	Skills                []string          `json:"skills"`
	SkillsDir             string            `json:"skills_dir"`
	PointerFile           string            `json:"pointer_file"`
	RedirectFile          string            `json:"redirect_file"`
	Gitignore             string            `json:"gitignore"`
	OrientationGeneration int               `json:"orientation_generation"`
	Files                 []string          `json:"files"`
	ShimsRemoved          []string          `json:"shims_removed,omitempty"`
	Warnings              []string          `json:"warnings,omitempty"`
}

func newJoinCmd() *cobra.Command {
	var as string
	cmd := &cobra.Command{
		Use:   "join",
		Short: "Select your identity and wire this machine's coding agent to the project.",
		Long: "join selects who you are on this project, among the contributors a human has\n" +
			"already put on the roster with `tiller add contributor`, and wires this\n" +
			"machine's harness to it. It resolves the acting id from --as, then\n" +
			"TILLER_CONTRIBUTOR, then, when both are unset, this checkout's own recorded\n" +
			".tiller/identity.yml, so a bare `tiller join` on an already-joined\n" +
			"checkout re-joins as the identity already on file. join never registers:\n" +
			"an id absent from the roster is refused, whichever source supplied it. It\n" +
			"writes the gitignored .tiller/identity.yml that records who you are on\n" +
			"this checkout, then for Claude Code installs the Tiller skills as\n" +
			"discoverable shims that delegate to `tiller guide`, keeps those\n" +
			"per-machine shims out of git, writes the managed pointer block into\n" +
			"AGENTS.md, and adds a CLAUDE.md redirect to it so the block loads in\n" +
			"Claude Code sessions. It is idempotent: running it twice leaves the\n" +
			"identity file, shims, AGENTS.md, and CLAUDE.md byte-identical.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJoin(cmd, as)
		},
	}
	cmd.Flags().StringVar(&as, "as", "", "the registered contributor id to join as (falls back to TILLER_CONTRIBUTOR, then .tiller/identity.yml)")
	return cmd
}

func runJoin(cmd *cobra.Command, as string) error {
	root, err := repoRoot()
	if err != nil {
		return emitFailure(cmd, "join", err)
	}
	// Confirm this is a Tiller repo before writing anything; outside one this is
	// the canonical not_a_repo environment failure (exit 3), like the other
	// commands, and nothing is written.
	tree, err := store.Load(root)
	if err != nil {
		return emitFailure(cmd, "join", err)
	}

	owner, err := resolveJoinContributor(tree.Manifest, root, as)
	if err != nil {
		return emitFailure(cmd, "join", err)
	}

	// Record who I am on this checkout: the gitignored, per-checkout identity file.
	if err := store.WriteLocalIdentity(root, owner.ID); err != nil {
		return emitFailure(cmd, "join", err)
	}

	refreshed, err := refreshProject(root, false)
	if err != nil {
		return emitFailure(cmd, "join", err)
	}
	names := skills.Names()
	installed := make([]string, 0, len(names))
	for _, skill := range names {
		installed = append(installed, gen.SkillShimName(skill))
	}
	result := joinResult{
		Harness:               "claude-code",
		Contributor:           ContributorInfo{ID: owner.ID, Name: owner.Name},
		Roster:                contributorInfos(tree.Manifest.Contributors),
		IdentityFile:          filepath.Join(".tiller", "identity.yml"),
		Skills:                installed,
		SkillsDir:             filepath.Join(".claude", "skills"),
		PointerFile:           "AGENTS.md",
		RedirectFile:          "CLAUDE.md",
		Gitignore:             ".gitignore",
		OrientationGeneration: refreshed.After,
		Files:                 refreshed.Files,
		ShimsRemoved:          refreshed.ShimsRemoved,
		Warnings:              refreshed.Warnings,
	}
	return emitSuccess(cmd, "join", result, renderJoinHuman(result))
}

// resolveJoinContributor resolves the id join selects: --as, then TILLER_CONTRIBUTOR, then,
// when both are unset, the id already recorded in this checkout's
// .tiller/identity.yml, so a bare re-join on a joined checkout selects the same
// identity again rather than demanding --as every time. Whatever source supplies
// the id, it is looked up on m's committed roster through the same primitive
// every mutation already validates the acting contributor against
// (store.ResolveContributor): join has no path of its own onto the roster, so an id
// absent from it, including one read back from identity.yml, is refused
// (ReasonContributorUnknown). Only `tiller add contributor` may put a person on the
// roster; TILLER_CONTRIBUTOR, like --as, only ever selects among who is already
// there.
//
// Both refusal paths replace store.ResolveContributor's own message with text join
// owes its own caller: that shared message is written for a mutation, whose
// fallback chain is --contributor then TILLER_CONTRIBUTOR then identity.yml, and whose
// fix is "run tiller join". join's chain is --as then TILLER_CONTRIBUTOR then
// identity.yml (no --contributor flag exists here), and the fix for a caller
// already running join can never be "run join". store.ResolveContributor's own
// text stays exactly as it is (design doc: "still correct post-split") for
// every mutation that shares it; this wrapping is local to join's surface.
func resolveJoinContributor(m *store.Manifest, root, as string) (store.Contributor, error) {
	id := strings.TrimSpace(as)
	if id == "" {
		id = strings.TrimSpace(os.Getenv("TILLER_CONTRIBUTOR"))
	}
	if id == "" {
		// The shared env-then-identity tail (resolveContributorFallback, add.go):
		// TILLER_CONTRIBUTOR, then this checkout's .tiller/identity.yml. Every
		// mutating command's own resolution (resolveActingContributorOrRefuse)
		// consumes the same function, so join and a mutation refuse a
		// present-but-corrupt identity file the same way (issue #52) instead
		// of each mirroring its own copy of the handling. The one exception is
		// a teammate's identity still spelled with the pre-schema-9 owner key:
		// join selects that id and rewrites the file with the current key.
		legacy, renamed, err := store.LegacyLocalIdentity(root)
		if err != nil {
			return store.Contributor{}, identityUnreadableError(err)
		}
		id = legacy
		if renamed == nil {
			fallback, _, err := resolveContributorFallback(root)
			if err != nil {
				return store.Contributor{}, identityUnreadableError(err)
			}
			id = fallback
		}
	}

	owner, err := store.ResolveContributor(m, id)
	if err == nil {
		return owner, nil
	}
	var env *store.EnvError
	if !errors.As(err, &env) {
		return store.Contributor{}, err
	}
	switch env.Reason {
	case store.ReasonContributorUnset:
		return store.Contributor{}, &store.EnvError{
			Code:   env.ExitCode(),
			Reason: env.Reason,
			Msg: "tiller join has no identity to select: pass --as <id>, set TILLER_CONTRIBUTOR, " +
				"or run it again once a prior join has recorded .tiller/identity.yml on this " +
				"checkout. If no one is registered yet, a human runs `tiller add contributor <id> " +
				"--name \"<name>\"` first.",
		}
	case store.ReasonContributorUnknown:
		return store.Contributor{}, &store.EnvError{
			Code:   env.ExitCode(),
			Reason: env.Reason,
			Msg: fmt.Sprintf(
				"contributor %q is not registered; %s. A human registers a new person with "+
					"`tiller add contributor %s --name \"<name>\"`, then this join selects it "+
					"(`tiller join --as %s`).",
				id, rosterClause(m.Contributors), id, id),
		}
	default:
		return store.Contributor{}, err
	}
}

// rosterClause names the committed roster for a refusal message: the
// registered ids in order when any exist, or that none do yet, so an
// unknown-id refusal states plainly what IS on the roster alongside what to
// do about the id that is not.
func rosterClause(owners []store.Contributor) string {
	if len(owners) == 0 {
		return "the roster is empty"
	}
	ids := make([]string, len(owners))
	for i, o := range owners {
		ids[i] = o.ID
	}
	return "the registered roster is " + strings.Join(ids, ", ")
}

// contributorInfos projects a roster onto the envelope's ContributorInfo shape.
func contributorInfos(owners []store.Contributor) []ContributorInfo {
	out := make([]ContributorInfo, 0, len(owners))
	for _, o := range owners {
		out = append(out, ContributorInfo{ID: o.ID, Name: o.Name})
	}
	return out
}

// renderJoinHuman keeps identity selection distinct from artifact maintenance.
func renderJoinHuman(r joinResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Joined as %s. Identity recorded; roster now %d contributor%s.\n", contributorLabel(r.Contributor), len(r.Roster), plural(len(r.Roster)))
	fmt.Fprintf(&b, "Project files kept current: orientation.md (generation %d).\n", r.OrientationGeneration)
	b.WriteString("\n**Wired the Claude Code harness**\n")
	fmt.Fprintf(&b, "  agent skills    %s/tiller-*    %d shims, gitignored\n", r.SkillsDir, len(r.Skills))
	fmt.Fprintf(&b, "  pointer block   %s\n", r.PointerFile)
	fmt.Fprintf(&b, "  redirect        %s\n", r.RedirectFile)
	for _, warning := range r.Warnings {
		fmt.Fprintf(&b, "  warning         %s\n", warning)
	}
	b.WriteString("\n**Next**\n")
	b.WriteString("  tiller status\n")
	b.WriteString("  tiller guide      lists your ranked next moves and the skill that fits")
	return b.String()
}

// plural is the "s" of "N contributors", dropped at exactly one.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// contributorLabel renders a contributor as "id (Name)", or just the id when the display
// name adds nothing.
func contributorLabel(o ContributorInfo) string {
	if o.Name != "" && o.Name != o.ID {
		return fmt.Sprintf("%s (%s)", o.ID, o.Name)
	}
	return o.ID
}
