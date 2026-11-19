package skills

import (
	"regexp"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// emDash is the byte the house style forbids in any authored prose.
const emDash = "—"

// harnessTools is the denylist proving each skill names only a filesystem, a
// shell, and the tiller CLI, never a harness-specific tool (skill-set.md, the
// harness-agnostic hard rule). The match is case-insensitive.
var harnessTools = []string{
	"bash", "read tool", "edit tool", "write tool", "grep tool",
	"glob tool", "view tool", "str_replace", "multiedit", "notebookedit",
	"apply_patch",
}

// TestNamesAreTheShippedSkills keeps the discoverable catalog in lifecycle order.
func TestNamesAreTheShippedSkills(t *testing.T) {
	require.Equal(t, []string{
		"orient", "derive-baseline", "decompose", "specify", "execute", "capture", "mend-drift", "amend", "design-brief", "code-review", "upgrade",
	}, Names())
	require.False(t, Has("break-down"), "break-down is retired into specify")
}

// TestUpgradeSkillCarriesMaintenance pins the maintenance rules that moved out
// of orientation and orient into the on-demand upgrade skill: the scopes, the
// identity boundary, Homebrew ownership, team adoption, the schema 6, 8, and 9
// notes, and phase-outcome recovery. Maintenance is not a lifecycle seam, so
// the skill carries no Readout section.
func TestUpgradeSkillCarriesMaintenance(t *testing.T) {
	text, ok := Lookup("upgrade")
	require.True(t, ok)
	for _, want := range []string{
		// Trigger.
		"explicitly asks to upgrade Tiller, migrate this checkout's schema, or refresh its generated guidance",
		"Maintenance is separate from choosing lifecycle work.",
		"authorizes only the maintenance it names",
		"is a reason to tell the developer the project needs `tiller upgrade --project-only`, not to run maintenance unasked",
		// Scopes.
		"`tiller upgrade` updates the installed binary through its installation owner",
		"supported schema migrations and generated instructions",
		"`tiller upgrade --project-only` maintains this checkout with the running binary, offline, with no network or package-manager calls and without Homebrew",
		"stale generated guidance, other checkouts, and a second checkout after a machine update",
		"deliberately excludes project maintenance",
		"`--binary-only` cannot combine with `--project-only`, and `--project-only` cannot select a version with `--version`",
		"One checkout's maintenance never updates other branches, worktrees, or projects.",
		"teammates incorporate the reviewed migration and maintain their own checkouts",
		// Identity.
		"Maintenance never selects a different checkout identity.",
		"never the routine refresh remedy",
		"Missing identity still calls for `tiller join --as <you>` after human registration.",
		"Artifact-only refresh needs no identity.",
		"through `--contributor`, then `TILLER_CONTRIBUTOR`, then local identity",
		// Homebrew ownership.
		"Verified Homebrew installations keep brew as their installation owner.",
		"delegates only the matching Tiller package to its owning brew, suppressing implicit dependent upgrades and cleanup",
		"genuine dependencies and metadata refresh remain brew's responsibility",
		"Exact version pins refuse there.",
		"Never overwrite Cellar/Caskroom files",
		"never bypass private-download authentication, including after a failure",
		"report that boundary rather than promising a global rollback",
		// Team adoption.
		"Adoption is coordinated Git work, not independent migrations in stale clones.",
		"retain a compatible executable and a recoverable committed baseline",
		"Pause team state mutations.",
		"Reconcile state and active owners on the designated integration branch.",
		"Inspect, review, and apply one migration once, in one designated checkout, then commit and publish it through normal Git.",
		"merge the migrated state into each continuing work branch without discarding its task-specific state",
		"refresh each checkout, restart consoles, reload agent instructions, and check schema, identity, task owner, branch, and legal next action",
		"Adoption is not authorization or an evidence waiver",
		"not permission to skip verification or to migrate a project",
		"A schema fence is local, not a distributed lock",
		"old binaries in unmigrated clones can still conflict",
		"Never use or mutate migrated state with an older binary.",
		"preserve new work rather than blindly restore old schema files",
		// Schema 6.
		"Schema 6 freezes execution ownership, not a feature-wide claim.",
		"`tiller migrate --dry-run --json`",
		"it writes nothing",
		"Save the whole report or its `data.ownership_review` outside source state.",
		"fill every initially empty `confirmed_owner`",
		"leaving all other snapshot fields unchanged",
		"`tiller migrate --ownership-review <file>`",
		"`tiller upgrade --project-only --ownership-review <file>`",
		"a stale or incomplete review refuses",
		"Schema-5 active work and open reviews keep their branches, PRs, statuses, and evidence.",
		"Only migrations crossing schemas 2 to 4 into schema 5 require draining old shared-branch work first",
		"including the human's final merges and sync",
		"Keep that binary's directory on PATH for the temporary drain session only, then leave the session.",
		// Schema 8.
		"Schema 8 adds the Tweak shelf.",
		"changes only the manifest version",
		"existing Bug and Chore records remain unchanged, and no work drain is required",
		"Older binaries refuse schema 8; upgrade every participant before resuming state mutations.",
		// Schema 9.
		"Schema 9 renames",
		"`contributors:`",
		"`contributor:`",
		"preserving the selected ID while renaming its field",
		"`tiller add contributor <id> --name \"<name>\"`",
		"`tiller remove contributor <id>`",
		"Task, Bug, Chore, and Tweak claims remain `owner`; ownership filters remain `--owner`.",
		"preserves IDs, claims, active work, and history without a work drain or ownership review",
		"Legacy command, flag, and environment names are not aliases.",
		// Recovery.
		"Read phase outcomes even on failure",
		"a binary replacement or source migration can have completed before refresh fails",
		"A successful binary update followed by a failed project phase is incomplete.",
		"retry `tiller upgrade --project-only` only after the named cause is repaired",
		"`mutation_recovery_needed`",
		"preserve the transaction journal and recover the transaction first",
		"Unknown durability is not proof of no writes.",
		"Never downgrade the schema or newer generated guidance, or erase caches to bypass a refusal: older binaries cannot read migrated state, and the cache holds the rollback record.",
		"Never infer unchanged files from a lost child result.",
		"restart existing serve processes and reload harness instructions as needed",
		"does not acquire this upgrade logic mid-process",
		"Retain a compatible old executable before the first new install",
		// Hand-off.
		"There is no `tiller readout upgrade`",
	} {
		require.Containsf(t, text, want, "upgrade pins %q", want)
	}
	require.NotContains(t, text, "## Readout", "maintenance is not a lifecycle seam")
	// The stale-branch sentence repeated the adoption sentence pinned above.
	require.NotContains(t, text, "Do not migrate stale branches independently.")
}

// TestOrientRoutesExplicitMaintenance pins that orient's maintenance copy left
// for the upgrade skill and one routing paragraph took its place.
func TestOrientRoutesExplicitMaintenance(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, text, "loads `tiller guide upgrade` directly")
	require.Contains(t, text, "authorizes only the maintenance it names")
	for _, gone := range []string{
		"Maintenance is separate from choosing lifecycle work.",
		"Verified Homebrew installations keep brew",
		"For schema-6 adoption",
		"Schema 8 adds Tweaks.",
		"Schema 9 renames the project roster",
		"Adoption is coordinated Git work",
		"Read phase outcomes even on failure",
	} {
		require.NotContainsf(t, text, gone, "orient leaves %q to the upgrade skill", gone)
	}
}

// TestSkillNamesDropIntake pins the unification: intake is only a command now,
// and decompose carries the rules the retired skill held.
func TestSkillNamesDropIntake(t *testing.T) {
	names := Names()
	require.Len(t, names, 11)
	require.NotContains(t, names, "intake")
	require.False(t, Has("intake"))
	_, ok := Lookup("intake")
	require.False(t, ok)
}

// TestDecomposeSkillCoversBothRoutes pins the three parts of the one skill: the
// shared mapping rules, the founding design, and the amendment.
func TestDecomposeSkillCoversBothRoutes(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)
	for _, want := range []string{
		"## What becomes what",
		"## Recording a founding design",
		"## Recording an amendment",
		"tiller propose amendment",
		"tiller add bug",
		"--source",
		"tiller intake --no-amendment",
		"tiller amend <A> --add-epic",
		"held_by_amendment",
		"feature_not_approved",
		"task_under_held_feature",
		"parent_held_elsewhere",
		"amendment_required",
		"including when nothing was staged",
	} {
		require.Containsf(t, text, want, "decompose carries %q", want)
	}
	require.Less(t, strings.Index(text, "## What becomes what"), strings.Index(text, "## Recording a founding design"))
	require.Less(t, strings.Index(text, "## Recording a founding design"), strings.Index(text, "## Recording an amendment"))
	require.NotContains(t, text, ".tiller/feedback/", "documents land under .tiller/design/ now")
}

// The ambient description carries the amendment door and the stop.
func TestDecomposeDescriptionCarriesBothDoors(t *testing.T) {
	d, ok := Description("decompose")
	require.True(t, ok)
	require.Contains(t, d, "tiller intake")
	require.Contains(t, d, "draft amendment")
	require.Contains(t, d, "stops for the human")
}

func TestSkillFrontmatterAndGeneratedShimsAreValidYAML(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			text, ok := Lookup(name)
			require.True(t, ok)
			description, ok := Description(name)
			require.True(t, ok)
			require.NotEmpty(t, description)
			for _, artifact := range []struct {
				label string
				text  string
				name  string
			}{
				{"embedded", text, name},
				{"shim", gen.SkillShim(name, description), gen.SkillShimName(name)},
			} {
				t.Run(artifact.label, func(t *testing.T) {
					body, ok := strings.CutPrefix(artifact.text, "---\n")
					require.True(t, ok)
					frontmatter, _, ok := strings.Cut(body, "\n---\n")
					require.True(t, ok, "frontmatter must close before the body")
					var metadata struct {
						Name        string `yaml:"name"`
						Description string `yaml:"description"`
					}
					require.NoError(t, yaml.Unmarshal([]byte(frontmatter), &metadata))
					require.Equal(t, artifact.name, metadata.Name)
					require.Equal(t, description, metadata.Description)
				})
			}
		})
	}
}

func TestCaptureWritesAProposalAndStopsBeforeTriage(t *testing.T) {
	text, ok := Lookup("capture")
	require.True(t, ok)
	for _, want := range []string{
		"name: capture",
		"tiller add bug",
		"tiller add chore",
		"--attach",
		"--source",
		"## Reproduction",
		"## Fixed when",
		"## What",
		"## Done when",
		"never approve",
		"status remains `proposed`",
		"`cancel` on a work node remains human-only",
		"`resolve` is delegable only on an explicit human instruction naming the shelf node and either `--clear` or the replacement carrier target",
	} {
		require.Containsf(t, text, want, "capture pins %q", want)
	}
	require.NotContains(t, text, "human-only cancel and resolve")
	start := strings.Index(text, "## Choreography")
	handoff := strings.Index(text, "## Hand-off")
	require.Positive(t, start)
	require.Greater(t, handoff, start)
	choreography := text[start:handoff]
	for _, forbidden := range []string{"tiller approve", "tiller claim", "tiller start", "tiller resolve", "tiller cancel"} {
		require.NotContains(t, choreography, forbidden)
	}
}

// readoutSeams are the skills with a rendered seam-exit block. mend-drift
// instead leaves a drafted fix for the human; capture stops at a shelf proposal.
var readoutSeams = []string{"orient", "derive-baseline", "decompose", "specify", "execute", "amend"}

// reviewArmChoreography is the review arm's slice of the code-review
// Choreography: from "## Choreography" up to the disposition arm's heading or,
// before that arm exists, the Hand-off.
func reviewArmChoreography(t *testing.T, text string) string {
	t.Helper()
	start := strings.Index(text, "## Choreography")
	require.Positive(t, start)
	rest := text[start:]
	end := len(rest)
	for _, marker := range []string{"\n### Disposition arm", "\n## Hand-off"} {
		if i := strings.Index(rest, marker); i > 0 && i < end {
			end = i
		}
	}
	return rest[:end]
}

// The review arm writes one review file and nothing else: no state, no Git
// writes, no test runs, and every finding graded and anchored the same way.
func TestCodeReviewArmIsReadOnlyAndWritesOneReviewFile(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	for _, want := range []string{
		"name: code-review",
		"epic, feature, task, bug, chore, or tweak",
		"or asking what to do next triggers neither arm",
		".tiller/reviews/<YYYY-MM-DD>-<node-id>.md",
		"never overwrite a review",
		"env GIT_OPTIONAL_LOCKS=0 tiller",
		"git --no-optional-locks",
		"--no-ext-diff --no-textconv",
		"--grep=<unit-id>",
		"under Limitations",
		"Judge findings against the code as it reads now",
		"a violated approved criterion or `preserve` criterion is a blocker, anything else grades by impact",
		"Traced", "Suspected",
		"Found in",
		"Already tracked", "Previously declined",
		"A defect that returns after its bug reached done is a new finding marked `regression of <id>`.",
		"The only write is the one new review file.",
		"Never sample, never cap",
		"--gate <gate-key>",
		"Never add a gate",
		"There is no `tiller readout code-review`",
		// A merged head is its own merge-base with integration, so done work is
		// diffed from the first parent of the integration merge that brought it in.
		"diff --no-ext-diff --no-textconv <merge>^1...<head>",
		"Found in names each node by key path with its ID",
		"drop the `git:` prefix",
	} {
		require.Containsf(t, text, want, "code-review pins %q", want)
	}
	// The never-a-next-move and never-reopen sentences repeated the trigger
	// sentence and the regression rule pinned above; orientation keeps "Done
	// work stays done".
	require.Contains(t, strings.Join(strings.Fields(gen.Orientation()), " "), "Done work stays done: never reopen it to accommodate an adjustment.")
	require.NotContains(t, text, "A review is never a suggested next move")
	require.NotContains(t, text, "Never reopen done work")
	require.NotContains(t, text, "merge-base <integration> <review-head>",
		"a merged head diffed from its own merge-base is empty")
	require.NotContains(t, text, "--ancestry-path",
		"the first merge on the ancestry path can be an unrelated merge after a fast-forward, or a later re-assembly")
	choreography := reviewArmChoreography(t, text)
	for _, forbidden := range []string{
		"tiller check", "tiller eval", "tiller sync", "tiller attest", "tiller claim",
		"tiller start", "tiller approve", "git fetch", "git checkout", "git commit", "git stash",
	} {
		require.NotContainsf(t, choreography, forbidden, "the review arm never runs %q", forbidden)
	}
}

// orient sends an explicit review or disposition request straight to the skill.
func TestOrientRoutesExplicitCodeReviews(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, text, "loads `tiller guide code-review` directly")
	require.Contains(t, text, "For a `code-review` review, recovery is limited to corrected reads.")
}

// Capture owns the one severity table and grades every bug it files: it always
// passes --severity, or leaves the bug unrated and says why in bug.md's Actual
// section, and hands an unrated bug to triage with the grade the approve needs.
// That severity never routes is orientation's rule
// (TestOrientationCarriesTheWholeSeverityRule in package gen), not capture's.
func TestCaptureGradesBugsBySeverity(t *testing.T) {
	text, ok := Lookup("capture")
	require.True(t, ok)
	for _, want := range []string{
		"| Severity | Meaning |",
		"| Blocker |", "| Important |", "| Medium |", "| Minor |",
		"Grade by impact, never by the size of the fix.",
		"--severity",
		"When it cannot be known yet, omit it and say so in `bug.md`'s Actual section",
		"tiller approve bug <node> --severity <level>",
		"Whatever the severity, the shelf's soft order decides what is next.",
	} {
		require.Containsf(t, text, want, "capture pins %q", want)
	}
	require.NotContains(t, text, "Severity never routes", "orientation states the routing rule")
	require.Contains(t, strings.Join(strings.Fields(gen.Orientation()), " "), "Severity never routes: it sorts and filters the console's Fix board")
}

// Code review grades with capture's table rather than its own copy, and a
// logged bug carries the review's severity as its field, never as prose. That
// severity never routes is orientation's rule, so code-review no longer says it.
func TestCodeReviewSeverityBecomesTheBugsField(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "Severity becomes the bug's `severity` field when captured.")
	require.NotContains(t, text, "it never becomes a queue position")
	require.Contains(t, strings.Join(strings.Fields(gen.Orientation()), " "), "Severity never routes")
	require.Contains(t, text, "the severity table in `tiller guide capture`")
	require.Contains(t, text, "Pass the row's severity as `--severity`")
	require.Contains(t, text, "Keep the review's confidence as the opening line of `bug.md`")
	for _, gone := range []string{
		"Severity is review prose only",
		"| Severity | Meaning |",
		"Keep the review's severity and confidence as the opening line",
		"it is prose, never a field",
	} {
		require.NotContainsf(t, text, gone, "code-review no longer carries %q", gone)
	}
}

// Orient's copy of the shelf and severity paragraph is gone; orientation
// carries it (TestOrientationCarriesTheWholeSeverityRule in package gen).
func TestOrientLeavesTheSeverityRuleToOrientation(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.NotContains(t, text, "Bugs are defects. Chores are housekeeping")
	require.NotContains(t, text, "Like `rename` and `link`, `tiller grade bug <node> <level>` is a direct command")
}

// How code-review decides merged or in flight, and which snapshot it judges, is
// exercised against real Git histories in code_review_selection_test.go, which
// runs the skill's documented commands rather than pinning their text.

// --attach takes only an epic, feature, or task, so a shelf target's findings
// attach where that shelf node was found, or nowhere.
func TestCodeReviewShelfTargetsAttachHonestly(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "`--attach` accepts only an epic, feature, or task")
	require.Contains(t, text, "its own `attached_to`, or omit `--attach`")
	require.NotContains(t, text, "For a bug, chore, or tweak target, Found in is the target.")
}

// A row handed to its unit's owner stays fixable by that owner.
func TestCodeReviewHandedOffRowStaysFixableByOwner(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "A row annotated `requested change on <unit>` stays open to \"fix\" by that unit's owner")
	require.Contains(t, text, "the unit's `owner` against the checkout's identity")
}

// The target's own contract is read, not only its feature's.
func TestCodeReviewReadsTheTargetsOwnContract(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "a task target's own `task.md`")
	require.Contains(t, text, "a bug's Fixed when, or a chore's or tweak's Done when")
}

// A new session finds its review file deterministically and asks rather than guessing.
func TestCodeReviewFindsTheReviewFileDeterministically(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "Order candidates by date, then suffix number")
	require.Contains(t, text, "When more than one candidate exists, ask which")
	require.Contains(t, text, "names no node and none is in this conversation, ask which review")
	require.NotContains(t, text, "use the newest file under")
}

// The attest line names the reviewed node's own noun.
func TestCodeReviewAttestLineNamesTheTargetsNoun(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "tiller attest <feature|task> <id> --gate <gate-key>")
	require.NotContains(t, text, "tiller attest feature <F-id>")
}

// Remote-tracking refs are read when they are the only copy; only a branch absent
// from both goes to Limitations, and a divergence is reported.
func TestCodeReviewRemoteBranchRulesAgree(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "absent locally and from remote-tracking refs")
	require.Contains(t, text, "When the local branch and its remote-tracking ref differ")
	require.NotContains(t, text, "A branch present only on the remote goes under Limitations")
}

// Located diffs leave out Tiller's own state, which the ID grep also matches.
func TestCodeReviewExcludesTillerStateFromLocatedDiffs(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "-- . ':(exclude).tiller'")
	require.Contains(t, text, "Skip `code-review:` disposition commits")
}

// A revise needs its prose edit drafted before staging, as amend requires.
func TestCodeReviewReviseNeedsTheDraftedEdit(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "write the drafted prose edit to the target's spec or contract before staging `--revise`")
}

// Deferring to capture and decompose borrows their shapes, never their commits, and
// the one commit lands on the integration branch.
func TestCodeReviewBorrowsShapesNotCommits(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "except their commits")
	require.Contains(t, text, "Commit on the integration branch")
}

// A review reports required behavior the change never built, anchored to the
// requirement rather than to changed lines.
func TestCodeReviewFindsWhatWasNeverBuilt(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "Anchors: absent")
}

// Reproductions are read off the code, never run, and the skill says so.
func TestCodeReviewReproductionsAreDerivedByReading(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "derived by reading, not run")
}

// Missing coverage is a tweak, not a chore.
func TestCodeReviewClassifiesCoverageAsATweak(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, text, "tweak for missing coverage")
	require.NotContains(t, text, "chore for missing coverage")
}

// The disposition is not a lifecycle seam. The review arm already says there is
// no code-review readout, so look only inside the disposition Hand-off.
func TestCodeReviewDispositionHasNoReadout(t *testing.T) {
	text, ok := Lookup("code-review")
	require.True(t, ok)
	i := strings.LastIndex(text, "### Disposition arm")
	require.GreaterOrEqual(t, i, 0)
	require.Contains(t, text[i:], "no `tiller readout amend` follows")
}

// TestLookupReturnsEachSkill confirms every named skill is embedded and non-empty,
// and that an unknown name is a clean miss rather than a panic. With amend
// embedded there is no unbuilt seam left to carry the clean-miss assertion, so a
// plainly bogus name carries it.
func TestLookupReturnsEachSkill(t *testing.T) {
	for _, name := range Names() {
		text, ok := Lookup(name)
		require.Truef(t, ok, "skill %q is embedded", name)
		require.NotEmpty(t, text, "skill %q has a body", name)
	}
	_, ok := Lookup("nonsense")
	require.False(t, ok)
}

// TestSkillsHaveFivePartAnatomy confirms each skill carries the frontmatter
// trigger plus the reasoning, choreography, and hand-off sections
// (skill-set.md, "The anatomy of a skill"). Readout is the fifth part, added
// alongside the `tiller readout <seam>` verb: the section
// that tells the agent to reproduce the seam's block verbatim rather than
// compose its own account of what happened. It is pinned separately below,
// scoped to readoutSeams, because mend-drift's product is a drafted fix for
// the human to approve, not a seam block the CLI renders, so it carries no
// Readout section.
func TestSkillsHaveFivePartAnatomy(t *testing.T) {
	for _, name := range Names() {
		text, _ := Lookup(name)
		require.Containsf(t, text, "name: "+name, "%s frontmatter names the skill", name)
		require.Containsf(t, text, "description:", "%s frontmatter carries the trigger", name)
		require.Containsf(t, text, "## Reasoning", "%s has a Reasoning section", name)
		require.Containsf(t, text, "## Choreography", "%s has a Choreography section", name)
		require.Containsf(t, text, "## Hand-off", "%s has a Hand-off section", name)
		// The choreography terminates in tiller commands.
		require.Containsf(t, text, "tiller ", "%s choreography drives the tiller CLI", name)
	}
	for _, name := range readoutSeams {
		text, _ := Lookup(name)
		require.Containsf(t, text, "## Readout", "%s has a Readout section", name)
	}
}

// TestSkillsCarryAReadoutSectionAndEndTheirChoreographyWithIt pins the two
// concrete facts TestSkillsHaveFivePartAnatomy's bare-section check cannot:
// each of the four readout-bearing skills' Choreography names the exact
// `tiller readout <seam>` command for ITS OWN seam (not a copy-pasted one from
// another skill), and each names where the agent's own account goes.
// mend-drift is scoped out the same way
// TestSkillsHaveFivePartAnatomy scopes it out; TestMendDriftHasNoReadoutSection
// pins its absence explicitly.
func TestSkillsCarryAReadoutSectionAndEndTheirChoreographyWithIt(t *testing.T) {
	for _, name := range readoutSeams {
		text, _ := Lookup(name)
		require.Containsf(t, text, "## Readout", "%s has a Readout section", name)
		require.Containsf(t, text, "tiller readout "+name,
			"%s choreography ends in its readout command", name)
		require.Containsf(t, text, "Notes from this run",
			"%s names where the agent's own account goes", name)
	}
}

// TestOrientNamesCountsOnlyNeedsAttentionAndQueueDrillDown pins the readout
// publication contract: one singular attention list summarizes Queue counts,
// and enumeration happens through the read-only queue command rather than
// inline node rows or mutation commands.
func TestOrientNamesCountsOnlyNeedsAttentionAndQueueDrillDown(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Equal(t, 1, strings.Count(text, "`Needs attention`"),
		"orient names one singular Needs attention list")
	for _, want := range []string{
		"It is counts-only",
		"inline `tiller queue` drill-down",
		"The queue command is the one place to enumerate a count",
	} {
		require.Containsf(t, text, want, "orient pins %q", want)
	}
	require.NotContains(t, text, "Do not expand it into concrete node rows",
		"counts-only and the queue sentence already bar inline rows")
	require.NotContains(t, text, "needs-you headers",
		"orient no longer describes per-kind inline mutation sections")
}

// The six readout sections point at the one readout rule in orientation
// (TestOrientationCarriesTheWholeReadoutRule in package gen) and keep only what
// is particular to their seam: the command, their notes, and the naming line.
func TestReadoutSectionsPointAtTheOrientationRule(t *testing.T) {
	for _, name := range readoutSeams {
		raw, _ := Lookup(name)
		text := strings.Join(strings.Fields(raw), " ") // skills hard-wrap prose
		i := strings.Index(text, "## Readout")
		require.GreaterOrEqualf(t, i, 0, "%s has a Readout section", name)
		readout := text[i:]
		if j := strings.Index(readout[len("## Readout"):], "## "); j >= 0 {
			readout = readout[:len("## Readout")+j]
		}
		require.Containsf(t, readout, "tiller readout "+name, "%s names its readout command", name)
		require.Containsf(t, readout, "Reproduce the block verbatim and last, per **Readout** in `.tiller/orientation.md`.", "%s points at the readout rule", name)
		require.NotContainsf(t, readout, "per the readout rule in", "%s names the label, not a description", name)
		require.Containsf(t, readout, "Notes from this run", "%s keeps its own notes list", name)
		require.Containsf(t, readout, "Name nodes by key path with the id in parentheses.", "%s keeps the naming line", name)
		for _, gone := range []string{
			"The block is Tiller's, not yours",
			"an id alone sends the developer off",
			"ends on its **Next:** ranked list",
			"never add a list of your own after it",
			"Keep it to facts about this run.",
			"always the block's **Next:** line",
		} {
			require.NotContainsf(t, text, gone, "%s leaves %q to orientation", name, gone)
		}
	}
	orient, _ := Lookup("orient")
	require.NotContains(t, orient, "Do not narrate the picture again in prose around the block")
	derive, _ := Lookup("derive-baseline")
	require.NotContains(t, derive, "Nothing follows the readout, not a second Next line")
	execute, _ := Lookup("execute")
	require.NotContains(t, execute, "print an\n**Up next:** list when they finish")
	require.NotContains(t, execute, "inside your run's scope it is your queue")
}

// TestMendDriftHasNoReadoutSection locks in the deliberate exception to the
// five-part anatomy: `tiller readout <seam>` is wired for the four skills
// readoutSeams names (internal/cli/readout.go), and extending it to mend-drift
// is no part of this task's scope. mend-drift's edge of done is a drafted fix
// left for the human's approval, not a rendered seam block, so its structure
// stays frontmatter, Reasoning, Choreography, Hand-off. Asserting the absence
// explicitly, rather than only omitting a positive check, keeps a future stray
// "## Readout" heading here from going unnoticed.
func TestMendDriftHasNoReadoutSection(t *testing.T) {
	text, ok := Lookup("mend-drift")
	require.True(t, ok)
	require.NotContains(t, text, "## Readout", "mend-drift renders no seam block; it drafts a fix for the human")
}

// TestOrientScopesTheCannotDisagreeClaim keeps orient's claim of agreeing with
// the CLI honest. The ranked list (views.Next, shared by `tiller next`, bare
// `tiller guide`, and the readout) carries no drift awareness, so a skill-less
// harness is never pointed at mend-drift by the list alone. The caveat scopes
// the claim to what the list computes from recorded tree state and names
// mend-drift as this skill's own addition, confirmed only by an actual
// `tiller drift` run.
func TestOrientScopesTheCannotDisagreeClaim(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, text, "cannot disagree for every move the list computes from the recorded tree",
		"the claim is scoped to what the list actually computes")
	require.Contains(t, text, "this skill's own addition", "mend-drift is named as the skill's own addition, not the list's")
	require.Contains(t, text, "takes an actual `tiller drift` run", "knowing it applies takes a live sweep, not a stored-state read")
}

// TestSkillsAreNeutralProse is the hard-rule assertion: no skill contains an em
// dash, and none names a harness-specific tool. This is what keeps the surface
// identical no matter who drives it.
func TestSkillsAreNeutralProse(t *testing.T) {
	for _, name := range Names() {
		text, _ := Lookup(name)
		require.NotContainsf(t, text, emDash, "skill %q must contain no em dash (house style)", name)
		lower := strings.ToLower(text)
		for _, tool := range harnessTools {
			require.NotContainsf(t, lower, tool, "skill %q names harness-specific tool %q", name, tool)
		}
	}
}

func TestSpecifyAcceptsOnlyExplicitBatches(t *testing.T) {
	text, ok := Lookup("specify")
	require.True(t, ok)
	for _, want := range []string{
		"explicit instruction naming several features",
		"every feature in a named epic",
		"soft order",
		"one `specify:` commit per feature",
		"one final `tiller readout specify`",
		"never runs `approve`",
		"ranked list suggests waiting",
	} {
		require.Containsf(t, text, want, "specify pins %q", want)
	}
}

// execute carries the batch: the lawful path for several bugs, chores, or
// tweaks under one review (TW-taq0ps). The pins are the rules an agent broke
// when no such path existed: a hand-named branch, commits naming no node, and
// one test for many bugs.
func TestExecuteCarriesTheBatchWorkLoop(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"An agent asked for a shared review of shelf work must use a batch, never a hand-named branch.",
		"Every commit on the batch branch carries the id of the member it serves.",
		"Each bug in the batch still needs its own regression test that would fail on its reproduced defect",
		"run them only on an explicit human instruction that names the members and asks for one review",
		"A batch is one unit of work as well: it takes one worktree for the whole batch, never one per member.",
		"Naming a batch, or asking for several named bugs, chores, or tweaks under one review, scopes the run to that batch",
		"each member with basis `batched`",
		"tiller sync --accept-fast-forward <batch>",
		"A batch has no recovery of its own",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
	order := func(steps ...string) {
		t.Helper()
		last := -1
		for _, step := range steps {
			at := strings.Index(text, step)
			require.GreaterOrEqualf(t, at, 0, "execute carries %q", step)
			require.Greaterf(t, at, last, "%q follows the step before it", step)
			last = at
		}
	}
	order(
		"tiller add batch core-model-fixes --title \"Core model fixes\" bugs/blank-on-reload bugs/stale-total bugs/lost-draft",
		"tiller claim batch core-model-fixes",
		"tiller start batch core-model-fixes",
		"git checkout -b tiller/batches/core-model-fixes",
		"each commit carrying the id of the member it serves",
		"tiller check batch core-model-fixes",
		"tiller submit batch core-model-fixes --pr \"$pr\"",
		"tiller status batch core-model-fixes",
	)
}

func TestExecuteCarriesTheShelfWorkLoop(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"tiller claim bug bugs/blank-on-reload",
		"tiller start bug bugs/blank-on-reload",
		"git checkout -b tiller/bugs/blank-on-reload",
		"tiller check bug bugs/blank-on-reload",
		"tiller submit bug bugs/blank-on-reload",
		"regression test",
		"ignores `review_grain`",
		"attached_to",
		"feature.md",
		"feature.eval.md",
		"task.md",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
	require.NotContains(t, text, "tiller eval bug")
	require.NotContains(t, text, "tiller approve bug")
}

// TestHandoffsDisambiguateApproveAccept pins the vocabulary line at the moment of
// handoff, corrected for a build with no accept verb: approve
// green-lights a proposed plan to start and is the only gate verb, completed
// work crosses to done on the human's merge as `tiller sync` observes it, and
// the agent translates a colloquial "accept this" by the node's state rather
// than guessing. Naming `accept` as a command here would send an agent at a verb
// that exits 2.
func TestHandoffsDisambiguateApproveAccept(t *testing.T) {
	for _, name := range []string{"decompose", "specify"} {
		text, ok := Lookup(name)
		require.True(t, ok)
		require.Containsf(t, text, "green-lights a proposed plan", "%s carries the approve side", name)
		require.Containsf(t, text, "There is no `accept`", "%s says the accept verb is gone", name)
		require.Containsf(t, text, "when the human merges and `tiller sync` observes it",
			"%s names what actually crosses completed work", name)
		require.Containsf(t, text, "translate by the node's state", "%s translates by state", name)
		require.NotContainsf(t, text, "never guess", "%s says what to do, so the never-guess clause is gone", name)
		require.Containsf(t, text, "`attest` records the human's acceptance", "%s names attest as the second gate verb", name)
		require.NotContainsf(t, text, "the only gate verb", "%s no longer calls approve the only gate verb", name)
	}
}

// TestQuestionsAreRecordedNotChatted pins that the four seams that add notes
// neither invite chat questions nor carry their own copy of the notes rule,
// which orientation now states (TestQuestionsAreRecordedNotChatted in package gen).
func TestQuestionsAreRecordedNotChatted(t *testing.T) {
	for _, name := range []string{"decompose", "amend", "specify", "execute"} {
		text, ok := Lookup(name)
		require.True(t, ok)
		require.NotContainsf(t, text, "Questions and commentary are yours to add freely", "%s does not invite chat questions", name)
		require.NotContainsf(t, text, "Commentary is yours to add freely", "%s leaves the notes rule to orientation", name)
	}
}

// TestOrientPointsAtOrientationForSharedRules pins that orient's recovery,
// external references, and node reference copies are gone and its sections
// point at orientation, which states each rule once
// (TestOrientationCarriesTheRecoveryProtocol, TestOrientationCarriesTheExternalReferencesRule,
// TestOrientationCarriesNodeReferenceConvention in package gen).
func TestOrientPointsAtOrientationForSharedRules(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, text, "## Recovery protocol\n\nFollow **Recovery protocol** in `.tiller/orientation.md`.\n")
	require.NotContains(t, text, "Skills correct themselves from the CLI's exit codes")
	require.NotContains(t, text, "This protocol is inherited by every skill")
	require.NotContains(t, text, "after two different fixes", "the retry bound leaves with orient's recovery copy")
	require.NotContains(t, text, "## External references")
	require.NotContains(t, text, "Never guess a URL")
	require.Contains(t, text, "## Node references\n\nName and address nodes by **Node references** in `.tiller/orientation.md`.\n")
	require.NotContains(t, text, "This convention is inherited by every skill")
	require.NotContains(t, text, "In command position an existing node takes either form")
	require.NotContains(t, text, "tiller ls plans")
	require.Contains(t, text, "`tiller ls` takes epics, features, tasks, bugs, chores, tweaks, questions, and amendments.")
}

// The skills that name the external references rule point at orientation.
func TestSkillsPointAtOrientationForExternalReferences(t *testing.T) {
	for _, name := range []string{"decompose", "specify", "amend", "capture"} {
		text, ok := Lookup(name)
		require.True(t, ok)
		require.Containsf(t, text, "follow **External references** in `.tiller/orientation.md`.", "%s points at orientation", name)
		require.NotContainsf(t, text, "in `tiller guide orient`", "%s no longer points at orient", name)
	}
}

// Every skill opens by assuming orientation and naming the remedy when it is stale.
func TestEverySkillAssumesOrientation(t *testing.T) {
	for _, name := range Names() {
		text, _ := Lookup(name)
		require.Containsf(t, text, "Read `.tiller/orientation.md` first if it is not in context", "%s assumes orientation", name)
		require.Containsf(t, text, "`tiller upgrade --project-only`", "%s names the stale-orientation remedy", name)
		require.Containsf(t, text, "# "+name+"\n\nThis skill assumes `.tiller/orientation.md`.", "%s opens with the line under its title", name)
	}
}

// The general command rule in orientation replaces notes that only said a
// command or flag does not exist; notes that state a fact keep the fact.
// amend's own no-probe sentence went the same way: amend keeps its read-only
// route, and orientation's preflight states the trial-mutation boundary.
func TestSkillsDropNotesTheCommandRuleCovers(t *testing.T) {
	amend, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, amend, "Use read-only Git inspection where history needs clarification.")
	require.NotContains(t, amend, "Do not probe with a mutation.")
	require.Contains(t, strings.Join(strings.Fields(gen.Orientation()), " "),
		"Use read-only history inspection as needed, never a trial mutation: every mutation appends events that stay in the record.")
	require.NotContains(t, amend, "invent a `--dry-run` flag")
	execute, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, execute, "Evidence is recorded by `tiller check` and `tiller eval`.")
	require.NotContains(t, execute, "There is no `tiller add evidence`")
}

// TestDecomposeAsksForNoEpicContract replaces the epic.eval breadcrumb this test
// used to pin. An epic has no acceptance contract: its bar is the
// sum of its features' contracts and it reaches done when they do, observed by
// sync. decompose telling an agent to author epic.eval.md sent it to write a file
// nothing reads, and `tiller eval epic` / `check epic` / `attest epic` all exit 2.
func TestDecomposeAsksForNoEpicContract(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)
	require.NotContains(t, text, "epic.eval", "an epic carries no acceptance contract")
	require.Contains(t, text, "an epic has no acceptance contract of its own.",
		"decompose says so rather than leaving it unstated")
	require.Contains(t, text, "epic.md", "the epic's one authored artifact is still named")
}

// TestDecomposeSaysWhatAFeatureNarrativeCarries pins the bar for feature.md prose.
func TestDecomposeSaysWhatAFeatureNarrativeCarries(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)
	require.Contains(t, text, "It is narrative, not a specification",
		"decompose says what a feature narrative is and is not")
}

// TestDecomposeCitesFoundingSources pins source citation on the founding adds
// and the coverage map in the readout notes.
func TestDecomposeCitesFoundingSources(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)
	start := strings.Index(text, "### A founding design")
	end := strings.Index(text, "### A later document")
	require.True(t, start >= 0 && end > start)
	require.Contains(t, text[start:end], "--source",
		"the founding choreography cites a source on its adds")
	require.Contains(t, text, "coverage map", "the readout notes carry a coverage map")
}

// TestDecomposeRoutesACorrectionInsideAnUnapprovedFeature pins the route for a
// correction that has no task home, and that the recovery line points at the
// placement rules rather than a vague instruction.
func TestDecomposeRoutesACorrectionInsideAnUnapprovedFeature(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)
	require.Contains(t, text, "When the feature is still `draft`")
	require.NotContains(t, text, "moving the add to where it belongs")
}

// retiredModel is the vocabulary the single-tree refactor removed. A skill that
// still names any of it does not merely read stale: `--intent` exits 2, `tiller
// schedule` and `tiller accept` are not commands, `epic.eval.md` is a file
// nothing reads, and `break-down` is a skill `tiller guide` refuses. Skill prose
// has no compiler, so this denylist is the only thing standing between a
// half-finished rewrite and an agent confidently running a command that fails.
var retiredModel = []string{
	"blitz", "break-down", "--intent", "epic.eval", "tiller schedule",
	"tiller accept", "accept epic", "accept feature", "tasks.yml",
}

// TestSkillsNameNothingFromTheRetiredModel runs that denylist over every shipped
// skill, frontmatter included, since the description is what a harness triggers
// on and what `tiller join` copies onto the shim.
func TestSkillsNameNothingFromTheRetiredModel(t *testing.T) {
	for _, name := range Names() {
		text, _ := Lookup(name)
		lower := strings.ToLower(text)
		for _, token := range retiredModel {
			require.NotContainsf(t, lower, token, "skill %q names the retired %q", name, token)
		}
	}
}

// TestExecuteSaysTasksArriveReady closes the other side of the same merge. With
// the plan gate approving the tasks alongside the spec, there is no second
// approval in the work loop, and a task's own implementation spec is already
// written for the agent to work from. An execute that still expected to propose
// or approve would stall on a gate that already passed.
func TestExecuteSaysTasksArriveReady(t *testing.T) {
	ex, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, ex, "Tasks arrive already `ready`", "no second approval in the loop")
	require.Contains(t, ex, "read the plan `specify` wrote in the task's `task.md`",
		"the loop works from the written implementation spec")
	require.NotContains(t, ex, "tiller approve task", "execute never approves a task")
}

// evalCriterionInSpecifyContract returns the id of the one eval-kind criterion in
// the contract specify prints, by scanning its criterion blocks for the eval
// binding. It reads the skill rather than hard-coding "C4" on purpose: the point
// of the test below is that the two files agree, and a constant here would just
// move the disagreement into the test.
func evalCriterionInSpecifyContract(t *testing.T) string {
	t.Helper()
	sp, ok := Lookup("specify")
	require.True(t, ok)
	var id, found string
	for _, line := range strings.Split(sp, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- id: C") {
			id = strings.TrimPrefix(trimmed, "- id: ")
		}
		if strings.Contains(trimmed, "binding: { type: eval") {
			require.Empty(t, found, "the contract shows exactly one eval-kind criterion")
			found = id
		}
	}
	require.NotEmpty(t, found, "specify's contract shows an eval-kind criterion")
	return found
}

// TestExecuteGradesTheContractsEvalCriterion is a cross-skill invariant with no
// compiler behind it and one real failure already to its name. execute's rubric
// lines are the set's ONLY --record-rubric-output example, and they named C3
// while the contract specify calls "the complete contract surface this build
// accepts" declares C3 as a `manual` binding and C4 as its only `eval` one. An
// agent following both skills verbatim got exit 2, "criterion C3 is a manual
// binding, not an eval binding". Nothing but this test can notice the two files
// drifting apart again.
func TestExecuteGradesTheContractsEvalCriterion(t *testing.T) {
	want := evalCriterionInSpecifyContract(t)

	ex, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, ex, "--criterion "+want,
		"execute grades the criterion specify's contract actually binds to eval")
	require.Contains(t, ex, "--record-rubric-output "+gates.RubricOutputPath("F-79dh3q", want),
		"and the recorded file is the output path the packet reports for that criterion")
	require.Contains(t, ex, `"criterion": "`+want+`"`,
		"as does the rubric_output document it shows")

	// The manual-binding criterion must appear in no rubric position at all,
	// which is the specific shape of the defect: a plausible id in a line that
	// cannot run.
	require.NotContains(t, ex, "--criterion C3", "the manual criterion is never graded")
	require.NotContains(t, ex, `"criterion": "C3"`, "nor recorded as rubric output")
}

// TestSkillsStayInsideTheProject is the #17 sweep at the skill surface: no skill
// references a source-repo document, by path or by name, because the agent
// surface must be complete inside the project and the project's resolved binary
// may not match any local Tiller source checkout anyway. The bare doc names are
// guarded too: the regression that prompted the issue said "artifact-schema.md"
// without any docs/spec prefix.
func TestSkillsStayInsideTheProject(t *testing.T) {
	outside := []string{
		"docs/spec",
		"artifact-schema",
		"cli-surface",
		"skill-set.md",
		"ui-surface",
		"distribution.md",
	}
	for _, name := range Names() {
		text, _ := Lookup(name)
		for _, token := range outside {
			require.NotContainsf(t, text, token, "skill %q points outside the project (%q)", name, token)
		}
	}
}

// TestEmbeddedHandoffsUseRunnableCommands keeps product guidance on the exact
// CLI grammar instead of shorthand an agent cannot execute.
func TestEmbeddedHandoffsUseRunnableCommands(t *testing.T) {
	decompose, ok := Lookup("decompose")
	require.True(t, ok)
	require.Contains(t, decompose, `tiller answer question <Q-id> --answer "<text>"`)

	specify, ok := Lookup("specify")
	require.True(t, ok)
	for _, command := range []string{
		"tiller add task <epic>/<feature>/<task-key> --title",
		"tiller link task <key> --depends-on <sibling>",
		"tiller spec feature submit-certification/certification-form-loading",
	} {
		require.Containsf(t, specify, command, "specify carries runnable command %q", command)
	}
	// The epic-grain contract trio used to be pinned here. All three exit 2 now:
	// `eval`, `check`, and `attest` answer for features and tasks only.
	for _, gone := range []string{"tiller eval epic", "tiller check epic", "tiller attest epic"} {
		require.NotContainsf(t, specify, gone, "specify must not name %q, which exits 2", gone)
	}

	execute, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, execute, "tiller attest feature <F-id> --criterion <C-id> --status pass")
}

// TestCorrectionsRerunTheReadoutWithSince pins the readout's two sizes:
// a seam exit keeps the full block, and a correction inside a
// gate closes with `tiller readout <seam> --since <sha>`, naming the commit
// the last block reported. Each skill that carries a correction loop says
// the --since form at the sentence that used to say "rerun the readout",
// and decompose's Readout section states the split itself, since that is
// the seam where the full block was re-emitted four times in one review.
// The general "rerun the current readout with --since" rule that specify and
// orient each restated now lives with the correction table in orientation,
// pinned by TestOrientationCarriesTheCorrectionTable.
func TestCorrectionsRerunTheReadoutWithSince(t *testing.T) {
	for skill, want := range map[string][]string{
		"decompose": {
			"rerun `tiller readout decompose --since <sha>`",
			"the commit the last block you reported",
			"A seam exit gets the full block",
		},
		"specify": {
			"rerun `tiller readout specify --since <sha>`",
		},
	} {
		text, ok := Lookup(skill)
		require.True(t, ok)
		for _, sentence := range want {
			require.Containsf(t, text, sentence, "%s says the --since form for corrections", skill)
		}
		require.NotContainsf(t, text, "rerun the current readout,", "%s no longer tells a correction to rerun the full block", skill)
	}
}

// TestSkillsPointAtTheCorrectionTable pins that correction routing is told
// once, in orientation's correction table, and that each skill which used to
// restate it keeps only a pointer. Each removed copy is named by one of its
// distinctive phrases so it cannot creep back.
func TestSkillsPointAtTheCorrectionTable(t *testing.T) {
	for _, name := range []string{"orient", "decompose", "specify", "amend", "execute"} {
		text, ok := Lookup(name)
		require.True(t, ok)
		require.Containsf(t, text, "the **Correction table** in `.tiller/orientation.md`", "%s points at the one correction table", name)
		require.NotContainsf(t, text, "the correction table in `tiller guide specify`", "%s no longer points at a table in specify", name)
	}
	for name, gone := range map[string][]string{
		"specify": {
			"| Correction | Route |",
			"Approval does not freeze cosmetic titles",
			"An explicit human correction that matches this table",
		},
		"orient": {
			"not a trial mutation or an invented dry-run",
			"- **Pending baseline proposal:**",
			"Bounded epic features may interleave.",
			"is already the developer's go for that action",
		},
		"decompose": {
			"An eligible unchanged feature outside that window uses the reviewed `move-feature` amendment route",
		},
		"amend": {
			"sink epic",
		},
	} {
		text, ok := Lookup(name)
		require.True(t, ok)
		for _, phrase := range gone {
			require.NotContainsf(t, text, phrase, "%s no longer restates the correction routing: %q", name, phrase)
		}
	}
	specify, ok := Lookup("specify")
	require.True(t, ok)
	require.Contains(t, specify, "## Correcting the feature plan")
	require.Contains(t, specify, "Human feedback on a specced but unapproved feature's prose stays at the same feature approval gate")
	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, orient, "Classify explicit human feedback before choosing a seam")
	decompose, ok := Lookup("decompose")
	require.True(t, ok)
	require.Contains(t, decompose, "Features from bounded epics may interleave.")
	require.Contains(t, decompose, "A canonical key changes with `tiller rename <noun> <node> --key <canonical-key>` only inside its proposal window.")
	amend, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, amend, "**Move unchanged work without replacing it.**")
	require.Contains(t, amend, "Approval rechecks the captured subtree and inherited settings atomically")
}

// TestAmendNamesTheShippedAmendmentSurface is the fidelity rule at this seam,
// the same guard TestExecuteDrivesVerifiedHappyPath keeps over the work loop.
// Every flag here was driven against the real binary; the denied list is the
// older sketch of this surface, which would send an agent at flags that exit 2
// (`--revise-eval` widened to `--revise`, `--reactivate` was cut because a
// rollup-backed done recomputes on its own, and there is no ADR surface in this
// build, so `tiller add adr` is not a command).
func TestAmendNamesTheShippedAmendmentSurface(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"tiller add amendment form-engine-rework",
		"--add-feature submit-certification/table-backed-forms",
		"--change-type replace",
		"--add-task submit-certification/rendering-modes/table-aware-rendering",
		"--supersede submit-certification/certification-form-loading",
		"--by submit-certification/table-backed-forms",
		"--cancel submit-certification/rendering-modes/embedded-json-fallback",
		"--superseded-by submit-certification/rendering-modes/table-aware-rendering",
		"--revise submit-certification/rendering-modes",
		"--amends-design design/solution-design/01-overview.md",
		"--triggered-by design/solution-design/04-form-engine.md",
		"tiller show amendment form-engine-rework",
		"tiller approve amendment form-engine-rework",
		"tiller withdraw amendment form-engine-rework --reason",
	} {
		require.Containsf(t, text, want, "amend's choreography drives the shipped surface: %q", want)
	}
	for _, gone := range []string{"--revise-eval", "--reactivate", "reactivate", "tiller add adr"} {
		require.NotContainsf(t, text, gone, "amend must not name %q, which this build has no verb for", gone)
	}
}

// TestAmendCarriesThePreconditionDiscipline pins the mechanism that makes a
// staged batch reviewable rather than merely written down: every precondition
// name the CLI recognizes, the all-or-nothing refusal, and the two guard shapes
// verified against the binary that a skill stating the happy path alone would
// leave an agent trusting wrongly (a supersede stages no status guard unless one
// is asked for, and a done task carries no revision anchor to compare).
func TestAmendCarriesThePreconditionDiscipline(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"target_status", "target_revision", "key_absent", "spec_revision", "eval_revision",
		"git hash-object",
		"fails the whole approval with no writes",
		"naming every stale operation rather than only the first",
		"A supersede staging no `target_status` is not refused",
		"`target_revision` reads nothing on a done task",
	} {
		require.Containsf(t, text, want, "amend carries the precondition discipline: %q", want)
	}
}

// TestAmendSaysTheReshapeClearsTheCascadeItAnswers pins the other half of
// mend-drift's cascade hand-off. A skill that staged the reshape and left the
// developer expecting a second mending pass would send them back through
// markers the batch already answered: internal/core/drift/drift.go exempts a
// cancelled node and a superseded feature from candidate production outright,
// and a revise re-pins the accepted basis the active_basis leg compares, so
// those candidates clear on apply. The candidates the reshape does not touch
// stay mend-drift's, which is the honest half of the same sentence.
func TestAmendSaysTheReshapeClearsTheCascadeItAnswers(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "exempts a cancelled node and a superseded feature from its sweep",
		"the skill says why a cancel and a supersede clear their own candidates")
	require.Contains(t, text, "re-pins the very basis the sweep compares against",
		"and why a revise clears an active_basis candidate")
	require.Contains(t, text, "stay `mend-drift`'s, one at a time",
		"without claiming the reshape clears candidates it never touched")
}

// TestAmendCarriesThePreservationDiscipline pins what keeps a rework honest: a
// replacement's contract binds preserve criteria to characterization tests
// captured BEFORE the behavior moves. Captured after, they record the new
// behavior and prove nothing, which is the failure this paragraph exists to
// prevent. The contract shape itself stays specify's, reached through
// `tiller guide specify` rather than restated here.
func TestAmendCarriesThePreservationDiscipline(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"preserve` criteria",
		"characterization tests",
		"captured before that behavior moves",
		"records the new behavior and proves nothing",
		"tiller guide specify",
	} {
		require.Containsf(t, text, want, "amend carries the preservation discipline: %q", want)
	}
}

// TestAmendStagesAndStopsForTheHuman pins the hand-off: staging executes
// nothing, approve applies the whole batch atomically, and both terminal verbs
// run only on an explicit instruction naming the amendment (the delegation
// model). An amend that ran its own approve would be the agent performing the
// cancels and supersessions the design reserves for the human's one act.
func TestAmendStagesAndStopsForTheHuman(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"the one act that executes the supersessions, the cancels, the revisions, and the materializations this skill only ever staged",
		"applies the whole batch atomically",
		"explicit human instruction naming the amendment",
		"otherwise you hand them the exact command and stop",
	} {
		require.Containsf(t, text, want, "amend stops for the human: %q", want)
	}
	d, ok := Description("amend")
	require.True(t, ok)
	require.Contains(t, d, "stops for the human's approve",
		"the shim's own trigger line carries the stop, since that is the ambient signal a harness reads")
}

// TestAmendNamesTheStagedKeyOrderingTrap pins a refusal the shipped
// implementation produces that no reading of the flag list predicts: a --by or
// --superseded-by naming a staged addition resolves only against operations
// already on the amendment, so a forward reference exits 2 as an unknown target.
// Driven live against the binary; without this the natural authoring order
// (destructive ops first, additions after) refuses and reads like the tree lost
// a node.
func TestAmendNamesTheStagedKeyOrderingTrap(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "stage the addition before the operation that names it",
		"the ordering rule is stated as an instruction")
	require.Contains(t, text, "names no feature or task this amendment can target",
		"with the exact refusal it produces, so the agent recognizes it")
}

// TestAmendStatesTheTwoWaysOutOfAStaleBatch pins the recovery guidance the
// staging surface can actually carry out. An earlier cut told the agent to
// "restage the operations it names," which nothing in the surface can do:
// `tiller amend` appends, and no verb edits or removes an operation already
// staged, so following that instruction adds a second copy of the operation
// beside the stale one and approve refuses exactly as before. The two real ways
// out are putting the tree back the way the batch was reviewed against, and
// withdrawing to reopen against the tree as it now stands. The denial below is
// what keeps the impossible instruction from creeping back.
func TestAmendStatesTheTwoWaysOutOfAStaleBatch(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "nothing edits or removes an operation already staged: staging appends",
		"the constraint that rules out editing a staged batch is stated, not implied")
	require.Contains(t, text, "put the tree back the way the batch was reviewed against",
		"the first way out")
	require.Contains(t, text, "withdraw the amendment and open a new one staged against the tree as it now stands",
		"the second way out, with the condition that picks it")
	require.NotContains(t, text, "restage the operations it names",
		"the surface has no verb that restages one operation of a batch")
	// The second way out runs a human gate, and Recovery is read on its own when
	// a refusal lands, so it restates the discipline rather than leaving it to
	// Hand-off and the choreography block: withdraw needs a --reason and needs
	// the human to have asked for it by name.
	require.Contains(t, text, "takes a required `--reason` and runs only on an explicit human instruction naming the amendment",
		"the recovery that reaches for a human gate says so where the recovery is read")
}

// TestAmendNamesTheReviseContractRefusal pins the one exit-2 refusal on this
// seam that comes from the tree rather than from how the call was made: a revise
// re-pins its target's basis by hashing the files the basis names, so a feature
// that never got its contract has nothing to re-pin
// (mutate.checkReviseTargets). Found live on a bare draft, where it used to
// surface as a raw git error.
func TestAmendNamesTheReviseContractRefusal(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "revise_target_incomplete", "the reason token the refusal carries")
	require.Contains(t, text, "Write or restore it, or drop the operation",
		"with what to do about it")
}

// TestAmendStatesTheAmendmentAddressingRule pins the rule that makes this noun
// unlike every other keyed one: a key outlives its holder, because a duplicate
// key is refused only among proposed amendments, so one key can be carried by
// several amendments over a project's life. The resolver prefers the open
// proposal (internal/core/resolve's preferOpenAmendment), which is what a key
// almost always means, and the skill has to state both halves: what the key
// reaches, and the one thing it cannot do, which is reach past a live proposal
// to a retired holder. An earlier cut of this file taught around the
// unpreferenced behavior instead ("every later call naming it exits 2,
// ambiguous amendment key"); that guidance is now false, and the denial below is
// what keeps it from creeping back.
func TestAmendStatesTheAmendmentAddressingRule(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, text, "A key names the open proposal whenever there is one",
		"the key resolves to the amendment still open")
	require.Contains(t, text, "a key no open amendment holds still names a single retired holder",
		"and a retired sole holder stays addressable by its key")
	require.Contains(t, text, "read those by the `A-` id their own `add` printed",
		"with the one thing a key cannot do")
	require.NotContains(t, text, "ambiguous amendment key",
		"the poisoned-key trap is fixed in the resolver, so the skill must not still teach around it")
}

// TestAmendChoreographyNamesAddQuestionsBlocksReach pins the other half of
// #49: the choreography never staged a question before this fix, so an agent
// reading it had no worked example of --blocks reaching a same-batch staged
// key, the reach docs/spec/skill-set.md's amend section already documents.
func TestAmendChoreographyNamesAddQuestionsBlocksReach(t *testing.T) {
	text, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"--add-question",
		"--blocks print-layouts/pdf-export",
		"resolving a same-batch staged epic, feature, or task key as readily as",
	} {
		require.Containsf(t, text, want, "amend's choreography stages a question with a same-batch blocks reach: %q", want)
	}
}

// TestDecomposeNamesTheIntakeRefusals pins the refusal messages the amendment
// route can meet, quoted so the agent recognizes them.
func TestDecomposeNamesTheIntakeRefusals(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)
	require.Contains(t, text, "intake amendments trace every operation: pass --source citing the feedback document",
		"the source_required refusal, quoted verbatim")
	require.Contains(t, text, "re-intake is a new intake: rename the document", "a landed name is refused, quoted verbatim")
	require.Contains(t, text, "tiller guide amend",
		"staging refusals otherwise point at amend's own guide rather than restating it")
}

// TestDecomposeHandsAmendmentToTheHuman pins the amendment hand-off: propose
// hands it over, approve is the human's one act, and what approval promotes
// rejoins the ordinary lifecycle.
func TestDecomposeHandsAmendmentToTheHuman(t *testing.T) {
	text, ok := Lookup("decompose")
	require.True(t, ok)
	for _, want := range []string{
		"`tiller approve amendment` is the one act that lifts the hold",
		"promotes each held epic and each held task to `ready`",
		"leaves each held feature `draft`",
		"Approve and withdraw each run only on an explicit human instruction naming the amendment",
		"`tiller withdraw amendment` removes every node the amendment still holds",
		"A materialized question sits on the ledger until `tiller answer question` closes it",
	} {
		require.Containsf(t, text, want, "decompose hands the amendment off: %q", want)
	}
}

// TestSkillsRequireShortQuestionTitlesForNewWrites pins both question creation
// paths. Direct questions and amendment-staged questions keep the full
// statement, but every new write also supplies the short title scanning
// surfaces render. A missing title is never taught as a valid command.
func TestSkillsRequireShortQuestionTitlesForNewWrites(t *testing.T) {
	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, orient, `tiller add question "<statement>" --title "<short title>"`,
		"orient's deferred briefing item carries a separate title")

	decompose, ok := Lookup("decompose")
	require.True(t, ok)
	require.Contains(t, decompose, `tiller add question "<statement>" --title "<short title>"`,
		"decompose's direct question carries a separate title")
	require.Contains(t, decompose, `tiller add question "<statement>" --title "<short title>" --blocks <node>`,
		"decompose's blocking question carries a separate title")

	amend, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, amend, "--add-question \"Keep the legacy embedded-JSON fallback for offline mode, or drop it once the table model ships?\" \\\n  --title \"Legacy offline fallback\"",
		"amend's staged question carries a separate title")

	require.Contains(t, decompose, "--add-question \"Keep or trim the legacy image parsers?\" \\\n  --title \"Legacy image parsers\"",
		"decompose's staged question carries a separate title")
}

// TestDeriveBaselineRecordsForwardWorkAsQuestions pins the brownfield zero-day
// design's forward-work pass (decisions 1, 3, 4, 6, 7): one question per
// unbuilt capability, cited sources, an explicit-only standalone arm, no
// captured bugs, and a hand-off that names the next door without opening it.
func TestDeriveBaselineRecordsForwardWorkAsQuestions(t *testing.T) {
	text, ok := Lookup("derive-baseline")
	require.True(t, ok)
	for _, want := range []string{
		"### Record forward work",
		"Open one question per unbuilt capability, not per file.",
		`--title "<capability>: unbuilt" --source "<path>#<section>"`,
		"A code comment or TODO counts only when a document corroborates it",
		"Read the question ledger, proposed amendments, and the shelves first",
		"A defect-shaped finding is a question too.",
		"Do not run `tiller add bug`, `tiller add chore`, or `tiller add tweak` from this seam.",
		"**Standalone arm.**",
		"Nothing in recorded state names this arm; start it only when the developer asks.",
		"Name that next move and stop",
		"scan existing sources for unbuilt work",
		// A rerun must see declined and redirected leads, not only open ones.
		"covered by any question, open or answered, by a proposed amendment, or by a bug, chore, or tweak",
		// The decision has to be recorded for that check to see it.
		`tiller answer question <Q-id> --answer "<text>"`,
		// Legacy sources compare against the old system, not the new surface.
		"the code the as-is docs describe: surface roots for brownfield, reference roots for legacy-migration",
		// Intake needs the source on disk; a moved source comes back from its blob.
		"recover a retired source with `git show <blob-object-id>` before intake",
	} {
		require.Containsf(t, text, want, "derive-baseline carries %q", want)
	}
	require.NotContains(t, text, "an empty tree ordinarily reaches decompose",
		"an empty tree reaches decompose only when a to-be design doc exists")
	require.NotContains(t, text, "never a captured bug", "the no-add-bug sentence carries that rule")
}

// TestEmptyTreeSkillsNeedToBeInputs pins the brownfield zero-day route in the
// skills a harness auto-selects or an agent follows on a fresh tree: decompose
// is due only with to-be inputs, orient's onboarding stop says so, and the
// doors a forward-work answer opens accept it.
func TestEmptyTreeSkillsNeedToBeInputs(t *testing.T) {
	decompose, ok := Lookup("decompose")
	require.True(t, ok)
	require.NotContains(t, decompose, "or when the tree is still empty")
	require.NotContains(t, decompose, "or the project is fresh and nothing has been decomposed")
	require.Contains(t, decompose, "an empty tree with only as-is docs has nothing to decompose")

	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.NotContains(t, orient, "derive-baseline for an unconfirmed existing-code project or decompose otherwise")
	require.Contains(t, orient, "decompose when a to-be design doc exists")

	require.Contains(t, decompose, "a source that a forward-work question cites, once the human answers that they want it built")

	capture, ok := Lookup("capture")
	require.True(t, ok)
	require.Contains(t, capture, "A human answer to a forward-work question that names a defect is that description.")
}

// orient runs `tiller next` and relays the list as **Ranked next** in
// orientation says: the relay rules (relay as written, name a human move that
// outranks the top agent move, end the reply, start the top agent move only on
// the developer's go, never rewrite the list) live there alone, and orient's
// word-for-word copy is gone.
func TestOrientRelaysTheRankedList(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	for _, want := range []string{
		"Run `tiller next`",
		"Relay it as **Ranked next** in `.tiller/orientation.md` says.",
		"Read the lay of the land, then relay the ranked list to the developer and end the reply.",
		"Their next message redirects you, or starts the top agent move only when it agrees to it, through its seam",
		"or the branch to continue on",
		"the feature touched by your most recent deliberate act that names one",
		"Within a kind, plan order decides",
	} {
		require.Containsf(t, text, want, "orient pins %q", want)
	}
	orientation := strings.Join(strings.Fields(gen.Orientation()), " ")
	relay := []string{
		"Relay the list as written.",
		"If any human move ranks above the top agent move, name it first.",
		"Start the top agent move in your next turn only when the developer's reply agrees to it: a go, a yes, any plain assent such as \"sounds good\", or an instruction naming that row.",
		"Any other reply is answered or acknowledged, and the list still waits for its go.",
		"You may say you would take a different item first and why; never rewrite the list.",
		"A ranked move is a suggestion, not authorization.",
	}
	for _, sentence := range relay {
		require.Containsf(t, orientation, sentence, "orientation's Ranked next carries %q", sentence)
		require.NotContainsf(t, text, sentence, "orient leaves %q to orientation", sentence)
	}
	for _, gone := range []string{"take the first match", "once the developer's reply does not redirect", "`Continuity note`", "`Order note`", "the route suggests waiting"} {
		require.NotContainsf(t, text, gone, "orient no longer carries %q", gone)
	}
}

// orient is the one home for how the ranked list is ranked and for the
// catalogue of moves and the seam each leads to; orientation says what the
// list is and how to relay it, and points here (TestOrientationDescribesRankedNext
// in package gen). Each move keeps the union of the clauses both copies carried.
func TestOrientDescribesRankedNext(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	for _, want := range []string{
		// the five tiers
		"Tiller ranks the moves by a fixed rule and never calls a model.",
		"The rule has five tiers, in this order, with the kind of move and then plan order breaking ties inside each.",
		"Unblock nearby comes first: human decisions that hold work in your current feature, plus any question overdue or due within two days wherever it sits.",
		"Continue is your own unfinished work, and your top Continue move is always kept in the five.",
		"Nearby is the rest of the current feature, then its epic.",
		"Recently touched is the features and epics you acted on within the last fourteen days, most recent day first.",
		"Everything else follows in plan order.",
		// the current feature
		"the feature touched by your most recent deliberate act that names one: a claim, start, submit, check, spec, approval, answer, attest, link, reassign, or release.",
		"Acts on an epic alone and amendment steps never set it, and sync, migrate, join, and other upkeep never count.",
		"Adding work only touches its parent for recency, an added task its feature and an added feature its epic, so a decomposition never sets it.",
		// flags and no identity
		"`tiller next --all` lists every legal move, `--json` prints the payload, and `--contributor` ranks for someone else.",
		"head the list as not personalized (the readout's **Next:** has no heading), the personal tiers are skipped, and plan order ranks the rest, though a question overdue or due within two days still ranks first",
		"When nothing is legal it prints `Caught up: nothing is legal to do right now.`",
		// what is not a row
		"Work in flight on another contributor's branch, work blocked by an unmet dependency, and an unclaimed shelf node are not rows.",
		"An unclaimed task matches no personal assignment until its claim succeeds.",
		// sort inside a tier
		"Inside a tier, moves sort by kind in a fixed order first: the baseline, your own continuing work, amendment review, closeout, claiming, resolving a blocker, plan approval, epic approval, specify, merges, answers, attests, triage, then decompose.",
		"Within a kind, plan order decides: epic order, feature order within an epic, and task order within a feature.",
		"Tier 1 sorts by due date first, overdue first.",
		// the moves, with the clauses only orientation's copy carried
		"and finish its outstanding checks and publication or submission",
		"`carrier_cancelled` means owned work has returned to its shelf; it remains a human needs-attention row with `resolve --clear` or `resolve --by`.",
		"Either way it ends at `tiller propose amendment`, which hands it to the human",
		"Shared feature/epic closeout appears once per reviewed unit, never per contributor.",
		"A coarse finish line arrives when every live riding task is `in_review` or `done`; at epic grain every independent override is already done.",
		"once authorized, `execute` derives the checks, rubric work, human acceptance, and submission from the feature contracts and required gates",
		"Terminal done work keeps its acceptance warnings and historical violations without starving live work.",
		"under an approved ready or active feature and an epic past decomposition, with no unmet recorded dependency or operative question hold",
		"naming `tiller claim task <key>` for that individual unit",
		"An open question leaves hold in one of two ways: an answer, which records a decision, or `tiller cancel question <Q-id> --reason \"...\"`, which records that none is needed.",
		"run them only on the human's instruction, otherwise name the exact command in Notes.",
		"You may `tiller link question <Q-id> --blocks <node>` to widen a question that reaches further than recorded, and say so in Notes.",
		"Several epics list in recorded soft order, the order `tiller queue --kind approve_epic` shows.",
		"ordering alone is not a mutation guard",
		"an epic-grain edge gates the epic's features exactly as a feature-grain edge gates the feature",
		"Among the eligible, recorded soft order chooses first: the owning epic's place in it, then the feature's own.",
		"The frontier is the project's live draft features",
		"re-enters through that noun's `resume <noun> <node> --reason \"...\"`",
		"Surface the wait and stop once submission state is published when remote-backed.",
		"(`tiller intake` a document that makes a case for change, capture a bug, chore, or tweak, or add a to-be design)",
		"without proposing epics from as-is docs or old specs",
		"The diagnostic makes no ownership claim and is omitted when no unfinished unclaimed task remains.",
		"A candidate mend-drift already answered that persists by design is not a move.",
		"Soft order arranges the render and the list among peers the graph leaves unordered and never gates execution.",
		"Independent chains permit parallel work",
	} {
		require.Containsf(t, text, want, "orient pins %q", want)
	}
	// The frontier residue ("never present the frontier as one person's
	// reserved work") is gone; orientation's Ownership paragraph says features
	// and epics have no owner.
	require.NotContains(t, text, "never present the frontier as one person's reserved work")
	require.Contains(t, strings.Join(strings.Fields(gen.Orientation()), " "), "Features and epics have no owner or claim, and claim changes never cascade.")
}

// execute defines what a run covers.
func TestExecuteDefinesTheRun(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"A run is the work the developer's instruction named, and nothing else.",
		"the other tasks in its feature or epic, and a coarse unit's shared closeout, are outside the run.",
		"At task grain, the reported merge of a feature's last task still carries its closeout through feature acceptance, under **Feature acceptance and merge closeout**.",
		"Naming a feature or epic that reviews at feature or epic grain scopes the run to that whole unit",
		"A go on a ranked row scopes the run to that row's node.",
		"When the run's work is done or held, stop with the readout.",
		"When the run covers the whole unit, continue with the unit's other eligible work and its finish line without another go; a run that named one task stops once that task is merged into the aggregate.",
		"Under coarse review, each checked task submits its riding ref on its own branch, commits state, and is merged into the aggregate.",
		"keep the run moving",
		"**Publication sequence** names the merge shapes",
		"A stop at a blocker still ends with the readout, with the blocker named in the notes.",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
	require.NotContains(t, text, "continuing eligible approved work without another go")
	require.NotContains(t, text, "before continuing the unit's eligible work without another go")
	require.NotContains(t, text, "the ordering rules above name the shapes")
	require.NotContains(t, text, "and the unit's shared closeout, are outside the run")
	require.NotContains(t, text, "continue with the unit's other eligible work without another go")
}

// Merge closeout works in the checkout that already holds the integration
// branch and never switches a checkout's branch to get there.
func TestExecuteMergeCloseoutNeverSwitchesACheckout(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"Work in the checkout that holds the configured integration branch, usually the primary checkout, under the worktree closeout rule above: update it safely, preserving unrelated changes, and never switch a checkout's branch to get there.",
		`# run in the checkout that already holds "$base" (git worktree list), usually the primary checkout; never switch a checkout's branch to get there`,
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
	require.NotContains(t, text, "check out the configured integration branch")
	require.NotContains(t, text, `git checkout "$base"`)
}

// Final review fixes outside execute: orient's go wording and the mend-drift
// answer test, decompose's recorded questions, code-review's checkout before a
// disposition, capture's dedupe reads, upgrade's checkout, and capture's
// reproduction rule. orient's Reasoning copy of the go wording and decompose's
// duplicate focused-question sentence are gone; the Hand-off copy and the
// question-only sentence stay.
func TestFinalReviewFixesOutsideExecute(t *testing.T) {
	pins := map[string][]string{
		"orient": {
			"Their next message redirects you, or starts the top agent move only when it agrees to it, through its seam",
			"A candidate mend-drift already answered that persists by design is not a move. `tiller guide mend-drift` carries the test: a `mend-drift:` commit on the candidate's files newer than its `since`.",
		},
		"decompose": {
			"Record a question only for missing product information that would materially change the boundaries or sequence.",
		},
		"code-review": {
			"#### Resolve the whole selection before writing anything\n\nA disposition writes planning state where its commands run. Before logging anything, be in the checkout that holds the integration branch; **Annotate and commit once** below says how.",
		},
		"capture": {
			"the open questions (`tiller ls questions`), and the proposed amendments (`tiller ls amendments --status proposed`)",
			"Reproduction gives the smallest steps from a named starting state",
		},
		"upgrade": {
			"Maintain the checkout that holds the integration branch, so the refresh reaches every branch cut from it; in a task worktree, say so before running.",
		},
	}
	for name, wants := range pins {
		text, ok := Lookup(name)
		require.True(t, ok)
		for _, want := range wants {
			require.Containsf(t, text, want, "%s pins %q", name, want)
		}
	}
	orient, _ := Lookup("orient")
	require.NotContains(t, orient, "only when it says to")
	require.NotContains(t, orient, "which redirects you, or starts the top agent move only when it agrees to it.")
	decompose, _ := Lookup("decompose")
	require.NotContains(t, decompose, "Record a focused question when missing product information would materially change boundaries or order")
	require.NotContains(t, decompose, "Ask focused questions when")
	require.NotContains(t, decompose, "Ask only for missing product information")
}

// Every `under **X**` that sits beside `tiller guide <skill>`, in orientation
// and in the skills, names a heading that skill has. A pointer group followed
// by "in `.tiller/orientation.md`" points into orientation and is checked by
// the orientation pointer tests instead.
func TestGuidePointersNameExistingHeadings(t *testing.T) {
	headings := map[string]map[string]bool{}
	for _, name := range Names() {
		text, ok := Lookup(name)
		require.True(t, ok)
		headings[name] = map[string]bool{}
		fenced := false
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "```") {
				fenced = !fenced
				continue
			}
			if fenced || !strings.HasPrefix(line, "#") {
				continue
			}
			headings[name][strings.TrimSpace(strings.TrimLeft(line, "#"))] = true
		}
	}

	sources := map[string]string{"orientation": gen.Orientation()}
	for _, name := range Names() {
		sources[name], _ = Lookup(name)
	}
	sentenceEnd := regexp.MustCompile(`\.\s+`)
	group := regexp.MustCompile(`under ((?:\*\*[^*]+\*\*(?:, and |, | and )?)+)`)
	bold := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	guide := regexp.MustCompile("`tiller guide ([a-z-]+)`")

	found := map[string]int{}
	for source, text := range sources {
		for _, sentence := range sentenceEnd.Split(strings.Join(strings.Fields(text), " "), -1) {
			targets := map[string]bool{}
			for _, m := range guide.FindAllStringSubmatch(sentence, -1) {
				targets[m[1]] = true
			}
			if len(targets) == 0 {
				continue
			}
			for _, loc := range group.FindAllStringSubmatchIndex(sentence, -1) {
				if strings.HasPrefix(strings.TrimLeft(sentence[loc[1]:], " "), "in `.tiller/orientation.md`") {
					continue
				}
				pointer := sentence[loc[0]:loc[1]]
				require.Lenf(t, targets, 1, "%s: %q sits beside more than one `tiller guide` target", source, pointer)
				var skill string
				for s := range targets {
					skill = s
				}
				require.Containsf(t, headings, skill, "%s: %q points into unknown skill %q", source, pointer, skill)
				for _, b := range bold.FindAllStringSubmatch(sentence[loc[2]:loc[3]], -1) {
					require.Truef(t, headings[skill][b[1]], "%s: %q names **%s**, which `tiller guide %s` has no heading for", source, pointer, b[1], skill)
					found[source]++
				}
			}
		}
	}
	t.Logf("guide pointer headings checked, by source: %v", found)
	require.NotZero(t, found["orientation"], "orientation carries guide pointers")
	require.NotZero(t, found["orient"], "orient carries guide pointers")
}

// Execute's framing and its contract-repair rule.
func TestExecuteFramesItsContentAndContractRepair(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, text, "This skill's Tiller-specific content is the discipline of the loop, not the building.")
	require.Contains(t, text, "an unreadable or invalid contract is uncertainty to repair, never a pass and not a human-only wait.")
	require.NotContains(t, text, "\nIts Tiller-specific content is")
}

// specify grounds the claims a plan rests on in the code they describe, and
// leaves what only building reveals as an open fact.
func TestSpecifyGroundsClaimsAboutCodeItDoesNotOwn(t *testing.T) {
	text, ok := Lookup("specify")
	require.True(t, ok)
	for _, want := range []string{
		"## Grounding claims about code",
		"cite where it was read",
		"the version, tag, or commit",
		"write it as an open fact for the task to determine",
		"Never write a provisional answer marked as assumed",
		"re-read each cited claim against its source",
		"the open facts this plan leaves for execution to determine",
	} {
		require.Containsf(t, text, want, "specify pins %q", want)
	}
}

// execute records a wrong factual claim in task.md as a departure rather
// than editing accepted prose, and bounds that departure.
func TestExecuteRecordsFactualCorrectionsAsDepartures(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"is a departure, not a plan correction",
		"leave the accepted `task.md` unedited",
		"in the task's implementation commit message",
		"Repeat it in the task's PR body when one exists",
		"drops a test or edge case the `task.md` plans",
		"The rule covers only the task being executed",
		"it is no longer a departure",
		"gathered from the riding tasks' implementation commit messages",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
}

// Skills that read task.md as a contract or correct it in place honor a
// recorded execution departure (review).
func TestReviewAndAmendHonorExecutionDepartures(t *testing.T) {
	review, ok := Lookup("code-review")
	require.True(t, ok)
	require.Contains(t, review, "judge the diff against the recorded departure")
	amend, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, amend, "is an execute departure, not a prose correction")
	// Orient's and specify's copies of the departure carve-out moved to the
	// correction table in orientation (TestOrientationCarriesTheCorrectionTable).
}

// TestSkillsRepairAMovedSurfaceThroughUnlink pins the moved-surface route: a
// migrate feature that moves its own recorded surface can only record done
// once the current location is linked and the old path unlinked.
func TestSkillsRepairAMovedSurfaceThroughUnlink(t *testing.T) {
	execute, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"`tiller unlink feature <feature> --surface <old-path>`",
		"a recorded path missing from the integration commit leaves nothing to fingerprint",
		"with a surface remedy had its recorded surface moved or removed",
	} {
		require.Containsf(t, execute, want, "execute pins %q", want)
	}
	mend, ok := Lookup("mend-drift")
	require.True(t, ok)
	require.Contains(t, mend, "dropped through `tiller unlink --surface` once the current location is linked")
	require.Contains(t, mend, "tiller unlink feature submit-certification/rendering-modes --surface")
}

// execute works each unit of work, scoped by review grain, in its own
// worktree by default, gives parallel tasks within a unit their own, defers to
// the developer's checkout instructions, joins inside every new worktree, and
// leaves other agents' worktrees alone.
func TestExecuteWorksInAWorktreeBesideOtherAgents(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"### Worktrees and other agents",
		"execute each unit of work in its own Git worktree",
		"A unit of work is what one review covers, so review grain sets it",
		"the whole feature at feature grain, the whole epic at epic grain",
		"One worktree carries the whole unit.",
		"Commit before every switch",
		"give each parallel task a temporary worktree of its own",
		`git switch --detach "$start_base"`,
		"Aggregate assembly runs in the unit's worktree",
		"A unit's worktree outlives its tasks",
		"Unless the developer's or project's own instructions say otherwise",
		"their instructions win over this default",
		"other agents and developers may be working this repository at the same time",
		"so a new worktree refuses mutations with `contributor_unset` until `tiller join --as <you>` runs inside it",
		"restore any tracked file it changed with `git restore`",
		"Run `resume` only inside that worktree, never in the primary checkout",
		"Integration-branch closeout follows the planning-commit rule under **Committing state** in `.tiller/orientation.md`",
		"never force the checkout",
		"belongs to someone else",
		"look for the work's canonical branch in `git worktree list`",
		"Remove only worktrees you created",
		// Procedure pins that followed their sentences out of orientation.
		"For a unit's first task, add a detached worktree at it and join inside",
		"Another agent's claim and start state lands on its task branch",
		"When Git reports the branch is checked out elsewhere, someone is using it: never force the checkout",
		"Feature checks, the draft PR, coarse submit, and requested changes run in the same worktree.",
		"Remove each one once its branch is committed, pushed when remote-backed, and merged into the aggregate.",
		"usually the primary checkout",
		"Set `worktree` to a new path outside the repository",
		`git worktree add --detach "$worktree" "$start_base"`,
		`git worktree add --detach "$worktree" "$base"`,
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
	require.NotContains(t, text, "Never let two builders share a worktree.",
		"a temporary worktree of its own per parallel task already says it")
	// In both entry blocks the worktree is added and joined before the claim
	// writes state, and claim/start state is committed only on the new branch.
	order := func(earlier, later string) {
		t.Helper()
		i, j := strings.Index(text, earlier), strings.Index(text, later)
		require.NotEqualf(t, -1, i, "missing %q", earlier)
		require.NotEqualf(t, -1, j, "missing %q", later)
		require.Lessf(t, i, j, "%q must precede %q", earlier, later)
	}
	order(`git worktree add --detach "$worktree" "$start_base"`, "tiller claim task submit-certification/certification-form-loading/schema-transcription")
	order(`cd "$worktree" && tiller join --as <you>                   # a new worktree has no identity; restore any refresh it reports
tiller claim task`, "tiller claim task submit-certification/certification-form-loading/schema-transcription")
	order(`git worktree add --detach "$worktree" "$base"`, "tiller claim bug bugs/blank-on-reload")
	order(`cd "$worktree" && tiller join --as <you>                   # a new worktree has no identity; restore any refresh it reports
tiller claim bug`, "tiller claim bug bugs/blank-on-reload")
	order(`git checkout -b "$task_branch" "$task_base"`, "# Commit and share task-specific claim/start state on the new task branch.")
}

// A done feature's eval leg is mended by a reviewed correct-eval amendment,
// while the spec and surface legs keep their persistence wording.
func TestSkillsCorrectADoneFeatureEvalThroughAmend(t *testing.T) {
	mend, ok := Lookup("mend-drift")
	require.True(t, ok)
	for _, want := range []string{
		"On the eval leg of a done feature, and for an `orphan_eval` on a done feature",
		"`tiller amend <A-id> --correct-eval <feature> --reason \"<why>\"`",
		"stop for the human's `tiller approve amendment <A-id>`",
		"Add `--retire <criterion-id>` for each check you deliberately removed",
		"re-pins `verified_against.eval`",
		"The spec leg and the surface leg of `stale_spec` have no such command",
		"tiller amend eval-fix --correct-eval submit-certification/certification-form-loading",
	} {
		require.Containsf(t, mend, want, "mend-drift pins %q", want)
	}
	require.NotContains(t, mend, "no command in this build re-pins it")

	amend, ok := Lookup("amend")
	require.True(t, ok)
	for _, want := range []string{
		"A done feature's eval bindings can be corrected in place",
		"`--correct-eval <feature> --reason \"<why>\"`",
		"`--retire <criterion-id>`",
		"`correct-eval.feature` event",
		"`correct_eval_changes_contract`",
		"`correct_eval_not_feature`",
		"`retire_misplaced`",
		"tiller amend form-engine-rework --correct-eval submit-certification/certification-form-loading",
	} {
		require.Containsf(t, amend, want, "amend pins %q", want)
	}
}

// A stale_spec candidate on the spec or surface leg persists by design once
// mended; mend-drift checks for a prior mend commit and orient does not route
// it again.
func TestMendDriftRecognizesAPersistingCandidate(t *testing.T) {
	mend, ok := Lookup("mend-drift")
	require.True(t, ok)
	require.Contains(t, mend, "persists by design")
	require.Contains(t, mend, "git log --oneline --grep='^mend-drift:'")
	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, orient, "persists by design is not a move")
}

// TestOrientationNeverSwitchesACheckoutForPlanningState pins that the skills
// defer to the orientation planning-commit rule and none offers a branch switch.
func TestOrientationNeverSwitchesACheckoutForPlanningState(t *testing.T) {
	review, ok := Lookup("code-review")
	require.True(t, ok)
	require.NotContains(t, review, "switch to the integration branch first")
	require.Contains(t, review, "follow the planning-commit rule under **Committing state** in `.tiller/orientation.md`")
	derive, ok := Lookup("derive-baseline")
	require.True(t, ok)
	require.Contains(t, derive, "follow the planning-commit rule under **Committing state** in `.tiller/orientation.md`")
	require.NotContains(t, derive, "before switching or staging")
	require.NotContains(t, derive, "safe branch switch")
	require.NotContains(t, derive, "Safely select the configured integration branch")
	exec, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, exec, "Integration-branch closeout follows the planning-commit rule under **Committing state** in `.tiller/orientation.md`: it runs in the checkout that holds the integration branch, never by switching another checkout's branch.")
}

// TestOrientCommitsTheQuestionItRecords pins that orient's Choreography commits
// the one question it may record, with exact paths.
func TestOrientCommitsTheQuestionItRecords(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	i, j := strings.Index(text, "## Choreography"), strings.Index(text, "## Readout")
	require.True(t, i >= 0 && j > i)
	choreo := text[i:j]
	require.Contains(t, choreo, "tiller add question")
	require.Contains(t, choreo, `git commit --only -m "orient: record <short title>" -- <same-exact-paths>`)
	require.Less(t, strings.Index(choreo, "tiller add question"), strings.Index(choreo, "tiller readout orient"))
	require.Contains(t, text, "Reads, plus the one scaffold and any question the briefing defers.")
	require.Contains(t, text, "orient lays no work down")
	require.NotContains(t, text, "orient lays nothing down")
}

// TestExecuteWorkingPlanIsNotThePlanOfRecord pins that a dated working plan is
// notes, and each task's approved task.md stays the plan of record.
func TestExecuteWorkingPlanIsNotThePlanOfRecord(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, text, "stays the plan of record and wins where they differ")
	require.NotContains(t, text, "routine single-task work uses its own approved `task.md`")
}

// TestDeriveBaselineStatesCurrentDriftAndContributors pins that derive-baseline
// no longer denies the baseline drift markers the code now has, keeps the fact
// that the console has no baseline approval action, and names the acting
// contributor rather than owner.
func TestDeriveBaselineStatesCurrentDriftAndContributors(t *testing.T) {
	text, ok := Lookup("derive-baseline")
	require.True(t, ok)
	require.NotContains(t, text, "There are no automatic baseline drift markers")
	require.NotContains(t, text, "acting owner")
	require.Contains(t, text, "the acting contributor")
	require.Contains(t, text, "`tiller drift` reports `stale_design` when tracked feature work crosses to done on a described path")
	require.Contains(t, text, "`tiller sync` may print an `as-is docs touched` notice. `mend-drift` owns those.")
	require.Contains(t, text, "The console has no approval action for baseline docs; confirmation is the `tiller approve design` command above.")
}

// specify's example rubric states observable pass conditions a second grader
// can reproduce, not a taste judgment.
func TestSpecifyRubricStatesPassConditions(t *testing.T) {
	text, ok := Lookup("specify")
	require.True(t, ok)
	require.Contains(t, text, "so two graders reach the same verdict")
	require.Contains(t, text, "Pass only if all hold:")
	require.NotContains(t, text, "Grade whether the copy reads naturally for a homeowner.")
}

// specify plans the task that wires the build and tests gates, and execute
// treats an unconfigured gate as machine work to repair.
func TestSpecifyPlansGateWiring(t *testing.T) {
	spec, ok := Lookup("specify")
	require.True(t, ok)
	require.Contains(t, spec, "still a commented template")
	exec, ok := Lookup("execute")
	require.True(t, ok)
	require.Contains(t, exec, "missing machine work to repair, not a human wait")
	require.Contains(t, exec, "`tiller init` leaves them as commented templates, because no stack exists at scaffold time.")
}

// specify's final pre-spec pass reads in order: ground the claims, then settle
// questions, both before the choreography.
func TestSpecifySettlesQuestionsBeforeChoreography(t *testing.T) {
	text, ok := Lookup("specify")
	require.True(t, ok)
	settle := strings.Index(text, "## Settle questions before approve")
	require.GreaterOrEqual(t, settle, 0)
	require.Greater(t, settle, strings.Index(text, "## Grounding claims about code"))
	require.Less(t, settle, strings.Index(text, "## The contract-grain rule"))
	require.Less(t, settle, strings.Index(text, "## Choreography"))
	require.Contains(t, text, "like C5 (in the example below)")
}

// capture looks for the finding before adding, so one finding is not filed
// twice, and it names the one search before listing every shelf.
func TestCaptureChecksForDuplicates(t *testing.T) {
	text, ok := Lookup("capture")
	require.True(t, ok)
	search := strings.Index(text, "Before adding, search for the finding: `tiller search <keyword>`")
	require.NotEqual(t, -1, search, "the duplicate check opens with the search")
	listing := strings.Index(text, "read the shelves (`tiller ls bugs`")
	require.Greater(t, listing, search, "listing the shelves is the fallback, after the search")
	require.Contains(t, text, "`regression of <id>`")
	require.Contains(t, text, "A reproduction derived by reading rather than run says so.")
}

// capture's severity rule must not tell the agent to always pass the flag and
// then to leave it off.
func TestCaptureSeverityRuleDoesNotContradictItself(t *testing.T) {
	text, ok := Lookup("capture")
	require.True(t, ok)
	require.NotContains(t, text, "Always pass `--severity`")
	require.Contains(t, text, "Pass `--severity` whenever the impact is known.")
}

// capture draws the chore/tweak line: a change to code or tests is a tweak.
func TestCaptureDrawsTheChoreTweakLine(t *testing.T) {
	text, ok := Lookup("capture")
	require.True(t, ok)
	require.Contains(t, text, "A change to code or tests, however small, is a tweak.")
	require.Contains(t, text, "a concrete defect, maintenance obligation, or minor adjustment")
}

// execute's Reasoning reads in the order an agent needs it, ending with the
// Recovery section after the transfer notes.
func TestExecuteSectionsAreInOrder(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	require.NotContains(t, text, "This skill is the thin one")
	headings := []string{
		"## Trigger",
		"## Reasoning",
		"### Worktrees and other agents",
		"### Claiming and starting",
		"### Shelf work",
		"### Batched shelf work",
		"### Working from the plan",
		"### Gate commands",
		"### Departures",
		"### Questions found while building",
		"### The loop around the building",
		"### Aggregate assembly",
		"### Publication sequence",
		"### Feature acceptance and merge closeout",
		"### Review grain and coarse submit guards",
		"### Missing review after merge",
		"### Exceptional human-directed transfer",
		"## Recovery",
		"## Choreography",
		"## Readout",
		"## Hand-off",
	}
	last := -1
	for _, h := range headings {
		i := strings.Index(text, "\n"+h+"\n")
		require.GreaterOrEqualf(t, i, 0, "execute has heading %q", h)
		require.Greaterf(t, i, last, "heading %q follows the previous one", h)
		last = i
	}
}

// Execute is the one home for the branch procedure orientation used to repeat:
// start's recorded names, bases, assembly, and legacy-ref collisions. That
// Tiller never creates, switches, or merges refs is orientation's statement
// (TestOrientationKeepsTheBranchBoundary in package gen), and the guessed-branch
// rule is the recorded-results sentence here.
func TestExecuteOwnsTheBranchProcedure(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"There is no separate feature start.",
		"Use ordinary Git to create independent task branches from configured integration, preserving unrelated work.",
		"Read back task start's `branch` and `base_branch`",
		"not a guessed name",
		"Task-grain dependencies wait for integration, never stacked task PRs.",
		"Never cherry-pick, squash, or copy task code into an aggregate.",
		"Tiller verifies both the recorded heads and current task tips are ancestors when submitting the aggregate.",
		"The aggregate is not a task workbench: build each task on its own branch and merge it in, and an intermediate merge into the aggregate is not a done crossing.",
		"Never automatically delete or rename local or remote refs, force-push, or rewrite recorded task branches: `sync` matches merges against the recorded branches, and rewritten history discards commits other people hold.",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
	require.NotContains(t, text, "Tiller records and verifies branch structure and never creates, switches, or merges refs")
	require.NotContains(t, text, "Never infer the next round's branch from an old PR.")
	require.Contains(t, strings.Join(strings.Fields(gen.Orientation()), " "), "Tiller records and verifies branch structure; it does not create, switch, or merge refs.")
}

// Execute is the one home for the publication sequence; these pins followed
// their sentences out of orientation's execution bullet.
func TestExecuteOwnsThePublicationSequence(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"A gate already passed on unchanged code is reused, which is normal and needs no `--rerun` to be sure; use `--rerun` only for a suspected flake or environment change.",
		"a gate left out whose pass is not current is marked stale, and the node advances only on a full green set",
		"At feature or epic grain the human runs the attest on the recorded aggregate branch, in the worktree that holds it, and commits it there before submit; tiller refuses it elsewhere, so name that branch in the handoff.",
		"A draft with acceptance outstanding, a refused formal submit, and an unpublished submission-state commit are distinct facts",
		"With a remote, recovery requires the real positive PR number; offline recovery omits `--pr` and retains a review ref.",
		"This is state-only closeout: no new PR and no coarse submit.",
		"Sync is advisory after a human merge and may already have recorded the feature `done` with violations.",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
}

// Re-entry after requested changes lives in one paragraph, and it still says
// prior gates may be stale and check runs again before resubmitting.
func TestExecuteReentryRechecksStaleGates(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	require.NotContains(t, text, "The loop is re-enterable.")
	for _, want := range []string{
		"`tiller resume task <id> --reason \"<what changed>\"` moves the task back to `in_progress` on the same task branch and keeps the existing review.",
		"Prior gate results may now be `stale`, so re-run `tiller check` before resubmitting, then complete both pushes and the submission-state readback; an old PR number on resumed or checked work does not mean the new changes were submitted.",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
}

// Execute is the one home for transfer; orient and amend keep their framing
// and point at its section.
func TestExecuteOwnsTheTransferProcedure(t *testing.T) {
	exec, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"Transfer is not an ownership override.",
		"a new same-noun, approved ready replacement with a distinct ID/key, recipient claim, new accepted basis path, and reciprocal lineage",
		"Transfer approval grants no assumption, reviewed answer, or automatic unblocking.",
		"`transfer_review_unobservable`",
		"Follow the response's exact `start_command` and `follow_through` list",
	} {
		require.Containsf(t, exec, want, "execute pins %q", want)
	}
	pointer := "**Exceptional human-directed transfer** in `tiller guide execute`"
	orient, ok := Lookup("orient")
	require.True(t, ok)
	require.Contains(t, orient, "Started unfinished work changes hands only through `tiller transfer task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>`, explicitly authorized by a human naming the work and recipient.")
	require.Contains(t, orient, pointer)
	require.NotContains(t, orient, "use execute's continuation protocol")
	require.NotContains(t, orient, "Closing an old PR, starting the replacement")
	amend, ok := Lookup("amend")
	require.True(t, ok)
	require.Contains(t, amend, "**Ownership-only continuation is not a scope amendment.**")
	require.Contains(t, amend, "A scope change still uses the existing review/amendment paths")
	require.Contains(t, amend, pointer)
	require.NotContains(t, amend, "while creating an approved, unstarted replacement with fresh identity")
	require.NotContains(t, amend, "The original remains historical, not relabeled.")
}

// Orient keeps what a briefing needs to say about execution and points at
// execute for the procedure. That a pure orient opens no review, and that
// no-publish instructions control, is orientation's statement alone.
func TestOrientPointsAtExecuteForExecutionDetail(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	require.NotContains(t, text, "Pure orient never opens a review")
	require.Contains(t, strings.Join(strings.Fields(gen.Orientation()), " "),
		"A pure orient request still briefs and stops. Explicit developer no-publish or earlier-stop instructions control.")
	for _, want := range []string{
		"That stop is the briefing before a seam is authorized.",
		"`tiller guide execute` carries that sequence.",
		"under **Aggregate assembly** and **Review grain and coarse submit guards** in `tiller guide execute`",
		"Every task owns its recorded working branch, even when it rides a coarse review.",
		"Workflow advice is separate from the node's lifecycle `next` moves.",
		"under **Publication sequence** and **Feature acceptance and merge closeout** in `tiller guide execute`",
		"The ranked list is pure: it cannot verify drafts, host state, or pushes.",
		"Recommendation order is not full mutation-guard parity",
	} {
		require.Containsf(t, text, want, "orient pins %q", want)
	}
	for _, gone := range []string{
		"Once the developer has instructed execute, continue related checks, evidence commits even on check exit `1`",
		"A coarse finish line still checks retired ancestors",
		"Feature/epic branches are aggregates, not task workbenches.",
		"Inspect authored contract bindings and the current applicable required gates alongside recorded results",
		"Unreadable or invalid contracts are uncertainty, never inferred acceptance.",
		"A riding task ref is not feature acceptance or formal coarse submission.",
	} {
		require.NotContainsf(t, text, gone, "orient no longer repeats execute: %q", gone)
	}
}

// Orient points at orientation's labeled ownership, contributors, and
// review-grain paragraphs rather than restating them.
func TestOrientPointsAtOrientationForOwnershipContributorsAndReviewGrain(t *testing.T) {
	text, ok := Lookup("orient")
	require.True(t, ok)
	for _, want := range []string{
		"Ownership gates execution, not judgment.",
		"follow **Ownership.** in `.tiller/orientation.md`.",
		"follows **Contributors.** in `.tiller/orientation.md`.",
		"Treat review-grain configuration as a gate decision and name its safe timing in the briefing",
		"follow **Review grain.** and **Review grain changes.** in `.tiller/orientation.md`.",
	} {
		require.Containsf(t, text, want, "orient pins %q", want)
	}
	for _, gone := range []string{
		"with unchanged delegation boundaries",
		"Reservations, review actors, creators, and attached shelf owners do not count.",
		"an affected feature review, an affected task in `in_progress`, `check`, or `in_review`, or an affected branch",
	} {
		require.NotContainsf(t, text, gone, "orient no longer restates orientation: %q", gone)
	}
}

// pointerPins lists each skill's pointers into orientation. Every skill opens by
// reading a missing named section as stale orientation, so each pointer names
// orientation's bold lead-in label or heading exactly
// (TestOrientationLabelsEveryPointerTarget in package gen).
var pointerPins = map[string][]string{
	"orient": {
		"Apply **Developer authorship** in `.tiller/orientation.md` throughout every seam.",
		"Relay it as **Ranked next** in `.tiller/orientation.md` says.",
		"follow **Ownership.** in `.tiller/orientation.md`.",
		"follow **Gates and delegation.** in `.tiller/orientation.md`.",
		"follows **Contributors.** in `.tiller/orientation.md`.",
		"follow **Review grain.** and **Review grain changes.** in `.tiller/orientation.md`.",
		"route it by the **Correction table** in `.tiller/orientation.md`.",
		"Follow **Recovery protocol** in `.tiller/orientation.md`.",
		"Name and address nodes by **Node references** in `.tiller/orientation.md`.",
		"per **Readout** in `.tiller/orientation.md`.",
	},
	"derive-baseline": {
		"follow the planning-commit rule under **Committing state** in `.tiller/orientation.md`.",
		"On refusal, follow **Recovery protocol** in `.tiller/orientation.md`.",
		"per **Readout** in `.tiller/orientation.md`.",
	},
	"decompose": {
		"follow **External references** in `.tiller/orientation.md`.",
		"On any refusal, follow **Recovery protocol** in `.tiller/orientation.md`.",
		"per **Readout** in `.tiller/orientation.md`.",
		"follow the **Correction table** in `.tiller/orientation.md` and **Structural-change preflight** beside it",
	},
	"specify": {
		"follow **External references** in `.tiller/orientation.md`.",
		"Apply **Managed-project independence** from `.tiller/orientation.md` while writing the plan",
		"following the **Correction table** in `.tiller/orientation.md` and `tiller guide amend`.",
		"On any refusal, follow **Recovery protocol** in `.tiller/orientation.md`.",
		"route a correction by the **Correction table** in `.tiller/orientation.md`.",
		"per **Readout** in `.tiller/orientation.md`.",
	},
	"execute": {
		"Apply **Managed-project independence** from `.tiller/orientation.md` before changing setup",
		"Follow **Worktrees** in `.tiller/orientation.md`.",
		"follows the planning-commit rule under **Committing state** in `.tiller/orientation.md`:",
		"route its answer through the **Correction table** in `.tiller/orientation.md`.",
		"takes the **Correction table** in `.tiller/orientation.md`, the route an out-of-bounds departure takes",
		"Apply **Developer authorship** from `.tiller/orientation.md` to all work",
		"On any refusal, follow **Recovery protocol** in `.tiller/orientation.md`.",
		"a gate that stays red past the retry bound under **Recovery protocol** in `.tiller/orientation.md`",
		"per **Readout** in `.tiller/orientation.md`.",
	},
	"capture": {
		"follow **External references** in `.tiller/orientation.md`.",
		"On a refusal, follow **Recovery protocol** in `.tiller/orientation.md`.",
	},
	"mend-drift": {
		"On any refusal, follow **Recovery protocol** in `.tiller/orientation.md`.",
	},
	"amend": {
		"follow **External references** in `.tiller/orientation.md`.",
		"The correction rules under **Correction routing** in `.tiller/orientation.md` distinguish",
		"use their rows in the **Correction table** in `.tiller/orientation.md`.",
		"On any refusal, follow **Recovery protocol** in `.tiller/orientation.md`",
		"per **Readout** in `.tiller/orientation.md`.",
	},
	"code-review": {
		"follow the planning-commit rule under **Committing state** in `.tiller/orientation.md`;",
		"On a refusal, follow **Recovery protocol** in `.tiller/orientation.md`.",
	},
	"upgrade": {
		"On a refusal, follow **Recovery protocol** in `.tiller/orientation.md`.",
	},
}

// TestSkillPointersNameOrientationLabels pins each pointer's wording and proves
// that every pointer into orientation, pinned or not, names a bold label or a
// heading orientation actually carries.
func TestSkillPointersNameOrientationLabels(t *testing.T) {
	orientation := strings.Join(strings.Fields(gen.Orientation()), " ")
	for name, pins := range pointerPins {
		text, ok := Lookup(name)
		require.Truef(t, ok, "%s exists", name)
		for _, pin := range pins {
			require.Containsf(t, text, pin, "%s points at orientation with %q", name, pin)
		}
	}
	const target = "`.tiller/orientation.md`"
	for _, name := range Names() {
		text, _ := Lookup(name)
		for _, line := range strings.Split(text, "\n") {
			for from := 0; ; {
				at := strings.Index(line[from:], target)
				if at < 0 {
					break
				}
				at += from
				from = at + len(target)
				start := strings.LastIndex(line[:at], ". ") + 1
				sentence := strings.TrimSpace(line[start:at])
				if sentence == "This skill assumes" || sentence == "Read" {
					continue
				}
				require.Containsf(t, sentence, "**", "%s points into orientation without a label: %q", name, sentence)
				parts := strings.Split(sentence, "**")
				for i := 1; i < len(parts); i += 2 {
					label := strings.TrimSuffix(parts[i], ".")
					found := strings.Contains(orientation, "**"+label+".**") || strings.Contains(orientation, "### "+label+" ")
					require.Truef(t, found, "%s names %q, which orientation does not label", name, label)
				}
			}
		}
	}
}

// No instruction text hedges a fact as true only "in this build".
func TestInstructionTextStatesFactsWithoutBuildHedges(t *testing.T) {
	for _, name := range Names() {
		text, _ := Lookup(name)
		require.NotContainsf(t, text, "in this build", "%s states the fact without the hedge", name)
	}
	require.NotContains(t, gen.Orientation(), "in this build")
}

// The review follow-ups: orient's residue and human-gate wording, upgrade's
// developer-owned commit, code-review's absent anchors for any target, and
// capture's severity grading for a bug whose impact is known. orient's
// steering-note sentence is gone as residue; orientation's "say you would take
// a different item first and why" carries the limit.
func TestSkillsCarryTheReviewFollowUps(t *testing.T) {
	orient, _ := Lookup("orient")
	require.NotContains(t, orient, "What the old notes described")
	require.NotContains(t, orient, "The list needs no side notes.")
	require.NotContains(t, orient, "Do not add a note that steers toward or away from a ranked row")
	require.Contains(t, strings.Join(strings.Fields(gen.Orientation()), " "), "You may say you would take a different item first and why; never rewrite the list.")
	require.Contains(t, orient, "that is `tiller approve epic <key>`, the human's move, not a seam. Name it and stop.")
	require.Contains(t, orient, "The list holds only legal moves, so every row can be acted on as printed.")

	upgrade, _ := Lookup("upgrade")
	require.Contains(t, upgrade, "The developer reviews and commits the reported project diff.")
	require.NotContains(t, upgrade, "Review and commit the reported project diff.")
	require.Contains(t, upgrade, "save the report outside source state")
	require.NotContains(t, upgrade, "outside .tiller/")

	review, _ := Lookup("code-review")
	require.Contains(t, review, "write `Anchors: absent`, and set Found in to the feature, or to the target itself for a bug, chore, or tweak target.")
	require.Contains(t, review, "Anchors: `<path>:<line>`, each with a short excerpt, or `absent` for required behavior never built.")

	capture, _ := Lookup("capture")
	require.Contains(t, capture, "Grade a bug by impact with `--severity`, on this scale:")
	require.NotContains(t, capture, "Grade every bug by impact")
}

// ruledDeletions names, per skill, every sentence or clause the prohibition
// review removed under ruling 1 (a repeat of an orientation boundary where the
// skill is not where the action happens) and ruling 2 (duplicates and
// residue), recorded in skill-set.md, "Decisions settled so far". Each phrase is one
// no surviving text in that skill carries, so a returning copy fails here.
var ruledDeletions = map[string][]string{
	"amend": {
		"Current ready status alone does not prove that work never started",
		"Do not probe with a mutation",
		"never use a prose correction to rewrite terminal history",
		"A task's parent must already be materialized",
		"Never manufacture replacement identities for an unchanged relocation",
		"Do not remove valid prerequisites just to make the grouped map sort cleanly",
		"Do not stage `.tiller/` or a whole node directory",
		"Do not restate the staged operation count",
	},
	"capture": {
		"Severity never routes",
		"files, never a directory",
	},
	"code-review": {
		"never approving anything",
		"A review is never a suggested next move",
		"Style and naming are never bugs",
		"it never becomes a queue position",
		"Never reopen done work",
		"change prose, source code, state, the manifest",
		"never become a reason to repair, fetch, refresh, or mutate anything",
		"Never cap the table",
	},
	"decompose": {
		"Do not use decompose to bypass that readiness gap",
		"do not impose an all-doc confirmation requirement",
		"Do not turn every unspecified detail into a blocking question",
		"silently fall back to subsystem-first planning",
		"do not drop later requirements",
		"Never rewrite terminal feature or task prose",
		"Record dependencies between features as state, never only as prose",
		"so never hand-edit that file",
		"never as exclusive claimants",
		"Tiller writes the state files and never commits them",
		"Do not stage `.tiller/` or a whole directory",
		"erase a valid edge",
		"Never hand-edit the node YAML to make a correction",
		"Do not ask whether to open an amendment when this list already maps the feedback",
		"never guess",
		"never the whole draft tree by default",
		"Shelf items you filed wait for their own triage",
		"never a bare id",
	},
	"derive-baseline": {
		"Do not broaden a path to a parent that contains but is outside the declared root",
		"never a captured bug",
		"Never hand-edit `baseline.yml` or a confirmation block",
		"never the external original",
		"Do not run an unrestricted commit against an index you have not scoped",
		"Tiller writes state and never commits it for you",
	},
	"design-brief": {
		"Never silently sample a subset",
		"Do not change canonical prose, source code, state YAML",
		"Do not claim, start, specify, approve, attest, record evidence, or commit anything",
		"Instruction refresh is a separate developer operation",
		"do not repair the project to make the export work",
		"Recorded path coverage does not verify implementation",
		"do not establish effective inherited holds",
	},
	"execute": {
		"never carry uncommitted work from one branch to another",
		"Never let two builders share a worktree",
		"Do not release completed work or change ownership",
		"Creation and approval never assign tasks",
		"Features and epics have no exclusive owner, and claims never cascade",
		"Tiller records and verifies branch structure and never creates, switches, or merges refs",
		"Never infer the next round's branch from an old PR",
		"do not claim or start an attached feature",
		"Never ask the question only in chat",
		"never an entire `.tiller/` directory",
		"do not start unrelated work",
		"Never hand-edit state to evade the refusal",
		"never adds commits to a branch after its final merge",
		"Green task gates, a riding ref, or an unpublished state commit never proves",
	},
	"mend-drift": {
		"never a ledger hand edit",
	},
	"orient": {
		"never approval, claiming, or starting",
		"never lifecycle work",
		"Never identify yourself, your agent, model, provider, or harness",
		"No generated-by notices, agent signatures",
		"Describe the work and observed verification, not the tool that produced it",
		"You are not doing the work at the seam in this reply",
		"Pure orient never opens a review",
		"Unknown references do not become current nodes",
		"These reads require no identity and write no files",
		"Relay the list as written",
		"If any human move ranks above the top agent move",
		"Start the top agent move in your next turn",
		"Any other reply is answered or acknowledged",
		"never rewrite the list",
		"A ranked move is a suggestion, not authorization",
		"do not hand it to the developer",
		"never blindly regenerate duplicates",
		"Ownership is per unit, never inherited from a feature",
		"Do not assign this work to the last finisher",
		"never present the frontier as one person's reserved work",
		"A draft waiting on unfinished work is no move",
		"do not infer its current host state",
		"unclaimed shelf work never does",
		"Do not add a note that steers toward or away from a ranked row",
		"do not repair sources, refresh instructions, record questions, or enter another skill",
		"never as the next line of an orient",
		"Do not expand it into concrete node rows",
	},
	"specify": {
		"Do not stop between them",
		"local Tiller may invoke them, never the reverse",
		"Do not turn planning-only CI skips into replacement Tiller checks",
		"never manufacture replacement merely to evade a lock",
		"do not annotate every name it mentions",
		"Do not cancel and redecompose merely to change its address",
		"never through a hand edit to node YAML",
		"Do not restate the criteria and their binding types",
		"never guess",
	},
	"upgrade": {
		"never claiming, starting, specifying, approving, or the next queued work",
		"Do not migrate stale branches independently",
	},
}

// TestSkillsDropTheRuledDeletions guards every ruling 1 and ruling 2 removal.
// It matches after whitespace normalization, so a copy that returns wrapped
// across lines still fails.
func TestSkillsDropTheRuledDeletions(t *testing.T) {
	for name, gone := range ruledDeletions {
		raw, ok := Lookup(name)
		require.Truef(t, ok, "%s exists", name)
		text := strings.Join(strings.Fields(raw), " ")
		for _, phrase := range gone {
			require.NotContainsf(t, text, phrase, "%s no longer carries %q", name, phrase)
		}
	}
	// The coverage sentence went to orientation, which does not name assumption
	// waivers, so design-brief keeps that half.
	brief, _ := Lookup("design-brief")
	require.Contains(t, brief, "Question targets do not establish assumption waivers.")
}

// TestSkillsKeepTheRuledOrientationRepeats pins both sides of each boundary
// ruling 1 kept in a skill: the action is irreversible, destructive to someone
// else's work, a human-only gate, or outward-facing, and the skill is where it
// happens. Each row pairs the skill's sentence, as it now reads, with the
// orientation sentence that states the same boundary, so neither copy can
// leave or drift unnoticed.
func TestSkillsKeepTheRuledOrientationRepeats(t *testing.T) {
	orientation := strings.Join(strings.Fields(gen.Orientation()), " ")
	for _, row := range []struct{ skill, sentence, orientation string }{
		{"design-brief", "Never silently overwrite an existing file.",
			"preserve existing output unless replacement is explicitly authorized"},
		{"derive-baseline", "Never edit reference code or relabel it as surface to get a command through.",
			"legacy-migration understands reference code without editing it"},
		{"derive-baseline", "Never move or retire files outside this project: they are outside this repository's history",
			"External imports preserve their external originals."},
		{"derive-baseline", "Do not retire an input until all its destination refs are recorded and recoverable.",
			"record recoverable `link design --from` provenance for every destination"},
		{"derive-baseline", "Never stash, reset, discard, or commit someone else's work to make the run look clean.",
			"is someone else's: never remove, reset, clean, stash, commit into, or check out over it."},
		{"derive-baseline", "Do not self-approve or derive duplicates to escape the baseline move the ranked list names: approval is a human gate.",
			"never self-approve without explicit instruction naming those docs"},
		{"amend", "Do not install or upgrade without direction.",
			"disclose the limitation rather than promise accepted relocation or upgrade without direction"},
		{"code-review", "The request authorizes only the review or the named disposition: never approve, claim, start, attest, resolve, or cancel anything.",
			"It authorizes only the read-only review or the named disposition; never approve, claim, or start anything."},
		{"code-review", "Leave the status to the human and never run it.",
			"are never yours to run, even on instruction"},
		{"execute", "it runs in the checkout that holds the integration branch, never by switching another checkout's branch.",
			"Never switch a checkout's branch to get there."},
		{"execute", "belongs to someone else: never remove, reset, clean, stash, commit into, or check out over it.",
			"is someone else's: never remove, reset, clean, stash, commit into, or check out over it."},
		{"execute", "Never automatically delete or rename local or remote refs, force-push, or rewrite recorded task branches: `sync` matches merges against the recorded branches",
			"never delete, rename, or rewrite local or remote history to make a branch fit: `tiller sync` matches merges against the recorded branches"},
		{"execute", "Never identify yourself, a model, provider, or harness as an author or contributor.",
			"Never identify yourself, an agent, model, provider, or harness as an author or contributor in any output"},
		{"execute", "Use the developer's configured Git identity, never an agent author or committer identity or an invented human identity.",
			"Use the developer's configured Git identity, never an agent author or committer identity, and never invent a human identity."},
		{"execute", "Never attest or restore a human gate yourself",
			"Never attest, never assert a fast-forward, and never perform the final integration merge."},
		{"execute", "Never commit to that branch after its final merge",
			"or commit to a review branch after its final integration merge: a force-push rewrites history"},
		{"execute", "Never commit to the merged review branch.",
			"or commit to a review branch after its final integration merge: a force-push rewrites history"},
		{"execute", "Never run the assertion yourself.",
			"`sync --accept-fast-forward`, and the final merge into the configured integration branch are never yours to run, even on instruction"},
		{"execute", "A fast-forward assertion remains the human's command, never the agent's.",
			"never assert a fast-forward"},
		{"execute", "never --accept-fast-forward",
			"never assert a fast-forward"},
		{"execute", "Never infer this authority from a general execute instruction or another developer's availability.",
			"only an explicit human instruction naming the work and recipient authorizes `tiller transfer"},
		{"execute", "Do not bypass required gates or developer no-publish limits.",
			"Explicit developer no-publish or earlier-stop instructions control."},
		{"mend-drift", "Never infer approval from permission to investigate, draft, commit, or mend",
			"Only explicit human instruction naming design-relative docs authorizes `tiller approve design <doc>...`"},
		{"orient", "Run `approve` only on explicit human instruction naming nodes or docs; baseline derivation never authorizes its own confirmation.",
			"on an explicit instruction naming the node and target, or the exact design docs, you may run `approve`"},
	} {
		text, ok := Lookup(row.skill)
		require.Truef(t, ok, "%s exists", row.skill)
		require.Containsf(t, text, row.sentence, "%s keeps its repeat %q", row.skill, row.sentence)
		require.Containsf(t, orientation, row.orientation, "orientation states the boundary %s repeats: %q", row.skill, row.orientation)
	}
}

// TestSkillsStateRuledReasonsAndPositiveForms pins what the prohibition review
// added to the skills: the reason each boundary in ruling 3 gained, and the
// positive form ruling 4 gave each exactly restatable prohibition.
func TestSkillsStateRuledReasonsAndPositiveForms(t *testing.T) {
	for name, wants := range map[string][]string{
		"design-brief": {
			// Ruling 3. Both sensitive-data sentences carry the reason, because
			// they sit in different sections.
			"Never export real customer records, credentials, secrets, or private production payloads to make examples realistic: the brief is shared outside the repository, beyond the project's access controls.",
			"Do not copy sensitive payloads out of a source just because the source is a requirement: the brief leaves the project's access controls.",
			// Kept because orientation's recorded-claims sentence does not name waivers.
			"Question targets do not establish assumption waivers.",
		},
		"derive-baseline": {
			// Ruling 3.
			"Never move or retire files outside this project: they are outside this repository's history, so the change cannot be reviewed or undone, and someone else may own them.",
			"Never use force: it discards uncommitted edits.",
		},
		"code-review": {
			// Ruling 3.
			"Never push: the commit holds proposed rows the developer has not seen, so publishing it is theirs.",
			// Ruling 4.
			"Recheck selected rows only.",
			"a later request opens its own amendment.",
		},
		"execute": {
			// Ruling 3.
			"or rewrite recorded task branches: `sync` matches merges against the recorded branches, and rewritten history discards commits other people hold.",
			"never commit the deletions without the new segment files, or the old events would be lost",
			"Never commit the deletions without the new segment files, or the old events are lost.",
			// Ruling 4.
			"Those files are reading context; the shelf node's own contract and gates are the ones that apply.",
			"A run keeps going after it records a question: stop only when",
			"Run check and each evidence commit as separate commands, so a failing check still commits its evidence.",
			"Run the other checks and commit their state even while a manual criterion is pending.",
			"At epic grain, submit the epic, not its riding features.",
			"Substitute real values for every placeholder before running a command.",
		},
		"upgrade": {
			// Ruling 3.
			"or erase caches to bypass a refusal: older binaries cannot read migrated state, and the cache holds the rollback record.",
		},
		"specify": {
			// Ruling 4.
			"and keep the batch to the named features.",
			"Every criterion binds to a check: a `test`",
			"Use ordinary sibling `tiller order` for preference, and a hard edge only for a true prerequisite.",
		},
		"decompose": {
			// Ruling 4.
			"Record every known cross-epic edge in this sweep yourself, as state.",
			// Kept: nothing else bars inventing future work.
			"do not invent speculative future work.",
		},
		"orient": {
			// Kept: orientation bars the edit, not proposing it in a briefing.
			"Do not propose a state-file edit as recovery.",
			"When every draft waits on unfinished work,",
		},
		"capture": {
			"Whatever the severity, the shelf's soft order decides what is next.",
		},
		"mend-drift": {
			// Ruling 4.
			"Leave that mend standing.",
		},
	} {
		text, ok := Lookup(name)
		require.Truef(t, ok, "%s exists", name)
		for _, want := range wants {
			require.Containsf(t, text, want, "%s states %q", name, want)
		}
	}
}

// The duplicate check before a forward-work question or a shelf item found in
// a document names the search, the way capture's does: an agent runs the
// command a skill names.
func TestDuplicateChecksNameTheSearch(t *testing.T) {
	for _, skill := range []string{"capture", "derive-baseline", "decompose"} {
		text, ok := Lookup(skill)
		require.True(t, ok)
		require.Containsf(t, text, "`tiller search <keyword>`", "%s names the search in its duplicate check", skill)
	}
}

// execute covers a feature prerequisite inside one epic review beside the
// task rule: the dependent starts from the aggregate once the prerequisite
// feature's tasks are submitted and merged into it.
func TestExecuteCoversSameEpicFeaturePrerequisiteStarts(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	for _, want := range []string{
		"A coarse dependent may branch from the aggregate after submitted prerequisites and submission-state commits are assembled there.",
		"Under epic review the same holds between features of one epic",
		"starts once every task of that feature is submitted and merged into the aggregate",
		"a refused start names the task still to submit or merge",
	} {
		require.Containsf(t, text, want, "execute pins %q", want)
	}
}

// Execute carries shelf recovery beside aggregate recovery: the same command
// and authorization, the nodes it applies to, and what it never does.
func TestExecuteCoversShelfReviewRecovery(t *testing.T) {
	text, ok := Lookup("execute")
	require.True(t, ok)
	flat := strings.Join(strings.Fields(text), " ")
	for _, want := range []string{
		"Explicit authorization to recover the review of a named feature, epic, bug, chore, or tweak also enters execute.",
		"tiller submit feature|epic|bug|chore|tweak <node> --recover-merged",
		"One merged PR may be recorded against every node it fixed",
		"Recovery refuses a `proposed`, `done`, or `cancelled` node and a node a live carrier still carries",
		"the result lists `missing_gates`",
		"crosses to `done` with basis `recovered`",
	} {
		require.Containsf(t, flat, want, "execute states %q", want)
	}
	description, ok := Description("execute")
	require.True(t, ok)
	require.Contains(t, description, "explicitly authorized historical review recovery")
}
