// Package gen builds deterministic orientation and live project projections.
// Everything here is a pure function of its inputs with no cobra and no stdout,
// so the same inputs always yield the same output.
package gen

// This file generates .tiller/orientation.md from distribution.md's four-part
// outline. The file is generated, committed, and keyed to the schema generation
// rather than the binary version, so every binary in a schema line emits the
// SAME bytes and re-running init is a no-op diff. The content is therefore a
// constant: it carries Tiller's rules, never project specifics, which are
// deferred to `tiller status` so the file itself stays stable.

import (
	"strconv"
	"strings"
)

// orientationSchemaGen is the schema generation orientation.md is keyed to. It
// changes only when the rules the file states change, whether the driver is a
// schema bump or a deliberate revision of the rules text, which is what keeps
// the committed file byte-stable across ordinary binary upgrades.
//
// The emitted header carries this generation for stale-guidance detection.
// Increment it when the rules change, independently of binary patch versions.
// Generation 48 ranks unclaimed execution by the reader's own task history
// first and names the Continuity note (issue #139). Generation 49 executes
// started work in its own Git worktree by default, defers to the developer's
// own checkout instructions, and assumes other agents share the repository.
// Generation 51 adds the shared verified-URL external reference writing rule
// (issue #95); generation 50 added reviewed done-feature eval binding correction.
// Generation 52 scopes the default worktree to the unit of work one review
// covers, so a coarse feature or epic shares one worktree, and parallel tasks
// within it get their own. Generation 53 has the human run a coarse review's
// attest on its recorded aggregate branch, committed there before submit, and
// records the grain a task started under (issues #146 to #150).
// Generation 54 reuses a gate already passed on unchanged code and says when to rerun (issue #152).
// Generation 55 ranked next replaces the route walk: the agent relays a ranked
// list of every legal move and starts the top agent move.
// Generation 56 refuses a plan gate while an open question holds the plan:
// specify settles its questions before approve, and execute records a mid-run
// question on the branch it builds and builds around it (issue #152).
// Generation 57 lets a human cancel a question that needs no decision and narrow what it blocks; an agent may only widen.
const orientationSchemaGen = 57

// Orientation returns the committed .tiller/orientation.md text. It is a pure
// function of constants for a generation, so two inits produce byte-identical
// files and a re-generation is a no-op diff with no git churn.
func Orientation() string {
	return orientationHeader + strconv.Itoa(orientationSchemaGen) + orientationBody
}

// OrientationGeneration returns the schema generation orientation.md is
// currently keyed to, the same value Orientation stamps into the header. It
// is exported so a caller comparing an on-disk file against the running
// binary (`tiller check readiness`, issue #41's stale-orientation gap) reads
// the one number gen already owns rather than keeping a private copy that can
// drift from it.
func OrientationGeneration() int {
	return orientationSchemaGen
}

// OrientationStampGeneration parses the generation number stamped into an
// orientation.md's header, the read side of the stamp Orientation writes. It
// reports ok false, rather than a guessed generation, when data does not
// begin with a recognizable Tiller orientation header: a missing file, a
// foreign one, or a stamp mangled beyond recognition all read the same way to
// a caller, none of them a positive claim about which generation wrote it.
func OrientationStampGeneration(data []byte) (int, bool) {
	rest, ok := strings.CutPrefix(string(data), orientationHeader)
	if !ok {
		return 0, false
	}
	end := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
	if end <= 0 {
		return 0, false
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

// orientationHeader and orientationBody sandwich the generation stamp. They are
// two constants rather than one only so the counter can sit inside the emitted
// bytes; nothing else about the text varies.
//
// orientationHeader's exact bytes are a cross-generation parse contract, not
// just this generation's prose: OrientationStampGeneration matches an
// on-disk file by this literal prefix, so it has to keep reading a file any
// past generation wrote, not only the current one. Rewording this string
// (moving a word, changing punctuation, retiring the backtick-fenced `tiller
// init`) breaks that parse for every already-committed orientation.md still
// carrying the old wording, silently, exactly when staleness is what the
// parser exists to detect. A change here needs its own generation bump and a
// parser that still recognizes the old header too; do not reword this
// constant to fix a typo or reflow a sentence without checking
// OrientationStampGeneration and its tests first.
const orientationHeader = `<!-- GENERATED by Tiller (` + "`tiller init`" + `), orientation generation `

const orientationBody = `. Do not hand-edit: this file is
regenerated and is byte-stable for a generation, so any edit is
overwritten on the next init or maintenance refresh. Live project specifics are
deferred to ` + "`tiller status`" + ` so this file stays stable. -->

# Orientation

This project uses Tiller to run its SDLC workflow. Specs, decomposition,
planning, and acceptance gates live in ` + "`.tiller/`" + `, under git, beside the code
they describe. This file is your front door: it explains what Tiller is, the
rules you must follow, how to work, and where things live. It depends on nothing
but a filesystem, a shell, and the ` + "`tiller`" + ` CLI.

## What Tiller is

Tiller holds the structure, the gates, and the state. You, the agent, do the
reasoning. The human approves at the gates. And Tiller guides rather than
controls: it offers structure and guidance you should follow, surfaced honestly
as guidance, not walls. The one boundary that genuinely holds sits outside
Tiller, at the final merge into the configured integration branch, which only the human performs. Everything you do
ultimately attributes to the person who directed you.

Plan the full known initial scope as bounded product and platform increments,
not permanent subsystem buckets. Infer progression from documented consumers
and outcomes; implement shared machinery for its consuming increment, retaining
known later needs in later features. Record true prerequisites as
` + "`depends_on`" + ` and preferred sibling sequence with ` + "`tiller order`" + `.
Specify tasks just in time. Apply the same placement rule to amendments and
intake: extend the same bounded outcome, create a new increment for a distinct
outcome or delivery phase, and preserve accepted history. Missing information
that materially changes boundaries or order needs a focused question, not an
invented journey. Propose wider changes for developer direction, not as an
unrequested replan.

## The rules you must follow

- Managed projects must never depend on Tiller. Setup, build, tests, CI,
  packaging, deployment, runtime, and merge eligibility must work without
  Tiller installed, release access, or ` + "`.tiller/`" + ` as an operational input.
  Never add Tiller packages, setup hooks, command wrappers, CI installation
  steps, release credentials, runtime imports, or required host checks.
  Optional or advisory Tiller CI jobs are not an alternative.
  Tiller may invoke an independently runnable project command such as
  ` + "`bin/ci`" + ` locally and record its result; that command must never call
  Tiller back. Planning and evidence files remain inputs to the local Tiller
  workflow, not to application commands or CI state validation.
  For planning-only changes, use ordinary repository change classification
  and report application checks as not applicable only for confirmed
  non-executable, non-test-consumed paths. Code, mixed, unknown, protected-input,
  and uncertain-history changes still run the normal project checks.
  Never blanket-exempt ` + "`.tiller/**`" + `, Markdown, or YAML, or invent
  replacement Tiller checks. If an existing or approved plan introduces this
  dependency, name the conflict and propose an independent alternative;
  do not implement it silently, provision a token, or rewrite accepted
  contracts without the applicable human review.
  This rule does not prohibit Tiller's own implementation, tests, release
  automation, or disposable verification from exercising Tiller.
  It is guidance, not a claim that Tiller can prevent arbitrary project edits.
- The developer owns and is responsible for the work. Never identify yourself,
  an agent, model, provider, or harness as an author or contributor in any
  output: code, comments, docs, commit messages, PR titles or descriptions,
  issues, reviews, reports, or replies. Never add generated-by notices,
  agent signatures, badges, footers, or agent co-author trailers. Use the
  developer's configured Git identity, never an agent author or committer
  identity, and never invent a human identity. Write about the change, its
  reasoning, and observed verification, not the tool that produced it. This
  rule applies across every skill and hand-off. It does not remove necessary
  technical references, real human credits, or required third-party notices.
- For authored prose, follow the External references section in
  ` + "`tiller guide orient`" + `. It is the shared writing rule across skills
  and hand-offs.
- Drive work to the edge of done and stop. You may propose, build, attach
  evidence through gates, run gates, and open PRs. A gate is the human's
  decision, not the human's keyboard: on an explicit instruction naming the
  node and target, or the exact design docs, you may run ` + "`approve`" + ` or ` + "`resolve`" + ` yourself, stating exactly
  what you are about to record and reporting what you recorded. A declarative
  past-tense claim ("they're all approved") is authorization to record only
  when the set is unambiguous: verify the recorded state first, then record,
  then report the delta. ` + "`cancel`" + ` is human-only: hand the exact ` + "`cancel`" + ` command
  to the human rather than run it. ` + "`attest`" + `, ` + "`sync --accept-fast-forward`" + `, and the final merge
  into the configured integration branch are never yours to run, even on instruction;
  hand the human the exact command. Merging task branches into their recorded
  feature or epic aggregate is authorized execution, not a done crossing.
- Mutate state only through ` + "`tiller`" + `. The state files are machine-owned: never
  hand-edit ` + "`epic.yml`" + `, ` + "`feature.yml`" + `, ` + "`task.yml`" + `, ` + "`bug.yml`" + `,
  ` + "`chore.yml`" + `, ` + "`tweak.yml`" + `, ` + "`design/baseline.yml`" + `, or the event stream, and never
  hand-edit this file. Every change to state lands through a command, so the event stream
  and the gate records stay honest. The one ` + "`.yml`" + ` you own is the manifest,
  ` + "`.tiller/tiller.yml`" + `: its configuration (gates, contributors, inputs) is
  human-owned, so edit freely.
- End lifecycle reasoning by recording it through a command. The reasoning
  is yours; the recording is the CLI's. The explicitly requested
  ` + "`design-brief`" + ` export is the exception: return a derived Markdown
  snapshot without changing source requirements, state, claims, gates,
  events, or Git history. Only an explicitly requested brief file may be
  written, never canonical prose or state; preserve existing output unless
  replacement is explicitly authorized. No approval or execution is implied.
  An explicitly requested ` + "`code-review`" + ` review writes only its review
  file under ` + "`.tiller/reviews/`" + `; its disposition records through
  ` + "`tiller`" + ` commands and one commit, and approves nothing.
- Commit the state Tiller writes. Every command records into git-tracked files
  under ` + "`.tiller/`" + `, and Tiller never commits them for you. The commit is part
  of the work: commit the state delta with, or right after, the change it
  describes, using plain ` + "`git`" + ` and this project's own commit conventions.
  One requirement rides on top of those conventions rather than replacing them:
  a commit you author while executing a task carries that task's id. A commit
  while executing a started shelf node carries that shelf node's own id, so
  ` + "`git log`" + ` can walk from either commit to the node that explains it. An
  uncommitted ` + "`.tiller/`" + ` change is not shared state and is not a review
  surface. The final integration merge is still the human's; committing is not. A diff that
  touches only ` + "`.tiller/`" + ` never needs a pull request: planning state, gate
  records, and events are CLI-written audit records, so commit them directly.
  A mid-task delta belongs on its own recorded task branch. A delta while executing a
  started shelf node belongs on that shelf node's branch; pre-start shelf state
  remains under the integration-branch fallback, as does other planning state.
  The PR ceremony is for code.
- Every started task has its own recorded branch, unchanged on resume. Claim the
  task, then ` + "`tiller start task`" + ` atomically fixes its owner, records its branch,
  and activates a ready parent feature. There is no separate feature start.
  At task grain the feature has no branch. At feature grain the aggregate
  uses ` + "`reviews/features/<epic>/<feature>`" + ` in the configured branch pattern;
  at epic grain it uses ` + "`reviews/epics/<epic>`" + `. Task branches use the full
  canonical task key. Read the recorded names rather than inventing them.
  Read back start's ` + "`branch`" + ` and ` + "`base_branch`" + ` before creating refs.
  Task start can record a missing aggregate after a grain change; a reopened
  coarse round records a new aggregate suffixed with the new task id, preserving
  merged refs. Never infer the next round's branch from an old PR.
  Create new independent task branches from the configured integration branch.
  Task-PR dependencies wait for integration; never invent stacked task PRs.
  A coarse dependent task may start from its aggregate once submitted prerequisites,
  including their submission-state commits, have been merged into it.
  Use ordinary Git to create these branches and to merge each submitted task's
  current tip into the aggregate. No cherry-pick or squash assembly: both the
  recorded review head and current task tip must remain ancestors of the aggregate.
  Assemble all riding task tips before feature checks, draft publication, or coarse
  submit. Resolve mechanical conflicts; stop for semantic conflicts needing a human
  decision. Resume requested changes on that task's existing branch, check and
  resubmit there, merge its new tip into the aggregate, rerun affected gates, and
  update the same aggregate PR. Do not ask the developer to do routine branch work.
  Preserve nonoverlapping task records, evidence, and events during Git assembly.
  Never resolve conflicting real decisions by hand-editing state YAML.
  Tiller records and verifies structure; it does not create, switch, or merge refs.
  Preserve unrelated work during switches. A retained legacy feature-prefix ref
  may block a new task ref: report both names and preserve history. Use a
  non-conflicting configured branch pattern for unstarted work, or ask the human
  to archive/rename the colliding ref deliberately; never auto-delete or rename
  local or remote history to make branch creation succeed.
- Execute each unit of work in its own Git worktree by default, leaving the
  primary checkout where the developer left it. A unit of work is what one
  review covers, so review grain sets it: the task at task grain, the whole
  feature at feature grain, the whole epic at epic grain, and always the bug,
  chore, or tweak itself. The developer's or project's own
  instructions about checkouts and worktrees take precedence over this default;
  follow them when they differ. Add the worktree detached at the updated start
  base (the configured integration branch, or a coarse dependent task's
  aggregate), then run ` + "`tiller join --as <you>`" + ` inside it: ` + "`.tiller/identity.yml`" + `
  is per checkout and gitignored, so a new worktree has no identity. Restore any
  tracked file join's guidance refresh changed rather than committing it with the
  work, and tell the developer the project needs ` + "`tiller upgrade --project-only`" + `.
  Claim, start, and create the recorded branch inside that worktree, so start's
  state follows onto the branch. At feature or epic grain the unit's one
  worktree holds each riding task branch and the aggregate in turn: a later task
  detaches it at its start base, commit before every switch, and assembly,
  checks, and submission run there too. Tasks built in parallel within a unit,
  by subagents or otherwise, each get a temporary worktree of their own, removed
  once merged into the aggregate. Re-enter through the unit's worktree holding
  the recorded branch, or add and join one for it, and run ` + "`resume`" + ` there.
  Integration-branch closeout runs where that branch is checked out, usually the
  primary checkout, and only when it holds no changes you did not make.
  Assume other agents and developers may be working this repository at the same
  time in their own worktrees. A worktree, branch, or uncommitted change you did
  not create for this work is someone else's: never remove, reset, clean, stash,
  commit into, or check out over it, and never force a checkout of a branch Git
  reports is checked out elsewhere. Another agent's claim lands on its task
  branch, so Tiller's own state may not show it: before claiming, look for the
  work's canonical branch in local worktrees, branches, and the remote. When one
  exists or Git surfaces a conflicting claim or start, stop for the human rather
  than choosing a winner. Remove only worktrees you created, and only after their
  work is committed and, when remote-backed, pushed.
- Schema 6 freezes execution ownership, not a feature-wide claim. Inspect an old
  checkout with ` + "`tiller migrate --dry-run --json`" + `; it writes nothing.
  Save the full report or its ` + "`data.ownership_review`" + ` outside source state.
  Have the human fill every initially empty ` + "`confirmed_owner`" + ` for started
  unfinished tasks, bugs, and chores, leaving all other snapshot fields unchanged.
  Apply with
  ` + "`tiller migrate --ownership-review <file>`" + ` or
  ` + "`tiller upgrade --project-only --ownership-review <file>`" + `.
  A stale or incomplete review refuses. Schema-5 active work and open reviews
  keep their branches, PRs, statuses, and evidence. Only migrations crossing
  schemas 2–4 into schema 5 require draining shared-branch work with a verified
  compatible old binary first, including final merges and sync.
  Retain a compatible executable and recoverable committed baseline. Pause
  team mutations, reconcile state and active owners on the designated integration
  branch, inspect/review/apply once in a designated checkout, then commit and
  publish through normal Git. Upgrade all participants and merge that migrated
  state into each continuing work branch without discarding task-specific state.
  Do not migrate stale branches independently. Refresh each checkout, restart
  consoles, reload agent instructions, and check schema, identity, task owner,
  branch, and legal next action before resuming. September 21 is an intended
  adoption window, not permission to skip verification or migrate a project.
  A schema fence is local, not a distributed lock; old binaries in unmigrated
  clones can still conflict. Never use one on migrated state. After adoption,
  recovery must preserve new work rather than blindly restore old schema files.
- Schema 8 adds the Tweak shelf. Migration from schema 7 changes only the
  manifest version, preserves existing Bug and Chore records, and requires no
  work drain. Old binaries refuse schema 8; upgrade every participant before
  resuming state mutations.
- ` + "`tiller upgrade`" + ` updates the installed binary and the current checkout,
  including supported schema migrations and generated instructions.
  ` + "`tiller upgrade --project-only`" + ` uses the running binary without network
  or package-manager calls; use it for stale instructions and other checkouts.
  ` + "`--binary-only`" + ` deliberately excludes project maintenance.
  These scopes are mutually exclusive; project-only cannot select a version.
  Maintenance never selects a different checkout identity: ` + "`tiller join --as <you>`" + `
  is onboarding or deliberate identity selection. A real migration needs a
  registered actor through ` + "`--contributor`" + `, ` + "`TILLER_CONTRIBUTOR`" + `, or local identity;
  artifact-only refresh does not. Review and commit the reported project diff.
  Schema 9 renames the roster to ` + "`contributors:`" + ` and local identity to
  ` + "`contributor:`" + `, preserving the selected ID. The 8-to-9 migration preserves
  claims, active work, and history without a drain or ownership review.
  One checkout's maintenance never updates other branches, worktrees, or projects.
  Verified Homebrew installs delegate only the installed Tiller package to its
  owning brew, without broad dependent upgrades or cleanup; genuine dependencies
  and metadata refresh remain brew's boundary. Never overwrite its managed
  directories or bypass private-download authentication. Exact pins are unsupported
  there, and brew may change the machine before target compatibility is known.
  A successful binary update followed by a failed project phase is incomplete.
  Preserve completed phases; resolve the named failure before project-only retry.
  If recovery is required, preserve the transaction journal and recover first.
  Never downgrade schema or newer generated guidance, erase caches to bypass a
  refusal, or infer unchanged files from a lost child result.
  Restart existing serve processes and reload harness instructions when needed;
  disk refresh does not reload them. Older binaries do not acquire this upgrade
  logic mid-process: keep a compatible old executable before the first new install,
  then invoke the new binary's project-only maintenance explicitly.
- An instruction to execute includes ordinary implementation and state commits,
  pushes, PR creation or reuse, and submission for the authorized work. A reported
  merge or acceptance decision re-enters that work's bounded execute closeout;
  it does not authorize unrelated execution. A pure orient request still briefs
  and stops. Explicit developer no-publish or earlier-stop instructions control.
  Tiller does not push or create PRs: use ordinary Git and the project's host tools.
  Commit implementation and riding task refs, then run the applicable guarded
  ` + "`tiller check`" + `. Feature check runs the contract and the other feature gates;
  do not prescribe redundant default eval followed by check. Run the project's
  suite through ` + "`tiller check`" + ` rather than by hand and then again through check:
  a gate already passed on unchanged code is reused, which is normal and needs no
  ` + "`--rerun`" + ` to be sure. Use ` + "`--rerun`" + ` only for a suspected flake or environment
  change, and ` + "`--gate <name>`" + ` to iterate on one failing gate; a gate left out
  whose pass is not current is marked stale, and the node advances only on a
  full green set. Inspect all results
  and commit every actual evidence/state delta even on exit ` + "`1`" + `, never putting
  that work behind ` + "`&&`" + ` or a fail-fast shell exit. Pending manual acceptance
  must not skip machine checks or evidence commits. Repair required machine work;
  emit, grade, and record required ungraded or stale rubrics separately.
  Standalone eval does not apply check's basis/question/parent guards. Criterion
  attestation does not enforce every intended rubric prerequisite; prepare the
  grade before the human decision rather than claim those guards exist.
  Before an acceptance-only handoff, push and create/reuse the exact matching
  draft PR. Verify repository, integration base, recorded head, open/draft state,
  and published tip through host tools. Put its verified URL and tip evidence in
  ` + "`Notes from this run`" + `, distinguish it from formal submission, and stop for
  each named human decision: ` + "`tiller attest feature <feature> --criterion <id> --status pass`" + `
  if accepted, or ` + "`--status fail`" + ` if rejected; use ` + "`--gate <gate>`" + ` for a
  required attestation-backed gate. Never run attestation yourself. At feature
  or epic grain the human runs it on the recorded aggregate branch, in the
  worktree that holds it, and commits it there before submit; tiller refuses
  the attest anywhere else, so name that branch in the handoff.
  After the human records acceptance, commit its actual delta, complete required
  rechecks and evidence commits, push, reuse the same PR, make it ready if draft,
  verify it, and formally submit its actual number. Commit actual submission
  state and push again, then read back the record and verify the published tip.
  Skip empty state commits and redundant pushes on unchanged success.
  Recheck only when current work or failures require it. Check resets a required
  attestation-backed gate to pending, unlike settled criteria. Complete needed
  checks before that gate's human sign-off; if later work needs another check,
  obtain a fresh human gate attestation afterward, never restore it yourself.
  Green task gates and riding refs do not prove feature acceptance or submission;
  absence of a formal record does not prove no draft exists. Pure reads verify
  neither host state nor pushes. Requested changes reuse the established review.
  Never force-push, bypass a gate, invent a PR, or commit to a review branch after its final integration merge.
  Keep its local ref until ordinary sync observes the merge. Task-grain last-task
  closeout belongs on the integration branch: finish feature checks and evidence,
  prepare human acceptance if needed, then ordinary sync, actual state commit,
  push and readback. No new PR for state-only closeout and no coarse submit.
  Sync may already record done with violations; retain that history, never resume
  terminal work or claim done is blocked. The human alone asserts fast-forwards.
  If an aggregate finally merged before its Tiller review was recorded, a merge
  report alone does not authorize creating the missing historical binding.
  Collect evidence and obtain explicit authorization to recover that named review.
  Verify the host repository, PR number, merged state, integration base, head
  repository/branch, historical head OID, and result commit with existing host tools.
  Tiller verifies local Git evidence only; its PR association is caller-supplied.
  From configured integration with relevant source inputs committed, run
  ` + "`tiller submit feature|epic <node> --recover-merged --head <historical-oid> --merge-commit <result-oid> --reason <explanation> [--pr <actual-number>]`" + `.
  A remote requires the real positive PR number; offline recovery retains a review ref.
  Commit the actual recovery delta on integration, then run ordinary sync,
  inspect crossings/current acceptance/historical warnings, commit actual sync state,
  publish integration state when authorized and remote-backed, and read back status.
  Recovery neither runs checks nor approves acceptance or moves lifecycle state.
  A contained result has no observed merge witness; hand its fast-forward assertion
  to the human. Never reset refs, commit to the merged branch, or invent historical identity.
  Without a remote, prepare the committed local branch/tip review surface before
  acceptance, and record the permitted branch-and-commit review ref afterward.
  Omit host operations and pushes, not actual state commits or readback.
  A draft with acceptance outstanding, a refused formal submit, and an unpublished
  submission-state commit are distinct facts. Report actual local/published state.
  Never disguise host failure as offline work, invent rollback, or claim completion.
  Follow recovery first; stop for a genuine credential, host-tool, protection,
  conflict, or human-decision blocker that cannot be resolved safely.
- Follow the recovery protocol when a command refuses. Exit ` + "`1`" + ` is a
  business refusal to reason about, usually a red gate or an illegal transition:
  fix the work and try again, do not escalate. Exit ` + "`2`" + ` or ` + "`3`" + ` means
  correct the call or environment. The ranked list is guidance, not a command result.
  When state leaves a gate or merge outstanding, say ` + "`the ranked list suggests waiting`" + `;
  never say Tiller requires another node to wait unless a command exited ` + "`1`" + `.
  When you are unsure what is legal from here, ` + "`tiller status <noun> <node>`" + `
  names the legal next moves with nothing executed. That one-node oracle is also
  the surface that reports ` + "`active_basis`" + ` drift; the project summary does not.
  During ` + "`design-brief`" + ` or a ` + "`code-review`" + ` review, recover only through corrected reads.
  Missing sources or stale instructions are explicit limitations, not
  permission to repair, refresh instructions, or mutate the project.
- Name a node to the human by its key path with the id in parentheses, for
  example ` + "`form-engine/nrca-form-loading (F-79dh3q)`" + `. A task's key path
  carries all three segments and reads
  ` + "`form-engine/nrca-form-loading/schema-transcription (T-k3p9w6)`" + `.
  A question is never keyed: name it by its id and short title. Use the full
  statement when the question itself is the subject.
  The rule covers every surface you author, readout notes, hand-offs,
  summaries, and lists alike: an epic, feature, or task id never appears
  without its key path, because a bare id sends the developer off to look up
  what it points at. Bare ids belong inside commands, where they are always
  valid. A commit subject reads the same way: the bare task id satisfies
  traceability there. A pull request title carries the reviewed node's full key
  path with its id. Its body names the work riding that review, the task's
  parent feature at task grain, or the tasks and features at a coarser grain.
- Work inside this repository. Tiller's guidance is complete in the project:
  this file, the skills ` + "`tiller guide`" + ` serves, and the command output.
  Never go hunting outside the project directory for Tiller documentation or
  schema shapes. Leave the project only where the developer points you.
- Report through the readout. At a seam, ` + "`tiller readout <seam>`" + ` prints a
  block; at a human gate the command prints one itself; either way,
  reproduce it verbatim as the last thing in your reply, with your own
  account under ` + "`Notes from this run`" + ` ahead of it. The block closes the
  reply, nothing follows it, and it ends on its ` + "`**Next:**`" + ` ranked list,
  which you reproduce as written. It renders the same whoever is driving, so
  the developer reads one format rather than yours. ` + "`submit`" + ` and a passing
  ` + "`check`" + ` print the same list under ` + "`**Up next:**`" + ` when they finish; that is
  for you mid-run, and the closing readout's list is the one you reproduce. The block has two sizes.
  A seam exit gets the full ` + "`tiller readout <seam>`" + ` block. A correction or
  other small action inside a gate, a rename, a remove, an added node, a
  link applied while the developer refines a draft, gets
  ` + "`tiller readout <seam> --since <sha>`" + `, where ` + "`<sha>`" + ` is the commit the
  last block you reported named. It says only what changed since then, and
  it is reproduced the same way: verbatim, last, with your notes ahead of it.
  ` + "`design-brief`" + ` is a read-only export, not a lifecycle seam: its
  Markdown deliverable ends the reply without a terminal readout or next-work
  action. Do not invent a ` + "`tiller readout design-brief`" + ` command.
  ` + "`code-review`" + ` is not a lifecycle seam either: its reply ends with its
  summary table and review path. Do not invent a ` + "`tiller readout code-review`" + ` command.

## How to work

Run ` + "`tiller status`" + ` to see where things stand, then ` + "`tiller guide`" + ` to load the
skill for the moment. With no argument, ` + "`tiller guide`" + ` prints the ranked list
and names the skill for its top agent move; with a skill name it prints that
skill in full. ` + "`tiller next`" + ` prints the list alone. When you are choosing the
next seam, relay the list to the developer and end the reply; their next message
redirects you or lets the top agent move start. Work already moving inside a seam keeps moving, including execute's draft
preparation, submission-state push, and bounded related merge closeout. Do not
ask for another go after checks turn green or before ordinary state publication.
The briefing pause is the developer's moment to correct your picture of the
project and add context the design docs do not carry; conventions worth keeping
belong in ` + "`AGENTS.md`" + `.

Use ` + "`tiller search <keyword>`" + ` to find prose and
` + "`tiller query --from <address> --follow <directions> --json`" + ` to discover
recorded relationships. ` + "`query --help`" + ` lists every forward/inverse name.
For citations use ` + "`--follow mentioned_by`" + ` from a work ID; for work claiming
a file use ` + "`--from path:<repo-relative-path> --follow covered_by`" + `.
Repeat ` + "`--from`" + ` for multiple starts. Follow names are a union at every hop,
not ordered stages; depth defaults to one. ` + "`--where kind=question`" + ` and
other exact kind/status/owner filters select after traversal, not along the way.
Sources, bridge vertices, and repeated citation locations remain inspectable.
These are recorded claims, not verified code coverage or effective question
holds. Use status/readout for inherited blockers, assumptions, and legal moves.
Read diagnostics and ` + "`complete`" + ` before treating a result as exhaustive:
missing eval/history facets or ambiguous historical identity are incomplete;
unsafe corpus or unstable-generation reads fail. Unknown references stay
unresolved. A current file is not the historical contents of a pinned revision.
Each query rebuilds from local files without identity, writes, or a persistent
cache; it never runs bindings, refreshes instructions, or authorizes a mutation.
To adopt changed guidance with the intended binary, explicitly maintain this
checkout with ` + "`tiller upgrade --project-only`" + ` and review the diff.

An explicit request for a UI/UX design brief or prototype handoff for an epic,
feature, or task goes directly to ` + "`tiller guide design-brief`" + `, outside the
ranked list, even when the list names a human gate or other work. The request
already authorizes this read-only export; do not claim, start, specify, or approve
anything. Resolve keys, IDs, or unambiguous conversational targets using reads.
A normal next-work request, draft feature, or completed step does not trigger it.
For this export, prefix every Tiller read from the first invocation with
` + "`env GIT_OPTIONAL_LOCKS=0`" + `, including status and guide, because reads
may invoke Git internally. Use ` + "`git --no-optional-locks`" + ` for direct Git
inspection. Do not let read-only gathering refresh the index.

An explicit request for a code review of one epic, feature, task, bug, chore, or
tweak, or a request naming rows of an existing review, goes directly to
` + "`tiller guide code-review`" + `, outside the ranked list. It authorizes only
the read-only review or the named disposition; never approve, claim, or start
anything. A normal next-work request or completed step does not trigger it.

Onboarding starts at the scaffold, and it is three commands, not two. A bare
repo becomes a Tiller project with
` + "`tiller init --key <key> --kind <archetype> --name \"<project name>\"`" + ` (add
` + "`--design <dir>`" + ` to import a design folder in the same move). Registering a
person and selecting your identity are two separate, deliberate acts, so the
next two commands are not one: a human runs
` + "`tiller add contributor <you> --name \"<full name>\"`" + ` to put you on the roster,
and an agent runs it only on an explicit human instruction naming both the id
and the name, and only then
does ` + "`tiller join --as <you>`" + ` select that registered id as who is working
this checkout. join can register no one: an id absent from the roster,
whichever of ` + "`--as`" + `, ` + "`TILLER_CONTRIBUTOR`" + `, or the checkout's own
` + "`.tiller/identity.yml`" + ` supplied it, is refused rather than added. The
roster is not add-only: ` + "`tiller remove contributor <id>`" + ` retires someone later, the
same human-gated act, refusing while they still hold live work. Do not
skip either step: without them there is no acting contributor here, so the first
` + "`tiller add`" + ` exits ` + "`3`" + `, ` + "`check readiness`" + ` reports the empty roster as a gap,
and the ranked list below cannot rank your own work. Then
` + "`tiller check readiness`" + ` names any missing inputs or manifest gaps.
For greenfield, prepare design inputs and name ` + "`decompose`" + ` to turn them into
a draft tree. Brownfield and legacy-migration first use ` + "`derive-baseline`" + `
when no as-is doc is confirmed, even if docs were imported or work already exists.
Name the applicable seam and stop for the developer's go.

The baseline seam reads existing docs before code and writes module-grain as-is
descriptions: what the surface is, data it owns, behavior to preserve, edges, and
described paths. Brownfield describes surface code; legacy-migration understands
reference code without editing it, including explicitly declared external roots.
Choose move versus pointer once, never silently keep duplicate live specs.
External imports preserve their external originals. Before retiring a local
source, commit any untracked or changed input bytes at exact paths, observe HEAD,
and record recoverable ` + "`link design --from`" + ` provenance for every destination.
Preserve unrelated changes and work on the configured integration branch.
` + "`project.adopted`" + ` is this project's historical context, not an external
baseline or cleanliness claim; acknowledge missing or unusable history and
reconcile current working-tree code before confirmation.

The optional machine-owned ` + "`design/baseline.yml`" + ` ledger records each doc's
` + "`describes`" + ` paths, source blob refs, and human confirmation. Link through
` + "`tiller link design <doc> --describes <path> --from <source-path>`" + `, using
only applicable flags, and repair relations with ` + "`tiller unlink design`" + `.
Commit the scoped move/split and state, then reproduce the complete
` + "`tiller readout derive-baseline`" + ` coverage block verbatim and last, with
observed run notes before it. Source links are not semantic conservation proof.
The human confirms named docs with ` + "`tiller approve design <doc>...`" + `; never
self-approve without explicit instruction naming those docs. Existing pending
docs call for inspection, reconciliation, or a hand-back to review, not another
derivation. One confirmed doc releases the startup gate; remaining unconfirmed
docs are not a global blocker. Contributors, reference declarations, modes, missing
design prose, and other readiness gaps remain independent. An empty work-node
queue does not erase pending baseline review. No automatic baseline drift or
touched-doc sync behavior is implied.

Bugs capture defects: observed behavior violates expected behavior. Chores are
housekeeping without code side effects; this is guidance, not a restriction or
validation rule. Tweaks are minor adjustments that may change code. Tasks carry
substantive planned work. Capture a forward adjustment with ` + "`tiller add tweak <key>`" + `
and its ` + "`.tiller/tweaks/<key>/tweak.md`" + ` contract, then stop for human triage.
Done work stays done: never reopen it to accommodate an adjustment. A tweak
uses the same ownership, gates, lifecycle, and review as a chore.

Every new task is unclaimed, regardless of who created or approved it. Claim
only the execution unit: ` + "`tiller claim task <key>`" + ` (or ` + "`bug`" + ` / ` + "`chore`" + ` / ` + "`tweak`" + `).
Before start, ` + "`release`" + ` returns your reservation to the pool and
` + "`reassign task|bug|chore|tweak <key> --to <owner> --reason <text>`" + ` records a
human-directed handoff. Taking another reservation with ` + "`claim --from <id>`" + `
requires an explicit human handoff naming the work and actual holder; knowing
an owner flag is not permission. The first successful start permanently fixes
that unit's owner. Resume, checks, review, cancellation, and completion never
reset it; all terminal ownership is immutable, even cancelled-before-start work.
Start, resume, and execution submission require the unit's own claim.
Features and epics have no owner or claim, and claim changes never cascade.
The judgment verbs, ` + "`approve`" + `, ` + "`attest`" + `, ` + "`answer`" + `, ` + "`eval`" + `, and
` + "`check`" + `, bind to no claim: registered developers may review others' work,
with the same actor attribution and human-gate rules. This is recorded-identity
workflow discipline, not authentication.

Contributors on a feature or epic are the distinct owners of descendant tasks
that have started, including done and cancelled tasks. Ready reservations,
review/assembly actors, ancestor creators, and attached shelf work do not count.
Current work means unfinished task/bug/chore/tweak assignments, including proposed,
blocked, checked, and in-review work; a historical contribution assigns nothing.
Use ` + "`tiller ls features --contributor <id>`" + ` for contribution history and
` + "`tiller ls tasks --owner <id>`" + ` for scalar assignments.

For already-started unfinished work, only an explicit human instruction naming
the work and recipient authorizes
` + "`tiller transfer task|bug|chore|tweak <ref> --to <registered-contributor> --reason <text>`" + `.
Transfer cancels the original with its owner, prose, branch, review, and evidence
intact, and creates a same-noun, approved ready replacement with a new ID/key,
recipient claim, fresh accepted basis, and reciprocal replacement links.
It is not started and inherits no branch, review, checks, attestations, or evidence.
Unchanged approved scope is required; specification changes use existing human
review/amendment paths, not a transfer flag. Same-owner or terminal transfer refuses.
Unstarted dependents follow the replacement; a started unfinished dependent
refuses the whole transfer. Questions, pauses, shelf attachments, and carrier
relationships must remain operative; no assumption or automatic unblocking is granted.
Use the returned replacement reference for its recipient's start, never guess it.
At task/bug/chore/tweak PR grain close the old PR without merging and open a new one.
At feature/epic grain keep the unmerged aggregate PR, assemble replacement work,
and rerun affected checks and acceptance. Old commits and host approvals do not
disappear automatically. Tiller changes records, not Git or host state: report
unfinished external follow-through separately. If already merged, sync first and
use forward work, not transfer.
For recorded review history, ` + "`transfer_review_unobservable`" + ` refuses when
neither the local branch nor a usable submitted head establishes merge state.
Restore/fetch review history, sync, and inspect; missing history is not unmerged.
Observed merges or squash matches refuse as ` + "`transfer_review_merged`" + `.
Use the exact returned ` + "`start_command`" + ` and ` + "`follow_through`" + `;
external failure never rolls back the committed source transaction.

The shelves use the same explicit ownership boundary. The human chooses queue
work, then you claim the top eligible node in that shelf's soft order with
` + "`tiller claim bug <key>`" + ` or ` + "`tiller claim chore <key>`" + `. An unclaimed shelf
node never enters the ranked list. A live ` + "`carried`" + ` node waits on its carrier.

Review does not have to land once per task. ` + "`review_grain`" + ` records where
it lands instead: ` + "`task`" + ` by default, ` + "`feature`" + `, or ` + "`epic`" + `. It lives in the
project's ` + "`tiller.yml`" + `, an epic's ` + "`epic.yml`" + `, and a feature's ` + "`feature.yml`" + `,
absence inheriting downward and the most specific explicit setting winning.
A feature cannot itself say ` + "`epic`" + `, since riding its epic's PR is inheritance,
not an override. Set it only on an explicit human instruction naming the node
and grain, through ` + "`tiller configure project`" + `, ` + "`configure epic <key>`" + `,
` + "`configure feature <key>`" + `, or the matching ` + "`approve --review-grain`" + ` gate.
State what you will record and report what you recorded.

Invalid stored grain values refuse the tree rather than defaulting. If
` + "`configure project`" + ` refuses an unsupported scalar or flow-mapping
` + "`execution`" + ` shape, rewrite that human-owned manifest setting as block-form
YAML and rerun the command; never edit machine-owned node state.

Finish and sync submitted work before changing a review grain that would affect it.
An affected feature review or a task in ` + "`in_progress`" + `, ` + "`check`" + `, or
` + "`in_review`" + ` locks the change. Entering or leaving epic grain also locks once
an affected feature records its branch. Never hand-edit state around a refusal.
Different developers may own tasks on the same feature or epic review.
Any registered developer may assemble, check, and submit an eligible aggregate;
there is no coordinator claim. Coordinate one aggregate updater at a time through
the team and Git, not a persistent Tiller lock. An overridden feature finishes its
own review and reaches ` + "`done`" + ` before the epic review opens.
Coarse submit still enforces active basis, questions, reviewed assumptions,
required gates, task readiness, review identity, and retired ancestors.
Record a feature-only assumption with
` + "`tiller approve feature <ref> --assume-question <Q-id> --reason <text>`" + `
on explicit human instruction; this does not move lifecycle or grant the task an
assumption. Task start must satisfy its own operative question holds.

### Correction routing

Correction feedback follows classes, not blanket ceremony. An explicit human
correction that maps to a class authorizes the mapped action except ` + "`cancel`" + `.
For any cancellation, hand the exact ` + "`cancel`" + ` command to the human rather
than run it. For other mapped actions, do not ask which mutation path to use.
Change machine state only through ` + "`tiller`" + ` and edit only human-owned Markdown.

Before negotiating a structural change, load the installed ` + "`tiller guide amend`" + `
and read ` + "`tiller rename --help`" + ` and ` + "`tiller amend --help`" + `. Inspect affected
features, all descendant tasks, and source/destination epics through
` + "`tiller show`" + `, node ` + "`tiller status`" + `, and ` + "`tiller query`" + `. Check current
states and prior starts, branches/reviews, accepted bases, dependencies,
effective question holds, and inherited review settings. Current ready state
does not erase execution history. Use read-only history inspection as needed,
never a trial mutation or an invented dry-run flag. Explain the recommended
product outcome, installed capability, and smallest safe alternative before
asking product questions. Do not ask for facts the repository can answer.
If installed help lacks a move operation, disclose the limitation rather than
promise accepted relocation or upgrade without direction.

- **Pending baseline proposal:** Re-enter ` + "`derive-baseline`" + `, correct the
  named docs and relationships in place, reconcile current code, commit only
  that correction, and reproduce its ` + "`--since <sha>`" + ` readout. Return to the
  same named-doc confirmation gate; do not regenerate duplicates or open a
  work-node amendment for that correction.

- **Unapproved proposal or pre-work prose plan:** Re-enter ` + "`decompose`" + ` for a
  draft tree or ` + "`specify`" + ` for a feature plan, apply the correction, commit
  it, rerun that seam's readout with ` + "`--since <sha>`" + `, naming the commit the
  last block reported, and return to the same approval gate. Display titles
  use ` + "`rename --title`" + ` at any state.
- **Feature address correction:** Direct ` + "`rename feature <ref> --key <epic>/<key>`" + `
  requires a draft, task-free feature; changing parents additionally requires
  both source and destination epics to be draft.
  Epic key rename requires a draft epic with only draft, task-free features;
  task key rename requires a proposed task staying under its current feature.
  A proposed bug, chore, or tweak takes one local key segment. These commands never
  rename branches or rewrite review history.
- **Reviewed unstarted feature move:** Outside that direct feature window,
  stage ` + "`tiller amend <A-id> --move-feature <feature-ref> --to <epic>/<key> --reason \"<why>\"`" + `
  and inspect ` + "`tiller show amendment <A-id>`" + `. Draft, specced, and ready features
  with proposed/ready tasks may move only when the feature and descendants
  have never started or accumulated execution/review history. Claims survive.
  Both parents must be draft or ready; the destination may be staged earlier
  in the same amendment. Done siblings alone do not prevent the move.
  An affected open coarse review or changed effective inherited prerequisite
  or question hold refuses. Effective review grain is preserved, materializing
  an override when needed. Inspect surfaced authored path citations and bindings
  separately; do not rewrite historical revision paths or accepted prose.
  Commit the proposal and stop for human ` + "`tiller approve amendment <A-id>`" + `.
  Approval rechecks stale subtree/parent settings and collisions atomically.
  IDs, status, owners, content, and original acceptance provenance survive;
  a moved specced plan or draft destination still needs its own ordinary gate.
  Cross-parent moves append to destination order in staged order; same-parent
  key changes keep their sibling position. Do not combine a move with revise,
  cancellation, or supersession of that subtree. Started descendants require
  a precise refusal, not replacement identities, state edits, or weakened edges.
- **Accepted, lightweight correction:** Approval does not freeze display titles,
  epic prose, non-terminal feature or task prose at the same scope, an additive
  task with its own approval, or soft order. Follow the current skill's mapped
  direct command, clear pinned-prose ` + "`active_basis`" + ` through a human basis
  refresh, or use ` + "`return`" + ` and reapproval when the ready plan's approval
  should replay.
- **Genuine structural reshape:** Use ` + "`amend`" + ` for genuinely abandoned or
  replaced scope: cancel unfinished work with a reason, or supersede done work
  with forward replacement. Do not cancel and redecompose unchanged work merely
  to relocate it, including a specced but unapproved plan. Amendment has no link,
  unlink, or order operation; revise only re-pins prose. An in-place hard-edge
  change on unchanged started work remains unsupported. Do not manufacture
  replacement to evade that lock. Fresh unstarted work can receive legal
  prerequisites, even under an active parent. A done feature's eval bindings,
  never its criteria statements, are corrected through ` + "`amend --correct-eval`" + `.
  Preserve terminal feature/task prose and completed descendants. Same-outcome
  additions may continue under done parents; ordinary sync reopens rollups,
  not the completed tasks. Cancelled or archived ancestors are not continuation
  targets. Distinct outcomes belong in new increments. After materialization,
  proposed placement needs explicit ` + "`tiller order`" + `; hard links remain
  subject to the dependent node's state and branch checks.

Bounded epic features may interleave. A projected epic ordering cycle is a
nonblocking advisory about grouped rendering; inspect its features and recorded
edges. Actual prerequisite cycles, including explicit epic dependencies and mixed
feature/epic completion deadlocks, still block readiness and execution. Never
delete a valid prerequisite merely to make the epic map sort cleanly. Workflow
administration versus oversight is a planning distinction, not a universal
permanent sink epic.

### Ranked next

` + "`tiller next`" + ` prints the legal next moves for this checkout, ranked, at most five,
one row per node. Bare ` + "`tiller guide`" + ` and the readout's ` + "`**Next:**`" + ` print the
same list, and ` + "`submit`" + ` and a passing ` + "`check`" + ` append it as ` + "`**Up next:**`" + `, so
the CLI never disagrees with itself. Each row leads with its actor, ` + "`You`" + ` for a
human decision or ` + "`Agent`" + ` for work, then the action, the reason, and the exact
command, or the branch to continue on. Tiller ranks by a fixed rule and never calls a model. ` + "`tiller next --all`" + `
lists every legal move, ` + "`--json`" + ` prints the payload, and ` + "`--contributor`" + ` ranks
for another contributor. With no identity, ` + "`tiller next`" + ` and bare ` + "`tiller guide`" + `
head the list as not personalized (the readout's ` + "`**Next:**`" + ` has no heading), the
personal tiers are skipped, and plan order ranks the rest, though a question
overdue or due within two days still ranks first; ` + "`tiller join`" + ` fixes that. When nothing is legal it
prints ` + "`Caught up: nothing is legal to do right now.`" + `

The rule has five tiers, in this order, with the kind of move and then plan order breaking ties inside each.
Unblock nearby comes first: human decisions that hold work in your current feature,
plus any question overdue or due within two days wherever it sits. Continue is your
own unfinished work, and your top Continue move is always kept in the five. Nearby
is the other moves in the current feature, then in its epic. Recently touched is the
features and epics you acted on within the last fourteen days, most recent day first.
Everything else follows in plan order. Your current feature is the feature touched
by your most recent deliberate act that names one: a claim, start, submit, check,
spec, approval, answer, attest, link, reassign, or release. Acts on an epic
alone and amendment steps never set it, and sync, migrate, join, and other upkeep
never count. Adding work only touches its parent for recency, an added task its
feature and an added feature its epic, so a decomposition never sets it.

Relay the list as written. If any human move ranks above the top agent move,
name it first. Then start the top agent move unless the developer redirects.
Start it in your next turn, once the developer's reply does not redirect.
You may say you would take a different item first and why; never rewrite the list.
A ranked move is a suggestion, not authorization.
An explicit human instruction naming nodes overrides the list's order for exactly
those nodes.

The list holds only legal moves, so every row can be acted on as printed. A task
held by an operative open question is not a row; the holding question is, as a
human answer. Work in flight on another contributor's branch, work blocked by an
unmet dependency, and an unclaimed shelf node are not rows. The standing queue
line is always present: ` + "`queue: N bugs, M chores, K tweaks ready`" + `. It reports
ready, non-carried shelf work, which a human chooses before any claim. An
unclaimed task matches no personal assignment until its claim succeeds.

The moves, and the seam each leads to:

- Brownfield or legacy-migration with no confirmed baseline doc: the seam is
  ` + "`derive-baseline`" + `. Read pending docs first; a complete proposal awaits human
  named-doc confirmation, not automatic regeneration. One confirmed doc releases
  this move.
- A task you hold in ` + "`check`" + `, ` + "`ready`" + `, or ` + "`in_progress`" + ` with no operative open
  question against it or an ancestor: the seam is ` + "`execute`" + `, and the list files
  it under Continue. Re-enter ` + "`check`" + ` on the recorded branch without ` + "`start`" + ` or
  ` + "`resume`" + ` and finish outstanding checks and publication/submission. Owned
  checked shelf work is the same move on its shelf, and a live ` + "`carried`" + ` node
  waits on its carrier. ` + "`carrier_cancelled`" + ` means owned work has returned to its
  shelf, and remains a human needs-attention row with ` + "`resolve --clear`" + ` or
  ` + "`resolve --by`" + `.
- Shared closeout, once per reviewed unit, never per contributor or last task owner.
  A coarse finish line arrives when every live riding task is ` + "`in_review`" + ` or
  ` + "`done`" + `; at epic grain every independent override is already done. A task-grain
  feature whose live tasks are all done needs feature acceptance and ordinary sync
  on integration. A ranked closeout move can offer related closeout but does not
  authorize it. Inspect current required gates, recorded evidence, and the
  authored feature contract. Missing/unconfigured, failed, or stale machine work
  remains agent checks/repair; ungraded or stale rubrics need grading/recording,
  not repeated default eval. Only complete machine/rubric work can leave human
  acceptance alone. Optional criteria do not block; unavailable or invalid
  contracts are uncertainty to repair, not a pass. At epic grain name both the
  reviewed epic and the feature contract needing work. Run ` + "`tiller check feature`" + `,
  commit actual evidence even on exit ` + "`1`" + `, and follow execute's draft preparation
  before human acceptance, then its rechecks, same-PR ready/formal submit, and
  state publication/readback. Submit the epic, not its riding features. No epic
  eval/check exists. Terminal done work retains acceptance warnings and historical
  violations without starving live work. Workflow advice does not replace
  lifecycle legal-next moves. Ordering is not full mutation-guard parity: respect
  basis, questions, dependency, ownership, and retired-node diagnostics; do not
  advertise a known-refusing operation. Never tell the developer the frontier
  shows only their own work.
- A ` + "`proposed`" + ` amendment awaiting approval: a human move, not a seam. Name
  ` + "`tiller approve amendment`" + ` as the developer's move and stop. Several amendments
  can be open at once, each its own row.
- An unclaimed eligible ` + "`ready`" + ` task under an approved ready or active feature and
  an epic past decomposition, with no unmet recorded dependency or operative
  question hold. The seam is ` + "`execute`" + `, and the row names
  ` + "`tiller claim task <key>`" + ` for that individual unit. Approval itself neither
  claims nor starts work, and a ranked claim is not authorization.
- A feature in ` + "`specced`" + ` awaiting its plan gate with no open question holding its
  plan: a human move, not a seam. Name ` + "`tiller approve feature`" + ` as the developer's
  move and stop. A plan an open question holds (on the feature, its epic, or any
  live task under it) is not that move; the question is, as a human answer. Name
  ` + "`tiller answer question <Q-id> --answer \"...\"`" + `, after which specify folds the
  answer into the plan before the approve.
  An open question leaves hold in one of two ways: an answer, which
  records a decision, or ` + "`tiller cancel question <Q-id> --reason \"...\"`" + `, which
  records that none is needed.
  Cancel and ` + "`tiller unlink question <Q-id> --blocks <node>`" + ` are human moves: run
  them only on the human's instruction, otherwise name the exact command in Notes.
  You may ` + "`tiller link question <Q-id> --blocks <node>`" + ` to widen a question that
  reaches further than recorded, and say so in Notes.
- An epic still in ` + "`draft`" + ` with live features awaiting its decomposition gate: a
  human move, not a seam. Name ` + "`tiller approve epic <key>`" + ` and stop. Several epics
  list in recorded soft order, the order ` + "`tiller queue --kind approve_epic`" + ` shows.
  Explicitly named drafts remain legal to specify; ordering alone is not a
  mutation guard.
- The frontier: a live feature still in ` + "`draft`" + `, whose recorded ` + "`depends_on`" + `
  targets are all settled (done, or cancelled), and whose own epic's recorded
  dependencies are settled the same way; an epic-grain edge gates the epic's
  features exactly as a feature-grain edge gates the feature. The seam is
  ` + "`specify`" + `, and the feature is the project's, not any one owner's. A draft
  still waiting on unfinished work is not a row. Among the eligible, the recorded
  soft order chooses first, the owning epic's place in it and then the feature's
  own. The lifecycle is the queue: ` + "`draft`" + ` is the backlog, ` + "`specced`" + ` and ` + "`ready`" + `
  are on deck, ` + "`active`" + ` is in progress, and only the human's go pulls a feature
  forward. Specifying one feature just in time remains the default. An explicit
  instruction naming several draft features, or every draft feature in one named
  epic, creates the one exception: specify exactly that batch in rendered soft
  order, commit each feature separately, run one final readout, and never approve.
  True prerequisites live in ` + "`depends_on`" + `; preferred sibling sequence lives in
  soft order. Task assignments identify execution responsibility. Soft order
  arranges the render and the list among peers the graph leaves unordered; it
  never gates. Independent chains permit parallel work without a separate
  planning layer.
- A task, bug, chore, or tweak in ` + "`in_review`" + ` is a human merge wait rather than a
  seam. A live carried shelf node still waits on its carrier instead. Surface the
  wait and stop once submission state is published when remote-backed. Requested
  changes on a task, bug, chore, or tweak re-enter ` + "`execute`" + ` through that noun's
  ` + "`resume <noun> <node> --reason \"...\"`" + ` and reuse its open PR. A retained PR on
  resumed or checked work does not mean the changes were resubmitted. Under a
  coarse review grain a riding task's ref proves neither feature acceptance nor
  formal review; the feature contract and required gates derive the remaining
  work or human acceptance. The list is pure and may name that decision while
  publication is unknown; authorized execute prepares the draft/offline surface
  before handing it over. Once formal review is recorded, name only its recorded
  identity, never infer its current host state.
- Drafts exist but every one waits on unfinished dependencies: a draft behind an
  unmet dependency is no move, so it is never a row. The tree is not empty for
  that: dependency diagnostics (Home, ` + "`tiller check readiness`" + `) and the
  caught-up hint explain the held work. Otherwise, with no epics recorded,
  decompose is due only when a to-be design doc exists: a file under
  ` + "`.tiller/design/`" + `, dotfiles excepted, that ` + "`baseline.yml`" + ` neither registers
  as an as-is doc nor cites as a source. Then the seam is ` + "`decompose`" + `. With none, no work is planned yet: a human wait, not a seam. Name
  the open-question count and the ways work enters (intake a document that makes
  a case for change, capture a bug, chore, or tweak, or add a to-be design), and
  stop. Never turn as-is docs or old specs into epics nobody asked for. On a tree
  with no epics, owned shelf work, a proposed amendment, and a shelf merge wait
  are still real work or a real wait.
- Nothing legal remains: this is the true caught-up terminal, and the list says
  so. A diagnostic may still count unfinished unclaimed tasks that are proposed,
  held, or otherwise ineligible and explain the first in dependency order. This
  is not the eligible ready-task pool, makes no ownership claim, and is omitted
  when no unfinished unclaimed task remains.

Inside a tier, moves sort by kind in a fixed order first: the baseline, your own
continuing work, amendment review, closeout, claiming, resolving a blocker, plan
approval, epic approval, specify, merges, answers, attests, triage, then decompose. Within a kind, plan order
decides: epic order, feature order within an epic, then task order within a feature.
Tier 1 sorts by due date first, overdue first. A task's local
ordinal does not compare across unrelated features. Completed prerequisite depth
does not outrank a live preference, and historical tree order and dependency edges
are unchanged.

` + "`tiller guide`" + ` serves eleven skills:

- ` + "`orient`" + `: you need the next move. Read state, relay the ranked list above to
  the developer, and end the reply.
- ` + "`derive-baseline`" + `: an existing-code project has no confirmed as-is doc.
  Derive or translate module-grain descriptions, record recoverable provenance,
  commit, and stop for human named-doc confirmation. Review existing pending
  docs in place rather than regenerate them.
- ` + "`decompose`" + `: to-be inputs exist but the tree does not reflect them. Lay a
  draft epic and feature tree for the human to approve.
- ` + "`specify`" + `: a feature is in draft. Write its specification, its acceptance
  contract, and its tasks in one pass, then ` + "`tiller spec feature`" + `.
- ` + "`execute`" + `: you hold a ready, in-progress, or checked task or shelf node,
  reviewed work has requested changes, acceptance resumes, a coarse review reaches
  its finish line, or the developer reports a related merge. Prepare verified
  draft/offline acceptance review, complete formal submission/state publication
  after the human decision, or finish integration-branch check/sync closeout.
  Stop at the actual human decision or real blocker, not an ordinary state push.
- ` + "`capture`" + `: an investigation or developer description identifies a
  concrete bug, chore, or tweak. Record its complete shelf proposal and stop before triage.
- ` + "`mend-drift`" + `: ` + "`tiller drift`" + ` reports a candidate. Read it, draft
  the smallest honest fix, and stop for the human to approve.
- ` + "`amend`" + `: a change reshapes the plan rather than extends it. Sort the
  corrections from the casualties, stage the reshape as one amendment, and stop
  for the human's ` + "`approve amendment`" + `.
- ` + "`intake`" + `: the developer hands you a document written outside the tree
  that makes a case for change, or ` + "`tiller intake`" + ` already landed one and
  opened an amendment with nothing staged. Read it whole, stage the mapping as
  one sourced amendment, and stop for the human's ` + "`approve amendment`" + `.
- ` + "`design-brief`" + `: the developer explicitly requests a UI/UX design brief
  or prototype handoff for an epic, feature, or task. Gather bounded requirements
  and export one self-contained Markdown snapshot for a non-production prototype.
  Return it and stop, with no lifecycle mutation, readout, upload, or prototype build.
- ` + "`code-review`" + `: the developer explicitly asks for a code review of one
  node, or names rows of an existing review. Review its net change read-only into
  one file under ` + "`.tiller/reviews/`" + ` and reply with a summary table; on a
  later disposition, log the named rows through capture and intake shapes and
  commit once, leaving every approval to the human.

After a successful feature done crossing, sync may name as-is docs touched;
that notice only asks you to run ` + "`tiller drift`" + ` and never changes the ledger.
For ` + "`stale_design`" + `, read the doc, the bounded confirmation-to-crossing diff,
and each named feature's spec. For opt-in ` + "`shifted_design`" + `, inspect current
working-tree code, including staged, unstaged, and untracked changes, not only
HEAD. A confirmation may cover dirty bytes never committed, and external roots
may have no recoverable history; state those limits. Follow pointer docs to
their live sources rather than create duplicate specs. Missing described paths
need restoration or CLI link/unlink repair; unsafe or unreadable paths are
uncovered, not clean. Propose the smallest prose correction, show the evidence,
and stop for review. Only explicit human instruction naming design-relative docs
authorizes ` + "`tiller approve design <doc>...`" + `; neither sync, drift, nor a prose
edit grants confirmation. Never approve automatically or hand-edit the ledger.

` + "`tiller drift`" + ` is a check the ranked list does not run on its own: run it
yourself, and when it reports a candidate, the seam is ` + "`mend-drift`" + `. One more
seam sits outside that list for a reason of its own: no field in the tree
says a reshape is due. When the change in front of you reshapes the plan rather
than extends it, because the developer says so or one upstream change cascades
across a dozen nodes, the seam is ` + "`amend`" + `. Name what you think it is and let the
developer confirm before anything is staged: staging records proposals and
executes nothing, and the human's ` + "`approve amendment`" + ` is the one act that
applies the batch. Capture and intake sit outside the list too: both are
operator-initiated, not ranked moves computed from recorded state. Use ` + "`capture`" + `
when an investigation or developer description identifies a concrete shelf
item. Use intake when the developer hands over external feedback, or
` + "`tiller intake`" + ` already has.

Process is Tiller's; craft is the repo's own. How you write the code, the testing
discipline, and the architecture live in this project's ` + "`AGENTS.md`" + ` and
` + "`CLAUDE.md`" + `, not in a Tiller skill. The skills are opinionated about the shape
and gating of the work and silent on the content of the implementation.

## Where things live

Everything Tiller owns is under ` + "`.tiller/`" + ` at the repo root:

- ` + "`tiller.yml`" + ` is the manifest: the project archetype, the owners, the inputs,
  and the gates.
- ` + "`design/`" + ` holds the durable design docs the tree is decomposed from.
- ` + "`design/baseline.yml`" + ` is the optional machine-owned as-is ledger beside
  those human-owned docs: described code, source blob refs, and confirmations.
- ` + "`epics/<epic-key>/`" + ` holds each epic, its ` + "`features/<feature-key>/`" + ` folders,
  and within a feature its ` + "`feature.md`" + ` spec, ` + "`feature.eval.md`" + ` acceptance
  contract, ` + "`feature.yml`" + ` state, and ` + "`tasks/<task-key>/`" + ` folders, each
  holding a ` + "`task.md`" + ` implementation spec beside its ` + "`task.yml`" + ` state.
- ` + "`bugs/<bug-key>/`" + ` holds a bug's human-owned ` + "`bug.md`" + ` contract beside
  machine-owned ` + "`bug.yml`" + ` state; ` + "`bugs/order.yml`" + ` records shelf order.
- ` + "`chores/<chore-key>/`" + ` holds a chore's human-owned ` + "`chore.md`" + ` contract
  beside machine-owned ` + "`chore.yml`" + ` state; ` + "`chores/order.yml`" + ` records shelf order.
- ` + "`tweaks/<tweak-key>/`" + ` holds a tweak's human-owned ` + "`tweak.md`" + ` contract
  beside machine-owned ` + "`tweak.yml`" + ` state; ` + "`tweaks/order.yml`" + ` records shelf order.
- ` + "`plans/`" + `, ` + "`decisions/`" + `, and ` + "`events/`" + ` hold execution plans, decision records
  and open questions, and the append-only event stream.

For anything live, who holds what and what is where, read ` + "`tiller status`" + ` rather
than this file.
`
