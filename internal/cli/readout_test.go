package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// layDraftEpicTree lays a repo with two draft epics: the shape a `tiller
// readout decompose` fires against right after `tiller add epic`.
// readout reads git facts through
// internal/vcs, so it needs a real git repo with at least one commit for
// Head() to resolve; initOwnedRepo has carried its own git init and initial
// commit since the drift build's commit-anchor task, so
// there is no second git setup to do here anymore.
func layDraftEpicTree(t *testing.T) {
	t.Helper()
	initOwnedRepo(t)

	_, _, code := execCLI(t, "add", "epic", "form-engine",
		"--title", "Form Engine", "--source", "design/04-form-engine.md", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "epic", "billing",
		"--title", "Billing", "--source", "design/05-billing.md", "--contributor", "jc")
	require.Equal(t, 0, code)
}

// TestReadoutCommandPrintsTheBlockForASeam pins the human render for
// decompose: the moment's own headline, the Verification section, and the
// needs-you section every readout carries.
func TestReadoutCommandPrintsTheBlockForASeam(t *testing.T) {
	layDraftEpicTree(t)
	out, _, code := execCLI(t, "readout", "decompose")
	require.Equal(t, 0, code)
	require.Contains(t, out, "**Decomposition laid down**")
	require.Contains(t, out, "**Verification**")
	requireNeedsYouSection(t, out)
	require.Contains(t, out, "- 2 epics await your approval · `tiller queue --kind approve_epic`")
	attention, next := splitAtNext(t, out)
	require.NotContains(t, attention, "tiller approve epic", "Needs attention enumerates the category instead of mutating named epics")
	require.Contains(t, next, "`tiller approve epic billing`", "the ranked list names each move's command")
}

// TestReadoutCommandRefusesAnUnknownSeam confirms an unrecognized seam is
// misuse (exit 2) naming the legal seams, "orient" among them.
func TestReadoutCommandRefusesAnUnknownSeam(t *testing.T) {
	layDraftEpicTree(t)
	_, errOut, code := execCLI(t, "readout", "nonsense")
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "orient")
	require.Contains(t, errOut, "amend", "the fifth seam is named in the refusal too")
}

// TestReadoutCommandPrintsTheAmendSeam pins the amendment content plus its
// counts-only queue seam. The concrete amendment remains in Produced, while
// Needs attention enumerates the category instead of repeating its identity or
// offering the mutation directly.
func TestReadoutCommandPrintsTheAmendSeam(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")
	_, _, code := execCLI(t, "amend", aid, "--revise", "form-engine/certification-form-loading",
		"--precondition", "target_status=draft", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "readout", "amend")
	require.Equal(t, 0, code)
	require.Contains(t, out, "**Amendments surveyed**")
	require.Contains(t, out, "iphone-only (")
	require.Contains(t, out, "1 operation staged")
	require.Contains(t, out, "Operations staged: 1")
	require.Contains(t, out, "Preconditions: 1 valid, 0 stale",
		"the staged target_status=draft precondition holds against the still-draft feature")
	require.Contains(t, out, "- 1 proposed amendment awaits your approval · `tiller queue --kind approve_amendment`")
	attention, next := splitAtNext(t, out)
	require.NotContains(t, attention, "tiller approve amendment", "attention drill-down enumerates before any mutation")
	require.Contains(t, next, "1. You: approve amendment iphone-only (")
}

func TestAmendReadoutRechecksRevisionPreconditionsAfterProseChanges(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "refresh-spec", "Review a specification revision")
	root, err := os.Getwd()
	require.NoError(t, err)
	specPath := filepath.Join(featureDir(t), "feature.md")
	revision := "git:" + gitOutput(t, root, "hash-object", "--", specPath)
	_, _, code := execCLI(t, "amend", aid, "--revise", "form-engine/certification-form-loading",
		"--precondition", "spec_revision="+revision, "--contributor", "jc")
	require.Equal(t, 0, code)

	before, _, code := execCLI(t, "readout", "amend")
	require.Equal(t, 0, code)
	require.Contains(t, before, "Preconditions: 1 valid, 0 stale")

	prose, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(specPath, append(prose, []byte("\nChanged since the amendment was staged.\n")...), 0o644))

	after, _, code := execCLI(t, "readout", "amend")
	require.Equal(t, 0, code)
	require.Contains(t, after, "Preconditions: 0 valid, 1 stale",
		"an amendment readout must still validate current working-tree bytes")
}

// TestReadoutCommandPrintsTheAmendSeamHonestlyEmpty pins the zero case: with
// nothing proposed anywhere in the tree, the block still renders sanely rather
// than blank or broken, and NeedsYou has nothing to say. "Preconditions: 0
// valid, 0 stale" is a real answer, not "not evaluated": the CLI caller always
// runs mutate.EvaluateAmendmentPreconditions this pass
// (readoutAmendmentPreconditions returns a non-nil, possibly empty map), so a
// genuine zero renders rather than the nil-means-never-ran fallback. The
// fixture is a bare repo with no epics at all, not layAmendmentTree's own
// (which lays a draft epic that itself awaits approval and would need its own
// gate to clear before NeedsYou could ever read empty).
func TestReadoutCommandPrintsTheAmendSeamHonestlyEmpty(t *testing.T) {
	initOwnedRepo(t)
	out, _, code := execCLI(t, "readout", "amend")
	require.Equal(t, 0, code)
	require.Contains(t, out, "**Amendments surveyed**")
	require.Contains(t, out, "Operations staged: 0")
	require.Contains(t, out, "Preconditions: 0 valid, 0 stale")
	require.Contains(t, out, "Nothing needs you right now.")
	require.Contains(t, out, "**Next:**", "the block still closes on a Next line")
}

// TestReadoutDegradesOnACorruptIdentityFile is readout's twin of
// TestGuideDegradesOnACorruptIdentityFile (guide_test.go): readout carries
// no --contributor flag and never calls deps(), so it is a read surface too, and
// a present but unparseable .tiller/identity.yml must degrade rather than
// refuse the way a mutation does. Issue #52's design ruling names both guide
// and readout as the surfaces that degrade; without this pin, a future
// change that pointed readout's owner resolution at the mutating, refusing
// sibling would turn this diagnostic surface dark on a corrupt file too,
// with nothing here to catch it.
func TestReadoutDegradesOnACorruptIdentityFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code)
	corruptIdentityFile(t, dir)

	out, _, code := execCLI(t, "readout", "orient", "--json")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, "identity_unreadable", "a read surface degrades to no id rather than refusing on a corrupt file")
}

func TestReadoutUsesQueueInputToSuppressABasisDriftedFeatureCheck(t *testing.T) {
	layApprovedFeatureWithTask(t)
	_, _, code := execCLI(t, "start", "task", soleTaskID(t), "--contributor", "jc")
	require.Equal(t, 0, code)
	setTaskStatus(t, "transcribe-the-schema", "in_progress", "done")

	clean, _, code := execCLI(t, "readout", "execute")
	require.Equal(t, 0, code)
	require.Contains(t, clean,
		"- 1 feature awaits a check run · `tiller queue --kind feature_check`",
		"the clean candidate establishes that the attention category is otherwise present")

	specPath := filepath.Join(featureDir(t), "feature.md")
	spec, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(specPath, append(spec, []byte("\nBasis drift after approval.\n")...), 0o644))

	drifted, _, code := execCLI(t, "readout", "execute")
	require.Equal(t, 0, code)
	require.NotContains(t, drifted, "tiller queue --kind feature_check",
		"readout gathering computes the same live basis suppression as /api/queue")
}

// TestReadoutJSONCarriesTheSameTextAsHuman pins the design's central promise:
// the terminal's human text and the --json "readout" field are the same
// string, so a harness reading --json reproduces exactly what a developer
// running the bare command would have read themselves.
func TestReadoutJSONCarriesTheSameTextAsHuman(t *testing.T) {
	layDraftEpicTree(t)
	human, _, code := execCLI(t, "readout", "decompose")
	require.Equal(t, 0, code)
	raw, _, code := execCLI(t, "--json", "readout", "decompose")
	require.Equal(t, 0, code)

	var env struct {
		Data struct {
			Readout string `json:"readout"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	require.Equal(t, strings.TrimSpace(human), strings.TrimSpace(env.Data.Readout),
		"terminal and harness see one readout, never two")
}

// TestReadoutGathersUnpushedReviewsOnlyForExecute is finding 2's covering
// assertion (final whole-branch review, batch 42-45): every gate readout used
// to pay for UnpushedReviews's git reads (UnpushedTip's merge-base plus
// ShortSHA, two subprocesses per in-review PR-backed task) even though only
// executeContent (internal/views/readout.go) ever reads the field. A test
// observing the subprocess count directly is impractical, so this asserts on
// gatherReadoutInput's own output, the CLI seam renderReadoutOf reads before
// handing off to views: it lays a real in-review, PR-backed task whose
// feature branch holds a commit its remote-tracking counterpart does not
// (TestSubmitConfirmationWarnsWhenTheStateCommitIsUnpushed's own fixture),
// the exact shape readoutUnpushedReviews finds something for, so execute's
// non-empty result and orient's nil one can only mean the gather ran for one
// moment and never for the other, not that there was nothing to find either
// way.
func TestReadoutGathersUnpushedReviewsOnlyForExecute(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, dir, "remote", "add", "origin", bare)

	tid := walkTaskToCheck(t)
	branch := loadTestTree(t).Tasks[tid].Branch
	gitRun(t, dir, "push", "-u", "origin", branch)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.txt"), []byte("state\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "state commit")

	_, _, code := execCLI(t, "submit", "task", tid, "--pr", "41", "--contributor", "jc")
	require.Equal(t, 0, code)

	tree, err := store.Load(dir)
	require.NoError(t, err)

	execIn := gatherReadoutInput(tree, views.MomentExecute, nil, "jc")
	require.NotEmpty(t, execIn.UnpushedReviews,
		"the fixture only proves the gate works if execute genuinely finds the unpushed review")

	orientIn := gatherReadoutInput(tree, views.MomentOrient, nil, "jc")
	require.Nil(t, orientIn.UnpushedReviews,
		"a non-execute moment must never run the gather at all, not merely omit it from the render")
}

// gitOutput runs a git command in dir and returns its trimmed stdout, for
// the shas the --since tests need to hand back to the command.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// layCorrectedDraftTree is the issue #94 correction loop on a scratch
// project: a decompose laydown committed (the sha the full block would have
// reported), then one of each correction the decompose skill maps (rename,
// link, add question, remove), committed again. It returns the repo root and
// the laydown's short sha.
func layCorrectedDraftTree(t *testing.T) (dir, since string) {
	t.Helper()
	layDraftEpicTree(t)
	dir, err := os.Getwd()
	require.NoError(t, err)
	for _, key := range []string{"form-engine/loading", "form-engine/schema", "form-engine/obsolete"} {
		_, _, code := execCLI(t, "add", "feature", key, "--title", "Feature "+key, "--contributor", "jc")
		require.Equal(t, 0, code)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "decompose: lay the tree down")
	since = gitOutput(t, dir, "rev-parse", "--short", "HEAD")

	_, _, code := execCLI(t, "rename", "feature", "form-engine/loading", "--title", "Certification form loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "link", "feature", "form-engine/loading", "--depends-on", "form-engine/schema", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "add", "question", "Which schema version does loading target?", "--title", "Schema version", "--blocks", "form-engine/loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	_, _, code = execCLI(t, "remove", "feature", "form-engine/obsolete", "--reason", "folded into loading", "--contributor", "jc")
	require.Equal(t, 0, code)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "decompose: corrections")
	return dir, since
}

// TestReadoutSinceRendersTheCorrectionBlock drives `tiller readout decompose
// --since <sha>` over a real correction loop and pins the block: the
// corrected headline naming both shas, one Changed line per recorded event
// in stream order, no full-block sections, and the queue and Next lines kept.
func TestReadoutSinceRendersTheCorrectionBlock(t *testing.T) {
	dir, since := layCorrectedDraftTree(t)
	head := gitOutput(t, dir, "rev-parse", "--short", "HEAD")

	out, errOut, code := execCLI(t, "readout", "decompose", "--since", since)
	require.Equal(t, 0, code, errOut)
	require.True(t, strings.HasPrefix(out, "**Decomposition corrected** · committed at `"+head+"` (since `"+since+"`)\n"), out)
	require.Contains(t, out, "**Changed**")
	require.Regexp(t, `- renamed form-engine/loading \(F-[a-z0-9]{6}\) title from "Feature form-engine/loading" to "Certification form loading"`, out)
	require.Regexp(t, `- linked 1 edge \(0 cross-epic\) on form-engine/loading \(F-[a-z0-9]{6}\)`, out)
	require.Regexp(t, `- added question Q-[a-z0-9]{6} \("Schema version"\); blocks form-engine/loading \(F-[a-z0-9]{6}\)`, out)
	require.Regexp(t, `- removed form-engine/obsolete \(F-[a-z0-9]{6}\)`, out)
	require.Less(t, strings.Index(out, "- renamed"), strings.Index(out, "- linked"))
	require.Less(t, strings.Index(out, "- linked"), strings.Index(out, "- added question"))
	require.Less(t, strings.Index(out, "- added question"), strings.Index(out, "- removed"))
	for _, absent := range []string{"**Produced**", "**Verification**", needsAttentionHeading, "Nothing needs you right now."} {
		require.NotContains(t, out, absent)
	}
	require.Contains(t, out, "\nqueue: 0 bugs, 0 chores, 0 tweaks ready\n")
	require.Contains(t, out, "**Next:**")

	full, _, code := execCLI(t, "readout", "decompose")
	require.Equal(t, 0, code)
	nextOf := func(block string) string { return block[strings.LastIndex(block, "**Next:**"):] }
	require.Equal(t, nextOf(full), nextOf(out), "the correction block's Next is the full block's, unchanged")
}

// TestReadoutSinceWithNothingRecordedSaysSo pins the honest empty case:
// --since HEAD has nothing after it, and the block says so under Changed
// rather than rendering an empty section.
func TestReadoutSinceWithNothingRecordedSaysSo(t *testing.T) {
	dir, _ := layCorrectedDraftTree(t)
	head := gitOutput(t, dir, "rev-parse", "--short", "HEAD")
	out, _, code := execCLI(t, "readout", "decompose", "--since", "HEAD")
	require.Equal(t, 0, code)
	require.Contains(t, out, "**Changed**\n\n- nothing recorded since `"+head+"`\n")
}

// TestReadoutSinceRefusesAShaGitCannotResolve pins the misuse: a --since git
// cannot resolve is exit 2, naming the flag and what it expects.
func TestReadoutSinceRefusesAShaGitCannotResolve(t *testing.T) {
	layCorrectedDraftTree(t)
	_, errOut, code := execCLI(t, "readout", "decompose", "--since", "nonsense")
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "--since")
	require.Contains(t, errOut, "nonsense")
	require.Contains(t, errOut, "committed at", "the message says which sha the flag expects: the one the last block reported")
}

// TestReadoutSinceRefusesADashLedValue pins the guard ahead of git: a value
// that reads as an option is the same misuse as an unknown sha, exit 2, not
// an environment error.
func TestReadoutSinceRefusesADashLedValue(t *testing.T) {
	layCorrectedDraftTree(t)
	_, errOut, code := execCLI(t, "readout", "decompose", "--since", "-x")
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "--since")
	require.Contains(t, errOut, "committed at")
}

// TestReadoutSinceRefusesACommitOffHeadsLine pins the second misuse: a real
// commit that is not an ancestor of HEAD (a sha from another branch) is exit
// 2 too, since nothing between it and HEAD is a correction to report.
func TestReadoutSinceRefusesACommitOffHeadsLine(t *testing.T) {
	dir, _ := layCorrectedDraftTree(t)
	trunk := gitOutput(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
	gitRun(t, dir, "checkout", "-q", "-b", "side")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "side.txt"), []byte("side\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "side commit")
	side := gitOutput(t, dir, "rev-parse", "--short", "HEAD")
	gitRun(t, dir, "checkout", "-q", trunk)

	_, errOut, code := execCLI(t, "readout", "decompose", "--since", side)
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "--since")
	require.Contains(t, errOut, "ancestor")
}

// TestReadoutSinceWithAnUnknownSeamKeepsTheSeamRefusal pins that the seam
// check still comes first: --since never turns an unknown seam into a
// different error.
func TestReadoutSinceWithAnUnknownSeamKeepsTheSeamRefusal(t *testing.T) {
	layCorrectedDraftTree(t)
	_, errOut, code := execCLI(t, "readout", "nonsense", "--since", "HEAD")
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "unknown seam")
}

// TestReadoutSinceJSONCarriesTheSameTextAsHuman is the correction block's
// copy of the design's central promise: the --json readout field is the
// piped human output, byte for byte, and the envelope names the since sha.
func TestReadoutSinceJSONCarriesTheSameTextAsHuman(t *testing.T) {
	_, since := layCorrectedDraftTree(t)
	human, _, code := execCLI(t, "readout", "decompose", "--since", since)
	require.Equal(t, 0, code)
	raw, _, code := execCLI(t, "--json", "readout", "decompose", "--since", since)
	require.Equal(t, 0, code)

	var env struct {
		Data struct {
			Moment  string `json:"moment"`
			Since   string `json:"since"`
			Readout string `json:"readout"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &env))
	require.Equal(t, "decompose", env.Data.Moment)
	require.Equal(t, since, env.Data.Since)
	require.Equal(t, strings.TrimSpace(human), strings.TrimSpace(env.Data.Readout),
		"terminal and harness see one correction block, never two")
}
