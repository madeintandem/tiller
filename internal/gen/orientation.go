// Package gen builds deterministic orientation and live project projections.
// Everything here is a pure function of its inputs with no cobra and no stdout,
// so the same inputs always yield the same output.
package gen

// This file generates .tiller/orientation.md from distribution.md's five-part
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
// first and names the Continuity note. Generation 49 executes
// started work in its own Git worktree by default, defers to the developer's
// own checkout instructions, and assumes other agents share the repository.
// Generation 51 adds the shared verified-URL external reference writing rule;
// generation 50 added reviewed done-feature eval binding correction.
// Generation 52 scopes the default worktree to the unit of work one review
// covers, so a coarse feature or epic shares one worktree, and parallel tasks
// within it get their own. Generation 53 has the human run a coarse review's
// attest on its recorded aggregate branch, committed there before submit, and
// records the grain a task started under (started grain).
// Generation 54 reuses a gate already passed on unchanged code and says when to rerun.
// Generation 55 ranked next replaces the route walk: the agent relays a ranked
// list of every legal move and starts the top agent move.
// Generation 56 refuses a plan gate while an open question holds the plan:
// specify settles its questions before approve, and execute records a mid-run
// question on the branch it builds and builds around it.
// Generation 57 lets a human cancel a question that needs no decision and narrow what it blocks; an agent may only widen.
// Generation 58 preserves settled gate-attestation guidance and permits explicit
// feature epic grain, with exact-path staging and isolated commits.
// Generation 59 records bug severity: capture grades, approve bug requires one, and grade changes it; severity never routes.
// Generation 60 names compacted events as the one exception to exact-path staging: stage exactly the paths sync, compact, migrate, or upgrade print.
// Generation 61 retires the intake skill: decompose carries both ways a document becomes work, a draft tree for the founding design and drafts held by one amendment for a later document.
// Generation 62 gives each shared rule one home: orientation defines its terms
// and carries the cross-skill rules, each seam's procedures live in its skill, a
// run and the go that starts it are defined, and maintenance moves to the
// upgrade skill.
const orientationSchemaGen = 62

// Orientation returns the committed .tiller/orientation.md text. It is a pure
// function of constants for a generation, so two inits produce byte-identical
// files and a re-generation is a no-op diff with no git churn.
func Orientation() string {
	return orientationHeader + strconv.Itoa(orientationSchemaGen) + orientationBody
}

// OrientationGeneration returns the schema generation orientation.md is
// currently keyed to, the same value Orientation stamps into the header. It
// is exported so a caller comparing an on-disk file against the running
// binary (`tiller check readiness`, the stale-orientation gap) reads
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
they describe. This file is your front door: it explains what Tiller is, its terms, the
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
documents that arrive through ` + "`tiller intake`" + `: extend the same bounded outcome, create a new increment for a distinct
outcome or delivery phase, and preserve accepted history. Missing information
that materially changes boundaries or order needs a focused question, not an
invented journey. Propose wider changes for developer direction, not as an
unrequested replan.

## Terms

- **seam**: a point in the lifecycle where one skill's work begins and ends.
- **live**: neither cancelled nor archived.
- **shelf**: the standing list of bugs, chores, or tweaks; each noun has its own shelf, kept in soft order.
- **frontier**: the live draft features whose own and owning epic's recorded dependencies are settled (` + "`done`" + ` or ` + "`cancelled`" + `), excluding drafts an open amendment holds.
- **review grain**, **coarse**: where review lands (` + "`task`" + `, ` + "`feature`" + `, or ` + "`epic`" + `); coarse means feature or epic.
- **unit of work**: what one review covers.
- **run**: the work the developer's instruction named, and nothing else.
- **aggregate**: the feature or epic review branch that submitted task branches merge into at coarse grain.
- **riding task**: a task whose review rides its feature's or epic's review; it submits a review ref, not its own PR.
- **review ref**: the branch and commit a submit records when the node has no PR of its own. A riding task's review ref is also called its riding ref.
- **finish line**: at coarse grain, every live riding task ` + "`in_review`" + ` or ` + "`done`" + `, and at epic grain every independent feature or task override already done.
- **closeout**: the bounded work that carries a reviewed unit to done after its finish line or its reported merge: feature checks, evidence, and review preparation up to the human's acceptance, then submission, sync, and the state commits that record them.
- **rollup**: a feature's or epic's ` + "`done`" + `, recorded by ` + "`tiller sync`" + ` from its children (at least one not cancelled, every live one done, and for a feature its gates checked) and recomputed when new work lands beneath it.
- **accepted basis**: the spec, contract, or task prose pinned at approval as ` + "`accepted_against`" + `; ` + "`active_basis`" + ` is drift from it.
- **operative question**: an open question that currently holds a node, directly or through an ancestor; a recorded feature-only assumption lifts its hold on that feature, never on its tasks.
- **held draft**: a node an open amendment created and holds inert until approval promotes it or withdrawal removes it.
- **carrier**, **carried**: a shelf node is carried while a live epic, feature, or task, its carrier, is recorded through ` + "`tiller resolve`" + ` as the work whose merge fixes it.

## The rules you must follow

- **Managed-project independence.** Managed projects must never depend on Tiller. Setup, build, tests, CI,
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
- **Developer authorship.** The developer owns and is responsible for the work. Never identify yourself,
  an agent, model, provider, or harness as an author or contributor in any
  output: code, comments, docs, commit messages, PR titles or descriptions,
  issues, reviews, reports, or replies. Never add generated-by notices,
  agent signatures, badges, footers, or agent co-author trailers. Use the
  developer's configured Git identity, never an agent author or committer
  identity, and never invent a human identity. Write about the change, its
  reasoning, and observed verification, not the tool that produced it. This
  rule applies across every skill and hand-off. It does not remove necessary
  technical references, real human credits, or required third-party notices.
- **External references.** In authored prose, link the first mention of an
  external library, framework, service, standard, tool, or other reference to
  its canonical home or the specific page relevant to the claim, using an HTTPS
  Markdown link. Use a URL only when it is present in the inputs or was fetched
  live in this session and returned a page identifying the intended reference.
  A failed fetch or a search snippet is not verification. Never guess a URL,
  even for a familiar name or a seemingly obvious homepage or domain. If neither
  source is available, write the name as plain text. This house rule applies
  across skills and hand-offs; it does not authorize fetching unrelated sources
  or changing accepted historical prose.
- **Gates and delegation.** Drive work to the edge of done and stop. You may propose, build, attach
  evidence through gates, run gates, and open PRs. A gate is the human's
  decision, not the human's keyboard: on an explicit instruction naming the
  node and target, or the exact design docs, you may run ` + "`approve`" + ` or ` + "`resolve`" + ` yourself, stating exactly
  what you are about to record and reporting what you recorded. A declarative
  past-tense claim ("they're all approved") is authorization to record only
  when the set is unambiguous: verify the recorded state first, then record,
  then report the delta. ` + "`cancel`" + ` on a work node is human-only: hand the exact ` + "`cancel`" + ` command
  to the human rather than run it, even on instruction. ` + "`cancel question`" + `, ` + "`unlink question`" + `, and ` + "`withdraw amendment`" + `
  are the human's decisions that you may record on an explicit instruction naming the target, as with ` + "`approve`" + `. ` + "`attest`" + `, ` + "`sync --accept-fast-forward`" + `, and the final merge
  into the configured integration branch are never yours to run, even on instruction;
  hand the human the exact command. Merging task branches into their recorded
  feature or epic aggregate is authorized execution, not a done crossing.
- A product decision only the human can make (behavior, acceptance, scope, or
  order) is recorded with ` + "`tiller add question`" + `, never asked only in chat, where
  no gate and no later session can see it. A clarification of the request itself
  (which node, which document, which of two readings) is asked in chat.
- Mutate state only through ` + "`tiller`" + `. The state files are machine-owned: never
  hand-edit ` + "`epic.yml`" + `, ` + "`feature.yml`" + `, ` + "`task.yml`" + `, ` + "`bug.yml`" + `,
  ` + "`chore.yml`" + `, ` + "`tweak.yml`" + `, ` + "`design/baseline.yml`" + `, or the event stream, and never
  hand-edit this file. Every change to state lands through a command, so the event stream
  and the gate records stay honest. The one ` + "`.yml`" + ` you own is the manifest,
  ` + "`.tiller/tiller.yml`" + `: its configuration (gates, contributors, inputs) is
  human-owned, so edit freely.
- Confirm an unfamiliar command or flag with ` + "`tiller <command> --help`" + ` before
  running it. Never guess one.
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
- **Committing state.** Commit the state Tiller writes. Every command records into git-tracked files
  under ` + "`.tiller/`" + `, and Tiller never commits them for you. The commit is part
  of the work: commit the state delta with, or right after, the change it
  describes, using plain ` + "`git`" + ` and this project's own commit conventions.
  Inspect the existing index first and preserve unrelated staged and unstaged
  work, including planning drafts. Stage exact reviewed authored/output paths,
  never a whole directory. The one exception is compacted events: when
  ` + "`tiller sync`" + `, ` + "`tiller compact`" + `, ` + "`tiller migrate`" + `, or
  ` + "`tiller upgrade`" + ` names paths to stage, stage exactly those, directories
  included. Isolate commits with the same exact paths so unrelated
  index entries stay out; if a named file has unrelated edits, isolate authorized
  hunks with a reviewed index or stop rather than commit its whole contents.
  One requirement rides on top of those conventions rather than replacing them:
  a commit you author while executing a task carries that task's id. A commit
  while executing a started shelf node carries that shelf node's own id, so
  ` + "`git log`" + ` can walk from either commit to the node that explains it. An
  uncommitted ` + "`.tiller/`" + ` change is not shared state and is not a review
  surface. The final integration merge is still the human's; committing is not. A diff that
  touches only ` + "`.tiller/`" + ` never needs a pull request: planning state, gate
  records, and events are CLI-written audit records, so commit them directly.
  Planning state commits on the integration branch, in the checkout that holds
  it. Planning state is what a command writes outside the execution of a started
  task or shelf node: decomposition, specification, capture, amendments, baseline
  docs, review dispositions, and questions recorded while orienting. Run the
  planning command itself in that checkout, so the state it writes lands there.
  That checkout is usually the primary checkout, since Git checks out a
  branch in only one worktree. Never switch a checkout's branch to get there. When this checkout is on
  another branch, use the worktree that has the integration branch checked out
  (` + "`git worktree list`" + `), or add one and join inside it. Commit there only
  when it holds no changes you did not make; otherwise stop and name what is in
  the way.
  A mid-task delta belongs on its own recorded task branch. A delta while executing a
  started shelf node belongs on that shelf node's branch; pre-start shelf state
  remains under the integration-branch fallback, as does other planning state.
  The PR ceremony is for code.
- Every started task has its own recorded branch, unchanged on resume. Tiller
  records and verifies branch structure; it does not create, switch, or merge
  refs. Read the recorded names rather than inventing them, and never delete,
  rename, or rewrite local or remote history to make a branch fit. Merging task
  branches into their recorded aggregate is agent work; the final integration
  merge is the human's. ` + "`tiller guide execute`" + ` carries the branch procedure
  under **Claiming and starting** and **Aggregate assembly**.
- **Worktrees.** Execute each unit of work in its own Git worktree by default, leaving the
  primary checkout where the developer left it. A unit of work is what one
  review covers, so review grain sets it: the task at task grain, the whole
  feature at feature grain, the whole epic at epic grain, and always the bug,
  chore, or tweak itself. The developer's or project's own
  instructions about checkouts and worktrees take precedence over this default;
  follow them when they differ. ` + "`.tiller/identity.yml`" + ` is per checkout and
  gitignored, so a new worktree has no identity until you run
  ` + "`tiller join --as <you>`" + ` inside it.
  Assume other agents and developers may be working this repository at the same
  time in their own worktrees. A worktree, branch, or uncommitted change you did
  not create for this work is someone else's: never remove, reset, clean, stash,
  commit into, or check out over it. Before claiming, look for the work's
  canonical branch in local worktrees, branches, and the remote; when one exists
  or Git surfaces a conflicting claim or start, stop for the human rather than
  choosing a winner. Remove only worktrees you created, and only after their
  work is committed and, when remote-backed, pushed. ` + "`tiller guide execute`" + `
  carries the worktree procedure under **Worktrees and other agents**.
- Maintenance is an explicit request. ` + "`tiller upgrade`" + ` updates the binary and this
  checkout; ` + "`tiller upgrade --project-only`" + ` refreshes this checkout with the running
  binary; ` + "`tiller join`" + ` selects identity and is never the refresh remedy. For
  anything more, load ` + "`tiller guide upgrade`" + `.
- An instruction to execute includes ordinary implementation and state commits,
  pushes, PR creation or reuse, and submission for the authorized work. A reported
  merge or acceptance decision re-enters that work's bounded execute closeout;
  it does not authorize unrelated execution. A pure orient request still briefs
  and stops. Explicit developer no-publish or earlier-stop instructions control.
  Tiller does not push or create PRs: use ordinary Git and the project's host tools.
  Never attest, never assert a fast-forward, and never perform the final
  integration merge. Never force-push, bypass a gate, invent a PR, or commit to a review branch after its final integration merge.
  Pure reads verify neither host state nor pushes. If an aggregate finally
  merged before its Tiller review was recorded, a merge report alone does not
  authorize recording it: recording the missing historical review needs explicit
  authorization naming it. ` + "`tiller guide execute`" + ` carries the sequence
  under **Publication sequence**, from checks and evidence commits through the
  draft review, human acceptance, formal submission, and state publication, with
  merge closeout and missing-review recovery in the sections after it.
- **Recovery protocol.** Follow the recovery protocol when a command refuses. Every skill inherits it
  and corrects itself from the CLI's exit codes without a human. Exit ` + "`1`" + ` is a
  business refusal to reason about, usually a red gate or an illegal transition:
  fix the work and try again, do not escalate. Stop and report when the same gate
  fails the same way after two different fixes, or when a failure does not
  reproduce when you run the gate's own command directly: name the gate, the
  failure, what you tried, and the evidence commits. Exit ` + "`2`" + ` or ` + "`3`" + ` means
  correct the call or environment. The ranked list is guidance, not a command result.
  When state leaves a gate or merge outstanding, say ` + "`the ranked list suggests waiting`" + `;
  never say Tiller requires another node to wait unless a command exited ` + "`1`" + `.
  When you are unsure what is legal from here, ` + "`tiller status <noun> <node>`" + `
  names the legal next moves with nothing executed. That one-node oracle is also
  the surface that reports ` + "`active_basis`" + ` drift; the project summary does not.
  During ` + "`design-brief`" + ` or a ` + "`code-review`" + ` review, recover only through corrected reads.
  Missing sources or stale instructions are explicit limitations, not
  permission to repair, refresh instructions, or mutate the project.
- **Node references.** Name a node to the human by its key path with the id in parentheses, for
  example ` + "`submit-certification/certification-form-loading (F-79dh3q)`" + `. A task's key path
  carries all three segments and reads
  ` + "`submit-certification/certification-form-loading/schema-transcription (T-k3p9w6)`" + `.
  A question is never keyed: name it by its id and short title. Use the full
  statement when the question itself is the subject.
  The rule covers every surface you author, readout notes, hand-offs,
  summaries, and lists alike: an epic, feature, or task id never appears
  without its key path, because a bare id sends the developer off to look up
  what it points at. Bare ids belong inside commands. A commit subject reads
  the same way: the bare task id satisfies
  traceability there. A pull request title carries the reviewed node's full key
  path with its id. Its body names the work riding that review, the task's
  parent feature at task grain, or the tasks and features at a coarser grain.
  In command position an existing node takes either form: a bare id is always
  valid, and a keyed noun also takes its key path. The ` + "`add`" + ` verbs are the
  exception, because they name a node that does not exist yet by the key path
  it will have, so every segment there must be a key:
  ` + "`tiller add feature <epic-key>/<feature-key>`" + ` and
  ` + "`tiller add task <epic-key>/<feature-key>/<task-key>`" + ` refuse a parent
  addressed by id with exit ` + "`2`" + `. A question is id-only in command position,
  since it is never keyed. This convention is inherited by every skill, and each
  one repeats the naming half at its own reporting moment, where a bare id
  actually leaks.
- Work inside this repository. Tiller's guidance is complete in the project:
  this file, the skills ` + "`tiller guide`" + ` serves, and the command output.
  Never go hunting outside the project directory for Tiller documentation or
  schema shapes. Leave the project only where the developer points you.
- **Readout.** Report through the readout. At a seam, ` + "`tiller readout <seam>`" + ` prints a
  block; at a human gate the command prints one itself; either way,
  reproduce it verbatim as the last thing in your reply, with your own
  account under ` + "`Notes from this run`" + ` ahead of it. The block closes the
  reply, nothing follows it, and it ends on its ` + "`**Next:**`" + ` ranked list,
  the one place the developer looks to learn what happens next, which you
  reproduce as written, with no list of your own after it. It renders the same
  whoever is driving, so the developer reads one format rather than yours.
  Do not restate the block in prose around it. The notes carry what the block
  cannot know. Keep the notes to facts about this run. Commentary is yours to
  add freely ahead of the block, never after it; a question for
  the human is a recorded question. ` + "`submit`" + ` and a passing
  ` + "`check`" + ` print the same list under ` + "`**Up next:**`" + ` when they finish; that list
  shows what the tree now allows; inside your run's scope it is your queue, and outside it, it is not authorization. The closing readout's list is the one you reproduce. The block has two sizes.
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
  ` + "`upgrade`" + ` is not a lifecycle seam either: its reply ends with its phase
  report. Do not invent a ` + "`tiller readout upgrade`" + ` command.

## How to work

Run ` + "`tiller status`" + ` to see where things stand, then ` + "`tiller guide`" + ` to load the
skill for the moment. With no argument, ` + "`tiller guide`" + ` prints the ranked list
and names the skill for its top agent move; with a skill name it prints that
skill in full. ` + "`tiller next`" + ` prints the list alone. When you are choosing the
next seam, relay the list to the developer and end the reply; their next message
redirects you, or starts the top agent move only when it says to. Work already moving inside a seam keeps moving, including execute's draft
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

An explicit request to upgrade Tiller, migrate this checkout's schema, or refresh
its generated guidance goes directly to ` + "`tiller guide upgrade`" + `, outside the
ranked list. It authorizes only the maintenance named. Entering a project, a
normal next-work request, or a completed step does not trigger it.

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
` + "`.tiller/identity.yml`" + ` supplied it, is refused rather than added.
Missing identity still calls for ` + "`tiller join --as <you>`" + ` after human
registration. The
roster is not add-only: ` + "`tiller remove contributor <id>`" + ` retires someone later, the
same human-gated act, refusing while they still hold live work. Do not
skip either step: without them there is no acting contributor here, so the first
` + "`tiller add`" + ` exits ` + "`3`" + `, ` + "`check readiness`" + ` reports the empty roster as a gap,
and the ranked list below cannot rank your own work. Then
` + "`tiller check readiness`" + ` names any missing inputs or manifest gaps.
For greenfield, prepare design inputs (` + "`tiller init --design`" + `, or ` + "`tiller intake --no-amendment`" + ` while no epic exists) and name ` + "`decompose`" + ` to turn them into
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
queue does not erase pending baseline review.

Bugs capture defects: observed behavior violates expected behavior. Chores are
housekeeping without code side effects; this is guidance, not a restriction or
validation rule. Tweaks are minor adjustments that may change code. A change to
code or tests, however small, is a tweak. Tasks carry
substantive planned work. Capture a forward adjustment with ` + "`tiller add tweak <key>`" + `
and its ` + "`.tiller/tweaks/<key>/tweak.md`" + ` contract, then stop for human triage.
Done work stays done: never reopen it to accommodate an adjustment. A tweak
uses the same ownership, gates, lifecycle, and review as a chore.

A bug carries a severity graded by impact: ` + "`blocker`" + `, ` + "`important`" + `, ` + "`medium`" + `,
or ` + "`minor`" + `. Capture files it with ` + "`tiller add bug <key> --severity <level>`" + `, or
leaves it unrated and says why in the contract when impact is not yet known.
` + "`tiller approve bug`" + ` refuses an unrated bug (` + "`severity_required`" + `) until
` + "`--severity <level>`" + ` grades it, so hand the human
` + "`tiller approve bug <key> --severity <level>`" + ` for one. Like ` + "`rename`" + ` and
` + "`link`" + `, ` + "`tiller grade bug <key> <level>`" + ` is a direct command: any
contributor may set or change a non-terminal bug's severity with it, from
` + "`proposed`" + ` through ` + "`in_review`" + `; a done or cancelled bug keeps its grade.
Chores and tweaks carry none. Severity never routes: it sorts and filters the
console's Fix board, while the next list, readouts, and the queue keep the
shelf's soft order.

**Ownership.** Every new task is unclaimed, regardless of who created or approved it. Claim
only the execution unit: ` + "`tiller claim task <key>`" + ` (or ` + "`bug`" + ` / ` + "`chore`" + ` / ` + "`tweak`" + `).
Before start, ` + "`release`" + ` returns your reservation to the pool and
` + "`reassign task|bug|chore|tweak <key> --to <contributor-id> --reason <text>`" + ` records a
human-directed handoff. Taking another reservation with ` + "`claim --from <id>`" + `
requires an explicit human handoff naming the work and actual holder; knowing
an owner flag is not permission. Never take another contributor's reservation
by changing ` + "`--contributor`" + `, or your acting identity, to theirs. The first successful start permanently fixes
that unit's owner. Resume, checks, review, cancellation, and completion never
reset it; all terminal ownership is immutable, even cancelled-before-start work.
Start, resume, and execution submission require the unit's own claim.
Features and epics have no owner or claim, and claim changes never cascade.
The judgment verbs, ` + "`approve`" + `, ` + "`attest`" + `, ` + "`answer`" + `, ` + "`eval`" + `, and
` + "`check`" + `, bind to no claim: registered developers may review others' work,
with the same actor attribution and human-gate rules. This is recorded-identity
workflow discipline, not authentication.

**Contributors.** Contributors on a feature or epic are the distinct owners of descendant tasks
that have started, including done and cancelled tasks. Ready reservations,
review/assembly actors, ancestor creators, and attached shelf work do not count.
Current work means unfinished task/bug/chore/tweak assignments, including proposed,
blocked, checked, and in-review work; a historical contribution assigns nothing.
Use ` + "`tiller ls features --contributor <id>`" + ` for contribution history and
` + "`tiller ls tasks --owner <id>`" + ` for scalar assignments.

Before start, a reservation changes hands through ` + "`reassign`" + `.
For already-started unfinished work, only an explicit human instruction naming
the work and recipient authorizes
` + "`tiller transfer task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>`" + `.
` + "`tiller guide execute`" + ` carries what transfer preserves, creates, and
refuses, and the Git and host follow-through it leaves, under
**Exceptional human-directed transfer**.

The shelves use the same explicit ownership boundary. The human chooses queue
work, then you claim the top eligible node in that shelf's soft order with
` + "`tiller claim bug <key>`" + `, ` + "`tiller claim chore <key>`" + `, or ` + "`tiller claim tweak <key>`" + `. An unclaimed shelf
node never enters the ranked list. A live ` + "`carried`" + ` node waits on its carrier.

**Review grain.** Review does not have to land once per task. ` + "`review_grain`" + ` records where
it lands instead: ` + "`task`" + ` by default, ` + "`feature`" + `, or ` + "`epic`" + `. It lives in the
project's ` + "`tiller.yml`" + `, an epic's ` + "`epic.yml`" + `, and a feature's ` + "`feature.yml`" + `,
absence inheriting downward and the most specific explicit setting winning.
Each home accepts task, feature, or epic, including an explicit epic override
on a feature. Set it only on an explicit human instruction naming the node
and grain, through ` + "`tiller configure project`" + `, ` + "`configure epic <key>`" + `,
` + "`configure feature <key>`" + `, or the matching ` + "`approve --review-grain`" + ` gate.
State what you will record and report what you recorded.

**Review grain changes.** Invalid stored grain values refuse the tree rather than defaulting. If
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
correction that maps to a class authorizes the mapped action except ` + "`cancel`" + ` on a work node.
For any such cancellation, hand the exact ` + "`cancel`" + ` command to the human rather
than run it. For other mapped actions, do not ask which mutation path to use.
The correction is already the developer's go for that action: carry it out
without asking the human to translate their correction into Tiller vocabulary.
Change machine state only through ` + "`tiller`" + `, never hand-edit machine state or
node YAML, and edit only human-owned Markdown. Then read the result back, commit
it, rerun the current readout with ` + "`--since <sha>`" + ` naming the commit the last
block you reported, and return to the row's existing approval, refresh,
reapproval, task-approval, or amendment gate. A seam exit keeps the full
readout; a correction inside a gate gets that lighter block.

**Structural-change preflight.** Before negotiating a structural change, load the installed ` + "`tiller guide amend`" + `
and read ` + "`tiller rename --help`" + ` and ` + "`tiller amend --help`" + `. Inspect affected
features, all descendant tasks, and source/destination epics through
` + "`tiller show`" + `, node ` + "`tiller status`" + `, and ` + "`tiller query`" + `. Check current
states and prior starts, branches/reviews, accepted bases, dependencies,
effective question holds, and inherited review settings. Current ready state
does not erase execution history. Use read-only history inspection as needed,
never a trial mutation. Explain the recommended
product outcome, installed capability, and smallest safe alternative separately
before asking product questions. Do not ask for facts the repository can answer.
If installed help lacks a move operation, disclose the limitation rather than
promise accepted relocation or upgrade without direction.

Approval does not freeze display titles, epic prose, additive child work, soft
order, or same-scope prose on a non-terminal feature or task. Accepted prose on
a done or cancelled feature or task is historical and is not revised in place.
Accepted basis, terminal history, and active structural work still have teeth.
Do not invent an amendment for work that only extends the accepted plan.

**Correction table.**

| Correction | Route |
|---|---|
| Pending baseline proposal | Re-enter ` + "`derive-baseline`" + `, correct the named docs and relationships in place, reconcile the current code they describe, commit only that correction, and reproduce its ` + "`--since <sha>`" + ` readout. Return to the same named-doc human confirmation gate. Do not regenerate duplicate docs or open a work-node amendment for that correction. |
| Unapproved proposal (draft tree) | Re-enter ` + "`decompose`" + `, apply its draft correction from the command list in ` + "`tiller guide decompose`" + `, commit it, rerun its readout with ` + "`--since <sha>`" + `, and return to the same epic approval gate, not ` + "`amend`" + `. |
| Specced but unapproved feature plan (pre-work prose plan) | Re-enter ` + "`specify`" + `, apply the unapproved-plan route in ` + "`tiller guide specify`" + `, commit it, rerun its readout with ` + "`--since <sha>`" + `, and return to the same feature approval gate. |
| Epic, feature, or task display title, at any state | Run ` + "`tiller rename <noun> <node> --title \"<display title>\"`" + `; no amendment. |
| Approved epic narrative wording | Edit ` + "`epic.md`" + ` and commit it; Git is the audit record because epic approval pins decomposition shape, not an epic prose basis; no amendment. |
| Approved non-terminal feature or task prose, same scope | A wrong factual claim found while executing a task is an execute departure, recorded with the work (its implementation commit message, and its PR when one exists), not an edit here. Otherwise edit the pinned Markdown and commit it, read ` + "`active_basis`" + ` with ` + "`tiller status <noun> <node>`" + `, then stop for the human to run ` + "`tiller approve <noun> <node> --refresh-basis`" + `. |
| Done or cancelled feature or task historical prose | Do not edit the accepted Markdown in place or prescribe ` + "`--refresh-basis`" + `. Route the correction into forward additive or replacement work; use ` + "`amend`" + ` and supersession as appropriate so accepted history stays intact. |
| Ready feature or task plan whose approval should replay | Run ` + "`tiller return <noun> <node> --reason \"<why the plan is being revised>\"`" + `, revise it, and stop for the human to approve again before work starts. |
| Approved feature needs one more task | Run ` + "`tiller add task <epic-key>/<feature-key>/<task-key> --title \"<what it accomplishes>\"`" + `, write the proposed ` + "`task.md`" + `, and stop for the human to run ` + "`tiller approve task <node>`" + `; the additive task has its own approval and needs no amendment. |
| Preferred sequence | Run ` + "`tiller order <noun> <node> [<node> ...]`" + `, where noun is epic, feature, or task; soft order never gates. |
| Required sequence before affected work starts | Add the hard edge with ` + "`tiller link <noun> <node> --depends-on <target>`" + `; remove a mistaken edge with ` + "`tiller unlink <noun> <node> --depends-on <target>`" + `. |
| Dependency change after affected work starts | Explain that an in-place edge edit is unsupported. Amendment does not unlock it or stage links/unlinks; only a genuine replacement can use fresh eligible work with new edges. Never manufacture replacement to evade the lock. |
| Genuine structural reshape (scope genuinely abandoned or replaced, including after accepted work starts) | Route the structural reshape to ` + "`amend`" + `, in the requested scope, using its cancel or supersede operation and fresh work as appropriate, preserving accepted history: cancel unfinished work with a reason, or supersede done work with forward replacement. Do not cancel and redecompose unchanged work merely to relocate it, including a specced but unapproved plan. Amendment has no link, unlink, or order operation; revise only re-pins prose. An in-place hard-edge change on unchanged started work remains unsupported. Do not manufacture replacement to evade that lock. Fresh unstarted work can receive legal prerequisites, even under an active parent. A done feature's eval bindings, never its criteria statements, are corrected through ` + "`amend --correct-eval`" + `. Preserve terminal feature/task prose and completed descendants. Same-outcome additions may continue under done parents; ordinary sync reopens rollups, not the completed tasks. Cancelled or archived ancestors are not continuation targets. Distinct outcomes belong in new increments. After materialization, proposed placement needs explicit ` + "`tiller order`" + `; hard links remain subject to the dependent node's state and branch checks. Propose wider replanning for developer direction. |
| Feature key or parent, draft and task-free feature | Run ` + "`tiller rename feature <ref> --key <epic>/<key>`" + `; changing parents additionally requires both source and destination epics to be draft. |
| Epic key | Run ` + "`tiller rename epic <ref> --key <canonical-key>`" + ` only on a draft epic with only draft, task-free features. |
| Task key | Run ` + "`tiller rename task <ref> --key <canonical-key>`" + ` only on a proposed task staying under its current feature. |
| Proposed bug, chore, or tweak key | Run ` + "`tiller rename bug <ref> --key <canonical-key>`" + `, or the same for a chore or tweak, using one local key segment. |
| Unchanged feature key or parent correction outside the draft, task-free window (reviewed unstarted feature move) | Run amend's preflight; eligible draft, specced, or ready features with never-started proposed/ready descendants use reviewed ` + "`move-feature`" + `, preserving identity and stopping at amendment approval: stage ` + "`tiller amend <A-id> --move-feature <feature-ref> --to <epic>/<key> --reason \"<why>\"`" + `, inspect ` + "`tiller show amendment <A-id>`" + `, commit the proposal, and stop for the human's ` + "`tiller approve amendment <A-id>`" + `. A started descendant refuses precisely: name it and its history as the refusal; do not manufacture replacement or edit state around the restriction. The eligibility detail, what survives a move, what refuses, and ordering, is in ` + "`tiller guide amend`" + `. |

A key rename never renames a branch or rewrites review history.

Bounded epic features may interleave. A projected epic ordering cycle is a
nonblocking advisory about grouped rendering, not an execution deadlock; inspect
its implicated features and recorded edges and explain them. Actual prerequisite
cycles, including explicit epic dependencies and mixed feature/epic completion
deadlocks, still block readiness and execution. Never delete a valid
prerequisite, or tell the developer to, merely to make the epic map sort cleanly.

### Ranked next

` + "`tiller next`" + ` prints the legal next moves for this checkout, ranked, at most five,
one row per node. Bare ` + "`tiller guide`" + ` and the readout's ` + "`**Next:**`" + ` print the
same list, and ` + "`submit`" + ` and a passing ` + "`check`" + ` append it as ` + "`**Up next:**`" + `, so
the CLI never disagrees with itself. Each row leads with its actor, ` + "`You`" + ` for a
human decision or ` + "`Agent`" + ` for work, then the action, the reason, and the exact
command, or the branch to continue on. ` + "`tiller guide orient`" + ` explains how the list is ranked and what each move leads to.

Relay the list as written. If any human move ranks above the top agent move,
name it first. Then end the reply. Start the top agent move in your next turn only when the developer's reply agrees to it: a go, a yes, any plain assent such as "sounds good", or an instruction naming that row. A reply that asks something else is answered, and the list still waits for its go. A reply naming a different row or other work redirects you to that.
You may say you would take a different item first and why; never rewrite the list.
A ranked move is a suggestion, not authorization.
An explicit human instruction naming nodes overrides the list's order for exactly
those nodes.

The list holds only legal moves, so every row can be acted on as printed. The
standing queue line is always present: ` + "`queue: N bugs, M chores, K tweaks ready`" + `. It
reports ready, non-carried shelf work, which a human chooses before any claim.

` + "`tiller guide`" + ` serves eleven skills:

- ` + "`orient`" + `: you need the next move. Read state, relay the ranked list above to
  the developer, and end the reply.
- ` + "`derive-baseline`" + `: an existing-code project has no confirmed as-is doc.
  Derive or translate module-grain descriptions, record recoverable provenance,
  commit, and stop for human named-doc confirmation. Review existing pending
  docs in place rather than regenerate them.
- ` + "`decompose`" + `: to-be inputs exist but the tree does not reflect them.
  For the founding design, lay a draft epic and feature tree for the human to
  approve. For a later document, ` + "`tiller intake`" + ` landed it under
  ` + "`.tiller/design/`" + ` and opened a draft amendment: stage the mapping as drafts the
  amendment holds, add concrete defects, chores, and tweaks straight to their
  shelves with ` + "`--source`" + `, and run ` + "`tiller propose amendment`" + `, even when
  nothing was staged.
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
- ` + "`design-brief`" + `: the developer explicitly requests a UI/UX design brief
  or prototype handoff for an epic, feature, or task. Gather bounded requirements
  and export one self-contained Markdown snapshot for a non-production prototype.
  Return it and stop, with no lifecycle mutation, readout, upload, or prototype build.
- ` + "`code-review`" + `: the developer explicitly asks for a code review of one
  node, or names rows of an existing review. Review its net change read-only into
  one file under ` + "`.tiller/reviews/`" + ` and reply with a summary table; on a
  later disposition, log the named rows through capture and decompose shapes and
  commit once, leaving every approval to the human.
- ` + "`upgrade`" + `: the developer explicitly asks to upgrade Tiller, migrate this
  checkout's schema, or refresh its generated guidance. Run the maintenance named,
  report each phase's outcome, and stop for the developer to review and commit the diff.

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
developer confirm before anything is staged: staging creates inert drafts the
amendment holds, and the human's ` + "`approve amendment`" + ` is the one act that
moves anything. Capture sits outside the list too: it is operator-initiated,
not a ranked move computed from recorded state. Use ` + "`capture`" + ` when an
investigation or developer description identifies a concrete shelf item. When the
developer hands over a document, run ` + "`tiller intake`" + ` to land it; the draft
amendment it opens then ranks as a decompose move.

Process is Tiller's; craft is the repo's own. How you write the code, the testing
discipline, and the architecture live in this project's ` + "`AGENTS.md`" + ` and
` + "`CLAUDE.md`" + `, not in a Tiller skill. The skills are opinionated about the shape
and gating of the work and silent on the content of the implementation.

## Where things live

Everything Tiller owns is under ` + "`.tiller/`" + ` at the repo root:

- ` + "`tiller.yml`" + ` is the manifest: the project archetype, the contributors, the inputs,
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
