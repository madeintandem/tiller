package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// compactClock pins the verb's clock to a fixed day for the test.
func compactClock(t *testing.T, now string) {
	t.Helper()
	at, err := time.Parse(time.RFC3339, now)
	require.NoError(t, err)
	old := compactNow
	compactNow = func() time.Time { return at }
	t.Cleanup(func() { compactNow = old })
}

// compactRepo is initRepo as a git checkout on main with one commit, since the
// verb asks git which branch it is on.
func compactRepo(t *testing.T) string {
	t.Helper()
	dir := initRepo(t)
	testgit.Init(t, dir)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")
	return dir
}

// layUncarriableEvent writes one loose event file dated when under dir with a
// key the event shape does not define, so no segment can carry it.
func layUncarriableEvent(t *testing.T, dir, id, when string) string {
	t.Helper()
	path := layLooseEvent(t, dir, id, when)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, []byte("surprise: true\n")...), 0o644))
	return path
}

// stageLine returns the git add line a compaction printed, and fails when there
// is none.
func stageLine(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  git add -A -- ") {
			return strings.TrimSpace(line)
		}
	}
	require.FailNow(t, "no git add line", out)
	return ""
}

// layLooseEvent writes one loose event file dated when under dir.
func layLooseEvent(t *testing.T, dir, id, when string) string {
	t.Helper()
	at, err := time.Parse(time.RFC3339, when)
	require.NoError(t, err)
	e := events.Event{ID: id, Type: "add.task", At: at, Contributor: "jc", Command: "tiller add task", Targets: []string{"T-aaaaaa"}}
	data, err := yaml.Marshal(e)
	require.NoError(t, err)
	path := filepath.Join(dir, events.RelPath(e))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func TestCompactVerbFoldsEligibleDaysAndPrintsTheStageCommand(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")
	layLooseEvent(t, dir, "EV-bbbbbb", "2026-11-03T11:00:00Z")
	layLooseEvent(t, dir, "EV-cccccc", "2026-11-04T10:00:00Z")
	layLooseEvent(t, dir, "EV-dddddd", "2026-11-11T09:00:00Z")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "four events")

	out, _, code := execCLI(t, "compact")
	require.Equal(t, 0, code, out)

	res, err := mutate.Compact(dir, mutate.CompactOptions{Now: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, res.Days, "nothing eligible is left")
	segments, err := filepath.Glob(filepath.Join(dir, ".tiller", "events", "2026", "11", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, segments, 2)
	rel := func(p string) string {
		r, err := filepath.Rel(dir, p)
		require.NoError(t, err)
		return filepath.ToSlash(r)
	}
	require.Equal(t, "Compacted 2 days of events (3 events) into 2 files. Stage them with:\n"+
		"  git add -A -- .tiller/events/2026/11/03 "+rel(segments[0])+" .tiller/events/2026/11/04 "+rel(segments[1])+"\n", out)
}

// TestCompactVerbStageCommandRunsWhenADayWasNeverCommitted runs the printed git
// add line in a repository where one compacted day was committed and another
// never was. Naming a directory git never tracked would make git add refuse the
// whole command, so the line leaves that directory out and still stages every
// segment and every deletion git can see.
func TestCompactVerbStageCommandRunsWhenADayWasNeverCommitted(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	committed := layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "one committed event")
	layLooseEvent(t, dir, "EV-bbbbbb", "2026-11-04T10:00:00Z")

	out, _, code := execCLI(t, "compact", "--all")
	require.Equal(t, 0, code, out)
	line := stageLine(t, out)
	require.NotContains(t, line, ".tiller/events/2026/11/04 ", "a day git never tracked is not named")

	git := exec.Command("git", strings.Fields(line)[1:]...)
	git.Dir = dir
	output, err := git.CombinedOutput()
	require.NoError(t, err, "%s\n%s", line, output)

	segments, err := filepath.Glob(filepath.Join(dir, ".tiller", "events", "2026", "11", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, segments, 2)
	staged := gitOutput(t, dir, "diff", "--cached", "--name-status")
	for _, segment := range segments {
		rel, err := filepath.Rel(dir, segment)
		require.NoError(t, err)
		require.Contains(t, staged, "A\t"+filepath.ToSlash(rel))
	}
	rel, err := filepath.Rel(dir, committed)
	require.NoError(t, err)
	require.Contains(t, staged, "D\t"+filepath.ToSlash(rel))
}

func TestCompactVerbStageListInJSONMatchesTheLine(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")

	out, _, code := execCLI(t, "compact", "--dry-run", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	day := data["days"].([]any)[0].(map[string]any)
	require.Equal(t, []any{day["segment"]}, data["stage"], "a day git never tracked is not in the stage list")
}

func TestCompactVerbReportsSkippedEventsWhenNoDayFolded(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	layUncarriableEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")

	out, _, code := execCLI(t, "compact")
	require.Equal(t, 0, code, out)
	require.Equal(t, "Nothing to compact.\n1 event could not be compacted and stays as its own file.\n", out)
}

func TestCompactVerbSaysNothingToCompact(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-10T10:00:00Z")

	out, _, code := execCLI(t, "compact")
	require.Equal(t, 0, code, out)
	require.Equal(t, "Nothing to compact.\n", out)
}

func TestCompactVerbAllIncludesToday(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-11T09:00:00Z")

	out, _, code := execCLI(t, "compact", "--all")
	require.Equal(t, 0, code, out)
	require.True(t, strings.HasPrefix(out, "Compacted 1 day of events (1 event) into 1 file. Stage them with:\n"), out)
	require.NoDirExists(t, filepath.Join(dir, ".tiller", "events", "2026", "11", "11"))
}

func TestCompactVerbDryRunWritesNothing(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	loose := layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")
	layLooseEvent(t, dir, "EV-bbbbbb", "2026-11-04T10:00:00Z")

	out, _, code := execCLI(t, "compact", "--dry-run")
	require.Equal(t, 0, code, out)
	require.Equal(t, "Would compact 2 days of events (2 events) into 2 files.\n", out)
	require.FileExists(t, loose)
	matches, err := filepath.Glob(filepath.Join(dir, ".tiller", "events", "2026", "11", "*.jsonl"))
	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestCompactVerbRefusesOffTheIntegrationBranch(t *testing.T) {
	dir := initGatedGitRepo(t, "true")
	declareIntegrationBranchCLI(t, dir)
	compactClock(t, "2026-11-11T12:00:00Z")
	loose := layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")
	gitRun(t, dir, "checkout", "-b", "task-branch")

	out, _, code := execCLI(t, "compact", "--json")
	require.Equal(t, 1, code, out)
	require.Contains(t, out, `"reason": "not_integration_branch"`)
	require.FileExists(t, loose)

	_, errOut, code := execCLI(t, "compact")
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "Events are compacted on the integration branch only. You are on task-branch; check out main and run this again.")
}

func TestCompactVerbJSONCarriesDaysAndStage(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "one event")

	out, _, code := execCLI(t, "compact", "--dry-run", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	require.Equal(t, true, data["dry_run"])
	require.EqualValues(t, 1, data["events"])
	days := data["days"].([]any)
	require.Len(t, days, 1)
	require.Equal(t, "2026-11-03", days[0].(map[string]any)["day"])
	require.Equal(t, []any{".tiller/events/2026/11/03", days[0].(map[string]any)["segment"]}, data["stage"])

	out, _, code = execCLI(t, "compact", "--json")
	require.Equal(t, 0, code, out)
	data = jsonData(t, out)
	require.NotContains(t, data, "dry_run")
	require.Len(t, data["stage"], 2)
	require.NoDirExists(t, filepath.Join(dir, ".tiller", "events", "2026", "11", "03"))
}

func TestCompactVerbNeedsNoIdentity(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "identity.yml"))

	_, _, code := execCLI(t, "compact")
	require.Equal(t, 0, code)
}

func TestHumanCompactionForms(t *testing.T) {
	require.Equal(t, "", humanCompaction(mutate.CompactResult{}, false))

	day := mutate.CompactedDay{Day: "2026-11-03", Events: 2, Directory: ".tiller/events/2026/11/03"}
	removed := mutate.CompactResult{Days: []mutate.CompactedDay{day}, Events: 2, Stage: []string{day.Directory}}
	require.Equal(t, "Removed 2 loose events already held in segments. Stage them with:\n  git add -A -- .tiller/events/2026/11/03", humanCompaction(removed, false))

	one := mutate.CompactedDay{Day: "2026-11-03", Events: 1, Directory: "d", Segment: "s"}
	skipped := mutate.CompactResult{Days: []mutate.CompactedDay{one}, Events: 1, Skipped: 1, Stage: []string{"d", "s"}}
	require.Equal(t, "Compacted 1 day of events (1 event) into 1 file. Stage them with:\n  git add -A -- d s\n1 event could not be compacted and stays as its own file.", humanCompaction(skipped, false))
	skipped.Skipped = 2
	require.Contains(t, humanCompaction(skipped, false), "2 events could not be compacted and stay as their own files.")
	require.Equal(t, "Would compact 1 day of events (1 event) into 1 file.\n2 events could not be compacted and stay as their own files.", humanCompaction(skipped, true))

	require.Equal(t, "Would remove 2 loose events already held in segments.", humanCompaction(removed, true))
	removed.Stage = nil
	require.Equal(t, "Removed 2 loose events already held in segments.", humanCompaction(removed, false), "no git add line with nothing to stage")
	require.Equal(t, "1 event could not be compacted and stays as its own file.", humanCompaction(mutate.CompactResult{Skipped: 1}, false))
}

// layDisagreeingDay compacts the day of when, then lays the same event loose
// again with different content, so that day refuses every later compaction.
func layDisagreeingDay(t *testing.T, dir, id, when string) string {
	t.Helper()
	layLooseEvent(t, dir, id, when)
	// A day laid down earlier may already refuse; this day must still fold.
	res, _ := mutate.Compact(dir, mutate.CompactOptions{Now: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)})
	require.Contains(t, compactedDayNames(res), when[:10])
	bad := layLooseEvent(t, dir, id, when)
	data, err := os.ReadFile(bad)
	require.NoError(t, err)
	changed := strings.Replace(string(data), "tiller add task", "tiller add feature", 1)
	require.NotEqual(t, string(data), changed)
	require.NoError(t, os.WriteFile(bad, []byte(changed), 0o644))
	return bad
}

// TestCompactVerbReportsTheDaysItFoldedBesideARefusedDay pins that a refused
// day does not hide the newer days that did compact: the verb fails, but it
// still prints what it folded, the line that stages it, and why the other day
// refused.
func TestCompactVerbReportsTheDaysItFoldedBesideARefusedDay(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	bad := layDisagreeingDay(t, dir, "EV-aaaaaa", "2026-11-01T10:00:00Z")
	layLooseEvent(t, dir, "EV-bbbbbb", "2026-11-02T10:00:00Z")

	_, errOut, code := execCLI(t, "compact")
	require.NotEqual(t, 0, code)
	require.Contains(t, errOut, "Compacted 1 day of events (1 event) into 1 file. Stage them with:")
	require.Contains(t, stageLine(t, errOut), "02-")
	require.Contains(t, errOut, "Some days were not compacted: mutate: compact 2026-11-01:")
	require.FileExists(t, bad)
	require.NoDirExists(t, filepath.Join(dir, ".tiller", "events", "2026", "11", "02"))

	layDisagreeingDay(t, dir, "EV-dddddd", "2026-11-04T10:00:00Z")
	layLooseEvent(t, dir, "EV-cccccc", "2026-11-03T10:00:00Z")
	out, _, code := execCLI(t, "compact", "--json")
	require.NotEqual(t, 0, code)
	data := jsonData(t, out)
	require.Len(t, data["days"], 1)
	refused := stringsOf(t, data["refused"])
	require.Len(t, refused, 2, "one entry per refused day")
	require.True(t, strings.HasPrefix(refused[0], "mutate: compact 2026-11-01: "), refused[0])
	require.True(t, strings.HasPrefix(refused[1], "mutate: compact 2026-11-04: "), refused[1])
}

// runStageLine runs the printed git add line from dir through a shell, the
// way a person would paste it, and fails when git refuses it.
func runStageLine(t *testing.T, dir, line string) {
	t.Helper()
	git := exec.Command("sh", "-c", line)
	git.Dir = dir
	output, err := git.CombinedOutput()
	require.NoError(t, err, "%s\n%s", line, output)
}

// TestCompactVerbStageLineLeavesASkippedEventUnstaged pins that a day
// directory compaction did not remove is not named whole: the line names the
// loose files it removed, so a skipped event and any stray file beside it
// stay out of the index.
func TestCompactVerbStageLineLeavesASkippedEventUnstaged(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	carried := layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")
	skipped := layUncarriableEvent(t, dir, "EV-bbbbbb", "2026-11-03T11:00:00Z")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "two events")
	raw, err := os.ReadFile(skipped)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skipped, append(raw, []byte("# edited\n")...), 0o644))
	stray := filepath.Join(filepath.Dir(skipped), "notes.txt")
	require.NoError(t, os.WriteFile(stray, []byte("keep me\n"), 0o644))

	out, _, code := execCLI(t, "compact")
	require.Equal(t, 0, code, out)
	line := stageLine(t, out)
	require.NotContains(t, strings.Fields(line), ".tiller/events/2026/11/03", "the surviving directory is not named")
	runStageLine(t, dir, line)

	staged := gitOutput(t, dir, "diff", "--cached", "--name-status")
	require.Contains(t, staged, "D\t"+relTo(t, dir, carried))
	require.Contains(t, staged, "A\t.tiller/events/2026/11/03-")
	require.NotContains(t, staged, relTo(t, dir, skipped))
	require.NotContains(t, staged, "notes.txt")
}

// TestCompactVerbStageLineRunsFromWhereTillerRan pins that the printed paths
// resolve from the directory tiller was run in, for a project in a
// subdirectory of its repository, run from the project root and from below
// it. The JSON stage list names the same paths.
func TestCompactVerbStageLineRunsFromWhereTillerRan(t *testing.T) {
	for _, below := range []string{"", "src"} {
		t.Run("from "+filepath.Join("app", below), func(t *testing.T) {
			repo := t.TempDir()
			project := filepath.Join(repo, "app")
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".tiller"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(project, ".tiller", "tiller.yml"), []byte(ownedManifest), 0o644))
			require.NoError(t, os.MkdirAll(filepath.Join(project, "src"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(project, "src", "main.go"), []byte("package main\n"), 0o644))
			testgit.Init(t, repo)
			compactClock(t, "2026-11-11T12:00:00Z")
			committed := layLooseEvent(t, project, "EV-aaaaaa", "2026-11-03T10:00:00Z")
			gitRun(t, repo, "add", "-A")
			gitRun(t, repo, "commit", "-m", "init")
			cwd := filepath.Join(project, below)
			t.Chdir(cwd)

			out, _, code := execCLI(t, "compact", "--dry-run", "--json")
			require.Equal(t, 0, code, out)
			planned := jsonData(t, out)["stage"]

			out, _, code = execCLI(t, "compact")
			require.Equal(t, 0, code, out)
			line := stageLine(t, out)
			require.Equal(t, "git add -A -- "+strings.Join(stringsOf(t, planned), " "), line)
			runStageLine(t, cwd, line)

			staged := gitOutput(t, repo, "diff", "--cached", "--name-status")
			require.Contains(t, staged, "D\t"+relTo(t, repo, committed))
			require.Contains(t, staged, "A\tapp/.tiller/events/2026/11/03-")
		})
	}
}

func relTo(t *testing.T, base, path string) string {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	require.NoError(t, err)
	return filepath.ToSlash(rel)
}

func stringsOf(t *testing.T, v any) []string {
	t.Helper()
	items, ok := v.([]any)
	require.True(t, ok, "%v", v)
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.(string)
	}
	return out
}

// TestRestageNamesAChildsStagePathsFromHere pins the upgrade handoff: the
// maintenance child runs in the project root, so the parent renames its stage
// paths from wherever tiller was run.
func TestRestageNamesAChildsStagePathsFromHere(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	t.Chdir(filepath.Join(root, "src"))
	c := &mutate.CompactResult{Stage: []string{".tiller/events/2026/11/03", ".tiller/events/2026/11/03-0123456789ab.jsonl"}}

	restage(root, c)

	require.Equal(t, []string{"../.tiller/events/2026/11/03", "../.tiller/events/2026/11/03-0123456789ab.jsonl"}, c.Stage)
	restage(root, nil)
}

// TestCompactVerbNeverNamesAnEventWithAnInvalidID pins the stage line against
// a crafted loose file: an id with shell metacharacters stays loose and is
// named by neither the printed line nor the JSON stage list, even when its
// directory survives and the line names files one by one.
func TestCompactVerbNeverNamesAnEventWithAnInvalidID(t *testing.T) {
	dir := compactRepo(t)
	compactClock(t, "2026-11-11T12:00:00Z")
	layLooseEvent(t, dir, "EV-aaaaaa", "2026-11-03T10:00:00Z")
	crafted := layLooseEvent(t, dir, "EV-a b;touch PWNED", "2026-11-03T11:00:00Z")
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(crafted), "notes.txt"), []byte("keep me\n"), 0o644))
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "a crafted event")

	out, _, code := execCLI(t, "compact", "--dry-run", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	require.EqualValues(t, 1, data["skipped"])
	for _, p := range stringsOf(t, data["stage"]) {
		require.NotContains(t, p, "PWNED")
	}

	out, _, code = execCLI(t, "compact")
	require.Equal(t, 0, code, out)
	line := stageLine(t, out)
	require.NotContains(t, line, "PWNED")
	runStageLine(t, dir, line)
	require.FileExists(t, crafted)
	require.NoFileExists(t, filepath.Join(dir, "PWNED"))
}

func TestShellWordQuotesOnlyWhatAShellWouldRead(t *testing.T) {
	require.Equal(t, ".tiller/events/2026/11/03-0123456789ab.jsonl", shellWord(".tiller/events/2026/11/03-0123456789ab.jsonl"))
	require.Equal(t, "../app/a@b%c+d=e:f,g", shellWord("../app/a@b%c+d=e:f,g"))
	require.Equal(t, "'a b;touch PWNED.yml'", shellWord("a b;touch PWNED.yml"))
	require.Equal(t, `'it'\''s'`, shellWord("it's"))
	require.Equal(t, "'$(x)'", shellWord("$(x)"))
}

// TestCompactVerbQuotesAPathAShellWouldSplit pins the human line's quoting:
// a stage path with a space is one shell word, and the JSON list stays raw.
func TestCompactVerbQuotesAPathAShellWouldSplit(t *testing.T) {
	day := mutate.CompactedDay{Day: "2026-11-03", Events: 1, Directory: "d", Segment: "s"}
	res := mutate.CompactResult{Days: []mutate.CompactedDay{day}, Events: 1, Stage: []string{"my project/d", "s"}}
	require.Contains(t, humanCompaction(res, false), "  git add -A -- 'my project/d' s")
	require.Equal(t, []string{"my project/d", "s"}, res.Stage)
}

func compactedDayNames(res mutate.CompactResult) []string {
	var days []string
	for _, d := range res.Days {
		days = append(days, d.Day)
	}
	return days
}

func TestRefusalsListsOneMessagePerRefusedDay(t *testing.T) {
	require.Equal(t, []string{"a", "b"}, refusals(errors.Join(errors.New("a"), errors.New("b"))))
	require.Equal(t, []string{"plain"}, refusals(errors.New("plain")))
}
