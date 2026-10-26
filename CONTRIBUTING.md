# Contributing to Tiller

Thanks for looking. Tiller is an opinionated SDLC workflow tool for humans and AI
coding agents working together. The one-line version and the principles behind
it live in [AGENTS.md](./AGENTS.md), which orients every contributor, human or
agent, and is worth reading first. This file is the mechanics: how to build and
test, how we work, and what a pull request needs.

## Set up

You need Go (the version pinned in [`go.mod`](./go.mod), currently 1.27.2) and
Node 22 (22.22.2 or newer in the 22 line, which is what CI runs) for the console
under `ui/` and the docs site under `site/`. Reshooting the site's screenshots
also needs `cwebp` (`brew install webp`); nothing else does. Then:

```sh
git clone https://github.com/madeintandem/tiller.git
cd tiller
make build        # go build ./...
make test         # the full Go suite, never from the cache
make lint         # go vet plus a gofmt gate on tracked Go files
cd ui && npm ci && npm test   # the console's suite
```

`make test` passes `-count=1` on purpose. `internal/e2e` builds the tiller
binary and drives it as a subprocess, so its real input is the whole module,
and Go's test cache would happily serve a stale green for a run that never
happened. The full suite takes a few minutes. `go test ./...` is the fast inner
loop, on the understanding that its green means less.

Two things keep the suite fast, and they're worth knowing before you write a
test. The store skips `fsync` under `go test` (a real binary always syncs; set
`TILLER_FSYNC=on` to run a test against the durable path, or `TILLER_FSYNC=off`
when a harness drives a real binary and wants the speed). And a fixture that
many tests share should be laid once and copied per test, the way
`lanternFixture` in `internal/core/mutate` does it, instead of being rebuilt
through a dozen mutations and 10 git processes every time.

The binary itself is `go build -o tiller ./cmd/tiller`. The [`verify`
skill](./.claude/skills/verify/SKILL.md) describes driving it end to end
against a scratch project, which is how we check changes to the CLI, `serve`,
and the console at their real surfaces.

## The embedded console

`tiller serve` ships the web console inside the binary. `ui/embed.go` embeds
`ui/dist`, and `ui/dist` is committed, so `go build` works with no Node
installed. The rule that follows: **when you change anything under `ui/src`,
run `make ui` and commit the rebuilt `ui/dist` in the same PR.** CI checks that
`ui/dist` matches `ui/src` and fails the PR when it doesn't, because a stale
`dist` ships a stale console and nobody notices until it's in someone's
browser.

## The docs site

`site/` is the Astro + Starlight docs site deployed to GitHub Pages. Most of
its pages are generated from the repo's own markdown (`docs/spec/`, this file,
`docs/releasing.md`, `docs/tiller-commands.md`) by `site/scripts/sync-docs.mjs`
on every dev and build, so edit the source in the repo, never the generated
copy. `cd site && npm ci && npm run dev` serves it at
`http://localhost:4321/tiller/`, and `site/AGENTS.md` has the rest.

## How we work

- **Tests first.** Write the failing test, then the code that makes it pass.
  No cheating the test to get green.
- **Specs change with the code.** The documents in `docs/spec/` describe the
  design and are fully changeable. When behavior changes, the spec that
  describes it changes in the same batch, and a spec that no longer matches
  what ships is a defect in the doc. Where the specs run ahead of the code,
  the [roadmap](./docs/roadmap.md) says which is which.
- **Plans track active work.** Dated implementation plans live in `docs/plans/`
  while they are active and are deleted when built. Before deleting one, move
  its durable decisions into the owning spec's settled decisions and any
  unfinished scope into the spec or the [roadmap](./docs/roadmap.md). Git keeps
  the full record; the working tree should not carry competing accounts of what is next.
- **Fast tests.** New tests run in parallel where they can, with `t.Parallel()` as the
  first statement (`internal/core/mutate` and `internal/testgit` do). A test
  that cannot (it calls `t.Setenv` or `t.Chdir`, or touches package-level
  state) stays serial and carries a `// Serial: <reason>.` comment. Other
  packages are still serial.
  Build test repositories with `internal/testgit`: `Init` and `Run` for a
  one-off, and `Template`, `Templates`, or `CopyWithResult` to lay a fixture
  once and copy it per test. A template's lay must not call `t.Setenv` or
  `t.Chdir`.
- **Write plainly.** Docs, commit messages, and PR descriptions are concrete
  and direct, prose over bullet soup, no em dashes. Lead with the outcome.
- **Keep it small.** SOLID, YAGNI, and aggressive about simplicity and about
  the amount of code it takes. Human-friendly names. No deprecated or
  undocumented APIs.

## Pull requests

- Branch from `main`. One concern per PR; a large change is easier to review
  as a short series than as one wall.
  When using Tiller, each task has its own recorded branch. Coarse feature/epic
  reviews use the recorded aggregate: the agent ordinary-merges submitted task
  tips and state into it and maintains one PR. Independent tasks start from
  integration; coarse dependents may start from the aggregate containing their
  submitted prerequisites. No stacked task PRs, cherry-pick, or squash assembly.
  Intermediate assembly is agent-authorized; the final merge to `main` is the
  maintainer's.
- Commit messages lead with a short prefix and a summary line, then say what
  changed and why in the body. The prefixes in use are `Fix:`, `Docs:`,
  `Test:`, `Build:`, and `Release:`; a feature can go without one. Reference
  the issue in the summary when there is one, for example `Fix: keep the
  console's theme choice across reloads (#<issue>)`.
- The PR description says what changed, why, and how you verified it, meaning
  the commands you ran and what they showed. Evidence over adjectives.
- CI must be green. A maintainer reviews and merges. For now that's one
  person, so expect a short wait rather than a fast one.
- Releases are tags, and [docs/releasing.md](./docs/releasing.md) has the
  whole procedure. Contributors never need to cut one.

## Sign your commits (DCO)

Every commit needs a Developer Certificate of Origin sign-off, which is a
single trailer line:

```
Signed-off-by: Your Name <you@example.com>
```

`git commit -s` adds it for you. Forgot? `git commit --amend -s` fixes the
last commit, and `git rebase --signoff main` fixes a whole branch.

By signing off you certify the [Developer Certificate of
Origin](https://developercertificate.org/): that you wrote the change or have
the right to submit it under the project's license, and that you understand it
will be public. Tiller is [Apache 2.0 licensed](./LICENSE), so that's what your
contribution is released under. We use the DCO instead of a contributor license
agreement because it says everything we need in one line, with no forms and no
lawyers standing between you and a merged PR.

Bundled third-party code and assets retain their upstream licenses, recorded
in [THIRD_PARTY_NOTICES.txt](./THIRD_PARTY_NOTICES.txt) and the website's notices.
Do not remove upstream copyrights or treat those components as Apache-licensed
Tiller contributions.

## Questions and bugs

Open an issue; the templates ask for what we need. A bug report wants the
tiller version, your OS, the harness if an agent was driving, and the smallest
reproduction you can manage. A feature request wants the problem before the
proposal. Not sure whether something is a bug? Open the issue anyway and say
so.

## Code of conduct

Everyone taking part in this project, in issues, pull requests, reviews, and
any space where the project is discussed, is expected to make it a
harassment-free experience for everyone, whatever their background or
identity. Be kind, be direct, and assume good faith. Disagree with the work
rather than the person, give and take feedback gracefully, own your mistakes,
and keep other people's private information private. Sexualized language or
attention, trolling, insults, personal or political attacks, and harassment of
any kind, public or private, aren't acceptable here.

The maintainers enforce this. Report anything that crosses the line to the
maintainers privately (the private channel [SECURITY.md](./SECURITY.md)
describes serves for this too). It'll be reviewed promptly and handled
confidentially. A maintainer will remove, edit, or reject contributions that
don't meet this standard and will explain why when it's appropriate to.
Consequences scale with the behavior, from a private word to a permanent ban.
This is a condensed form of the Contributor Covenant 2.1, which is the
standard we hold ourselves to.
