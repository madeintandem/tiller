package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/spf13/cobra"
)

// This file implements `tiller intake <file or folder>` (docs/spec/cli-surface.md).
// intake is the front door for a recurring input decompose and add never fit:
// an external feedback document (a code review, a product critique, user
// research) that implies a batch of heterogeneous changes to a living tree.
// It does exactly two things, both mechanical: it lands the document as a
// fingerprinted source artifact under .tiller/design/, and it opens a
// draft amendment carrying that fingerprint as its Source. It stages no
// operations itself — the explosion of feedback into epics, features, tasks,
// and questions is model judgment the decompose skill guides (`tiller guide
// decompose`), staged afterward through the ordinary `tiller amend` surface,
// which already requires every op to cite this artifact.
//
// Who may run it: both, under the standard delegation rules (cli-surface.md,
// "The actor model"). intake carries no verb entry in lifecycle's verbActors
// table (table.go), so it defaults to ActorEither, the same default add
// amendment and amend already get.

// intakeLandingDir is where intake lands everything: a file at
// <dir>/<basename>, a folder at <dir>/<folder name> with its relative paths
// kept. Tiller knows a document is an amendment's source from the amendment
// record, not from the folder, so there is one shelf. Amendments minted before
// this landed under .tiller/feedback/ keep their stored paths and keep
// validating; nothing there moves.
var intakeLandingDir = filepath.Join(".tiller", "design")

// intakeKeySlugRe matches a run of characters keyRe does not accept, in the
// slug intakeDefaultKey builds from an artifact's basename.
var intakeKeySlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// intake's own success envelope reuses AmendmentData directly (the same
// shape `add amendment` already renders), since intake mints nothing beyond
// an amendment carrying a Source — amendmentResultView already carries it
// once Source rides on mutate.AmendmentResult (amendment_cmds.go).

func newIntakeCmd() *cobra.Command {
	var key, title string
	var noAmendment bool
	cmd := &cobra.Command{
		Use:   "intake <file or folder>",
		Short: "Land an external document or folder and open its amendment.",
		Long: "intake copies a file into .tiller/design/<name>, or a folder into\n" +
			".tiller/design/<folder name>/ with its relative paths kept, fingerprints\n" +
			"the landed copy (the same git-blob idiom specs and criteria already use;\n" +
			"a folder carries one fingerprint per file), and opens a draft amendment\n" +
			"whose Source cites that artifact. A folder opens one amendment covering\n" +
			"every file in it. The amendment\n" +
			"opens with no operations staged: explode the feedback into the tree's\n" +
			"grammar with `tiller guide decompose`, then stage each operation with\n" +
			"`tiller amend --source <citation>` (every op on an intake-minted\n" +
			"amendment must cite one), review the whole batch with `tiller show\n" +
			"amendment`, hand it over with `tiller propose amendment`, and the human\n" +
			"crosses the gate with `tiller approve amendment`.\n" +
			"Re-intake of a changed document is a new intake: a name already\n" +
			"present under .tiller/design/ is refused rather than overwritten,\n" +
			"since the old artifact's fingerprint keeps the old amendment's\n" +
			"citations honest.\n\n" +
			"--no-amendment lands the document (or folder) and records that it landed,\n" +
			"opening nothing: the way a founding design comes into a project with no\n" +
			"epics, for decompose to turn into a draft tree. It is refused once the\n" +
			"project has any epic; a later document comes in as an amendment.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if noAmendment {
				return runIntakeNoAmendment(cmd, args[0], key, title)
			}
			return runIntake(cmd, args[0], key, title)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "the amendment's key (defaults to a slug of the file or folder name)")
	cmd.Flags().StringVar(&title, "title", "", `the amendment's title (defaults to "Intake: <file or folder name>")`)
	cmd.Flags().BoolVar(&noAmendment, "no-amendment", false, "land the document as a founding design and open no amendment (only before any epic exists)")
	addContributorFlag(cmd)
	return cmd
}

// runIntakeNoAmendment lands a founding design: the same landing as runIntake,
// then one design_intake event instead of an amendment. It is refused once any
// epic exists, held or not, because from then on a document is a proposal
// against a living tree and belongs in an amendment.
func runIntakeNoAmendment(cmd *cobra.Command, srcPath, key, title string) error {
	if strings.TrimSpace(key) != "" || strings.TrimSpace(title) != "" {
		return emitFailure(cmd, "intake", &usageError{
			reason: "no_amendment_misuse",
			msg:    "--key and --title name an amendment; --no-amendment opens none",
		})
	}
	isDir, err := statIntakeSource(srcPath)
	if err != nil {
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
	if _, err := store.ResolveContributor(tree.Manifest, d.Contributor); err != nil {
		return emitFailure(cmd, "intake", err)
	}
	if len(tree.Epics) > 0 {
		return emitFailure(cmd, "intake", &usageError{
			reason: "amendment_required",
			msg:    "this project already has epics; a later document comes in as an amendment: tiller intake <path>",
		})
	}
	destRel, err := intakeDestination(tree.Root, srcPath)
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}
	source, err := landIntakeSource(tree.Root, srcPath, destRel)
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}
	files := 1
	if isDir {
		files = len(source.Files)
	}
	if err := mutate.RecordDesignIntake(tree, d, destRel, files); err != nil {
		return emitFailure(cmd, "intake", err)
	}
	view := struct {
		Path  string `json:"path"`
		Files int    `json:"files"`
	}{destRel, files}
	return emitSuccess(cmd, "intake", view, renderIntakeNoAmendmentHuman(destRel, files))
}

// renderIntakeNoAmendmentHuman confirms the landing and points at decompose.
func renderIntakeNoAmendmentHuman(destRel string, files int) string {
	noun := "files"
	if files == 1 {
		noun = "file"
	}
	return fmt.Sprintf("Landed %s (%d %s) as a founding design; no amendment opened.\n\n**Next**\n  tiller guide    decompose the design into a draft tree",
		destRel, files, noun)
}

func runIntake(cmd *cobra.Command, srcPath, key, title string) error {
	isDir, err := statIntakeSource(srcPath)
	if err != nil {
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

	name := intakeName(srcPath)
	if strings.TrimSpace(key) == "" {
		if isDir {
			key = intakeSlug(name)
		} else {
			key = intakeDefaultKey(name)
		}
	}
	if strings.TrimSpace(title) == "" {
		title = "Intake: " + name
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
	destRel, err := intakeDestination(tree.Root, srcPath)
	if err != nil {
		return emitFailure(cmd, "intake", err)
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
	source, err := landIntakeSource(tree.Root, srcPath, destRel)
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}

	res, err := mutate.AddAmendment(tree, d, mutate.AmendmentInput{
		Key:         key,
		Title:       title,
		TriggeredBy: []string{destRel},
		Source:      source,
	})
	if err != nil {
		return emitFailure(cmd, "intake", err)
	}

	view := amendmentResultView(res)
	return emitSuccess(cmd, "intake", view, renderIntakeHuman(source.Path, view.Ref))
}

// statIntakeSource validates <file or folder> before any write and reports
// which it is. A file must exist, be readable, and be regular; a folder must
// hold at least one regular file that is not OS junk. A symlink to nowhere, a
// permission-denied path, or an empty folder are refused the same way, since
// none of them is a document intake can land.
func statIntakeSource(path string) (isDir bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, &usageError{
			reason: "invalid_source",
			msg:    fmt.Sprintf("intake source %q does not exist or is unreadable: %v", path, err),
		}
	}
	if info.IsDir() {
		files, err := listLandableFiles(path)
		if err != nil {
			return false, &usageError{
				reason: "invalid_source",
				msg:    fmt.Sprintf("intake source %q is unreadable: %v", path, err),
			}
		}
		if len(files) == 0 {
			return false, &usageError{
				reason: "invalid_source",
				msg:    fmt.Sprintf("intake source %q holds no documents to land", path),
			}
		}
		return true, nil
	}
	if !info.Mode().IsRegular() {
		return false, &usageError{
			reason: "invalid_source",
			msg:    fmt.Sprintf("intake source %q is not a regular file or a folder", path),
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return false, &usageError{
			reason: "invalid_source",
			msg:    fmt.Sprintf("intake source %q does not exist or is unreadable: %v", path, err),
		}
	}
	_ = f.Close()
	return false, nil
}

// listLandableFiles lists, sorted, the slash-separated paths relative to dir of
// every regular file beneath it that importDocs would copy: nonregular entries
// and OS junk are skipped, as the copy skips them.
func listLandableFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() || junkFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

// intakeName is the name a source lands under: its last path element, taken
// from the absolute path so `.` and `../x/` name what they point at.
func intakeName(srcPath string) string {
	abs, err := filepath.Abs(srcPath)
	if err != nil {
		abs = filepath.Clean(srcPath)
	}
	return filepath.Base(abs)
}

// intakeDestination is the landing path (repo-relative, slash-separated) for
// srcPath, refusing reserved state names and artifact_exists when that name is
// already present under .tiller/design/. It writes nothing.
func intakeDestination(root, srcPath string) (string, error) {
	name := intakeName(srcPath)
	if strings.EqualFold(name, filepath.Base(store.BaselineRelPath)) {
		return "", &usageError{
			reason: "invalid_design",
			msg:    "baseline.yml is reserved state; rename the document or folder before intake",
		}
	}
	destRel := filepath.ToSlash(filepath.Join(intakeLandingDir, name))
	present, err := store.PathExistsNoSymlinks(root, filepath.Join(root, filepath.FromSlash(destRel)))
	if err != nil {
		return "", err
	}
	if present {
		return "", intakeArtifactExistsErr()
	}
	return destRel, nil
}

// landIntakeSource copies srcPath to destRel (from intakeDestination) and
// fingerprints what landed, returning the amendment Source for it: a file
// carries its blob revision, a folder carries one revision per landed file.
// It opens no amendment, so a caller that only wants the document landed
// (`--no-amendment`) uses it alone.
func landIntakeSource(root, srcPath, destRel string) (*store.AmendmentSource, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	rev := vcs.New(root)
	if !info.IsDir() {
		if _, err := importDocs(root, srcPath, filepath.FromSlash(filepath.Dir(destRel))); err != nil {
			return nil, err
		}
		// The fingerprint is read AFTER the copy, over the now-on-disk artifact:
		// WorktreeBlobRef hashes the working tree via `git hash-object`, which
		// reads whatever content sits at the path whether or not git tracks it
		// yet. Intake never runs `git add`; the hash still comes back honest. It
		// is not vcs.WorktreeSurface, which refuses paths inside .tiller/.
		ref, err := mutate.WorktreeBlobRef(rev, root, destRel)
		if err != nil {
			return nil, err
		}
		return &store.AmendmentSource{Path: destRel, Revision: ref.Value}, nil
	}

	if _, err := importDocs(root, srcPath, filepath.FromSlash(destRel)); err != nil {
		return nil, err
	}
	rels, err := listLandableFiles(filepath.Join(root, filepath.FromSlash(destRel)))
	if err != nil {
		return nil, err
	}
	files := make([]store.AmendmentSourceFile, 0, len(rels))
	for _, rel := range rels {
		path := destRel + "/" + rel
		ref, err := mutate.WorktreeBlobRef(rev, root, path)
		if err != nil {
			return nil, err
		}
		files = append(files, store.AmendmentSourceFile{Path: path, Revision: ref.Value})
	}
	return &store.AmendmentSource{Path: destRel, Files: files}, nil
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
	return intakeSlug(strings.TrimSuffix(basename, filepath.Ext(basename)))
}

// intakeSlug lowercases name and collapses every run of characters outside
// [a-z0-9] to one hyphen, trimmed at both ends. A folder's name is slugged
// whole: it has no extension to drop.
func intakeSlug(name string) string {
	slug := intakeKeySlugRe.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(slug, "-")
}

// intakeArtifactExistsErr refuses landing over a name intake already
// placed under .tiller/design/. The message is pinned verbatim.
func intakeArtifactExistsErr() error {
	return &usageError{
		reason: "artifact_exists",
		msg:    "re-intake is a new intake: rename the document or remove the old artifact",
	}
}

// renderIntakeHuman is the house confirmation voice (join.go's
// renderJoinHuman, init.go's initNextSteps): a headline naming what landed,
// then a **Next** section pointing at tiller guide, which routes the draft
// amendment to the agent.
func renderIntakeHuman(destRel, ref string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Landed %s as amendment %s.\n", destRel, ref)
	b.WriteString("\n**Next**\n")
	b.WriteString("  tiller guide    the agent turns the document into staged operations")
	return b.String()
}
