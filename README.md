# Tiller

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./tiller-logo-inverted.svg" />
  <img src="./tiller-logo.svg" alt="Tiller boat and wave logo" width="240" />
</picture>

[![CI](https://github.com/madeintandem/tiller/actions/workflows/ci.yml/badge.svg)](https://github.com/madeintandem/tiller/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)

**Your team and your agents, shipping software you'd put your name on.**

Tiller helps a team of people and coding agents deliver high-quality software
fast, with the confidence to ship it. It's one binary, a CLI with agent skills
and a local console, that keeps the plan, the checks, and the state of a
project as plain files in your repo. Your agent does the work. A person makes
the calls.

[Website](https://madeintandem.github.io/tiller/) ·
[Quickstart](https://madeintandem.github.io/tiller/quickstart/) ·
[Concepts](https://madeintandem.github.io/tiller/concepts/) ·
[Console](https://madeintandem.github.io/tiller/console/) ·
[CLI reference](https://madeintandem.github.io/tiller/cli/)

![The tiller console showing an Acme Platform project: its counts and drift, the decisions that need a human, work in flight, and what is next](docs/assets/console-home.png)

## Install

The current release is **v0.21.4**, for macOS and Linux on amd64 and arm64.

```sh
# macOS, with Homebrew
brew install --cask madeintandem/tap/tiller

# macOS or Linux, with the installer
curl -fsSL https://raw.githubusercontent.com/madeintandem/tiller/main/install.sh | sh
```

The [installer](./install.sh) is short and worth reading first. It fetches the
binary for your platform and verifies its checksum before placing it. Run as
a regular user, it installs to `~/.local/bin`, so add that to your `PATH` if
it isn't there already. Go 1.27 users can also run
`go install github.com/madeintandem/tiller/cmd/tiller@latest`, and checksummed
archives are on the [releases page](https://github.com/madeintandem/tiller/releases).
Check that `tiller --version` reports 0.21.4.

## Start on the repo you already have

Most projects aren't new, so this is the usual way in:

```sh
cd your-repo
tiller init --name "Your App" --key yourapp --kind brownfield
tiller add contributor robin --name "Robin Hale"   # use your own ID and name
tiller join --as robin
git add -A && git commit -m "Add Tiller"
```

`join` adds a clearly marked block to your `AGENTS.md` and `CLAUDE.md` and
leaves the rest of each file alone. It also installs eleven gitignored skill
shims for Claude Code beside any skills you already have. Then ask your agent:

> Run `tiller guide` and follow what it says.

On an existing codebase, that starts the `derive-baseline` skill. The agent
writes down what the code does today and you confirm it, so every later change
can prove the old behavior still works. In Claude Code you can also name the
skill directly, for example "use the tiller-derive-baseline skill." Any other
harness reaches the same skills through `tiller guide`.

Starting fresh instead? Use `--kind greenfield`, put a design doc or two in
`.tiller/design/`, and `tiller guide` sends the agent to `decompose`, which
proposes epics and features for you to approve. The
[Quickstart](https://madeintandem.github.io/tiller/quickstart/) walks a
scratch project from install to a merged task.

To watch it all, run `tiller serve` and open the console.

## What you get

**Move at agent speed, together.** An agent can finish a task in minutes. What
slows a team down is everything around the code: what's next, who has it, and
whether it's really done. Tiller plans just far enough ahead, with each feature
specified when its turn comes. `tiller next` ranks every legal move, the
decisions waiting on you included. Claimed tasks run on their own branches, so
people and agents work side by side without collisions.

**Done means done, every time.** Every piece of work knows what done means
before anyone starts it. A feature's spec, acceptance criteria, and tasks are
approved before the first line of code. Your own build, test, and eval commands
are gates that block done, and Tiller records each result with the revision it
ran against. Once a spec is approved it's pinned, so if it moves, Tiller flags
the drift before work goes on.

**Know where things stand, and know it's right.** An agent can carry work right
up to the line and turn every check green. Only a human merge gets it to done,
and Tiller sees that merge by watching git. The console gives everyone the same
picture: a board from draft to done, every task one click deep, and a report
you can hand a client.

**One plan for the whole crew.** Agent work tends to live in one person's chat
window. Tiller moves the plan, the decisions, and the state into the repo,
where every person and every agent reads the same thing. It works with Claude
Code, Codex, Pi, or a plain shell, because the whole agent surface is the CLI,
the files, and skills served by the binary.

## How it fits together

1. **Agent: decompose.** Propose epics and features from your design docs.
2. **You: approve the shape.**
3. **Agent: specify.** Write one feature's spec, acceptance criteria, and tasks.
4. **You: approve the plan.**
5. **Agent: build and pass the checks.**
6. **Agent and you: review.** Per task, per feature, or per epic.
7. **You: merge.** `tiller sync` records done.

Tiller itself never branches, pushes, or merges. Git stays yours.

## Where it fits

Tiller is for building products. Finding them is a different job.

**Reach for it when**
- You can write down what you want before an agent builds it
- More than one person is on the work
- Someone outside the team needs to see where things stand

**Skip it for**
- Spikes, prototypes, and research code
- A solo tool still finding its shape (Tiller itself, for one)
- Work where the plan changes faster than you can write it down

## The opinions behind it

- **Files are the source of truth.** Markdown for prose, YAML for state, git as
  the spine. No database and no hosted service, so cloning the repo clones the
  plan.
- **Prose and state never share a file.** A node's `.md` is yours. Its `.yml`
  is Tiller's. The CLI churns state all day without touching a sentence a
  person wrote.
- **A gatekeeper, not an orchestrator.** Tiller declares and records gates. It
  never runs the agent's reasoning or drives someone else's tooling.
- **The merge is the done gate.** It's the one boundary that holds, and a
  person controls it.
- **Guidance, not control.** Tiller can't make an agent follow the process and
  doesn't pretend to. The developer drives and answers for the result.

The name isn't an accident. A tiller steers the boat, but somebody's hand has
to be on it.

## Status

Tiller is pre-1.0 and in active use on real projects. It covers the
stretch from "we have specs and a plan" through "it's merged and done." The
schema may change between minor versions until 1.0, and a binary that meets a
tree it can't safely write stops and says so instead of guessing. The
[roadmap](./docs/roadmap.md) says what's built and what isn't.

## Learn more

- [Concepts](https://madeintandem.github.io/tiller/concepts/): the tree, gates,
  review grain, claims, bugs and chores, amendments, and existing-code baselines
- [Design specs](./docs/spec/) and the [design history](./docs/design-history.md)
- [Every command](./docs/tiller-commands.md), or `tiller --help`
- Contributing: [`AGENTS.md`](./AGENTS.md), then [`CONTRIBUTING.md`](./CONTRIBUTING.md)

Tiller is built by [Made In Tandem](https://madeintandem.com), a software
consultancy in Chicago, and it's how we run human-plus-AI delivery on our own
client work. Bringing it to a team?
[Get training from the people who built it](https://madeintandem.com/training/?utm_source=tiller&utm_medium=github&utm_campaign=training&utm_content=readme).

Licensed under [Apache 2.0](./LICENSE); see [NOTICE](./NOTICE) for attribution
and [TRADEMARKS](./TRADEMARKS) for use of the Tiller name and logo. Bundled
third-party components keep their upstream licenses, listed in
[THIRD_PARTY_NOTICES.txt](./THIRD_PARTY_NOTICES.txt).
