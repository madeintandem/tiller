---
title: Concepts
description: The mental model behind tiller, the tree, the lifecycle and its gates, who does what, drift and amendments, and the surfaces, with diagrams.
---

Tiller holds the structure, the gates, and the state of a software project. The agent does the reasoning. The human approves at the gates. Everything below is that one sentence, unpacked.

## The tree

Work is a single hierarchy: epics, then features, then tasks. Three shelves of leaf work sit beside the tree rather than inside it: bugs, chores, and tweaks. A shelf is a flat, ordered list of one kind of leaf work, with no parents or children ([more on those below](#bugs-chores-and-tweaks)). Each grain carries more detail than the one above it. An epic is a narrative. A feature is a narrative plus a specification and an acceptance contract. A task carries the implementation detail. A pull request represents exactly one node, a task by default, or the feature or epic when the [review grain](#review-grain) moves review upward. Every node has a stable id (`E-mu0v91`, `F-dv5ppv`, `T-h389ih`) and a human key path (`foundation`, `foundation/app-scaffold`, `foundation/app-scaffold/wire-the-build`). The id survives renames; the key is how people talk about it.

Every node is two files. The `.md` is prose and it's yours: intent, specification, acceptance criteria. The `.yml` is state and it's tiller's: status, owner, gate results, revisions. That split is what lets the CLI churn state all day without touching a word a human wrote, and it's what keeps two people's work from colliding in a merge.

Initial decomposition proposes epic and feature narratives through the full known initial scope. Features get detailed specifications, acceptance contracts, and tasks just in time, normally one named feature at a time rather than the entire future tree.

An epic is a bounded delivery increment, not the permanent home of a subsystem. Recommend the progression from documented consumer outcomes, including operators, developers, and API consumers, and interleave product work with the small platform increments it genuinely needs. Related capabilities can recur in later epics. Design shared machinery with later needs in mind without implementing all of them before its first consumer.

Features express honestly acceptable outcomes; their own contracts must be satisfiable at that boundary. Keep one unfinished feature when no sensible split exists rather than presenting incomplete behavior as complete. Tasks may be technical implementation units. True execution prerequisites belong in `depends_on`; preferred sibling sequence belongs in `tiller order`. Eventual architectural integration alone does not establish a prerequisite, and zero cross-epic edges is valid for independent work.

<figure class="diagram-figure">
<svg class="diagram" viewBox="0 0 980 310" role="img" aria-label="Epic, feature, and task side by side, each with a prose file that grows more detailed from left to right and a state file tiller owns; the task carries implementation detail while Review grain decides whether the task, feature, or epic owns the pull request.">
  <defs><marker id="d2a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 L10 5 L0 10 z" class="arrow"/></marker></defs>
  <text x="20" y="26" class="h">Epic</text><text x="20" y="44" class="muted small">a narrative</text>
  <text x="360" y="26" class="h">Feature</text><text x="360" y="44" class="muted small">narrative, specification, contract</text>
  <text x="700" y="26" class="h">Task</text><text x="700" y="44" class="muted small">implementation detail</text>
  <rect x="20" y="58" width="260" height="72" rx="6" class="box"/>
  <text x="32" y="80" class="mono">epic.md</text><text x="268" y="80" text-anchor="end" class="muted small">yours</text>
  <text x="32" y="102" class="small">What this slice of the product is, why,</text>
  <text x="32" y="120" class="small">and which features belong to it.</text>
  <rect x="20" y="144" width="260" height="50" rx="6" class="recess"/>
  <text x="32" y="166" class="mono">epic.yml</text><text x="268" y="166" text-anchor="end" class="muted small">tiller's</text>
  <text x="32" y="184" class="small muted">status, gates, depends_on</text>
  <rect x="360" y="58" width="260" height="128" rx="6" class="box"/>
  <text x="372" y="80" class="mono">feature.md</text><text x="608" y="80" text-anchor="end" class="muted small">yours</text>
  <text x="372" y="102" class="small">The narrative, then the specification:</text>
  <text x="372" y="120" class="small">what it does, and what it must not break.</text>
  <text x="372" y="146" class="mono">feature.eval.md</text>
  <text x="372" y="166" class="small">The acceptance contract: criteria with</text>
  <text x="372" y="182" class="small">bindings tiller can run.</text>
  <rect x="360" y="200" width="260" height="50" rx="6" class="recess"/>
  <text x="372" y="222" class="mono">feature.yml</text><text x="608" y="222" text-anchor="end" class="muted small">tiller's</text>
  <text x="372" y="240" class="small muted">status, gates, accepted_against</text>
  <rect x="700" y="58" width="260" height="160" rx="6" class="box"/>
  <text x="712" y="80" class="mono">task.md</text><text x="948" y="80" text-anchor="end" class="muted small">yours</text>
  <text x="712" y="102" class="small">The approach: which files, which seams,</text>
  <text x="712" y="120" class="small">what to build first, how to prove it.</text>
  <text x="712" y="146" class="small">Pinned as the basis when the plan is</text>
  <text x="712" y="164" class="small">approved; an edit after that is drift</text>
  <text x="712" y="182" class="small">until a human re-records it.</text>
  <text x="712" y="200" class="small muted">Review at the resolved grain:</text>
  <text x="712" y="216" class="small muted">task by default, or feature / epic.</text>
  <rect x="700" y="232" width="260" height="50" rx="6" class="recess"/>
  <text x="712" y="254" class="mono">task.yml</text><text x="948" y="254" text-anchor="end" class="muted small">tiller's</text>
  <text x="712" y="272" class="small muted">status, gates, review, accepted basis</text>
  <line x1="280" y1="94" x2="358" y2="94" class="line" marker-end="url(#d2a)"/>
  <text x="320" y="86" text-anchor="middle" class="small muted">decomposed</text>
  <line x1="620" y1="94" x2="698" y2="94" class="line" marker-end="url(#d2a)"/>
  <text x="660" y="86" text-anchor="middle" class="small muted">specified</text>
  <text x="660" y="112" text-anchor="middle" class="small muted">just in time</text>
  <text x="20" y="302" class="small muted">Each grain says more than the one before it. Prose is human-owned, state is machine-mutated, and they never share a file.</text>
</svg>
<figcaption>The tree as progressive elaboration: an epic is narrative, a feature adds a specification and an acceptance contract, a task carries the implementation detail. Every grain is a prose file you own beside a state file tiller owns.</figcaption>
</figure>

## The lifecycle and its gates

Each grain moves through a short list of states, and the states are the queue. `draft` is the backlog, `specced` and `ready` are on deck, `active` and `in_progress` are in motion, and `done` is done. An agent can drive any node to the edge of done. But three crossings belong to the human, and tiller marks them as such on every surface:

- **The epic gate**, `tiller approve epic`. The human approves the shape of the tree, not the details.
- **The plan gate**, `tiller approve feature`. The human approves a feature's specification, its acceptance contract, and its tasks in one act. The tasks ride that gate from `proposed` to `ready`, and each one's spec gets pinned as its accepted basis, the exact revision of the prose the human approved.
- **The merge.** Every task reaches `in_review` through `tiller submit task`. At task grain, the task owns the PR. At feature or epic grain, the task records branch-and-head provenance and rides the reviewed node's shared PR. Only a human merge crosses the work to `done`, and tiller learns of it by watching git (`tiller sync`), never by being told.

Everything above a task still completes by rollup. A feature is done when its tasks are done and its own required gates have run green; an epic is done when its features are. At a coarse review grain, one feature or epic merge can cross several riding tasks together, and no feature or epic gains a separate `in_review` lifecycle state.

Completion is history, not a permanent ban on continuation. New work serving the same bounded outcome can be added beneath a done epic or feature; a distinct outcome or delivery phase warrants a new increment, even in the same subsystem. Additions keep their ordinary specification and approval gates. The addition itself does not immediately change the parent's stored status: normal sync reopens the rollup, clears current parent completion/review fields, and stales feature gates as appropriate. Completed tasks and terminal feature/task prose remain accepted history. Cancelled or archived ancestors are not executable continuation targets.

<figure class="diagram-figure">
<svg class="diagram" viewBox="0 0 960 350" role="img" aria-label="Three lanes, epic, feature, and task, each a ribbon of states from left to right. The human-only crossings are approve epic, approve feature, and the reviewed pull request merge. Tasks always enter in_review; Review grain decides whether their review is owned by the task, feature, or epic. Done flows upward by rollup.">
  <defs>
    <marker id="d1a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 L10 5 L0 10 z" class="arrow"/></marker>
    <marker id="d1b" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 L10 5 L0 10 z" class="arrow-accent"/></marker>
  </defs>
  <!-- epic lane -->
  <text x="12" y="74" class="h">Epic</text>
  <rect x="90" y="57" width="86" height="26" rx="13" class="pill"/><circle cx="104" cy="70" r="4" class="st-draft"/><text x="114" y="74" class="small">draft</text>
  <rect x="230" y="57" width="86" height="26" rx="13" class="pill"/><circle cx="244" cy="70" r="4" class="st-ready"/><text x="254" y="74" class="small">ready</text>
  <rect x="440" y="57" width="86" height="26" rx="13" class="pill"/><circle cx="454" cy="70" r="4" class="st-done"/><text x="464" y="74" class="small">done</text>
  <rect x="600" y="57" width="86" height="26" rx="13" class="pill"/><circle cx="614" cy="70" r="4" class="st-archived"/><text x="624" y="74" class="small">archived</text>
  <line x1="176" y1="70" x2="228" y2="70" class="line accent" marker-end="url(#d1b)"/>
  <text x="203" y="50" text-anchor="middle" class="accent-text small">approve epic</text>
  <line x1="526" y1="70" x2="598" y2="70" class="line" marker-end="url(#d1a)"/>
  <text x="562" y="62" text-anchor="middle" class="small muted">archive</text>
  <!-- feature lane -->
  <text x="12" y="164" class="h">Feature</text>
  <rect x="90" y="147" width="86" height="26" rx="13" class="pill"/><circle cx="104" cy="160" r="4" class="st-draft"/><text x="114" y="164" class="small">draft</text>
  <rect x="230" y="147" width="86" height="26" rx="13" class="pill"/><circle cx="244" cy="160" r="4" class="st-specced"/><text x="254" y="164" class="small">specced</text>
  <rect x="370" y="147" width="86" height="26" rx="13" class="pill"/><circle cx="384" cy="160" r="4" class="st-ready"/><text x="394" y="164" class="small">ready</text>
  <rect x="510" y="147" width="86" height="26" rx="13" class="pill"/><circle cx="524" cy="160" r="4" class="st-active"/><text x="534" y="164" class="small">active</text>
  <rect x="650" y="147" width="86" height="26" rx="13" class="pill"/><circle cx="664" cy="160" r="4" class="st-done"/><text x="674" y="164" class="small">done</text>
  <line x1="176" y1="160" x2="228" y2="160" class="line" marker-end="url(#d1a)"/>
  <text x="203" y="180" text-anchor="middle" class="small mono muted">spec feature</text>
  <line x1="316" y1="160" x2="368" y2="160" class="line accent" marker-end="url(#d1b)"/>
  <text x="343" y="140" text-anchor="middle" class="accent-text small">approve feature</text>
  <line x1="456" y1="160" x2="508" y2="160" class="line" marker-end="url(#d1a)"/>
  <text x="483" y="180" text-anchor="middle" class="small mono muted">start</text>
  <!-- task lane -->
  <text x="12" y="264" class="h">Task</text>
  <rect x="90" y="247" width="86" height="26" rx="13" class="pill"/><circle cx="104" cy="260" r="4" class="st-proposed"/><text x="114" y="264" class="small">proposed</text>
  <rect x="230" y="247" width="86" height="26" rx="13" class="pill"/><circle cx="244" cy="260" r="4" class="st-ready"/><text x="254" y="264" class="small">ready</text>
  <rect x="370" y="247" width="86" height="26" rx="13" class="pill"/><circle cx="384" cy="260" r="4" class="st-in_progress"/><text x="394" y="264" class="small">in_progress</text>
  <rect x="510" y="247" width="86" height="26" rx="13" class="pill"/><circle cx="524" cy="260" r="4" class="st-check"/><text x="534" y="264" class="small">check</text>
  <rect x="650" y="247" width="86" height="26" rx="13" class="pill"/><circle cx="664" cy="260" r="4" class="st-in_review"/><text x="674" y="264" class="small">in_review</text>
  <rect x="790" y="247" width="86" height="26" rx="13" class="pill"/><circle cx="804" cy="260" r="4" class="st-done"/><text x="814" y="264" class="small">done</text>
  <line x1="176" y1="260" x2="228" y2="260" class="line accent" marker-end="url(#d1b)"/>
  <text x="203" y="284" text-anchor="middle" class="accent-text small">with the plan gate</text>
  <line x1="316" y1="260" x2="368" y2="260" class="line" marker-end="url(#d1a)"/>
  <text x="343" y="284" text-anchor="middle" class="small mono muted">start</text>
  <line x1="456" y1="260" x2="508" y2="260" class="line" marker-end="url(#d1a)"/>
  <text x="483" y="284" text-anchor="middle" class="small mono muted">check</text>
  <line x1="596" y1="260" x2="648" y2="260" class="line" marker-end="url(#d1a)"/>
  <text x="623" y="284" text-anchor="middle" class="small mono muted">submit</text>
  <line x1="736" y1="260" x2="788" y2="260" class="line accent" marker-end="url(#d1b)"/>
  <text x="763" y="284" text-anchor="middle" class="accent-text small">reviewed PR merge, seen by sync</text>
  <!-- rollups -->
  <polyline points="833,247 833,214 693,214 693,175" class="line dashed" marker-end="url(#d1a)"/>
  <text x="765" y="208" text-anchor="middle" class="small muted">rollup over tasks</text>
  <polyline points="693,147 693,114 483,114 483,85" class="line dashed" marker-end="url(#d1a)"/>
  <text x="590" y="108" text-anchor="middle" class="small muted">rollup over features</text>
  <text x="12" y="322" class="small muted">Review grain moves PR ownership, not lifecycle states.</text>
  <text x="12" y="340" class="small muted">Dashed arrows remain rollups; magenta marks human crossings.</text>
</svg>
<figcaption>The lifecycle ribbon stays task-shaped at every review grain. Tasks enter review one by one; the configured task, feature, or epic owns the PR whose merge lets sync record their crossings.</figcaption>
</figure>

## Review grain

Review grain decides where the human reviews work, without changing how tasks get planned, claimed, checked, or recorded. The default is `task`: one PR per task. At `feature` grain every task still works on its own branch, the agent merges each submitted task into a shared feature review branch, and one PR opens once every live task in the feature reaches `in_review` or `done`. `epic` repeats that shape across the features riding one shared epic branch. Bugs, chores, and tweaks ignore the setting; each always owns its own PR.

| Resolved grain | PR represents | Execute stops when |
|---|---|---|
| `task` | one task | that task reaches `in_review` with its own PR or local review reference, and its state commits are published when a remote exists |
| `feature` | one feature | every live task is `in_review` or `done`, feature gates are green, and the feature review and state commits are recorded and published |
| `epic` | one epic | every riding feature's live tasks are `in_review` or `done`, its gates are green, independent overrides are done, and the epic review and state commits are recorded and published |

Green checks are intermediate. The agent commits and pushes, creates or reuses the PR, records its number with `tiller submit`, and pushes the submission state. Tiller records these facts but never pushes or opens a PR itself. A project with no remote records a local review reference instead.

The setting can live at three homes, and the most specific explicit value wins:

```sh
tiller configure project --review-grain feature
tiller configure epic foundation --review-grain epic
tiller configure feature foundation/auth --review-grain task
```

A feature can only say `task` or `feature`; epic participation comes from inheritance. Choose the grain before work starts: Tiller refuses a change that would reinterpret submitted or started work. The [review grain contract](/tiller/spec/cli-surface/#review-grain) carries branch assembly, every refusal, and recovery of an aggregate review that merged before it was recorded.

## Who does what

Three acts, three actors, and tiller keeps them apart on purpose. The agent **reasons**: it reads the inputs and the tree, writes the prose, does the work, runs the gates, and proposes. Tiller **records**: every conclusion lands through a command into the state files and the event stream, illegal moves get refused, and the next move gets named. When an agent skill finishes a run, `tiller readout <skill>` prints its readout, a fixed summary of what happened, what was verified, and what needs the human, which the agent reproduces verbatim. The human **approves**: the tree, each plan, the review gates, the answers to questions, and the merge.

Tiller can't make an agent follow the process and doesn't pretend to. On one machine the agent and the human share a filesystem, a shell, and git credentials, so any check that tried to stop the agent from running a human-only verb would be theater. The human-only gates are guidance the skills and `orientation.md` ask the agent to honor. The one boundary that actually holds is the merge, and only a human performs it. The developer drives, and the developer answers for the result.

<figure class="diagram-figure">
<svg class="diagram" viewBox="0 0 960 300" role="img" aria-label="Three columns: the agent reasons, tiller records, the human approves. The agent drives state through tiller commands; tiller tells the human what needs them; the human approves back through tiller and merges in git; tiller observes the merge.">
  <defs>
    <marker id="d3a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 L10 5 L0 10 z" class="arrow"/></marker>
    <marker id="d3b" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 L10 5 L0 10 z" class="arrow-accent"/></marker>
  </defs>
  <rect x="20" y="30" width="230" height="160" rx="6" class="box"/>
  <text x="36" y="56" class="h">The agent reasons</text>
  <text x="36" y="84" class="small">reads the inputs and the tree</text>
  <text x="36" y="104" class="small">writes specs, contracts, tasks</text>
  <text x="36" y="124" class="small">does the work, runs the gates</text>
  <text x="36" y="144" class="small">proposes, stops at each gate</text>
  <text x="36" y="170" class="small muted">never approves or marks done</text>
  <rect x="365" y="30" width="230" height="160" rx="6" class="box"/>
  <text x="381" y="56" class="h">Tiller records</text>
  <text x="381" y="84" class="small">state, events, gate results</text>
  <text x="381" y="104" class="small">refuses illegal moves, says why</text>
  <text x="381" y="124" class="small">names the next move</text>
  <text x="381" y="144" class="small">provides live status, orientation</text>
  <text x="381" y="170" class="small muted">never does the reasoning</text>
  <rect x="710" y="30" width="230" height="160" rx="6" class="box"/>
  <text x="726" y="56" class="h">The human approves</text>
  <text x="726" y="84" class="small">the tree, then each plan</text>
  <text x="726" y="104" class="small">answers questions, attests</text>
  <text x="726" y="124" class="small">paces which feature is next</text>
  <text x="726" y="144" class="small">merges the PR</text>
  <text x="726" y="170" class="small muted">the one boundary that holds</text>
  <line x1="250" y1="108" x2="363" y2="108" class="line" marker-end="url(#d3a)"/>
  <text x="307" y="78" text-anchor="middle" class="mono muted">add · spec</text>
  <text x="307" y="96" text-anchor="middle" class="mono muted">start · check</text>
  <text x="307" y="128" text-anchor="middle" class="mono muted">submit</text>
  <line x1="595" y1="90" x2="708" y2="90" class="line" marker-end="url(#d3a)"/>
  <text x="652" y="70" text-anchor="middle" class="mono muted">queue · readout</text>
  <text x="652" y="110" text-anchor="middle" class="mono muted">serve</text>
  <line x1="708" y1="150" x2="597" y2="150" class="line accent" marker-end="url(#d3b)"/>
  <text x="652" y="170" text-anchor="middle" class="mono accent-text">approve</text>
  <text x="652" y="186" text-anchor="middle" class="mono accent-text">attest · answer</text>
  <rect x="365" y="236" width="230" height="44" rx="6" class="recess"/>
  <text x="480" y="263" text-anchor="middle" class="mono">git: branches, PRs, the merge</text>
  <polyline points="135,190 135,258 363,258" class="line" marker-end="url(#d3a)"/>
  <text x="145" y="250" class="small muted">commits, opens the PR</text>
  <polyline points="825,190 825,258 597,258" class="line accent" marker-end="url(#d3b)"/>
  <text x="815" y="250" text-anchor="end" class="small accent-text">merges</text>
  <line x1="480" y1="236" x2="480" y2="192" class="line dashed" marker-end="url(#d3a)"/>
  <text x="490" y="218" class="small muted">sync observes the merge</text>
</svg>
<figcaption>Reasoning, recording, approving: the agent drives state through tiller, tiller tells the human what needs them, the human approves through tiller and merges in git, and tiller records done by observing the merge.</figcaption>
</figure>

`tiller status <noun> <node>` names the legal next moves from any state, with nothing executed. A command that refuses says why and what to do about it. Exit `1` is a business answer (a red gate, an illegal move), `2` a bad call, `3` a broken environment.

## Contributors and claims

Identity lives in the repo. A committed roster in `.tiller/tiller.yml` says who may hold work, and `tiller add contributor` and `tiller remove contributor`, deliberate human acts, maintain it. A gitignored `.tiller/identity.yml`, written by `tiller join --as <id>`, says who this checkout is. There's no global config and no machine default; a fresh clone carries the roster and runs its own `join`.

New tasks start unclaimed, no matter who creates or approves them. `tiller claim task <key>` reserves one task for you, and bugs, chores, and tweaks have the same claim verbs. Before the work starts, `tiller release` returns your reservation to the unclaimed pool, and `tiller reassign`, a human decision, moves it to someone else. Taking over another person's reservation with `tiller claim` needs `--from` naming its current holder and an explicit human handoff. Switching your checkout's identity to grab work is never the way to do it.

The first start fixes the owner for good. After that, nothing reopens ownership: not resuming, checks, review, cancellation, or completion. A task that is cancelled before it ever starts keeps the owner it had, too. Features and epics have no owner and no claim, and a claim on a task never spreads to its parent or siblings.

The first task start in a ready feature makes the feature active and records the branch the task works on, plus the shared review branch when the review grain calls for one. Dependencies, base branches, open questions, retired ancestors, and review-round guards still apply at start. When the human says a feature can go ahead despite an open question, `tiller approve feature <ref> --assume-question <Q-id> --reason <text>` records that assumption for the feature only. It doesn't move the feature's state, and its tasks don't silently inherit it. Recorded identity is workflow discipline, not authentication: it says who directed an action, and judgment verbs keep their own rules about which ones need a human.

A feature or epic lists its contributors instead of an owner. A contributor is anyone who owns a started task beneath it, including tasks that are done, cancelled, or transferred, and tasks under a cancelled feature. Holding an unstarted reservation, reviewing, creating work, or owning an attached bug, chore, or tweak doesn't make someone a contributor. `tiller ls features --contributor <id>` finds where a person has contributed, and `tiller ls tasks --owner <id>` finds what is assigned to them. A person's current work is every unfinished assignment they hold, whether it is proposed, blocked, checked, or in review, not everything under a feature they once worked on. Shared closeout work, such as the final feature check after the last task lands, shows up once per feature or epic rather than once per contributor. When several people share one feature or epic PR, agree among yourselves which one updates it at a time. A suggested route is a suggestion, not permission.

### Exceptional transfer

Handing started, unfinished work to someone else is a human decision. The agent runs `tiller transfer task|bug|chore|tweak <ref> --to <contributor-id> --reason <text>` only on an explicit instruction that names the work and the recipient. Before start, use `reassign` instead.

Transfer doesn't relabel the original. It cancels the original and keeps its prose, attribution, branch, review, and evidence as history. Then it creates a replacement of the same kind with a new ID and key, claimed by the recipient, approved and ready but not started, with a fresh accepted basis and links in both directions between the two. The replacement inherits no branch, review, passing checks, attestations, or evidence. The recipient starts it with the command transfer prints, and carries the implementation over with ordinary Git.

The approved scope has to be unchanged. If the work needs a different specification, that goes through human review or an amendment, and there is no flag to skip it. Transfer refuses when the original already belongs to the recipient, when the work is already finished, or when a started, unfinished task depends on it. Unstarted dependents are pointed at the replacement, links to finished work stay as history, and questions, pauses, attachments, and carrier links keep working.

Tiller doesn't touch Git refs or the code host. At task, bug, chore, or tweak review grain, close the old PR without merging it and open a new one. At feature or epic grain, keep the unmerged shared PR, update it with the replacement, and rerun the affected checks and acceptance. Commits and approvals already on that PR don't disappear. If the original's review has already merged, run `tiller sync` and handle the rest as forward work instead.

## Gates and evidence

Required gates are `build`, `tests`, and `evals`. They're command-backed, configured in `tiller.yml`, and they block done. Optional review gates (code, security, performance, accessibility) are attestation-backed and non-blocking, visible encouragement rather than enforcement. `tiller check` runs the required gates for a node and records the result on it, with the revision it ran against. `tiller eval` runs the acceptance contract, and `tiller attest` records a human's review. A gate result goes `stale` when the thing it verified moves, so last week's green can't quietly stand in for this week's.

## Corrections, drift and amendments

Approval creates an accepted basis and structural history. It doesn't freeze cosmetic titles, epic narrative, additive child work, soft order, or same-scope prose on a non-terminal feature or task. The route depends on what the correction changes:

| Route | Use it for | What happens next |
|---|---|---|
| **Direct** | A display title at any state, approved epic narrative, one more task under an approved feature, soft order, or a dependency edge before affected work starts | Run the mapped command or edit human-owned `epic.md`. An additive task gets its own task approval; the other direct changes need no new gate. |
| **Refresh** | Same-scope accepted prose on a non-terminal feature or task | Edit the Markdown, review the `active_basis` drift, then a human runs `approve --refresh-basis`. State does not move; the accepted bytes do. A wrong fact found while executing a task is instead a departure recorded in its commit and review, with `task.md` unedited. |
| **Return and reapprove** | A ready, unstarted feature or task plan whose original approval should be replayed | `return` moves it back across the pre-work gate, the plan is revised, and the ordinary approval happens again. |
| **Amend** | Genuine replacement, cancellation, or a supported structural batch | Stage supported operations with preconditions and stop for amendment approval. Done or cancelled feature and task prose stays historical; corrections move forward through additive or replacement work with supersession where appropriate. |

Amendment cannot link, unlink, or order. `revise` re-pins non-terminal prose, not dependencies; it does not unlock started work. An in-place hard-edge edit on unchanged started work remains unsupported, even where a refusal hints at amendment. A genuine replacement can carry prerequisites on fresh eligible work, never a replacement manufactured just to evade a lock. A fresh task under an active feature can still receive legal sibling prerequisites.

An unchanged, unstarted feature can move to another epic through a reviewed `move-feature` amendment that keeps its identity, and features from different epics may interleave when their real prerequisites allow it. [How skills route correction feedback](/tiller/skills/#how-skills-route-correction-feedback) has the eligibility rules for a move and what an epic ordering cycle means.

An amendment may stage an epic and then its features in one batch. A new feature's tasks follow materialization and ordinary specification. Approval appends new epics to root soft order, not to whatever position the narrative suggests; intended placement needs a separate explicit `tiller order`, whose supplied sibling list replaces the prior list and may be partial. Post-approval hard links remain legal only while the dependent is eligible. A planning request authorizes the proposal, not its approval or an unsolicited broader replan.

```sh
# direct display correction, at any state
tiller rename feature foundation/app-scaffold --title "Application scaffold"

# direct additive work under an approved feature, with its own gate
tiller add task foundation/app-scaffold/document-setup --title "Document local setup"
tiller approve task foundation/app-scaffold/document-setup

# accept corrected same-scope prose without moving state
tiller approve feature foundation/app-scaffold --refresh-basis

# replay a ready plan's approval before work starts
tiller return task foundation/app-scaffold/document-setup --reason "revise the setup contract"

# a post-start structural correction is reviewed history
tiller add amendment app-scaffold-reshape --title "Reshape application setup"
```

Before approval, prose feedback re-enters the proposal seam. `decompose` can rename a draft, task-free feature's key and move it between draft epics, record eligible hard-edge corrections, or remove unwanted proposal nodes before returning to the epic gate. `specify` revises feature plans and proposed tasks at the feature gate. Unchanged feature relocation outside the direct rename window, including a specced plan, uses the reviewed move route, not cancellation and recreation. `tiller remove` is permanent proposal deletion, not cancellation, and it accepts only a proposed task, a task-free draft feature, or a task-free draft epic subtree.

Approving a feature plan pins its spec and contract, and each task's spec, as `accepted_against` revisions. Edit one afterwards and tiller reports `active_basis`, and `start`, `check`, and `submit` refuse until a human follows the refresh, return, or amendment route above. `tiller drift` sweeps the other markers too: a done node whose spec or code surface no longer matches what was verified (`stale_spec`), a plan made under an assumption whose answer has since arrived (`answered_question`), or a recorded surface or eval reference that no longer exists. Detection is tiller's, the proposed fix is the agent's through `mend-drift`, and approval is the human's.

`tiller intake` starts the amendment path from an external review or field report. It lands the document under `.tiller/feedback/` and opens the proposed amendment it implies. The agent still classifies each finding first, so lightweight corrections stay direct and only the structural casualties enter the batch.

## Bugs, chores, and tweaks

Choose a **Bug** for a defect: observed behavior violates an agreed expectation. Choose a **Chore** for housekeeping without code side effects, such as documentation or administrative upkeep. Choose a **Tweak** for a minor adjustment that may change code. Choose a **Task** under a feature for substantive planned work. These are choice guidelines, not file-diff restrictions: Tiller does not enforce a no-code rule for Chores.

For example, changing a completed button from "Save" to "Save changes" to improve the wording is a Tweak, even when the label lives in application code. If the button text violates its agreed requirement, record a Bug instead. A new button or a broader interaction redesign belongs in planned Task work.

Keep changes forward-only. Never reopen or rewrite done work to express a new request. Add a new Bug, Chore, Tweak, or Task with attachment or source context as appropriate; the completed record stays history. Adding planned descendants may recompute feature and epic rollups, but it does not reopen merge-backed done execution nodes.

Bugs, Chores, and Tweaks are leaf work nodes beside the tree, at `.tiller/bugs/<key>/`, `.tiller/chores/<key>/`, and `.tiller/tweaks/<key>/`. None has parents or children. Bug contracts describe reproduction, expected and actual behavior, and fixed-when. Chore and Tweak contracts describe what, why, and done-when. A Tweak uses `tweak.md`, `tweak.yml`, a canonical `tweaks/<key>` address, and a `TW-` ID.

Under the surface bugs, chores, and tweaks carry the task's own lifecycle: `proposed` (captured, awaiting triage), `ready` (triaged by a human's approval), `in_progress`, `check`, `in_review`, `done`. Each carries its own branch and its own PR, so a shelf node reaches `done` the way a task does. A human merges, `tiller sync` observes it, and nobody marks it by hand.

`attached_to` records where a bug, chore, or tweak was found (an epic, feature, or task id) and it's provenance only, reading context for the agent, never a parent and never a gate. `resolved_by` is a different pointer, the tree work whose merge actually fixes the shelf node, set with `tiller resolve`. A resolved node reads the derived phase `carried`. It keeps its own stored state, drops out of the queue, and moves with its carrier, crossing to `done` with `done_basis: carried` in the same run that records the carrier's own `done`.

`capture` is the agent skill for this seam. Given an investigation or a developer's description, it writes one complete proposed contract, with source, attachment, and shelf dependencies where they apply, commits it, and stops. It never approves, claims, starts, resolves, or cancels the node it just wrote. Triage stays a human act. `tiller approve bug`, `tiller approve chore`, or `tiller approve tweak` is what moves a captured contract from `proposed` to `ready`, the same way `approve feature` is the plan gate for a feature.

Once claimed, a bug, chore, or tweak ranks like any other owned work. The reader's own ready or in-progress tasks come first, then their owned bugs, chores, and tweaks in shelf order. An unclaimed bug, chore, or tweak is never a ranked next move, and neither is a `carried` one, since its motion belongs to its carrier. The console's Fix lens is the board for all of it, six columns in task vocabulary, Proposed through Closed.

```sh
tiller add bug blank-on-reload --title "Form blanks on reload" --attach foundation/app-scaffold
tiller add tweak button-text --title "Clarify save button text" --attach foundation/app-scaffold
# Complete .tiller/tweaks/button-text/tweak.md, then await human triage.
tiller approve bug blank-on-reload
tiller claim bug blank-on-reload
tiller start bug blank-on-reload
tiller check bug blank-on-reload
tiller submit bug blank-on-reload --pr 51
# ... a human reviews and merges the PR ...
tiller sync
```

## Existing-code baseline

A brownfield or legacy-migration project starts from code that predates Tiller, so routing sends
the agent to `derive-baseline` first: it writes as-is design docs for the existing code and records
what each one describes. A human confirms each doc with `tiller approve design`, and ordinary
decomposition and work routing resume once one doc is confirmed. After that, drift watches the
confirmed docs against the code they describe, and `mend-drift` proposes the prose repair. The
[skills page](/tiller/skills/#establish-an-existing-code-baseline) covers what the skill does, and the
[artifact schema](/tiller/spec/artifact-schema/#baseline-ledger-designbaselineyml) defines the ledger.

## Managed-project independence

Tiller manages the development workflow, not the project's ability to run. Setup, build, tests, CI, packaging, deployment, runtime, and merge eligibility must work without Tiller installed, access to its releases, or `.tiller/` operational or state-validation inputs. This extends the same independence the harness has to the managed project. Even optional Tiller CI jobs are out of bounds.

Local Tiller may run an independently runnable command such as `bin/ci` and record its result, but `bin/ci` must not call Tiller. Planning and evidence can stay committed in `.tiller/` without becoming application or CI gate inputs. Planning-only CI uses ordinary repository tooling and runs full project checks whenever classification is uncertain, never a blanket planning-path skip or replacement Tiller validation.

Tiller's own implementation, tests, releases, and disposable verification are exempt. The [distribution contract](/tiller/spec/distribution/#managed-project-independence) owns the detailed boundary and conservative planning-only CI pattern.

## The surfaces

Tiller is one static binary with the console's assets and the skill text embedded, so the CLI, the console, the generated orientation, and the skills always agree with each other. It reads and writes `.tiller/` in your repository, under git, next to the code. A coding agent reaches it three ways that are really one: the CLI, the skills served by `tiller guide`, and the committed `.tiller/orientation.md`. `tiller join` wires a harness by writing files it already watches (a managed block in `AGENTS.md`, a redirect in `CLAUDE.md`, thin skill shims), and the dependency arrow points the safe way. The harness never depends on tiller. Remove tiller and the harness doesn't notice.

The console, `tiller serve`, is a small set of lenses over that same tree: Home (the desk, what needs you and what's in flight), Explore (browse, filter, search, and a map of the whole project with its dependency arcs), Build (the board), Fix (the shelf board), Report (numbers and charts on how the project is going over time), History (what changed since you last looked), and the node page each of those opens into. [The console](/tiller/console/) has the full tour.

<figure class="diagram-figure">
<svg class="diagram" viewBox="0 0 980 390" role="img" aria-label="Three columns: the tiller binary with its CLI, skills, and console; the repository with .tiller under git and the files join wrote; the coding agent's harness that reads those files and runs the CLI. The harness never depends on tiller.">
  <defs><marker id="d4a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 L10 5 L0 10 z" class="arrow"/></marker></defs>
  <rect x="20" y="40" width="240" height="200" rx="6" class="box"/>
  <text x="36" y="66" class="h">tiller, one static binary</text>
  <rect x="36" y="84" width="208" height="34" rx="3" class="recess"/><text x="48" y="106" class="mono">tiller &lt;command&gt;</text><text x="232" y="106" text-anchor="end" class="small muted">CLI</text>
  <rect x="36" y="128" width="208" height="34" rx="3" class="recess"/><text x="48" y="150" class="mono">tiller guide &lt;skill&gt;</text><text x="232" y="150" text-anchor="end" class="small muted">skills</text>
  <rect x="36" y="172" width="208" height="34" rx="3" class="recess"/><text x="48" y="194" class="mono">tiller serve</text><text x="232" y="194" text-anchor="end" class="small muted">console</text>
  <text x="36" y="228" class="small muted">console and skills embedded</text>
  <rect x="330" y="20" width="330" height="330" rx="6" class="box"/>
  <text x="346" y="46" class="h">your repository, under git</text>
  <rect x="346" y="62" width="298" height="168" rx="3" class="recess"/>
  <text x="358" y="84" class="mono">.tiller/</text>
  <text x="370" y="104" class="mono small">tiller.yml</text><text x="632" y="104" text-anchor="end" class="small muted">project, roster, gates</text>
  <text x="370" y="122" class="mono small">epics/&lt;key&gt;/epic.md + .yml</text>
  <text x="370" y="140" class="mono small">features/&lt;key&gt;/feature.md,</text>
  <text x="370" y="158" class="mono small">  feature.eval.md, feature.yml</text>
  <text x="370" y="176" class="mono small">tasks/&lt;key&gt;/task.md + .yml</text>
  <text x="370" y="194" class="mono small">events/ · feedback/ · design/</text>
  <text x="370" y="212" class="mono small">orientation.md</text><text x="632" y="212" text-anchor="end" class="small muted">generated</text>
  <text x="370" y="228" class="mono small">identity.yml</text><text x="632" y="228" text-anchor="end" class="small muted">gitignored</text>
  <rect x="346" y="244" width="298" height="92" rx="3" class="recess"/>
  <text x="358" y="264" class="small">written by <tspan class="mono">tiller join</tspan>:</text>
  <text x="370" y="284" class="mono small">AGENTS.md</text><text x="632" y="284" text-anchor="end" class="small muted">managed pointer block</text>
  <text x="370" y="302" class="mono small">CLAUDE.md</text><text x="632" y="302" text-anchor="end" class="small muted">a redirect to it</text>
  <text x="370" y="320" class="mono small">.claude/skills/tiller-*</text><text x="632" y="320" text-anchor="end" class="small muted">shims</text>
  <rect x="720" y="40" width="240" height="140" rx="6" class="box"/>
  <text x="736" y="66" class="h">the agent's harness</text>
  <text x="736" y="92" class="small">reads the pointer block, then</text>
  <text x="736" y="110" class="small">orientation.md; discovers the</text>
  <text x="736" y="128" class="small">skill shims; runs the tiller CLI</text>
  <text x="736" y="160" class="small muted">never depends on tiller</text>
  <line x1="260" y1="146" x2="328" y2="146" class="line" marker-end="url(#d4a)"/>
  <text x="294" y="134" text-anchor="middle" class="small muted">reads and</text>
  <text x="294" y="164" text-anchor="middle" class="small muted">writes state</text>
  <polyline points="660,290 690,290 690,110 718,110" class="line" marker-end="url(#d4a)"/>
  <text x="698" y="206" class="small muted">watches</text>
  <polyline points="840,180 840,372 140,372 140,242" class="line dashed" marker-end="url(#d4a)"/>
  <text x="490" y="362" text-anchor="middle" class="small muted">runs tiller in the repository, like any other tool</text>
</svg>
<figcaption>The surface map: one binary embedding the CLI, the skills, and the console; the repository carrying .tiller under git and the files join wrote; the harness reading what it already watches and running the CLI. The dependency arrow points the safe way: the harness never depends on tiller. The human's surfaces, the console and the merge, are in the figure above.</figcaption>
</figure>

## Getting a project onto tiller

Install once per machine, `init` once per repository, `add contributor` once per person, `join` once per checkout (each Git worktree included), then hand the repository to your agent to decompose and approve the tree it proposes. The [Quickstart](/tiller/quickstart/) walks it with real output.

The diagram below is the greenfield path. Brownfield and legacy-migration insert `derive-baseline`
and human confirmation after join and before ordinary decomposition; see
[Existing-code baseline](#existing-code-baseline).

After onboarding, `tiller upgrade` keeps the binary and the current checkout's schema and generated
guidance current, and `tiller upgrade --project-only` maintains a checkout offline with the running
binary. `join` selects identity; it is not routine maintenance. The
[Quickstart](/tiller/quickstart/#keep-a-checkout-current) has the commands.

<figure class="diagram-figure">
<svg class="diagram" viewBox="0 0 960 120" role="img" aria-label="Six steps left to right: install, tiller init, tiller add contributor, tiller join, decompose, tiller serve, with who does each underneath.">
  <defs><marker id="d5a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 L10 5 L0 10 z" class="arrow"/></marker></defs>
  <rect x="20" y="28" width="130" height="34" rx="17" class="pill"/><text x="85" y="49" text-anchor="middle" class="mono">install</text>
  <rect x="175" y="28" width="130" height="34" rx="17" class="pill"/><text x="240" y="49" text-anchor="middle" class="mono">tiller init</text>
  <rect x="330" y="28" width="130" height="34" rx="17" class="pill"/><text x="395" y="49" text-anchor="middle" class="mono" font-size="10">tiller add contributor</text>
  <rect x="485" y="28" width="130" height="34" rx="17" class="pill"/><text x="550" y="49" text-anchor="middle" class="mono">tiller join</text>
  <rect x="640" y="28" width="130" height="34" rx="17" class="pill"/><text x="705" y="49" text-anchor="middle" class="mono">decompose</text>
  <rect x="795" y="28" width="130" height="34" rx="17" class="pill"/><text x="860" y="49" text-anchor="middle" class="mono">tiller serve</text>
  <line x1="150" y1="45" x2="173" y2="45" class="line" marker-end="url(#d5a)"/>
  <line x1="305" y1="45" x2="328" y2="45" class="line" marker-end="url(#d5a)"/>
  <line x1="460" y1="45" x2="483" y2="45" class="line" marker-end="url(#d5a)"/>
  <line x1="615" y1="45" x2="638" y2="45" class="line" marker-end="url(#d5a)"/>
  <line x1="770" y1="45" x2="793" y2="45" class="line" marker-end="url(#d5a)"/>
  <text x="85" y="84" text-anchor="middle" class="small muted">you, once per machine</text>
  <text x="240" y="84" text-anchor="middle" class="small muted">you, once per repo</text>
  <text x="395" y="84" text-anchor="middle" class="small muted">you, once per person</text>
  <text x="550" y="84" text-anchor="middle" class="small muted">you, once per checkout</text>
  <text x="705" y="84" text-anchor="middle" class="small muted">your agent proposes,</text>
  <text x="705" y="100" text-anchor="middle" class="small accent-text">you approve the epics</text>
  <text x="860" y="84" text-anchor="middle" class="small muted">you, any time</text>
</svg>
<figcaption>Onboarding: install, init, add contributor, join, decompose, serve. Productive in minutes, and the agent's first proposal is the human's first decision.</figcaption>
</figure>

## Where things live

```
.tiller/
  tiller.yml            project, contributors roster, gates, schema version
  orientation.md        generated; the agent's front door (committed, do not hand-edit)
  identity.yml          who this checkout is (gitignored)
  local.yml             your own preferences for this checkout (optional, gitignored)
  design/               your input documents
  feedback/             documents landed by tiller intake
  amendments/           reviewed, recorded changes to the plan
  decisions/            open questions and architecture decision records
  reviews/              code-review files written by the code-review skill
  events/YYYY/MM/DD/    one immutable event per file
  epics/<key>/
    epic.md  epic.yml
    features/<key>/
      feature.md  feature.eval.md  feature.yml
      tasks/<key>/
        task.md  task.yml
  bugs/<key>/
    bug.md  bug.yml
  chores/<key>/
    chore.md  chore.yml
  tweaks/<key>/
    tweak.md  tweak.yml
```

The design documents under [Design](/tiller/spec/artifact-schema/) carry every field, state, and rule in full. This page is the shape of it.

## Glossary

- **Accepted basis**: the revision of a node's prose a human approved, pinned at the gate; an edit afterward is drift.
- **Amendment**: a reviewed batch of structural changes to approved work, applied only on human approval.
- **Attestation**: a human's recorded review on an optional review gate, through `tiller attest`.
- **Bug**: shelf work for a defect, where observed behavior violates an agreed expectation.
- **Carried**: a shelf node resolved by tree work; it reaches done when its carrier does.
- **Chore**: shelf work for housekeeping, such as documentation or administrative upkeep.
- **Claim**: a reservation of one ready task or shelf node; the first start fixes its owner.
- **Contributor**: a person on the committed roster; on a feature or epic, an owner of one of its started tasks.
- **Drift**: a recorded basis, surface, or assumption that no longer matches the files; `tiller drift` lists it.
- **Epic**: a bounded delivery increment, written as a narrative, approved at the epic gate.
- **Feature**: an acceptable outcome under an epic, with a specification and an acceptance contract.
- **Gate**: a check recorded on a node; required gates block done, review gates do not.
- **Orientation**: the generated `.tiller/orientation.md`, the first page an agent reads.
- **Question**: an open decision recorded as a node, which can hold the work it names.
- **Readout**: the fixed summary `tiller readout <skill>` prints at the end of an agent skill's run.
- **Review grain**: where the PR sits for tree work: the task (default), the feature, or the epic.
- **Rollup**: how features and epics reach done, from their children's done plus their own gates.
- **Shelf**: a flat, ordered list of one kind of leaf work beside the tree: bugs, chores, or tweaks.
- **Skill**: guidance for one agent seam, served by `tiller guide <skill>`.
- **Sync**: `tiller sync`, which observes merges in git and records the done crossings they imply.
- **Task**: the unit of implementation under a feature, worked on its own branch.
- **Tweak**: shelf work for a minor adjustment that may change code.
