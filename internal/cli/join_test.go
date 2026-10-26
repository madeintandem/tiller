package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/skills"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// initRepo scaffolds a fresh Tiller repo in a temp dir and chdirs into it, the
// common setup for the join acceptance tests. init is a pure scaffold now, so the
// repo starts with an empty roster and no identity; add contributor and join are what
// record each.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "greenfield")
	require.Equal(t, 0, code, "init scaffolds the repo")
	return dir
}

// registerContributor runs `add contributor` for id (name defaults to id when blank), the
// fixture step every join-after-init test needs now that join only selects
// among already-registered contributors: registration is add contributor's job, done once,
// explicitly, before any join that selects the id it registers.
func registerContributor(t *testing.T, id, name string) {
	t.Helper()
	if name == "" {
		name = id
	}
	_, _, code := execCLI(t, "add", "contributor", id, "--name", name)
	require.Equal(t, 0, code, "add contributor registers %s before join", id)
}

// readFile reads a file under the repo, failing the test if it is missing.
func readFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(parts...))
	require.NoError(t, err)
	return string(data)
}

// TestJoinRecordsIdentityAndWiresAgents is the core acceptance: once a human has
// registered the owner with `add contributor`, join --as selects it, writes the
// gitignored .tiller/identity.yml, and wires the Claude Code harness (skill
// shims, shim gitignore, AGENTS.md pointer block). join itself never touches
// the roster: it is already exactly what add contributor left it.
func TestJoinRecordsIdentityAndWiresAgents(t *testing.T) {
	dir := initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")

	out, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code, "join succeeds in a Tiller repo")
	require.Contains(t, out, "Joined as jc (JC Grubbs)", "summary reports the selected identity")
	// The summary compresses the shims to a count (they are always the whole
	// shipped set, seven of them now that intake ships; the full list rides in
	// the JSON envelope), per the 2026-07-26 output-shape pass.
	require.Contains(t, out, strconv.Itoa(len(skills.Names()))+" shims", "summary carries the shim count")
	require.Contains(t, out, "tiller guide      lists your ranked next moves and the skill that fits",
		"the closing hint describes bare guide's ranked list")
	require.NotContains(t, out, "names the seam that fits")

	// The roster is exactly what add contributor left it: join never touches it.
	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 1)
	require.Equal(t, "jc", m.Contributors[0].ID)
	require.Equal(t, "JC Grubbs", m.Contributors[0].Name)

	// The gitignored per-checkout identity records who I am here.
	id, err := store.ReadLocalIdentity(dir)
	require.NoError(t, err)
	require.Equal(t, "jc", id)
	require.FileExists(t, filepath.Join(dir, ".tiller", "identity.yml"))

	// Every registered skill lands as a discoverable shim delegating to guide,
	// including new seams on a checkout that joined an earlier distribution.
	for _, name := range skills.Names() {
		shimPath := filepath.Join(dir, ".claude", "skills", "tiller-"+name, "SKILL.md")
		require.FileExists(t, shimPath, "shim installed for %s", name)
		body := readFile(t, shimPath)
		require.Contains(t, body, "name: tiller-"+name, "frontmatter names the shim")
		require.Contains(t, body, "tiller guide "+name, "shim delegates to tiller guide")
	}

	// Each installed trigger comes from the live catalog, not a stale summary.
	for _, name := range skills.Names() {
		description, ok := skills.Description(name)
		require.True(t, ok)
		shim := readFile(t, dir, ".claude", "skills", "tiller-"+name, "SKILL.md")
		require.Contains(t, shim, "description: "+description)
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".claude", "skills"))
	require.NoError(t, err)
	require.Len(t, entries, len(skills.Names()), "exactly the shipped skills, no retired shim")
	require.NoDirExists(t, filepath.Join(dir, ".claude", "skills", "tiller-break-down"),
		"the retired seam installs no shim")

	// The shims are per-machine and must not be committed.
	gi := readFile(t, dir, ".gitignore")
	require.Contains(t, gi, ".claude/skills/tiller-*/", "shims are gitignored")

	// The pointer block is wired into AGENTS.md.
	agents := readFile(t, dir, "AGENTS.md")
	require.Contains(t, agents, "<!-- tiller:begin", "begin marker present")
	require.Contains(t, agents, "drive work to the edge of done and stop", "the hard rule is inline")
	require.Contains(t, agents, ".tiller/orientation.md", "points at orientation")

	// CLAUDE.md carries the managed redirect, not a copy: Claude Code auto-loads
	// CLAUDE.md, and the redirect sends it to AGENTS.md where the pointer block
	// lives. The pointer block's own prose must not be duplicated here.
	claude := readFile(t, dir, "CLAUDE.md")
	require.Contains(t, claude, "<!-- tiller:begin", "the redirect is a managed region")
	require.Contains(t, claude, "AGENTS.md", "the redirect points at AGENTS.md")
	require.NotContains(t, claude, "drive work to the edge of done", "the redirect does not copy the pointer block")
	require.Contains(t, out, "CLAUDE.md", "summary reports the redirect")
}

// TestJoinUpsertsTillerIgnoreLinesIntoExistingGitignore pins the fix: on a
// repo that already has a .gitignore (every brownfield repo), init leaves the
// file alone and join upserts the whole Tiller block, so identity.yml and the
// cache are ignored there too, not only on a greenfield seed.
func TestJoinUpsertsTillerIgnoreLinesIntoExistingGitignore(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	gitRun(t, dir, "init", "-q")
	existing := "node_modules/\n*.log\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0o644))

	_, _, code := execCLI(t, "init", "--name", "X", "--key", "x", "--kind", "brownfield")
	require.Equal(t, 0, code)
	require.Equal(t, existing, readFile(t, dir, ".gitignore"), "init leaves an existing .gitignore alone")

	registerContributor(t, "jc", "JC Grubbs")
	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	gi := readFile(t, dir, ".gitignore")
	require.True(t, strings.HasPrefix(gi, existing+"\n"), "the human's own entries stay first, then one blank line")
	ignored := gitOut(t, dir, "-c", "core.excludesFile=/dev/null", "check-ignore", "--no-index", "--",
		"node_modules/pkg/index.js", "app.log",
		"artifacts/tiller/gates/result.json", ".tiller/.cache/index", ".bin/tiller", ".tiller/identity.yml",
		".tiller/local.yml", ".tiller/board.md", ".tiller/orientation.md",
		".claude/skills/tiller-orient/SKILL.md", ".claude/skills/custom/SKILL.md",
		"artifacts/project/result.json", "src/artifacts/tiller/source.go", "src/.bin/tool",
		"src/.tiller/.cache/index", "src/.tiller/identity.yml", "src/.tiller/local.yml", "src/.tiller/board.md",
		"src/.claude/skills/tiller-orient/SKILL.md")
	require.ElementsMatch(t, []string{
		"node_modules/pkg/index.js", "app.log",
		"artifacts/tiller/gates/result.json", ".tiller/.cache/index", ".bin/tiller", ".tiller/identity.yml",
		".tiller/local.yml", ".claude/skills/tiller-orient/SKILL.md",
	}, strings.Fields(ignored))
}

// TestJoinIgnoreUpsertIsIdempotent: a second join appends nothing.
func TestJoinIgnoreUpsertIsIdempotent(t *testing.T) {
	dir := initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")
	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)
	first := readFile(t, dir, ".gitignore")

	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)
	require.Equal(t, first, readFile(t, dir, ".gitignore"), "a re-join leaves .gitignore byte-identical")
}

func TestRefreshPreservesLegacyBoardAndGitIndex(t *testing.T) {
	for _, args := range [][]string{{"join", "--as", "jc"}, {"upgrade", "--project-only"}, {"migrate"}} {
		t.Run(args[0], func(t *testing.T) {
			root := initRepo(t)
			registerContributor(t, "jc", "JC Grubbs")
			gitRun(t, root, "init", "-q", "-b", "main")
			path := filepath.Join(root, ".tiller", "board.md")
			require.NoError(t, os.WriteFile(path, []byte("Historical content\n"), 0o644))
			gitRun(t, root, "add", ".tiller/board.md", ".tiller/orientation.md")
			index := gitOut(t, root, "ls-files", "--stage")
			ignore := readFile(t, root, ".gitignore") + "\n/.tiller/board.md\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(ignore), 0o644))
			_, stderr, code := execCLI(t, args...)
			require.Equal(t, 0, code, stderr)
			require.Equal(t, index, gitOut(t, root, "ls-files", "--stage"))
			require.Equal(t, "Historical content\n", readFile(t, path))
			require.Contains(t, readFile(t, root, ".gitignore"), "/.tiller/board.md\n")
		})
	}
}

// TestJoinSecondOwnerKeepsFirst confirms join never touches the roster: once
// two contributors are registered in order (add contributor jc, then add contributor bob),
// joining as each in turn leaves the roster order and contents untouched and
// only ever moves the local identity, which the last join wins. This is also
// the join-as-a-different-registered-id pin: the second join flips
// .tiller/identity.yml away from the first.
func TestJoinSecondOwnerKeepsFirst(t *testing.T) {
	dir := initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")
	registerContributor(t, "bob", "Bob")

	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)
	id, err := store.ReadLocalIdentity(dir)
	require.NoError(t, err)
	require.Equal(t, "jc", id)

	_, _, code = execCLI(t, "join", "--as", "bob")
	require.Equal(t, 0, code)

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 2)
	require.Equal(t, "jc", m.Contributors[0].ID, "registration order is preserved; join never reorders the roster")
	require.Equal(t, "bob", m.Contributors[1].ID)

	// The last join wins the local identity; the roster keeps both, unchanged.
	id, err = store.ReadLocalIdentity(dir)
	require.NoError(t, err)
	require.Equal(t, "bob", id, "join-as-a-different-registered-id flips identity.yml")
}

// TestJoinIsIdempotent confirms re-running join with the same --as leaves the
// roster, identity file, shims, AGENTS.md, .gitignore, and the generated
// orientation.md byte-identical. writeGenerated now runs
// unconditionally on every join, so this is also the byte-stable
// half of that guarantee: an already-current orientation.md is untouched by a
// second join. join no longer touches the roster at all, so registration
// happens once, before either join.
func TestJoinIsIdempotent(t *testing.T) {
	dir := initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")

	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	snap := func() map[string]string {
		out := map[string]string{}
		out["tiller.yml"] = readFile(t, dir, ".tiller", "tiller.yml")
		out["identity.yml"] = readFile(t, dir, ".tiller", "identity.yml")
		out["AGENTS.md"] = readFile(t, dir, "AGENTS.md")
		out["CLAUDE.md"] = readFile(t, dir, "CLAUDE.md")
		out[".gitignore"] = readFile(t, dir, ".gitignore")
		out["orientation.md"] = readFile(t, dir, ".tiller", "orientation.md")
		for _, name := range skills.Names() {
			out[name] = readFile(t, dir, ".claude", "skills", "tiller-"+name, "SKILL.md")
		}
		return out
	}

	before := snap()
	_, _, code = execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code, "second join succeeds")
	after := snap()

	require.Equal(t, before, after, "join is byte-stable across runs")
}

// TestJoinBareRejoinRefreshesStaleOrientation confirms a bare re-join, with
// neither --as nor TILLER_CONTRIBUTOR set, falls back to the checkout's recorded
// .tiller/identity.yml, rewrites a hand-aged orientation.md back to the
// binary's current generation (unconditional writeGenerated), and
// echoes the identity it fell back to, so a re-join after a binary upgrade
// needs no flag at all.
func TestJoinBareRejoinRefreshesStaleOrientation(t *testing.T) {
	dir := initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")
	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	orientationPath := filepath.Join(dir, ".tiller", "orientation.md")
	current := strconv.Itoa(gen.OrientationGeneration())
	aged := strings.Replace(readFile(t, orientationPath), "orientation generation "+current, "orientation generation 1", 1)
	require.NotEqual(t, readFile(t, orientationPath), aged, "the replace actually aged the on-disk stamp")
	require.NoError(t, os.WriteFile(orientationPath, []byte(aged), 0o644))

	// A bare re-join: no --as, no TILLER_CONTRIBUTOR. It must fall back to jc, the
	// identity the first join above already recorded.
	t.Setenv("TILLER_CONTRIBUTOR", "")
	out, _, code := execCLI(t, "join")
	require.Equal(t, 0, code, "a bare re-join falls back to the recorded identity")
	require.Contains(t, out, "Joined as jc (JC Grubbs)", "the bare re-join echoes the identity it fell back to")

	require.Equal(t, gen.Orientation(), readFile(t, orientationPath),
		"re-join refreshes orientation.md back to the current generation even though the roster did not change")
}

// Both supported refresh paths install newly cataloged skills on joined checkouts
// without changing the route or rewriting discovery on a repeat refresh.
func TestInstructionRefreshInstallsMissingRequestedSkills(t *testing.T) {
	for _, skill := range []string{"design-brief", "code-review"} {
		for _, command := range []string{"join", "migrate"} {
			t.Run(skill+"/"+command, func(t *testing.T) {
				dir := initRepo(t)
				registerContributor(t, "jc", "JC Grubbs")
				_, stderr, code := execCLI(t, "join", "--as", "jc")
				require.Equal(t, 0, code, stderr)
				route := mustJSON(t, "guide", "--json")
				shimPath := filepath.Join(dir, ".claude", "skills", "tiller-"+skill, "SKILL.md")
				wantShim := readFile(t, shimPath)
				require.NoError(t, os.Remove(shimPath))
				orientationPath := filepath.Join(dir, ".tiller", "orientation.md")
				aged := strings.Replace(gen.Orientation(),
					"orientation generation "+strconv.Itoa(gen.OrientationGeneration()),
					"orientation generation 33", 1)
				require.NoError(t, os.WriteFile(orientationPath, []byte(aged), 0o644))

				for range 2 {
					_, stderr, code = execCLI(t, command)
					require.Equal(t, 0, code, stderr)
					require.Equal(t, wantShim, readFile(t, shimPath))
					require.Equal(t, gen.Orientation(), readFile(t, orientationPath))
					require.Equal(t, route, mustJSON(t, "guide", "--json"))
				}
			})
		}
	}
}

// TestJoinSelectsTrimmedEnvOwner is join's half of the trim symmetry
// pin: a padded TILLER_CONTRIBUTOR still selects the same registered contributor on a
// bare join, matching the mutation side (identity_resolution_test.go's
// TestMutationEchoesTrimmedEnvFallback) now that both share the same trimmed
// resolveContributorFallback tail.
func TestJoinSelectsTrimmedEnvOwner(t *testing.T) {
	initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")
	t.Setenv("TILLER_CONTRIBUTOR", " jc ")

	out, _, code := execCLI(t, "join")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "Joined as jc (JC Grubbs)")
}

func TestJoinWiringRefusesAStaleSharedFile(t *testing.T) {
	dir := initRepo(t)
	plan, err := planProjectRefresh(dir, false)
	require.NoError(t, err)

	agentsPath := filepath.Join(dir, "AGENTS.md")
	human := []byte("# Human edit\n")
	require.NoError(t, os.WriteFile(agentsPath, human, 0o644))

	err = plan.tree.CommitDerived("refresh project artifacts", plan.writes)
	require.Error(t, err)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)
	require.Equal(t, human, []byte(readFile(t, agentsPath)))
	require.NoDirExists(t, filepath.Join(dir, ".claude"))
	require.NoFileExists(t, filepath.Join(dir, "CLAUDE.md"))
}

// TestJoinPreservesHumanAgentsContent confirms join touches only the fenced
// region: prior human content in AGENTS.md, both before and after the region, is
// left untouched, while a stale region body is overwritten.
func TestJoinPreservesHumanAgentsContent(t *testing.T) {
	dir := initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")

	header := "# Acme\n\nHuman-authored orientation that join must never touch.\n\n"
	stale := "<!-- tiller:begin (managed by `tiller join`; edits inside are overwritten) -->\nSTALE\n<!-- tiller:end -->"
	footer := "\n\n## Conventions\n\nKeep this section verbatim.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(header+stale+footer), 0o644))

	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	agents := readFile(t, dir, "AGENTS.md")
	require.Contains(t, agents, "Human-authored orientation that join must never touch.")
	require.Contains(t, agents, "Keep this section verbatim.")
	require.Contains(t, agents, "drive work to the edge of done and stop", "canonical body installed")
	require.NotContains(t, agents, "STALE", "stale region overwritten")
}

// TestJoinPreservesHumanClaudeContent confirms the CLAUDE.md redirect has the
// same posture as the AGENTS.md block: prior human content, both before and
// after the fenced region, is left byte-for-byte untouched, while a stale region
// body is overwritten with the canonical redirect.
func TestJoinPreservesHumanClaudeContent(t *testing.T) {
	dir := initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")

	header := "# Acme for Claude\n\nHuman-authored Claude notes that join must never touch.\n\n"
	stale := "<!-- tiller:begin (managed by `tiller join`; edits inside are overwritten) -->\nSTALE\n<!-- tiller:end -->"
	footer := "\n\n## House rules\n\nKeep this section verbatim.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(header+stale+footer), 0o644))

	_, _, code := execCLI(t, "join", "--as", "jc")
	require.Equal(t, 0, code)

	claude := readFile(t, dir, "CLAUDE.md")
	require.True(t, strings.HasPrefix(claude, header), "human bytes before the region untouched")
	require.True(t, strings.HasSuffix(claude, footer), "human bytes after the region untouched")
	require.Contains(t, claude, "AGENTS.md", "canonical redirect installed")
	require.NotContains(t, claude, "STALE", "stale region overwritten")
}

// TestJoinNeedsIdentity confirms join refuses (exit 3) inside a repo when
// neither --as nor TILLER_CONTRIBUTOR gives it an id, with join's own text: it names
// --as, join's real flag, never --contributor (a flag join does not have and the
// generic store.ResolveContributor message advertises), and it points a fresh
// project at registering someone rather than telling the caller to run the
// very command they are already running.
func TestJoinNeedsIdentity(t *testing.T) {
	initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")

	out, _, code := execCLI(t, "join", "--json")
	require.Equal(t, 3, code, "no identity to record is a recoverable environment refusal")
	require.Contains(t, out, store.ReasonContributorUnset)
	require.Contains(t, out, "--as", "the refusal names join's own flag")
	require.NotContains(t, out, "--contributor", "join carries no --contributor flag; that belongs to mutating commands, not join")
	require.Contains(t, out, "tiller add contributor", "the refusal points a fresh project at registering someone first")
}

// TestJoinResolvesIdentityFromEnv confirms TILLER_CONTRIBUTOR never registers a
// fresh id: an id supplied by the environment is looked up on the roster
// exactly like one supplied by --as, and an unregistered one is refused
// rather than fabricated into the manifest, closing the incident this
// design exists to prevent.
func TestJoinResolvesIdentityFromEnv(t *testing.T) {
	dir := initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "sam")

	out, _, code := execCLI(t, "join", "--json")
	require.Equal(t, 3, code, "an unregistered id from TILLER_CONTRIBUTOR is refused, never registered")
	require.Contains(t, out, store.ReasonContributorUnknown)
	require.Contains(t, out, "roster is empty", "the refusal says plainly that no one is registered yet")
	require.Contains(t, out, "tiller add contributor sam", "the refusal points at registering the attempted id")

	m, err := store.LoadManifest(dir)
	require.NoError(t, err)
	require.Empty(t, m.Contributors, "the refused join wrote no one onto the roster")
}

// TestJoinResolvesIdentityFromEnvWhenRegistered is the positive half of
// TestJoinResolvesIdentityFromEnv above: a REGISTERED id supplied only
// through TILLER_CONTRIBUTOR, with no --as, resolves and joins exactly like one
// supplied by --as would, and the checkout's identity file records it.
func TestJoinResolvesIdentityFromEnvWhenRegistered(t *testing.T) {
	dir := initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")
	t.Setenv("TILLER_CONTRIBUTOR", "jc")

	out, _, code := execCLI(t, "join")
	require.Equal(t, 0, code, "a registered id supplied only through TILLER_CONTRIBUTOR joins")
	require.Contains(t, out, "Joined as jc (JC Grubbs)")

	id, err := store.ReadLocalIdentity(dir)
	require.NoError(t, err)
	require.Equal(t, "jc", id, "identity.yml records the env-resolved identity")
}

// TestJoinRefusesUnknownAs confirms --as naming an id absent from the roster is
// refused rather than treated as a registration: only `tiller add contributor` may
// put a person on the roster, and join has no path around that. The refusal
// names the id that failed, the registered roster it was checked against, and
// the exact command a human runs to fix it (design doc: an unknown id "refuses
// ... naming the registered roster and the human path").
func TestJoinRefusesUnknownAs(t *testing.T) {
	initRepo(t)
	registerContributor(t, "jc", "JC Grubbs")

	out, _, code := execCLI(t, "join", "--as", "nobody", "--json")
	require.Equal(t, 3, code, "an unregistered --as id is refused")
	require.Contains(t, out, store.ReasonContributorUnknown)
	require.Contains(t, out, "nobody", "the refusal names the id that was not found")
	require.Contains(t, out, "jc", "the refusal names the registered roster")
	require.Contains(t, out, "tiller add contributor nobody", "the refusal points at registering the id")
}

// TestJoinRefusesCorruptIdentityFile confirms the same fix at bare join too:
// with neither --as nor TILLER_CONTRIBUTOR, a present but unparseable
// .tiller/identity.yml refuses by name (identity_unreadable, exit 3) rather
// than falling through to the contributor_unset text, which would otherwise
// suggest running join again once join has already recorded a file that is
// merely corrupt, a loop.
func TestJoinRefusesCorruptIdentityFile(t *testing.T) {
	dir := initRepo(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	registerContributor(t, "jc", "JC Grubbs")
	corruptIdentityFile(t, dir)

	out, _, code := execCLI(t, "join", "--json")
	require.Equal(t, 3, code, "a corrupt identity file refuses rather than masquerading as absent")
	require.Contains(t, out, store.ReasonIdentityUnreadable)
	require.Contains(t, out, filepath.Join(dir, ".tiller", "identity.yml"), "the refusal names the file")
	require.Contains(t, out, "tiller join --as", "the refusal points at the fix")
	require.NotContains(t, out, store.ReasonContributorUnset, "corruption is never reported as absence")
}

// TestJoinOutsideRepoExitsThree confirms join refuses outside a Tiller repo with
// the environment exit code, before it resolves or records any identity.
func TestJoinOutsideRepoExitsThree(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	out, _, code := execCLI(t, "join", "--json")
	require.Equal(t, 3, code, "join outside a Tiller repo is an environment failure")
	require.Contains(t, out, "not_a_repo")
	require.NoDirExists(t, filepath.Join(dir, ".claude"), "no shims written outside a repo")
	require.NoFileExists(t, filepath.Join(dir, ".tiller", "identity.yml"), "no identity written outside a repo")
}

func TestJoinReportsContributorIdentity(t *testing.T) {
	root := initRepo(t)
	registerContributor(t, "jc", "JC")
	out, _, code := execCLI(t, "join", "--as", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	require.Equal(t, map[string]any{"id": "jc", "name": "JC"}, data["contributor"])
	require.NotContains(t, data, "owner")
	identity, err := store.ReadLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "jc", identity)
}
