# Design history

The founding decisions behind Tiller: what was chosen, what was rejected, and why. This is not a changelog or a contract. The [specs](spec/) own current behavior, and the [roadmap](roadmap.md) says what is built and what is not.

The responsibility split runs through every decision below. Tiller holds the structure, the gates, and the state. The agent does the reasoning. The human decides at the gates and controls the merge.

## Everything lives in the repo

**Decision.** Project intent and execution state live beside the code, under git. Markdown carries human-owned prose; YAML carries machine-mutated state; the two never share a file. Stable ids identify nodes, readable keys locate them, and revision references record what a decision or gate was based on. Configuration lives in the repo too: `tiller.yml` is the committed team manifest, and nothing is read from global, user, or machine config.

Identity follows the same rule. `tiller init` creates a project and `tiller join` selects who is working in a checkout, writing a gitignored `.tiller/identity.yml`. Registering a contributor is a separate, deliberate write (`tiller add contributor`), so `join` only selects someone already on the roster. Per-developer preferences go in a gitignored `.tiller/local.yml` beside it, hand-edited and never written by Tiller.

**Rejected.** A database or a separate planning service, and a config file in the home directory. An early version let `join` register an unknown identity on the fly.

**Why.** A plan in another tool drifts from the code it describes. In the repo, a branch carries its plan with it, review covers plan changes like code, and git is the audit trail. Machine config would make two checkouts behave differently for reasons no diff shows. Letting `join` register people meant a typo or a borrowed identity silently became a roster member.

## One tree of epics, features, and tasks

**Decision.** Work is one progressively elaborated tree. An epic is narrative, a feature adds a specification and an acceptance contract, and a task carries the implementation detail. Each task lives in its own folder with its own prose and state. Specification, acceptance contract, and task breakdown are one just-in-time planning step per feature, behind one approval, taken when that feature's turn comes. Epics and features are bounded product outcomes, not permanent subsystem buckets. Hard dependencies gate work; soft order only refines recommendations.

**Rejected.** The first design had two planning axes: the tree, plus blitzes (time-boxed batches with their own scheduling layer) and an effort axis for sizing. Tasks lived in one shared ledger, and a separate break-down step decomposed features ahead of specification. All of it was removed.

**Why.** Two axes meant two places to decide what comes next, and they disagreed. The lifecycle states already form a queue, and the human paces the work by choosing which feature moves next. Planning every task up front produced plans that were stale before anyone built them. A shared ledger made every agent edit the same file. Soft order exists so the human can arrange the backlog without inventing dependencies that would then block real work.

## Human gates and review grain

**Decision.** Gates are declared and recorded by Tiller and decided by people. The human approves an epic, approves each feature's plan, attests acceptance that a machine cannot check, and merges. Required gates (build, tests, evals) block done; optional reviews are visible but never block. Done for a task is an observed human merge, not a command. Feature and epic done are rollups, recomputed when new work lands beneath them. An agent can drive work to the edge of done but never marks accepted or done, and it never infers a human's approval.

Review grain sets what one pull request covers: a task (the default), a whole feature, or a whole epic, with narrower settings overriding broader ones. Changing grain is constrained by live work so a setting cannot retarget a review already under way.

**Rejected.** An `accept` verb and an epic acceptance file, which made done a ceremony. Treating branch ancestry alone as proof of a merge. Tiller running review tools or the agent itself. Task-only review, which was the first simplification.

**Why.** The one boundary that holds regardless of agent behavior is the merge, and the human controls it. Evidence-backed done means the record cannot claim work landed when it did not. Merge observation needs a witness, content comparison for squashes, or an explicit human assertion, because ancestry can look like a merge when it is not one. Task-only review produced too many small pull requests for tightly coupled work, so grain widened review scope without bringing back a second planning axis.

## Harness-agnostic skills

**Decision.** The agent surface is the CLI, plain files, and embedded skills. A committed `.tiller/orientation.md` is the zero-dependency front door; `join` writes thin, gitignored skill shims for the harnesses that discover skills by convention. A skill uses only what every harness has: a filesystem, a shell, and the `tiller` CLI. Tiller generates the readouts at workflow boundaries, so every harness relays the same account. Tiller guides the agent and labels its guidance honestly; it does not pretend to control the agent.

Two rules extend this. The developer who directs the work owns it, so nothing authored identifies an agent, model, or harness as an author. A managed project never depends on Tiller: its build, tests, CI, and merge eligibility must work without Tiller installed.

**Rejected.** An MCP server or any single vendor integration. Model-written summaries at handoffs. Tiller orchestrating a harness. A CI emitter that would have made managed projects call Tiller.

**Why.** Different people drive the same project with different tools, so the surface has to be identical for all of them. Files and a CLI work everywhere and a human can read them. A vendor integration would make Tiller only as portable as that vendor.

## Change is forward work

**Decision.** A change after decomposition is an amendment: a staged, reviewed proposal that records its operations and the live facts they depend on. Approval validates the whole proposal and applies it in one transaction. Done work is never rewritten; it is superseded by new work. Intake brings outside feedback in the same way, with the source copied and cited.

**Rejected.** Editing done records to fit a change of mind, and blanket immutability that left no correction path at all.

**Why.** Every real project pivots. When the plan changes, the code the team already merged is treated like any existing codebase: the change becomes new work that builds on it or replaces it. Done records keep describing what was actually built and accepted, and the reviewer sees the whole change as one proposal.

## Brownfield baseline

**Decision.** Brownfield is the general case and greenfield the degenerate one. `project.kind` and explicit code-root modes say whether existing code is a mutable surface or a read-only reference, and reference protection keeps a surface declaration from granting permission to change a reference tree. An optional baseline ledger maps existing design docs to the code paths they describe. The `derive-baseline` skill reads existing docs and code and proposes that mapping; a human confirms each doc against current code. Baseline drift follows the same detect, propose, approve loop as all drift.

**Rejected.** Assuming existing docs describe existing code because they are in the repo. Treating existing code as a convention to inherit.

**Why.** Adopting Tiller proves nothing about whether old docs are true. A confirmed doc is a claim a human made at a known revision, which later drift checks can test. What cannot be compared stays visibly uncovered, never clean.

## Bugs, chores, and tweaks as leaf work

**Decision.** Bugs, chores, and tweaks are independent leaf nodes on their own shelves, each with its own prose, state, claim, gates, and pull request, following the task lifecycle. A bug is a defect, a chore is housekeeping without code side effects, and a tweak is a minor adjustment that may change code, such as better button text. Substantive planned work is still a task under a feature. Attaching a shelf item to a node records context only. An item carried by another piece of work waits on that carrier and comes back into view if the carrier is cancelled. The console's Fix board shows all three together.

**Rejected.** Records that could close only by pointing at a feature task, and a separate tracker beside the tree.

**Why.** Much real work is small and belongs to no feature. Forcing it under one invents structure, and a parallel tracker splits the queue. Shelf items share the tree's gates, routing, and history.

## The console reads, the CLI writes

**Decision.** `tiller serve` is a local, read-only console over the working tree. It shows decisions, progress, evidence, and drift, and where a human action is due it shows the exact CLI command to copy. It never mutates project state. Every surface reads one shared model in Go, the same one the CLI uses, so the console and the CLI cannot disagree about what is ready or blocked. Missing or partial data never renders as all-clear.

**Rejected.** Console-only interpretation of project state, and console write paths ahead of a design that protects them.

**Why.** The first browser trial lost facts between storage and presentation. The fix was to correct the shared model and carry those facts through both surfaces, not to teach the browser its own rules. Keeping writes in the CLI keeps one place where every guard runs. Planned console writes must map one-to-one onto CLI commands.

## Open questions as first-class nodes

**Decision.** A question is a recorded node with a short title, a full statement, and the epics, features, or tasks it blocks. Open holds drive readiness and routing. `approve feature` and `approve task` refuse while an open question holds the plan, with no assumption escape at that gate. The `specify` skill records each question that only the human can settle against the narrowest node it affects, and folds answers into the plan before approval. During execution, an agent records a new question, builds around the held work, and asks when nothing legal remains.

A question that no longer needs a decision is cancelled, not answered. Cancelled is its own final state; it releases every hold, keeps its reason and history, and raises no drift because no answer arrived. A question that matters again is asked anew. Cancelling and narrowing a question are the human's moves; an agent may only widen what a question blocks.

**Rejected.** Questions asked only in chat. A warning at approval instead of a refusal. Fake answers to clear a moot question. Reopening a cancelled question.

**Why.** A question in chat is invisible to every gate and every later session. A plan approved with an open question stalls at the first task, after the human has left; answering at the plan gate costs minutes. A warning repeats that failure whenever nobody reads it. Forcing an answer onto a moot question leaves a false decision on record that later readers trust.

## Agent usage, aggregate and never personal

**Decision.** Tiller estimates the model tokens a project's agents spent and credits them to the work item and the workflow stage each call served, so a team can choose models and shape specs with evidence rather than impressions. Small readers, one per harness and owned by Tiller in Go, turn the coding agents' own session logs into neutral per-session files committed under `.tiller/usage/`. Calls are credited by replaying the Tiller commands each session ran. Figures are raw token counts with cache reads always shown apart, summed across the team; no file, payload, or chart attaches a metric to a person. Capture is off until a project turns it on, records no event, and rides `tiller sync` or a human's `tiller usage`, never an instruction to the agent. Reading those logs is a bounded exception to reading nothing outside the repo. The console shows the result on the Report page's Agent use tab, with partial coverage drawn as partial, never as low usage.

**Rejected.** Dollar costs and price tables. Per-person views, filters, or rankings. Asking the agent to count or report its own tokens. Existing multi-harness tools such as ccusage and tokscale, which report daily or per-session totals, too coarse when one session spans many work items, and would be an outside install. OpenTelemetry ingestion, for now. Recording a capture event, which would put a person's name beside the token counts on History and Home.

**Why.** A spike over a client platform build's real logs from Claude Code, OhMyPi, and Codex credited 74 percent of 6.15 billion tokens to a work item, and 99.2 percent of the tokens it credited to a task's implementation fell between that task's first start and last submit, so the replay agrees with Tiller's own record. Subagents spent over half the tokens, which is why they are credited through their parent session. Cache reads were 96.8 percent of all tokens, which is why they are always shown separately. Claude Code deletes transcripts after 30 days by default, so capture has to run while work is current; that is why it rides `sync`. JC's ruling on people was absolute: never put a metric on a person; it always stays with the work.
