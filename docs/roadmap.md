# Roadmap

What Tiller does today, what is designed but not built, and what it deliberately leaves out. The [specs](spec/) describe some surfaces ahead of the code; this page is how to tell which. The [design history](design-history.md) explains why the built parts took their shape.

## Built

- **Work and gates.** One epic, feature, and task tree with just-in-time feature planning. Independent bugs, chores, and tweaks. Required command and eval gates, optional human reviews, and review grain at task, feature, or epic. A human merge backs task and shelf completion; feature and epic completion are recomputed.
- **Contributors and routing.** A registered contributor roster, per-checkout identity, claims and guarded transfer of tasks and shelf items, question holds, hard dependencies and soft order, and the ranked list of next legal moves (`tiller next`, guide, and every readout).
- **Questions.** Recorded questions that hold plans and work, answered or cancelled by the human, with links that widen or narrow what they block.
- **Plan changes.** Reviewed amendments and sourced intake, revision, cancellation, and supersession lineage, eval-binding corrections on done features, and identity-preserving moves of never-started feature subtrees.
- **Existing code and drift.** Code-root and reference protection, baseline derivation and confirmation, work-node and baseline drift, and read-only repair guidance.
- **Agent guidance.** Embedded skills for every stage of the workflow, a generated orientation file, gitignored skill shims written by `join`, and deterministic lifecycle readouts.
- **Console.** A read-only local console (`tiller serve`) with Home, Build, Fix, Explore (tree, map, search, filters, relationships), Report, History, and node detail. Refresh is manual or on navigation.
- **Storage and distribution.** Journaled, lock-guarded mutation batches, schema migrations, checksum-verified `tiller upgrade`, release automation, an installer, a Homebrew cask, and the documentation site.

## Not built yet

These are retained scope, not a promised sequence.

- **Console writes and live updates.** Write actions that map one-to-one onto CLI commands, an explicit state-commit action, a filesystem watcher or polling refresh, separate tree and host freshness, and a general uncommitted-state indicator. Report export (print, Markdown, static HTML) and an executive summary. Map filters and export, and layouts that hold up at narrow widths.
- **CLI commands.** `doctor`, `block` and `unblock`, `commit state`, and `add design`, `add adr`, and `add evidence`. [The command reference](tiller-commands.md) marks which verbs ship.
- **Distribution.** A launcher with a per-project engine cache and version pin, `upgrade --project`, update nudges, more harness discovery adapters, and lossless preservation of unknown YAML fields.
- **Baseline coverage.** Automatic baseline routing for uncovered code roots, accepted-basis drift sweeps for shelf items, and completion fingerprints for submodules.
- **Amendments and intake.** Automatic splitting of large intakes into several proposals, staged `order` and `link` operations, and the remaining [amendment input validation](spec/artifact-schema.md#deferred-amendment-input-validation).
- **Search and reads.** Ranked prose search and Explore filters by activity phase or unowned work.
- **Hardening questions.** Repair of merge-produced duplicate ids, evidence garbage collection, and bulk reaffirmation of stale criteria. These are open questions, not known regressions.
- **Guided discovery.** Tiller covers execution, from specs and a plan to merged and done. Covering discovery, from the first meeting to a plan, is a later goal.

## Out of scope

- **An MCP server or vendor integration.** The agent surface is the CLI, files, and skills.
- **Orchestrating agents or running review tools.** Tiller records gates; the agent reasons and the human decides.
- **A CI emitter or any Tiller step in a managed project's CI.** Managed projects never depend on Tiller.
- **Global, user, or machine configuration.** Everything lives in the repo.
- **A model call inside Tiller.** Ranking, routing, and readouts are deterministic.
