# AGENTS.md

Orientation for any coding agent or contributor working in this repo. Read this first. `CLAUDE.md` points here, and so should any future harness-specific file. As real codebases land under this repo, they get their own sub-`AGENTS.md` with the specifics. This root file stays about Tiller as a whole.

## What Tiller is

Tiller is an opinionated SDLC workflow tool for humans and AI coding agents working together. It organizes what a software project needs to stay coherent: specifications, decomposition, planning, execution tracking, and documentation that stays in agreement with the code. The whole thing rests on one conviction: design, planning, specs, and docs should live with the code, in the repo, under git, not in a separate database or a tool you tab away to.

Tiller is designed around three surfaces. A CLI and a set of skills plug into a coding agent's harness (Claude Code, Codex, Pi, and the like). A lightweight local web UI is the human-in-the-loop surface for watching progress, resolving issues, and exercising the gates.

## What we're building now

v1 targets the execution half of a project: the stretch from "we have specs and a plan" through "it's merged and done." The longer aim is continuous coverage from the first discovery meeting all the way to production deployment, but v1 scopes deliberately to execution and leaves guided discovery for a later version.

The five specs in `docs/spec/` describe the on-disk model, CLI, skills, console, and distribution. Read the relevant spec before working in an area it covers. It explains the current contract and marks designs that are not implemented; read it for context, not for permission. The [roadmap](docs/roadmap.md) says what is built, what is designed but not built, and what is out of scope.

## How MiT uses it

Made In Tandem uses Tiller to run human-plus-AI delivery, with several engineers and their agents working in parallel while a human stays product owner, architect, reviewer, and acceptance tester.

## Guiding principles

These are the calls already made. They're the spine of the design. Weigh new decisions against them.

1. **Tiller holds the structure, the gates, and the state. The agent does the reasoning. The human approves at the gates.** This is the test for whether a capability belongs in Tiller at all. If it asks Tiller to reason, or to orchestrate someone else's tool, it probably doesn't belong.
2. **Files are the source of truth.** Markdown for prose, YAML for state, git as the spine. No database. The UI reads the working tree.
3. **Prose and state never share a file.** A node's `.md` is human-owned intent and acceptance criteria; its `.yml` is machine-mutated status, ownership, and gate results. The split is what lets the CLI churn state without touching a human's words, and what keeps two people's work from colliding.
4. **Tiller is a gatekeeper, not an orchestrator.** It declares and records gates; it never runs the review tooling or the agent's reasoning. Required gates (build, tests, evals) block done. Optional review gates (code, security, performance, accessibility) are visible but non-blocking: encouragement, not enforcement.
5. **The done gate is the PR merge.** An agent can drive work to the edge of done and turn every gate green. Only a human merge crosses to done. An agent never marks accepted and never marks done.
6. **One tree, progressively elaborated.** Work is a single hierarchy: epics, then features, then tasks. Epics and features express bounded product or platform outcomes, not permanent subsystem buckets; related capabilities can recur in later delivery increments. Each level carries more concrete prose than the one above it: an epic is narrative, a feature is narrative plus specification, a task carries the implementation detail. There is no separate effort or sequencing layer. The lifecycle states are the queue, and the human paces the work by choosing which feature moves next.
7. **Brownfield is the general case; greenfield is the degenerate one.** Existing code is input to understand, never a convention to inherit. `project.kind` selects how existing code is treated: a mutable surface, or read-only reference.
8. **Decomposition is Tiller's output, not a required input shape.** Tiller takes generic input docs, runs a readiness gate, and proposes the epic and feature tree for a human to approve. Decomposition runs in two stages: the project decomposes into epics and features with their narratives, and each feature is specified and broken into tasks just in time when its turn comes, not all up front.
9. **Drift control is active assist.** Tiller detects suspected drift between specs, docs, tests, evals, and code, proposes the fix, and the human approves it. Detect, propose, approve.
10. **The agent surface is CLI plus skills, living inside the harness.** No MCP in v1. A generated `.tiller/orientation.md`, committed with the repo, is the zero-dependency front door that points an agent at both. The human-in-the-loop UI is a local web server (`tiller serve`), read-mostly, with every write action mapping one-to-one to a CLI command.
11. **Model and harness agnostic.** The agent surface is identical no matter who drives it. Claude Code, Codex, and a local open-weights model all see the same CLI and the same skills, and Tiller never bakes in a dependency on a capability only one vendor has. A skill uses only what every harness has: a filesystem, a shell, and the `tiller` CLI. This is the reason the surface is CLI plus files plus skills rather than an integration with any single tool.
12. **Change after decomposition is forward work, not history editing.** A project of any complexity will pivot mid-flight, and Tiller treats a late-breaking change the way it treats everything else, as brownfield, where the brownfield is the team's own recent code. Done work is never rewritten to fit a change of mind: merge-backed done (a task) stays done and is superseded by new work, while rollup-backed done (a feature, an epic) recomputes when new work lands beneath it. The vehicle for a large change is the amendment, a reviewed and recorded change to the plan.
13. **Tiller guides, it does not control. The developer is in control and is the responsible party.** Tiller cannot make an agent follow its process, and it does not pretend to. It has no way to force the agent to do anything or to stop it from doing anything. What it offers is structure, gates, and guidance the agent should follow, surfaced through the skills and labeled honestly as guidance rather than dressed up as walls. The one boundary that genuinely holds sits outside Tiller, where the human controls the merge. Everything else is the developer's discipline, exercised through their agent, and every action attributes to the person who directed it. Tiller is a dev tool: the developer drives, and the developer answers for the result.
14. **Managed projects never depend on Tiller.** Setup through production, CI, and merge eligibility must work without Tiller, release access, or `.tiller/` operational or state-validation inputs. Local Tiller may invoke independently runnable project commands, never the reverse; even optional Tiller CI jobs are out of bounds. This extends harness independence to the project, without restricting Tiller's own implementation, tests, releases, or disposable verification. The [distribution contract](docs/spec/distribution.md#managed-project-independence) owns the detailed boundary and planning-only CI pattern.

## Conventions for this repo

- Specs are markdown in `docs/spec/`. Each one owns a slice of the surface and the others point at it rather than restating it: the artifact schema owns the on-disk model, the CLI surface owns the commands. That ownership is about keeping five documents from contradicting each other, not about ranking a document above the code.
- Plans live in `docs/plans/`, dated and topic-named, while they are active, and are deleted when built. Specs describe the current design; plans sequence work still to do. Before deleting a plan, move its durable decisions into the owning spec's settled decisions and any unfinished scope into the spec or the [roadmap](docs/roadmap.md). Git preserves the original.
- `AGENTS.md` is where repo context lives. `CLAUDE.md` is a pointer to it, so the same orientation serves every harness. Keep that pattern as sub-projects appear.
- Specs are living documents and are fully changeable. Change them the way you change code: when the behavior changes, the doc that describes it changes in the same batch, and a spec that no longer matches what ships is a defect in the doc. Building something the spec did not anticipate is ordinary development, not a deviation to justify; write down what you built and why. The specs still run ahead of the code in places, describing surfaces not yet built, so a reader needs to know which is which: the [roadmap](docs/roadmap.md) is where that answer lives.
- When you write docs here, write plainly: concrete, direct, prose over bullet-soup, and no em dashes.
- Create GitHub issues only in `madeintandem/tiller`. Never create issues in other project repositories, including projects inspected while investigating Tiller.

## Engineering Principles

- Always follow SOLID principles both when writing code and reviewing it
- Always consider YAGNI, especially when considering a new dependency or an abstraction
- Build using TDD, do not cheat, write meaningful tests that prove correctness
- Be aggressive about simplicity, always be asking if there is a simpler way to do something, simple doesn't equal clever
- Avoid overly large classes/modules/components/etc, if a code file hits 500 lines start thinking about decomposition, if it hits 1000 lines, do something about it
- Don't use deprecated or undocumented APIs
- Write idiomatic code in whatever language you're writing, don't invent new patterns or conventions
- Choose human-friendly names for things, variables, classes, modules, functions, write code that's easy for human to understand

### When Using Subagents

Use faster agents for mechanical tasks and fix rounds, while keeping an independent task reviewer and the strongest available reviewer for larger blocks of work. When dependency order allows work to be done concurrently, spin up subagents in parallel.

## Current implementation state

Read the [roadmap](docs/roadmap.md) for what is built and what remains open, and the [design history](docs/design-history.md) for why the design took its current shape. Specs remain the maintained reference for each surface. Running code takes precedence when prose disagrees.
