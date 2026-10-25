package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file tests `tiller intake <file>` (intake.go): the artifact lands
// under .tiller/feedback/, its fingerprint travels onto the minted
// amendment's Source, the basename-collision refusal is pinned verbatim, and
// the integration seam with task-1-brief.md's per-op source rule is proven
// end to end (an intake-minted amendment refuses a sourceless staged op).

// writeFeedbackDoc writes a scratch feedback document OUTSIDE the tiller repo
// (a separate temp dir), the shape a real external document has: intake never
// assumes its source lives inside .tiller/ or even inside the project.
func writeFeedbackDoc(t *testing.T, basename, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, basename)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestIntakeLandsArtifactAndMintsAmendmentWithSource(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "Acme-review.md", "the review body\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)

	data := jsonData(t, out)
	require.Equal(t, "proposed", data["status"])
	require.Equal(t, "acme-review", data["key"], "default key is a slug of the basename")
	require.Equal(t, "Intake: Acme-review.md", data["title"])
	require.Equal(t, []any{".tiller/feedback/Acme-review.md"}, data["triggered_by"])
	require.Equal(t, []any{}, data["operations"], "intake mints the amendment shell only; nothing stages yet")

	source, ok := data["source"].(map[string]any)
	require.True(t, ok, "source must round-trip in the envelope: %s", out)
	require.Equal(t, ".tiller/feedback/Acme-review.md", source["path"])
	revision, _ := source["revision"].(string)
	require.Regexp(t, `^git:[0-9a-f]{40}$`, revision)

	landed, err := os.ReadFile(filepath.Join(".tiller", "feedback", "Acme-review.md"))
	require.NoError(t, err)
	require.Equal(t, "the review body\n", string(landed))
}

// Final review item 3: a file already filed under .tiller/feedback/ (here an
// archive subfolder) is a single-file copy that cannot recurse into its own
// output, so intake lands it at the feedback root like any other document.
func TestIntakeAcceptsAFileFromInsideTheFeedbackFolder(t *testing.T) {
	initOwnedRepo(t)
	archived := filepath.Join(".tiller", "feedback", "archive", "review.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(archived), 0o755))
	require.NoError(t, os.WriteFile(archived, []byte("archived review\n"), 0o644))

	out, stderr, code := execCLI(t, "intake", archived, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out+stderr)
	require.Equal(t, []any{".tiller/feedback/review.md"}, jsonData(t, out)["triggered_by"])

	landed, err := os.ReadFile(filepath.Join(".tiller", "feedback", "review.md"))
	require.NoError(t, err)
	require.Equal(t, "archived review\n", string(landed))
	kept, err := os.ReadFile(archived)
	require.NoError(t, err)
	require.Equal(t, "archived review\n", string(kept), "the source is read, never moved")
}

// TestIntakeShowAmendmentJSONRoundTripsSource is the acceptance's own line:
// "the fingerprint round-trips through `show amendment --json`" — a SEPARATE
// read, after intake has already returned, sees the same source.
func TestIntakeShowAmendmentJSONRoundTripsSource(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "review.md", "body\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	aid, _ := jsonData(t, out)["id"].(string)
	require.NotEmpty(t, aid)

	out, stderr, code = execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	source, ok := data["source"].(map[string]any)
	require.True(t, ok, "show amendment --json must carry the intake-minted source: %s", out)
	require.Equal(t, ".tiller/feedback/review.md", source["path"])
	revision, _ := source["revision"].(string)
	require.Regexp(t, `^git:[0-9a-f]{40}$`, revision)
}

func TestIntakeAcceptsKeyAndTitleOverrides(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "review.md", "body\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc",
		"--key", "custom-key", "--title", "Custom Title", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "custom-key", data["key"])
	require.Equal(t, "Custom Title", data["title"])
}

func TestIntakeDefaultKeySlugifiesAMessyBasename(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "Acme Review v2 (final).md", "body\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "acme-review-v2-final", data["key"])
}

func TestIntakeRefusesMissingFile(t *testing.T) {
	initOwnedRepo(t)
	missing := filepath.Join(t.TempDir(), "nope.md")

	out, _, code := execCLI(t, "intake", missing, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "invalid_source"`)
}

func TestIntakeRefusesNonRegularFile(t *testing.T) {
	initOwnedRepo(t)
	dir := t.TempDir() // a directory, not a regular file

	out, _, code := execCLI(t, "intake", dir, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "invalid_source"`)
}

// TestIntakeRefusesBasenameCollision pins the exact refusal message
// task-4-brief.md quotes verbatim: re-intake of a document sharing a basename
// with one already landed is a new intake, not an overwrite.
func TestIntakeRefusesBasenameCollision(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "review.md", "first body\n")
	_, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)

	// A second document sharing only the basename, landed from a different
	// source directory (writeFeedbackDoc always uses a fresh t.TempDir()).
	second := writeFeedbackDoc(t, "review.md", "second body, edited\n")
	out, _, code := execCLI(t, "intake", second, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "artifact_exists"`)
	require.Contains(t, out, "re-intake is a new intake: rename the document or remove the old artifact")

	// The refused second call must not have clobbered the first artifact.
	landed, err := os.ReadFile(filepath.Join(".tiller", "feedback", "review.md"))
	require.NoError(t, err)
	require.Equal(t, "first body\n", string(landed))
}

// TestIntakeKeyCollisionLeavesNoArtifactAndNoAmendment is the reviewer's own
// live repro (fix round 1, Critical): a proposed amendment already holds the
// key intake's default (or an explicit --key) would also use. Before the
// fix, the artifact copied THEN AddAmendment refused amendment_exists,
// orphaning the copy on disk with nothing referencing it — every retry,
// even under a different --key, would hit the basename-collision refusal
// for a document that never actually landed. Pinned end to end: refusal
// reason amendment_exists, NO artifact on disk, and the tree still carries
// only the one amendment opened by hand.
func TestIntakeKeyCollisionLeavesNoArtifactAndNoAmendment(t *testing.T) {
	initOwnedRepo(t)
	_, stderr, code := execCLI(t, "add", "amendment", "review-b", "--title", "Hand-opened first", "--contributor", "jc")
	require.Equal(t, 0, code, stderr)

	src := writeFeedbackDoc(t, "review-b.md", "the review body\n")
	out, _, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "amendment_exists"`)

	require.NoFileExists(t, filepath.Join(".tiller", "feedback", "review-b.md"),
		"the precheck must refuse before the copy, not after")

	tree := loadCwdTree(t)
	require.Len(t, tree.Amendments, 1, "only the hand-opened amendment must exist; intake must not have minted a second one")

	// A retry with a fresh --key must succeed cleanly: the collision left no
	// artifact behind to trip the basename-collision refusal on a document
	// that never actually landed.
	out, stderr, code = execCLI(t, "intake", src, "--contributor", "jc", "--key", "review-b-retry", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	require.Equal(t, "review-b-retry", data["key"])
}

// TestIntakeRefusesUnknownOwnerLeavingNoArtifact covers the arc the intake
// implementation's own Concerns section justified the owner pre-check with,
// but never itself tested (fix round 1, Important): an --contributor naming
// nobody in the project's roster must refuse before the artifact copies,
// leaving neither an artifact nor an amendment behind.
func TestIntakeRefusesUnknownOwnerLeavingNoArtifact(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "review.md", "body\n")

	out, _, code := execCLI(t, "intake", src, "--contributor", "nobody", "--json")
	require.Equal(t, 3, code)
	require.Contains(t, out, `"reason": "contributor_unknown"`)

	require.NoFileExists(t, filepath.Join(".tiller", "feedback", "review.md"))
	tree := loadCwdTree(t)
	require.Empty(t, tree.Amendments)
}

func TestIntakeRefusesInvalidKey(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "review.md", "body\n")

	out, _, code := execCLI(t, "intake", src, "--contributor", "jc", "--key", "Not A Key!", "--json")
	require.Equal(t, 2, code)
	// The reason is mutate.PrecheckAddAmendment's own "amendment_key_invalid"
	// (fix round 1: intake no longer keeps a separate CLI-side "invalid_key"
	// check that would have preempted this with a different reason for the
	// identical defect `add amendment` itself already reports this way).
	require.Contains(t, out, `"reason": "amendment_key_invalid"`)

	// A refused key must not have left the artifact copied with nothing to
	// reference it (the pre-check runs before the copy).
	require.NoFileExists(t, filepath.Join(".tiller", "feedback", "review.md"))
}

// TestIntakeHumanOutputNamesTheNextStep pins the house confirmation voice's
// forward pointer: `tiller guide intake` for the explosion (task-4-brief.md).
func TestIntakeHumanOutputNamesTheNextStep(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "review.md", "body\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, ".tiller/feedback/review.md")
	require.Contains(t, out, "tiller guide intake")
}

// --- the integration seam with task 1's amend --source rule ---

// TestIntakeThenAmendWithoutSourceRefusesPerTask1Rule is the acceptance's own
// proof that tasks 1 and 4 meet: a sourceless op staged onto an
// intake-minted amendment is refused by StageOperation's own rule
// (task-1-brief.md), exercised here for the first time end to end (task 1's
// own tests could only manufacture the doc.Source != nil shape by a disk-level
// poke, since nothing before this task ever set it for real).
func TestIntakeThenAmendWithoutSourceRefusesPerTask1Rule(t *testing.T) {
	layAmendmentTree(t)
	src := writeFeedbackDoc(t, "review.md", "add a table-backed forms feature\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	aid, _ := jsonData(t, out)["id"].(string)
	require.NotEmpty(t, aid)

	out, _, code = execCLI(t, "amend", aid, "--add-feature", "form-engine/table-backed-forms",
		"--change-type", "create", "--title", "Table-backed forms", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "source_required"`)
	require.Contains(t, out, "intake amendments trace every operation: pass --source citing the feedback document")
}

// TestIntakeThenAmendWithSourceStages completes the seam's positive side: the
// same staged op succeeds once --source cites the landed document.
func TestIntakeThenAmendWithSourceStages(t *testing.T) {
	layAmendmentTree(t)
	src := writeFeedbackDoc(t, "review.md", "add a table-backed forms feature\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	aid, _ := jsonData(t, out)["id"].(string)

	out, stderr, code = execCLI(t, "amend", aid, "--add-feature", "form-engine/table-backed-forms",
		"--change-type", "create", "--title", "Table-backed forms",
		"--source", ".tiller/feedback/review.md#L1-L1", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	ops := data["operations"].([]any)
	require.Len(t, ops, 1)
}

// TestIntakeSourceHonestyArc is task-5-brief.md's own acceptance, pinned end
// to end in one test: intake lands the artifact and mints the amendment,
// `show amendment` reads the fresh document valid, editing it makes show say
// stale, approve refuses naming both hashes, restoring the file makes
// approve land.
func TestIntakeSourceHonestyArc(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "review.md", "the original review body\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	data := jsonData(t, out)
	aid, _ := data["id"].(string)
	require.NotEmpty(t, aid)
	source, ok := data["source"].(map[string]any)
	require.True(t, ok, "intake's own envelope carries source: %s", out)
	recorded, _ := source["revision"].(string)
	require.NotEmpty(t, recorded)

	landedPath := filepath.Join(".tiller", "feedback", "review.md")
	original, err := os.ReadFile(landedPath)
	require.NoError(t, err)

	// Fresh: show amendment reads the source valid.
	out, stderr, code = execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, stderr)
	source, _ = jsonData(t, out)["source"].(map[string]any)
	require.Equal(t, "valid", source["status"])
	require.Equal(t, recorded, source["actual"])

	// Edit the artifact: show now says stale, naming the live hash.
	require.NoError(t, os.WriteFile(landedPath, []byte("the EDITED review body\n"), 0o644))
	out, stderr, code = execCLI(t, "show", "amendment", aid, "--json")
	require.Equal(t, 0, code, stderr)
	source, _ = jsonData(t, out)["source"].(map[string]any)
	require.Equal(t, "stale", source["status"], "show amendment must say stale once the document changed")
	live, _ := source["actual"].(string)
	require.NotEmpty(t, live)
	require.NotEqual(t, recorded, live)

	// Approve refuses, naming both hashes, exit 2 (a tree fact, not a staged
	// precondition — the same misuse class every other non-precondition phase-1
	// gate in this file already answers, cli-surface.md's own approve-amendment
	// paragraph).
	out, stderr, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 2, code, stderr)
	require.Contains(t, out, `"reason": "source_changed"`)
	require.Contains(t, out, recorded)
	require.Contains(t, out, live)
	require.Contains(t, out, "re-read it and re-stage, or withdraw")

	// Restore the file: approve lands.
	require.NoError(t, os.WriteFile(landedPath, original, 0o644))
	out, stderr, code = execCLI(t, "approve", "amendment", aid, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	require.Equal(t, "approved", jsonData(t, out)["status"])
}

// TestShowAmendmentHumanRendersSourceLiveAcrossAllThreeStates is the fold-in
// review finding: renderAmendmentHuman's live: [status], ... switch
// (amendment_cmds.go) shipped with zero tests, valid/changed/vanished
// alike, unlike the sibling precondition rendering
// (TestShowAmendmentRendersLiveValidity's own [valid]/[stale]/[unknown]
// pins). The reviewer's own reproduction: reorder that switch's cases so
// Actual is checked before Note, and a vanished document renders "live:
// [stale], actual " with nothing after it — the exact confusion checkSource's
// own doc comment (amendment_approve.go) names the distinct vanish message
// as existing to prevent — and no test caught it. This is that test: the
// human (non-JSON) render, across all three states, with the vanished arm
// asserting the blank-actual shape never appears.
func TestShowAmendmentHumanRendersSourceLiveAcrossAllThreeStates(t *testing.T) {
	initOwnedRepo(t)
	src := writeFeedbackDoc(t, "review.md", "the original review body\n")

	out, stderr, code := execCLI(t, "intake", src, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr)
	aid, _ := jsonData(t, out)["id"].(string)
	require.NotEmpty(t, aid)
	landedPath := filepath.Join(".tiller", "feedback", "review.md")

	// Valid: the fresh artifact.
	human, stderr, code := execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr)
	require.Regexp(t, `live: \[valid\], actual git:[0-9a-f]{40}`, human)

	// Changed: an edit, both the status word and the live hash render.
	require.NoError(t, os.WriteFile(landedPath, []byte("edited\n"), 0o644))
	human, stderr, code = execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr)
	require.Regexp(t, `live: \[stale\], actual git:[0-9a-f]{40}`, human)

	// Vanished: distinct wording, and — the exact regression this test
	// exists to catch — never a blank "actual" standing in for the missing
	// hash.
	require.NoError(t, os.Remove(landedPath))
	human, stderr, code = execCLI(t, "show", "amendment", aid)
	require.Equal(t, 0, code, stderr)
	require.Contains(t, human, "live: [stale], the feedback document no longer exists in the working tree")
	require.NotContains(t, human, "live: [stale], actual ",
		"a vanished document must never render a blank actual hash")
	require.NotContains(t, human, "actual \n", "no line may end in a bare, valueless actual")
}
