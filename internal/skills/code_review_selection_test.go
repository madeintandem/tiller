package skills

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// The code-review skill decides, per unit, whether to judge the integration
// branch or the unit's own branch head. These tests build small real Git
// histories and run the commands the skill documents for that decision, so a
// procedure that picks the wrong snapshot fails here on a real repository
// rather than passing because the prose still contains a command.
//
// This is not a skill interpreter. The decision sequence below is written out
// in Go; what comes from the skill is every Git command it runs (each must
// appear verbatim in the skill's locate block) and the one documented rule
// this procedure turns on, what a failed ancestry check means.

const (
	reviewMerged         = "merged"
	reviewInFlight       = "in flight"
	reviewNotEstablished = "not established"
	reviewNotLocated     = "not located"
)

// reviewUnit is what the skill reads from a unit's state file.
type reviewUnit struct {
	id         string
	status     string
	branch     string // recorded branch; the ref may be gone
	reviewHead string // review_ref.head, without the git: prefix
	aggregate  *reviewUnit
}

// reviewSelection is the outcome of the documented selection for one unit.
type reviewSelection struct {
	class    string
	evidence string
	snapshot string // the revision whose code findings are judged against
	located  string // the diff or commits that locate the unit's change
}

// fixOnBranchAllowed mirrors the disposition rule: "fix on its branch" is only
// for a row found in unmerged code.
func (s reviewSelection) fixOnBranchAllowed() bool { return s.class == reviewInFlight }

// reviewProcedure runs the skill's documented commands in one repository.
type reviewProcedure struct {
	t           *testing.T
	dir         string
	block       string
	integration string
}

// codeReviewLocateBlock returns the fenced command block under the review
// arm's "Locate each unit's change" step.
func codeReviewLocateBlock(t *testing.T) string {
	t.Helper()
	text, ok := Lookup("code-review")
	require.True(t, ok)
	_, rest, found := strings.Cut(text, "#### 2. Locate each unit's change\n")
	require.True(t, found, "the review arm has a locate step")
	_, rest, found = strings.Cut(rest, "```sh\n")
	require.True(t, found, "the locate step has a command block")
	block, _, found := strings.Cut(rest, "\n```")
	require.True(t, found, "the locate block is closed")
	return block
}

// failedAncestryRule returns what the block says a failed ancestry check
// means: the text of its "# exit 1:" comment.
func failedAncestryRule(t *testing.T, block string) string {
	t.Helper()
	for _, line := range strings.Split(block, "\n") {
		if rule, ok := strings.CutPrefix(line, "# exit 1: "); ok {
			return rule
		}
	}
	t.Fatal("the locate block no longer says what a failed ancestry check (exit 1) means")
	return ""
}

// run executes one documented command line. The template must appear verbatim
// in the skill's block; values replace its <placeholders>. It returns stdout
// and the exit code.
func (p reviewProcedure) run(template string, values ...string) (string, int) {
	p.t.Helper()
	require.Containsf(p.t, p.block, template, "the code-review locate block documents %q", template)
	line := strings.NewReplacer(values...).Replace(template)
	cmd := exec.Command("sh", "-c", line)
	cmd.Dir = p.dir
	cmd.Env = testgit.Env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		require.Truef(p.t, ok, "%s: %v\n%s", line, err, stderr.String())
		code = exit.ExitCode()
	}
	return strings.TrimSpace(stdout.String()), code
}

// refExists is fixture plumbing, not part of the documented selection: it
// answers whether the recorded branch still names a commit.
func (p reviewProcedure) refExists(branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = p.dir
	cmd.Env = testgit.Env()
	return cmd.Run() == nil
}

func (p reviewProcedure) show(commits []string) string {
	var out []string
	for _, commit := range commits {
		diff, code := p.run("git --no-optional-locks show --no-ext-diff --no-textconv <commit> -- . ':(exclude).tiller'", "<commit>", commit)
		require.Zero(p.t, code)
		out = append(out, diff)
	}
	return strings.Join(out, "\n")
}

// integrationEvidence looks for a unit's work on integration when its head is
// not an ancestor: the recorded done, commits carrying its ID, or one commit
// that is the branch's whole change.
func (p reviewProcedure) integrationEvidence(u reviewUnit, head, mergeBase string) (evidence string, commits []string) {
	if u.status == "done" {
		evidence = "recorded done"
	}
	ids, code := p.run("git --no-optional-locks log --format=%H --grep=<unit-id> <merge-base>..<integration> -- . ':(exclude).tiller'",
		"<unit-id>", u.id, "<merge-base>", mergeBase, "<integration>", p.integration)
	require.Zero(p.t, code)
	if ids != "" {
		return "unit ID commits", strings.Fields(ids)
	}
	branchPatch, code := p.run("git --no-optional-locks diff --no-ext-diff --no-textconv <merge-base> <head> -- . ':(exclude).tiller' | git patch-id --stable",
		"<merge-base>", mergeBase, "<head>", head)
	require.Zero(p.t, code)
	integrationPatches, code := p.run("git --no-optional-locks log --first-parent --no-merges -p --no-ext-diff --no-textconv <merge-base>..<integration> -- . ':(exclude).tiller' | git patch-id --stable",
		"<merge-base>", mergeBase, "<integration>", p.integration)
	require.Zero(p.t, code)
	if patch := strings.Fields(branchPatch); len(patch) > 0 {
		for _, line := range strings.Split(integrationPatches, "\n") {
			if fields := strings.Fields(line); len(fields) == 2 && fields[0] == patch[0] {
				return "whole-change commit", []string{fields[1]}
			}
		}
	}
	return evidence, nil
}

// selectSnapshot follows the documented selection for one unit.
func (p reviewProcedure) selectSnapshot(u reviewUnit) reviewSelection {
	p.t.Helper()
	head := u.reviewHead
	branchExists := u.branch != "" && p.refExists(u.branch)
	if branchExists {
		head = testgit.Run(p.t, p.dir, "rev-parse", "refs/heads/"+u.branch)
	}

	// No head: the commits carrying the unit's ID are all there is.
	if _, code := p.run("git --no-optional-locks cat-file -e <head>^{commit}", "<head>", head); code != 0 {
		ids, code := p.run("git --no-optional-locks log --format=%H --grep=<unit-id> <integration>", "<unit-id>", u.id, "<integration>", p.integration)
		require.Zero(p.t, code)
		if ids == "" {
			return reviewSelection{class: reviewNotLocated}
		}
		return reviewSelection{class: reviewMerged, evidence: "unit ID commits", snapshot: p.integration, located: p.show(strings.Fields(ids))}
	}

	_, ancestry := p.run("git --no-optional-locks merge-base --is-ancestor <head> <integration>", "<head>", head, "<integration>", p.integration)
	if ancestry == 0 {
		merges, code := p.run("git --no-optional-locks rev-list --first-parent --merges --reverse <integration>", "<integration>", p.integration)
		require.Zero(p.t, code)
		for _, merge := range strings.Fields(merges) {
			_, contains := p.run("git --no-optional-locks merge-base --is-ancestor <head> <merge>", "<head>", head, "<merge>", merge)
			_, parentLacks := p.run("! git --no-optional-locks merge-base --is-ancestor <head> <merge>^1", "<head>", head, "<merge>", merge)
			if contains == 0 && parentLacks == 0 {
				diff, code := p.run("git --no-optional-locks diff --no-ext-diff --no-textconv <merge>^1...<head> -- . ':(exclude).tiller'", "<merge>", merge, "<head>", head)
				require.Zero(p.t, code)
				return reviewSelection{class: reviewMerged, evidence: "ancestry", snapshot: p.integration, located: diff}
			}
		}
		ids, code := p.run("git --no-optional-locks log --format=%H --grep=<unit-id> <integration>", "<unit-id>", u.id, "<integration>", p.integration)
		require.Zero(p.t, code)
		return reviewSelection{class: reviewMerged, evidence: "ancestry", snapshot: p.integration, located: p.show(strings.Fields(ids))}
	}

	mergeBase, code := p.run("git --no-optional-locks merge-base <integration> <head>", "<integration>", p.integration, "<head>", head)
	require.Zero(p.t, code)
	branchDiff := func() string {
		diff, code := p.run("git --no-optional-locks diff --no-ext-diff --no-textconv <merge-base> <head> -- . ':(exclude).tiller'", "<merge-base>", mergeBase, "<head>", head)
		require.Zero(p.t, code)
		return diff
	}

	// The one rule read from the skill: what a failed ancestry check means.
	rule := failedAncestryRule(p.t, p.block)
	if strings.HasPrefix(rule, "in flight") {
		return reviewSelection{class: reviewInFlight, snapshot: head, located: branchDiff()}
	}
	require.Truef(p.t, strings.HasPrefix(rule, "inconclusive"), "a failed ancestry check is in flight or inconclusive, got %q", rule)

	evidence, commits := p.integrationEvidence(u, head, mergeBase)
	if evidence == "" && u.aggregate != nil {
		aggregateHead := testgit.Run(p.t, p.dir, "rev-parse", "refs/heads/"+u.aggregate.branch)
		if _, rides := p.run("git --no-optional-locks merge-base --is-ancestor <head> <aggregate-head>", "<head>", head, "<aggregate-head>", aggregateHead); rides == 0 {
			if aggregate := p.selectSnapshot(*u.aggregate); aggregate.class == reviewMerged {
				evidence = "aggregate: " + aggregate.evidence
			}
		}
	}
	switch {
	case evidence != "":
		located := branchDiff()
		if len(commits) > 0 {
			located = p.show(commits)
		}
		return reviewSelection{class: reviewMerged, evidence: evidence, snapshot: p.integration, located: located}
	case branchExists:
		return reviewSelection{class: reviewInFlight, snapshot: head, located: branchDiff()}
	default:
		return reviewSelection{class: reviewNotEstablished}
	}
}

// reviewFixture is one small repository: a main integration branch and a task
// branch whose handler line carries a defect.
type reviewFixture struct {
	t   *testing.T
	dir string
}

const (
	reviewTaskID     = "T-aaa111"
	reviewTaskBranch = "tiller/epic/feat/handler"
	defectLine       = "handler with the defect"
	fixedLine        = "handler fixed"
)

func newReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	f := reviewFixture{t: t, dir: t.TempDir()}
	testgit.Init(t, f.dir)
	f.write("app.txt", "base\n")
	f.write(".tiller/state.yml", "status: ready\n")
	f.commit("base")
	return f
}

func (f reviewFixture) write(path, content string) {
	f.t.Helper()
	full := filepath.Join(f.dir, path)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(f.t, os.WriteFile(full, []byte(content), 0o644))
}

func (f reviewFixture) git(args ...string) string {
	f.t.Helper()
	return testgit.Run(f.t, f.dir, args...)
}

func (f reviewFixture) commit(message string) string {
	f.t.Helper()
	f.git("add", "-A")
	f.git("commit", "-q", "-m", message)
	return f.git("rev-parse", "HEAD")
}

// buildTask commits the task's work, defect included, on its own branch and
// returns the branch head. main is left checked out.
func (f reviewFixture) buildTask() string {
	f.t.Helper()
	f.git("checkout", "-q", "-b", reviewTaskBranch)
	f.write("app.txt", "base\n"+defectLine+"\n")
	f.write(".tiller/state.yml", "status: in_progress\n")
	f.commit(reviewTaskID + ": add the handler")
	f.write("handler_test.txt", "covers the handler\n")
	head := f.commit(reviewTaskID + ": cover the handler")
	f.git("checkout", "-q", "main")
	return head
}

// moveIntegration lands unrelated work on main so no merge can fast-forward.
func (f reviewFixture) moveIntegration() {
	f.t.Helper()
	f.write("other.txt", "unrelated trunk work\n")
	f.commit("Unrelated trunk work")
}

func (f reviewFixture) squash(branch, message string) {
	f.t.Helper()
	f.git("merge", "-q", "--squash", branch)
	f.commit(message)
}

func (f reviewFixture) rebaseMerge(branch string) {
	f.t.Helper()
	f.git("checkout", "-q", "-b", "rebased", branch)
	f.git("rebase", "-q", "main")
	f.git("checkout", "-q", "main")
	f.git("merge", "-q", "--ff-only", "rebased")
	f.git("branch", "-q", "-D", "rebased")
}

// integrateLaterFix lands a later unit's fix for the defect on main.
func (f reviewFixture) integrateLaterFix() {
	f.t.Helper()
	f.write("app.txt", "base\n"+fixedLine+"\n")
	f.commit("B-bbb222: fix the handler")
}

func (f reviewFixture) procedure(t *testing.T) reviewProcedure {
	return reviewProcedure{t: t, dir: f.dir, block: codeReviewLocateBlock(t), integration: "main"}
}

func (f reviewFixture) fileAt(rev, path string) string {
	f.t.Helper()
	return f.git("show", rev+":"+path)
}

// Every way a task's work can reach integration selects the integration
// snapshot, whether or not the branch ref was kept, and a defect a later unit
// already fixed on integration is not in the code the review judges. A failed
// ancestry check alone never selects the branch head.
func TestCodeReviewSelectsIntegrationForMergedWork(t *testing.T) {
	for _, tt := range []struct {
		name      string
		integrate func(f reviewFixture)
		evidence  string
	}{
		{"merge commit", func(f reviewFixture) {
			f.moveIntegration()
			f.git("merge", "-q", "--no-ff", "-m", "Merge "+reviewTaskBranch, reviewTaskBranch)
		}, "ancestry"},
		{"fast-forward", func(f reviewFixture) {
			f.git("merge", "-q", "--ff-only", reviewTaskBranch)
		}, "ancestry"},
		{"squash", func(f reviewFixture) {
			f.moveIntegration()
			f.squash(reviewTaskBranch, "epic/feat/handler ("+reviewTaskID+") (#12)")
		}, "unit ID commits"},
		{"squash with a rewritten message", func(f reviewFixture) {
			f.moveIntegration()
			f.squash(reviewTaskBranch, "Ship the handler (#12)")
		}, "whole-change commit"},
		{"rebase", func(f reviewFixture) {
			f.moveIntegration()
			f.rebaseMerge(reviewTaskBranch)
		}, "unit ID commits"},
	} {
		for _, ref := range []string{"retained ref", "deleted ref"} {
			t.Run(tt.name+"/"+ref, func(t *testing.T) {
				t.Parallel()
				f := newReviewFixture(t)
				head := f.buildTask()
				tt.integrate(f)
				f.integrateLaterFix()
				if ref == "deleted ref" {
					f.git("branch", "-q", "-D", reviewTaskBranch)
				}
				// Not yet synced: the status still reads in flight.
				unit := reviewUnit{id: reviewTaskID, status: "in_review", branch: reviewTaskBranch, reviewHead: head}

				got := f.procedure(t).selectSnapshot(unit)

				require.Equal(t, reviewMerged, got.class, "merged work is never classified from a failed ancestry check alone")
				require.Equal(t, tt.evidence, got.evidence)
				require.Equal(t, "main", got.snapshot, "merged work is judged against integration")
				require.Contains(t, got.located, defectLine, "the located change still shows what the unit changed")
				judged := f.fileAt(got.snapshot, "app.txt")
				require.Contains(t, judged, fixedLine)
				require.NotContains(t, judged, defectLine, "a defect a later unit fixed on integration is not rediscovered")
				require.Contains(t, f.fileAt(head, "app.txt"), defectLine, "the obsolete branch head still carries it")
				require.False(t, got.fixOnBranchAllowed(), "a merged branch takes no further commits")
			})
		}
	}
}

// Work that is on no integration commit and still has its branch is in flight:
// it is judged at its branch head and may be fixed there.
func TestCodeReviewSelectsTheBranchHeadForInFlightWork(t *testing.T) {
	t.Parallel()
	f := newReviewFixture(t)
	head := f.buildTask()
	f.moveIntegration()
	unit := reviewUnit{id: reviewTaskID, status: "in_review", branch: reviewTaskBranch, reviewHead: head}

	got := f.procedure(t).selectSnapshot(unit)

	require.Equal(t, reviewInFlight, got.class)
	require.Equal(t, head, got.snapshot)
	require.Contains(t, got.located, defectLine)
	require.Contains(t, f.fileAt(got.snapshot, "app.txt"), defectLine)
	require.True(t, got.fixOnBranchAllowed())
}

// With the branch gone and nothing on integration carrying the work, the
// crossing cannot be established. The unit is not reviewed as live work at its
// stale recorded head, and nothing is sent back to a branch that is gone.
func TestCodeReviewStatesALimitationWhenTheCrossingIsNotEstablished(t *testing.T) {
	t.Parallel()
	f := newReviewFixture(t)
	head := f.buildTask()
	f.moveIntegration()
	f.git("branch", "-q", "-D", reviewTaskBranch)
	unit := reviewUnit{id: reviewTaskID, status: "in_review", branch: reviewTaskBranch, reviewHead: head}

	got := f.procedure(t).selectSnapshot(unit)

	require.Equal(t, reviewNotEstablished, got.class)
	require.Empty(t, got.snapshot)
	require.False(t, got.fixOnBranchAllowed())

	text, _ := Lookup("code-review")
	require.Contains(t, text, "| Crossing not established:")
}

// A recorded done is evidence on its own: sync observed the crossing or the
// human asserted it, so the unit is judged against integration even when Git
// shows no commit for it.
func TestCodeReviewTrustsARecordedDoneCrossing(t *testing.T) {
	t.Parallel()
	f := newReviewFixture(t)
	head := f.buildTask()
	f.moveIntegration()
	f.integrateLaterFix()
	unit := reviewUnit{id: reviewTaskID, status: "done", branch: reviewTaskBranch, reviewHead: head}

	got := f.procedure(t).selectSnapshot(unit)

	require.Equal(t, reviewMerged, got.class)
	require.Equal(t, "recorded done", got.evidence)
	require.Equal(t, "main", got.snapshot)
	require.Contains(t, got.located, defectLine, "the head's diff still locates the lines")
	require.False(t, got.fixOnBranchAllowed())
}

// A riding task crosses with its aggregate. When the aggregate is squashed
// into integration under the feature's ID, neither the task's ID nor its own
// patch is on integration, and the aggregate's crossing stands for the task.
func TestCodeReviewRidingTaskCrossesWithItsAggregate(t *testing.T) {
	t.Parallel()
	const aggregateBranch = "tiller/reviews/features/feat"
	f := newReviewFixture(t)
	head := f.buildTask()
	f.git("checkout", "-q", "-b", aggregateBranch)
	f.git("merge", "-q", "--no-ff", "-m", "Assemble "+reviewTaskID, reviewTaskBranch)
	f.write("second.txt", "a second riding task\n")
	f.commit("T-ccc333: add the second piece")
	f.git("checkout", "-q", "main")
	f.moveIntegration()
	aggregate := &reviewUnit{id: "F-fff000", status: "active", branch: aggregateBranch}
	unit := reviewUnit{id: reviewTaskID, status: "in_review", branch: reviewTaskBranch, reviewHead: head, aggregate: aggregate}

	before := f.procedure(t).selectSnapshot(unit)
	require.Equal(t, reviewInFlight, before.class, "an unmerged aggregate leaves its riding task in flight")
	require.True(t, before.fixOnBranchAllowed())

	f.squash(aggregateBranch, "epic/feat (F-fff000) (#20)")
	f.integrateLaterFix()

	after := f.procedure(t).selectSnapshot(unit)
	require.Equal(t, reviewMerged, after.class)
	require.Equal(t, "aggregate: unit ID commits", after.evidence)
	require.Equal(t, "main", after.snapshot)
	require.NotContains(t, f.fileAt(after.snapshot, "app.txt"), defectLine)
	require.False(t, after.fixOnBranchAllowed())
}

// With no head left at all, the integration commits carrying the unit's ID are
// the located change, judged against integration.
func TestCodeReviewLocatesAUnitWithNoHeadByItsIDCommits(t *testing.T) {
	t.Parallel()
	f := newReviewFixture(t)
	f.buildTask()
	f.moveIntegration()
	f.squash(reviewTaskBranch, "epic/feat/handler ("+reviewTaskID+") (#12)")
	f.git("branch", "-q", "-D", reviewTaskBranch)
	unit := reviewUnit{id: reviewTaskID, status: "done", branch: reviewTaskBranch, reviewHead: strings.Repeat("0", 39) + "1"}

	got := f.procedure(t).selectSnapshot(unit)

	require.Equal(t, reviewMerged, got.class)
	require.Equal(t, "main", got.snapshot)
	require.Contains(t, got.located, defectLine)
}
