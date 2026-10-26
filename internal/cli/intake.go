package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/spf13/cobra"
)

// This file implements `tiller intake <file>` (docs/spec/cli-surface.md).
// intake is the front door for a recurring input decompose and add never fit:
// an external feedback document (a code review, a product critique, user
// research) that implies a batch of heterogeneous changes to a living tree.
// It does exactly two things, both mechanical: it lands the document as a
// fingerprinted source artifact under .tiller/feedback/, and it opens a
// proposed amendment carrying that fingerprint as its Source. It stages no
// operations itself — the explosion of feedback into epics, features, tasks,
// and questions is model judgment a skill guides (`tiller guide intake`, a
// later task), staged afterward through the ordinary `tiller amend` surface,
// which task 1 already requires to cite this artifact on every op.
//
// Who may run it: both, under the standard delegation rules (cli-surface.md,
// "The actor model"). intake carries no verb entry in lifecycle's verbActors
// table (table.go), so it defaults to ActorEither, the same default add
// amendment and amend already get.

// feedbackDir is the directory intake lands artifacts under, a sibling of
// design/: genesis inputs and ongoing inputs are different shelves (the
// design doc's own framing). Unlike design/, it is not part of init's
// skeletonDirs; the first intake creates it on demand (importDocs's
// WriteRepositoryFileAtomic already creates its target directory), the same lazy
// creation every other per-feature/per-task folder in this codebase gets.
const feedbackDir = "feedback"

// intakeKeySlugRe matches a run of characters keyRe does not accept, in the
// slug intakeDefaultKey builds from an artifact's basename.
var intakeKeySlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// intake's own success envelope reuses AmendmentData directly (the same
// shape `add amendment` already renders), since intake mints nothing beyond
// an amendment carrying a Source — amendmentResultView already carries it
// once Source rides on mutate.AmendmentResult (amendment_cmds.go).

func newIntakeCmd() *cobra.Command {
	var key, title string
	cmd := &cobra.Command{
		Use:   "intake <file>",
		Short: "Land an external feedback document and open its amendment.",
		Long: "intake copies <file> into .tiller/feedback/, fingerprints the landed\n" +
			"copy (the same git-blob idiom specs and criteria already use), and opens\n" +
			"a proposed amendment whose Source cites that artifact. The amendment\n" +
			"opens with no operations staged: explode the feedback into the tree's\n" +
			"grammar with `tiller guide intake`, then stage each operation with\n" +
			"`tiller amend --source <citation>` (every op on an intake-minted\n" +
			"amendment must cite one), review the whole batch with `tiller show\n" +
			"amendment`, and cross the human gate with `tiller approve amendment`.\n" +
			"Re-intake of a changed document is a new intake: a basename already\n" +
			"landed under .tiller/feedback/ is refused rather than overwritten,\n" +
			"since the old artifact's fingerprint keeps the old amendment's\n" +
			"citations honest.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIntake(cmd, args[0], key, title)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "the amendment's key (defaults to a slug of the file's basename)")
	cmd.Flags().StringVar(&title, "title", "", `the amendment's title (defaults to "Intake: <basename>")`)
	addContributorFlag(cmd)
	return cmd
}

func runIntake(cmd *cobra.Command, srcPath, key, title string) error {
	if err := statIntakeSource(srcPath); err != nil {
		return emitFailure(cmd, "intake", err)
	}

	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}

	// The owner is resolved (never claimed) here, before any write: a
	// misconfigured checkout (no `tiller join` yet) must refuse before the
	// artifact copy, not after, or a retry once the owner is fixed would find
	// the FIRST attempt's orphaned artifact sitting at the destination and be
	// refused as a basename collision for a document that never actually
	// landed an amendment. AddAmendment re-resolves the same owner from the
	// same deps a moment later; that second resolution is cheap and keeps
	// AddAmendment's own contract (it always resolves its own owner) intact.
	if _, err := store.ResolveContributor(tree.Manifest, d.Contributor); err != nil {
		return emitFailure(cmd, "intake", err)
	}

	basename := filepath.Base(srcPath)
	if strings.TrimSpace(key) == "" {
		key = intakeDefaultKey(basename)
	}
	if strings.TrimSpace(title) == "" {
		title = "Intake: " + basename
	}

	// The basename-collision check runs first among the two copy-blocking
	// checks below: for the straightforward re-intake of the same document
	// (same basename, so the same default key too), BOTH this check and
	// PrecheckAddAmendment right after it would fire, and the basename
	// message ("rename the document or remove the old artifact") is the
	// more specific, actionable answer for that exact case — the brief's own
	// acceptance pins this message for basename collision specifically.
	// Order between the two does not affect the no-partial-write invariant
	// (either one refuses before the copy below runs); it only decides which
	// message a caller sees when both happen to be true at once.
	destRel := filepath.Join(".tiller", feedbackDir, basename)
	destAbs := filepath.Join(tree.Root, destRel)
	present, err := store.PathExistsNoSymlinks(tree.Root, destAbs)
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}
	if present {
		return emitFailure(cmd, "intake", intakeArtifactExistsErr())
	}

	// mutate.PrecheckAddAmendment runs every remaining AddAmendment
	// precondition that does not itself require a write: the key's grammar,
	// whether a PROPOSED amendment already holds it, and whether HEAD
	// resolves. It must run BEFORE the artifact copy below, not just before
	// the mint: the own live repro was `tiller add amendment
	// review-b` (a hand-opened amendment, no artifact ever landed for it,
	// so the basename check above cannot catch this) then `tiller intake
	// review-b.md` (whose default key also slugifies to "review-b") — the
	// artifact copied, THEN AddAmendment refused amendment_exists, orphaning
	// the copy for every retry (the collision refusal's own wording,
	// "re-intake is a new intake", falsely implying a prior successful
	// intake that never happened). Running the identical check
	// mutate.AddAmendment itself runs, ahead of the copy, closes that
	// window: intake.go no longer keeps its own separate key-format check
	// (formerly an "invalid_key" duplicate of amendmentKeyRe's own pattern,
	// with a different reason string than `add amendment` itself reports
	// for the identical defect) — PrecheckAddAmendment is the one source of
	// truth for "is this key usable", consolidating the two rather than
	// letting them drift (see keyRe's own cross-reference comment, init.go).
	if _, err := mutate.PrecheckAddAmendment(tree, d, key); err != nil {
		return emitFailure(cmd, "intake", err)
	}

	// The copy is a plain filesystem write, outside mutate's transactional
	// commit (no lock, no journal entry, no event of its own), the same
	// out-of-band discipline `tiller init --design`'s own importDocs call
	// already has for .tiller/design/: the artifact is provenance the
	// amendment's own commit below references, not a tracked node write in
	// its own right. Every AddAmendment precondition that does not require
	// this copy has already cleared above (owner, key format, key
	// collision, HEAD resolution), so nothing left below this line refuses
	// for a reason that predates the copy.
	if _, err := importDocs(tree.Root, srcPath, filepath.Join(".tiller", feedbackDir)); err != nil {
		return emitFailure(cmd, "intake", err)
	}

	// The fingerprint is read AFTER the copy, over the now-on-disk artifact:
	// WorktreeBlobRef (transition.go) hashes the working tree via `git
	// hash-object`, which reads whatever content sits at the path regardless
	// of whether git tracks it yet (internal/vcs's own
	// TestWorktreeBlobHash_Untracked pins this: a brand-new, never-committed
	// file still hashes). The fresh copy this call just wrote is untracked —
	// intake never runs `git add` — and the hash still comes back honest.
	ref, err := mutate.WorktreeBlobRef(vcs.New(tree.Root), tree.Root, destRel)
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}

	res, err := mutate.AddAmendment(tree, d, mutate.AmendmentInput{
		Key:         key,
		Title:       title,
		TriggeredBy: []string{destRel},
		Source:      &store.AmendmentSource{Path: destRel, Revision: ref.Value},
	})
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}

	view := amendmentResultView(res)
	return emitSuccess(cmd, "intake", view, renderIntakeHuman(destRel, view.Ref, res.ID))
}

// statIntakeSource validates <file> before any write: it must exist, be
// readable, and be a regular file. A directory, a symlink to nowhere,
// or a permission-denied path are all refused the same way, since none of
// them is a document intake can land.
func statIntakeSource(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return &usageError{
			reason: "invalid_source",
			msg:    fmt.Sprintf("intake source %q does not exist or is unreadable: %v", path, err),
		}
	}
	if !info.Mode().IsRegular() {
		return &usageError{
			reason: "invalid_source",
			msg:    fmt.Sprintf("intake source %q is not a regular file", path),
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return &usageError{
			reason: "invalid_source",
			msg:    fmt.Sprintf("intake source %q does not exist or is unreadable: %v", path, err),
		}
	}
	_ = f.Close()
	return nil
}

// intakeDefaultKey derives the amendment key `--key` defaults to when the
// caller does not supply one: the artifact's basename, its extension
// dropped, lowercased, and every run of characters outside [a-z0-9] collapsed
// to one hyphen, trimmed at both ends (the same grammar keyRe/amendmentKeyRe
// both validate against). A basename whose stem slugifies to nothing (a bare
// ".md", say) yields an empty key, which mutate.PrecheckAddAmendment's own
// format check refuses same as a malformed --key would — an honest refusal
// asking for an explicit --key, not a manufactured one.
func intakeDefaultKey(basename string) string {
	stem := strings.TrimSuffix(basename, filepath.Ext(basename))
	slug := intakeKeySlugRe.ReplaceAllString(strings.ToLower(stem), "-")
	return strings.Trim(slug, "-")
}

// intakeArtifactExistsErr refuses landing over a basename intake already
// placed under .tiller/feedback/. The message is pinned verbatim.
func intakeArtifactExistsErr() error {
	return &usageError{
		reason: "artifact_exists",
		msg:    "re-intake is a new intake: rename the document or remove the old artifact",
	}
}

// renderIntakeHuman is the house confirmation voice (join.go's
// renderJoinHuman, init.go's initNextSteps): a headline naming what landed,
// then a **Next** section pointing at the skill that guides the explosion.
// The next-step command lines carry the bare id, not ref (which composes a
// key path and an id with a space and parentheses, not a single shell
// token), the same bare-id convention approve's own Next lines use
// (readout.go).
//
// Forward reference: `tiller guide intake` names a skill Task 6 is the one
// that lands (internal/skills has no "intake" entry yet as of this task).
// Printing the pointer now, ahead of its target existing, is deliberate and
// owned by the coordinator's own sequencing, not a gap in this task: the
// controller has confirmed Task 6 lands before this branch ships, so by the
// time a real user sees this line, `tiller guide intake` already resolves.
func renderIntakeHuman(destRel, ref, amendmentID string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Landed %s as amendment %s.\n", destRel, ref)
	b.WriteString("\n**Next**\n")
	b.WriteString("  tiller guide intake                 explode the feedback into staged operations\n")
	fmt.Fprintf(&b, "  tiller show amendment %s    review the batch before approval", amendmentID)
	return b.String()
}
