package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/spf13/cobra"
)

// This file implements `tiller init`, the mechanical scaffold (cli-surface.md,
// "Bootstrapping a project"). It reasons about nothing: it writes a commented
// manifest carrying the flags it was given plus the skeleton directories from
// the artifact-schema.md Directory layout, and it is idempotent on re-run.

// skeletonDirs are the directories tiller init lays down under .tiller/
// (artifact-schema.md, Directory layout). Generated orientation.md and the
// git-ignored .cache/ are produced separately from the bare scaffold.
var skeletonDirs = []string{
	"design",
	"epics",
	"plans",
	"amendments",
	"decisions/adr",
	"events",
}

// keyRe is the key-segment grammar (artifact-schema.md, IDs): lowercase
// alphanumerics in hyphen-joined segments. A project key is a single segment.
// mutate/amendment.go's amendmentKeyRe is the identical pattern, a
// mutate-local copy this var cannot be imported into (mutate cannot depend on
// cli); that file's own comment on amendmentKeyRe names this var as its twin.
// intake.go deliberately does not keep a third copy for the amendment key it
// mints: it validates through mutate.PrecheckAddAmendment instead, which is
// what amendmentKeyRe itself already backs, so there are only ever two
// regexes to keep in sync, not three.
var keyRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// initResult is the JSON payload of a successful init: the project identity it
// recorded, the manifest path, and whether this run wrote a new manifest (false
// on an idempotent re-run that found one already present). init records no
// contributor: a human registers the first one with `tiller add contributor`, and `tiller
// join` then selects it as this checkout's identity.
type initResult struct {
	Project ProjectInfo         `json:"project"`
	Path    string              `json:"path"`
	Created bool                `json:"created"`
	Design  *designImportResult `json:"design,omitempty"`
}

// designImportResult reports a --design import in the success envelope: the
// absolute source the docs were copied from and how many regular files landed
// under .tiller/design/. It is omitted entirely when --design was not given, so
// a plain init's envelope is unchanged.
type designImportResult struct {
	From  string `json:"from"`
	Files int    `json:"files"`
}

// designSource is the validated provenance of a --design import: the absolute
// source path and the time it was imported. It is nil when --design is unset.
type designSource struct {
	abs string
	at  string
}

func newInitCmd() *cobra.Command {
	var name, key, kind string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new .tiller/ workspace in the current directory.",
		Long: "init writes a commented tiller.yml manifest carrying the flags it was\n" +
			"given plus the skeleton directories, and is idempotent on re-run. It is a\n" +
			"pure scaffold: it reasons about nothing and records no identity. The roster\n" +
			"ships empty; a human registers its first contributor with `tiller add contributor\n" +
			"<id> --name \"<name>\"`, then `tiller join --as <id>` records who you are\n" +
			"on this checkout.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, name, key, kind)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "human-readable project name (required)")
	cmd.Flags().StringVar(&key, "key", "", "short project key in [a-z0-9-] (required)")
	cmd.Flags().StringVar(&kind, "kind", "", "archetype: greenfield | brownfield | legacy-migration (required)")
	cmd.Flags().String("design", "", "import a folder of design docs into .tiller/design/ (copy-out-only; point at a clean folder of just your design docs)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("kind")
	return cmd
}

func runInit(cmd *cobra.Command, name, key, kind string) error {
	if err := validateInitFlags(name, key, kind); err != nil {
		return emitFailure(cmd, "init", err)
	}

	root, err := os.Getwd()
	if err != nil {
		return emitFailure(cmd, "init", err)
	}

	// Validate the optional --design import source up front, before any scaffold
	// write, so a bad source never leaves a half-scaffolded repo.
	design, err := resolveDesignSource(cmd, root)
	if err != nil {
		return emitFailure(cmd, "init", err)
	}

	result, err := scaffold(root, name, key, kind, design)
	if err != nil {
		return emitFailure(cmd, "init", err)
	}

	human := fmt.Sprintf("Initialized %q (%s, %s).", result.Project.Name, result.Project.Key, result.Project.Kind)
	if !result.Created {
		human = fmt.Sprintf(".tiller/ already initialized for project %q; skeleton ensured.", result.Project.Name)
	}
	if result.Design != nil {
		human += fmt.Sprintf("\nImported %d design files from %s into .tiller/design/.", result.Design.Files, result.Design.From)
	}
	// A --design re-run over an existing manifest imports nothing (scaffold
	// gates the copy on the manifest being written), so say so rather than
	// leaving "skeleton ensured" to imply the docs arrived.
	if design != nil && !result.Created {
		human += "\nDesign import skipped: .tiller/ was already initialized, and --design imports only on the init that scaffolds. Copy the docs under .tiller/design/ by hand."
	}
	// On a fresh scaffold, route the user forward through the onboarding arc so the
	// output does not stop at "initialized". This is guidance text only:
	// init still reasons about nothing and never decomposes. A re-run over an
	// existing project skips the arc, since the project is already past onboarding.
	if result.Created {
		human += "\n\n" + initNextSteps(result)
	}
	return emitSuccess(cmd, "init", result, human)
}

// initNextSteps routes fresh projects through input review, contributor registration,
// join, readiness, and decomposition. Imports do not establish a baseline, and
// legacy reference declarations remain necessary even when docs were imported.
// Gate configuration is guidance, not an additional readiness prerequisite.
func initNextSteps(result initResult) string {
	first := "Put your design inputs under .tiller/design/."
	if result.Design != nil {
		first = "Review the imported design under .tiller/design/."
	}
	switch result.Project.Kind {
	case store.KindBrownfield:
		first += " Reconcile existing documentation against the current code; document what is missing."
	case store.KindLegacyMigration:
		first += " Reconcile documentation against the old system's code and describe the intended replacement."
	}
	first += "\n     For in-repo docs, choose a move into .tiller/design/ when Tiller takes ownership,\n" +
		"     or a pointer page there when another consumer needs the original location.\n" +
		"     Record committed source provenance with `tiller link design --from` before retiring translated sources.\n" +
		"     External --design imports are copies; keep their source documents.\n" +
		"     --design imports only on the init that scaffolds."

	steps := []string{first}
	if result.Project.Kind == store.KindLegacyMigration {
		steps = append(steps, "Declare the old system's existing root under inputs.code in .tiller/tiller.yml\n"+
			"     with mode: reference before readiness. Keep it separate from writable code.")
	}
	// Commands have their own lines. Registration must precede join because
	// join selects an existing contributor, and both precede readiness's roster check.
	steps = append(steps,
		"Register yourself as a contributor (a human, once per project):\n       tiller add contributor <id> --name \"<full name>\"",
		"Join, wiring your identity and your agent:\n       tiller join --as <id>",
	)
	if result.Project.Kind == store.KindBrownfield || result.Project.Kind == store.KindLegacyMigration {
		steps = append(steps,
			"Derive or translate the existing-code baseline, reading existing docs first:\n       tiller guide derive-baseline",
			"Review the linked design docs. A human confirms named docs without changing their prose:\n       tiller approve design <doc>\n     One confirmed doc releases the baseline startup gate.")
	}
	steps = append(steps, "tiller check readiness")
	// Greenfield's first step copied design inputs, which make decompose due.
	// An existing-code project's next seam after its baseline depends on
	// whether a to-be design doc exists, so plain guide names it below.
	if result.Project.Kind == store.KindGreenfield {
		steps = append(steps, "tiller guide decompose")
	}
	steps = append(steps,
		"From there the feature frontier paces the work, one feature specified\n"+
			"     when its turn comes. This lists your ranked next moves and the skill that fits:\n       tiller guide",
	)
	var out strings.Builder
	out.WriteString("**Next**")
	for i, step := range steps {
		fmt.Fprintf(&out, "\n  %d. %s", i+1, step)
	}
	if result.Project.Kind == store.KindGreenfield {
		out.WriteString("\n\nThe build and tests gates wire later in .tiller/tiller.yml, once a stack\nexists; the evals gate is already active.")
	} else {
		out.WriteString("\n\nWire the build and tests gates in .tiller/tiller.yml from your existing stack.\nGate configuration is separate from readiness; the evals gate is already active.")
	}
	return out.String()
}

// resolveDesignSource validates the optional --design flag up front, before any
// scaffold write. It returns nil when the flag is unset, the validated source
// otherwise, and a misuse error (exit 2) when the path is missing, not a
// directory, empty, or overlapping root's .tiller/design/. It only reads the
// source here; the copy-out happens once the scaffold exists.
func resolveDesignSource(cmd *cobra.Command, root string) (*designSource, error) {
	raw, _ := cmd.Flags().GetString("design")
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return nil, &usageError{reason: "invalid_design", msg: fmt.Sprintf("design source %q is not a usable path: %v", raw, err)}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, &usageError{reason: "invalid_design", msg: fmt.Sprintf("design source %q does not exist", raw)}
	}
	if !info.IsDir() {
		return nil, &usageError{reason: "invalid_design", msg: fmt.Sprintf("design source %q is not a directory", raw)}
	}
	// Overlap is refused before the emptiness walk too, which would otherwise
	// descend the whole checkout when pointed at it or an ancestor.
	overlaps, err := importOverlaps(root, abs, filepath.Join(".tiller", "design"))
	if err != nil {
		return nil, &usageError{reason: "invalid_design", msg: fmt.Sprintf("design source %q cannot be compared with .tiller/design/: %v", raw, err)}
	}
	if overlaps {
		return nil, &usageError{reason: "invalid_design", msg: fmt.Sprintf("design source %q contains or lies inside .tiller/design/; point --design at a separate folder of just your design docs", raw)}
	}
	if !dirHasFiles(abs) {
		return nil, &usageError{reason: "invalid_design", msg: fmt.Sprintf("design source %q is empty", raw)}
	}
	return &designSource{abs: abs, at: time.Now().UTC().Format(time.RFC3339)}, nil
}

// validateInitFlags rejects a malformed flag value as misuse (exit 2). The
// required-flag presence check is cobra's; this enforces the value grammar.
func validateInitFlags(name, key, kind string) error {
	if strings.TrimSpace(name) == "" {
		return &usageError{reason: "invalid_name", msg: "project name must not be empty"}
	}
	if !keyRe.MatchString(key) {
		return &usageError{reason: "invalid_key", msg: fmt.Sprintf("project key %q must match [a-z0-9]+(-[a-z0-9]+)*", key)}
	}
	if !store.ValidProjectKind(kind) {
		return &usageError{reason: "invalid_kind", msg: fmt.Sprintf("project kind %q must be one of greenfield, brownfield, legacy-migration", kind)}
	}
	return nil
}

// scaffold creates the skeleton directories (idempotently) and writes the
// manifest only if one is not already present, so a re-run never clobbers a
// human-edited manifest. When a manifest is already present, it is loaded and
// validated before anything else in this function writes: a refused load (a
// schema_version mismatch, most concretely) must leave a pre-existing tree
// exactly as it found it, not partially rewritten (skeleton directories
// created, a root .gitignore seeded) on the way to the refusal. It returns
// whether a new manifest was written.
func scaffold(root, name, key, kind string, design *designSource) (initResult, error) {
	tillerDir := filepath.Join(root, ".tiller")
	manifestPath := filepath.Join(tillerDir, "tiller.yml")
	present, err := store.PathExistsNoSymlinks(root, manifestPath)
	if err != nil {
		return initResult{}, err
	}
	created := !present

	if created {
		// An orphaned orientation from a newer binary refuses before the
		// manifest write, so the refusal changes nothing.
		if err := refuseNewerCheckoutOrientation(root); err != nil {
			return initResult{}, err
		}
		if design != nil {
			entries, err := os.ReadDir(design.abs)
			if err != nil {
				return initResult{}, err
			}
			// A docs import cannot adopt another project's confirmations.
			// Fold names because source and destination filesystems may differ.
			for _, entry := range entries {
				if strings.EqualFold(entry.Name(), filepath.Base(store.BaselineRelPath)) {
					return initResult{}, &usageError{reason: "invalid_design", msg: fmt.Sprintf("design source %q contains reserved baseline.yml state; import a prose-only directory without that ledger", design.abs)}
				}
			}
		}
		var adopted *store.AdoptedAnchor
		if kind == store.KindBrownfield || kind == store.KindLegacyMigration {
			if head, err := vcs.New(root).Head(); err == nil && head != "" {
				adopted = &store.AdoptedAnchor{Commit: "git:" + head, At: time.Now().UTC().Format(time.RFC3339)}
			}
		}
		manifest := renderManifest(name, key, kind, design, detectIntegrationBranch(root), adopted)
		if err := store.WriteRepositoryFileAtomic(root, filepath.Join(".tiller", "tiller.yml"), []byte(manifest)); err != nil {
			return initResult{}, err
		}
	}

	// Validate before any of the writes below. On a fresh scaffold this always
	// succeeds, since the manifest just written above carries
	// store.CurrentSchemaVersion; on a re-run over an existing project, this is
	// the whole-tree clean stop the comment above describes.
	manifest, err := store.LoadManifest(root)
	if err != nil {
		return initResult{}, err
	}
	// A rerun must not downgrade a newer binary's orientation, and it refuses
	// before ensuring the skeleton so the refusal changes nothing. A fresh
	// scaffold already checked above, before writing the manifest.
	if !created {
		if err := refuseNewerCheckoutOrientation(root); err != nil {
			return initResult{}, err
		}
	}

	for _, d := range skeletonDirs {
		if err := store.MkdirRepositoryAll(root, filepath.Join(".tiller", d), 0o755); err != nil {
			return initResult{}, err
		}
	}

	// Copy the --design source out into .tiller/design/ when one was given and
	// this run wrote the manifest. Gating on `created` keeps the import and its
	// manifest provenance in lockstep: a re-run over an existing manifest does not
	// re-import (and could not record provenance without clobbering a human-edited
	// manifest). The source was validated up front, so a failure here is a real IO
	// error, not misuse.
	var imported *designImportResult
	if design != nil && created {
		n, err := importDocs(root, design.abs, filepath.Join(".tiller", "design"))
		if err != nil {
			return initResult{}, err
		}
		imported = &designImportResult{From: design.abs, Files: n}
	}

	// Refresh the byte-stable committed orientation.
	if err := writeGenerated(root); err != nil {
		return initResult{}, err
	}

	return initResult{
		Project: ProjectInfo{Name: manifest.Project.Name, Key: manifest.Project.Key, Kind: manifest.Project.Kind},
		Path:    filepath.Join(".tiller", "tiller.yml"),
		Created: created,
		Design:  imported,
	}, nil
}

// detectIntegrationBranch reports the branch a fresh project integrates on: the
// branch HEAD is on at scaffold time, which is the branch the human ran init from
// and therefore the branch their merges land on. It seeds master in a master repo
// and main in a main repo, so init never writes a value that contradicts the repo
// it is standing in, and it deliberately does not probe origin/HEAD: a detected
// remote default would silently name a branch the team may not integrate on. It
// degrades to "main" rather than failing, since init must scaffold in a directory
// that is not a git repo at all. This is read-only observation, not reasoning:
// init still decides nothing.
func detectIntegrationBranch(root string) string {
	if b, err := vcs.New(root).HeadBranch(); err == nil && b != "" {
		return b
	}
	return "main"
}

// renderManifest produces the commented tiller.yml template (cli-surface.md: "a
// commented tiller.yml carrying the flags it was given"). The name is rendered
// as a JSON string, which is a valid YAML double-quoted scalar, so an arbitrary
// project name cannot break the file. The key and kind are already grammar- and
// set-validated, so they are safe bare scalars. integrationBranch is rendered the
// same quoted way, since a branch name may carry characters YAML reads (it is the
// branch `sync` observes done crossings into; artifact-schema.md, Project
// manifest). Design provenance and the optional HEAD observation are inputs:
// rendering does not query Git or the clock.
func renderManifest(name, key, kind string, design *designSource, integrationBranch string, adopted *store.AdoptedAnchor) string {
	code := "  code: []\n"
	anchor := ""
	if kind == store.KindBrownfield || kind == store.KindLegacyMigration {
		code = "  code:\n    - { root: ., mode: surface }\n"
		anchor = "  # adopted observes this repository's HEAD at init, not an external reference root.\n" +
			"  # It does not assert a clean worktree. Old manifests or unavailable HEADs may omit it.\n" +
			"  # Never substitute a later HEAD for an absent adoption observation.\n"
		if adopted != nil && adopted.Commit != "" {
			anchor += fmt.Sprintf("  adopted: { commit: %s, at: %s }\n", jsonString(adopted.Commit), jsonString(adopted.At))
		}
	}
	if kind == store.KindLegacyMigration {
		code += "    # Declare the old system's existing location before readiness:\n" +
			"    # - { root: ../some-legacy, mode: reference }\n"
	}
	return fmt.Sprintf(`# Tiller project manifest. Generated by "tiller init"; edit freely.
# Inline comments describe each block; .tiller/orientation.md covers where things live.

schema_version: %d
schema_epoch: initial

project:
  name: %s
  key: %s
  kind: %s            # greenfield | brownfield | legacy-migration
  # requires_tiller: ">= 1.0"   # optional advisory minimum binary
%s

# The humans who hold work. A human registers each one with
# `+"`tiller add contributor <id> --name \"<name>\"`"+`.
contributors: []

# Where decomposition reads from. Put design docs under .tiller/design/.
inputs:
  docs:
    - design/
  current_state: []
%s%s
# Required gates are command-backed and define clean completion. The evals gate
# is active by default: it runs each feature's acceptance contract (the *.eval.md
# criteria) through "tiller eval". The contract gates the feature; tasks gate on
# build and tests: task gates catch a broken task before dependents build on it,
# at every review grain, and a gate already passed on unchanged code is reused
# rather than rerun.
# Fill in build and tests with the commands your project runs.
gates:
  required:
    evals: { kind: command, run: "tiller eval ${node.noun} ${node.id}", on: [feature] }
    # build: { kind: command, run: "<build command>", on: [feature, task] }
    # tests: { kind: command, run: "<test command>",  on: [feature, task] }
  optional:
    code_review:          { kind: attestation, on: pr }
    security_review:      { kind: attestation, on: pr }
    performance_review:   { kind: attestation, on: pr }
    accessibility_review: { kind: attestation, on: pr }

git:
  branch_pattern: "tiller/${node.key}"
  integration_branch: %s   # the branch merges land on; "tiller sync" observes done crossings into it
  pr_coherence: true

drift:
  checks: [stale_spec, active_basis, answered_question, missing_surface, orphan_eval, stale_design]
`, store.CurrentSchemaVersion, jsonString(name), key, kind, anchor, code, renderImportedFrom(design), jsonString(integrationBranch))
}

// renderImportedFrom renders the inputs.imported_from provenance lines for a
// --design import, or the empty string when no design was imported. The empty
// case keeps a plain init's manifest byte-identical. The source path and import
// timestamp are written as quoted scalars so an arbitrary path is safe, and the
// canonical docs still live under .tiller/design/; this block is provenance only.
func renderImportedFrom(design *designSource) string {
	if design == nil {
		return ""
	}
	return fmt.Sprintf("  imported_from:\n    - { path: %s, at: %s }\n", jsonString(design.abs), jsonString(design.at))
}

// seededGitignore is the repo-root .gitignore tiller init lays down for a
// project that has none: the generated paths, the per-checkout identity
// file, and the per-checkout settings file, rendered from tillerIgnoreGroups
// so init and join share one list. join adds the last group (the skill shims)
// when it installs them, and upserts every group into a .gitignore init left
// alone.
var seededGitignore = renderIgnoreGroups(tillerIgnoreGroups[:3])

// writeGenerated refreshes committed orientation and seeds a missing ignore file.
// Loading first preserves schema refusals; both writes share the derived transaction.
func writeGenerated(root string) error {
	for attempt := range 16 {
		tree, err := store.Load(root)
		if err == nil {
			tillerDir := filepath.Join(root, ".tiller")
			orientationPath := filepath.Join(tillerDir, "orientation.md")
			gitignorePath := filepath.Join(root, ".gitignore")

			orientation, orientationSnapshot, readErr := store.ReadRegularFileSnapshot(root, orientationPath)
			if readErr != nil {
				return readErr
			}
			generation, _ := gen.OrientationStampGeneration(orientation)
			if err := refuseNewerOrientation(generation); err != nil {
				return err
			}
			_, gitignoreSnapshot, readErr := store.ReadRegularFileSnapshot(root, gitignorePath)
			if readErr != nil {
				return readErr
			}

			writes := []store.PlannedWrite{
				store.PlanWrite(orientationPath, orientationSnapshot, []byte(gen.Orientation())),
			}
			if !gitignoreSnapshot.Exists() {
				writes = append(writes, store.PlanWrite(gitignorePath, gitignoreSnapshot, []byte(seededGitignore)))
			}
			err = tree.CommitDerived("write generated project files", writes)
		}
		if err == nil {
			return nil
		}
		var env *store.EnvError
		if !errors.As(err, &env) || (env.Reason != store.ReasonLocked && env.Reason != store.ReasonMutationStale) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return &store.EnvError{Code: 3, Reason: store.ReasonLocked, Msg: "write generated project files remained contended after 16 attempts"}
}

// refuseNewerCheckoutOrientation reads root's committed orientation and applies
// the shared newer-generation refusal.
func refuseNewerCheckoutOrientation(root string) error {
	orientation, _, err := store.ReadRegularFileSnapshot(root, filepath.Join(root, ".tiller", "orientation.md"))
	if err != nil {
		return err
	}
	generation, _ := gen.OrientationStampGeneration(orientation)
	return refuseNewerOrientation(generation)
}

// jsonString renders s as a double-quoted scalar that is valid in both JSON and
// YAML, so a project name with spaces or quotes is written safely.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// repoRoot finds the repo root by walking up from the working directory to the
// nearest ancestor that holds .tiller/tiller.yml, like git's discovery. When no
// ancestor qualifies it returns the working directory, so the caller's load
// yields the canonical not_a_repo environment error. Only observed absence
// keeps walking: no .tiller entry, a regular file named .tiller, or a .tiller
// directory without tiller.yml. A symlink refusal at an ancestor selects it,
// so the load fails loudly there. Any other inspection failure (an unreadable
// .tiller, most concretely) is a boundary refusal, as in maintenance
// discovery: walking past it would redirect the command to an outer project.
func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		present, err := projectMarkerPresent(dir)
		if err != nil {
			return "", err
		}
		if present {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
		dir = parent
	}
}

// projectMarkerPresent reports whether dir claims a project. A symlinked
// .tiller or tiller.yml claims it (the load then refuses); an inspection error
// is reported as project_marker_unreadable rather than read as absence.
func projectMarkerPresent(dir string) (bool, error) {
	marker := filepath.Join(dir, ".tiller")
	info, err := os.Lstat(marker)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, markerUnreadable(marker, err)
	case info.Mode().IsRegular():
		return false, nil
	}
	present, err := store.PathExistsNoSymlinks(dir, filepath.Join(marker, "tiller.yml"))
	if errors.Is(err, rootpath.ErrUnsafe) {
		return true, nil
	}
	if err != nil {
		return false, markerUnreadable(marker, err)
	}
	return present, nil
}

func markerUnreadable(marker string, err error) error {
	return &store.EnvError{Code: 3, Reason: store.ReasonProjectMarkerUnreadable, Msg: fmt.Sprintf("cannot inspect project marker %s: %v; fix its permissions rather than running from an outer project", marker, err)}
}
