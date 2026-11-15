# CLI: the tiller command surface

This is a pointer page. The contract lives in `docs/spec/cli-surface.md`, and `docs/tiller-commands.md` is the reference of shipped commands (the site's CLI page is generated from it by `site/scripts/sync-docs.mjs`). This page says which sections apply, what the code adds, and where the two differ. Checked against HEAD `7c4ca1a` (v0.24.0, schema 15).

## What this surface is

The `tiller` binary's command layer. `cmd/tiller/main.go` is one line of work: it hands `os.Args[1:]` to `cli.Run` and exits with the code that returns. `internal/cli` is the cobra shell: it parses arguments, resolves the project and the acting contributor, calls the core packages, and renders the result as human text or as the `--json` envelope. Its package comment states the rule it keeps: it holds no domain logic and is the only package allowed to depend on cobra.

`NewRootCmd` in `internal/cli/root.go` registers 48 top-level commands, and cobra adds `completion` and `help`. The registered set matches the index in `docs/tiller-commands.md` except for `grade` (see the last section). Its users today are a developer at a terminal, a coding agent driving the same commands from inside its harness, the console (which shows commands to copy and runs none), and Tiller itself, when an upgrade parent starts the selected binary as a child and decodes its envelope.

What the CLI is and what it refuses to be: `docs/spec/cli-surface.md`, "What the CLI is". The command tree, the verb table, nouns, and address resolution: "Grammar". One row per command with its actor and what it touches: "Command reference" and "Who may run what". The shipped syntax, flag by flag: `docs/tiller-commands.md`, "Command index", "Conventions", and the stage sections from "Onboard" through "Reads".

This page accounts for commands as commands: the tree, flag and argument contracts, output, the envelope and exit codes, identity resolution, readouts and guide, intake, and the maintenance commands. The lifecycle rules a verb enforces once it reaches the core belong to the storage and lifecycle page. Not registered, and so not as-is behavior: `doctor`, `block`, `unblock`, `commit state`, `add adr`, `add design`, `add evidence`, and the proposed `--commit` and `--dry-run` flags on ordinary mutations (`docs/spec/cli-surface.md`, "Grammar", the "Unbuilt command design" paragraph; `docs/tiller-commands.md`, "Not built").

## Data it owns

It owns no persisted state. Every file a command changes belongs to a schema another page describes, and the CLI reaches it through `internal/store` and `internal/core/mutate`. What the CLI owns is compiled in: the `version` constant in `internal/cli/root.go` (stamped by hand at each release), the envelope and error-body shapes in `internal/cli/envelope.go`, the stable reason strings it sets itself (`bad_args`, `unknown_command`, `not_found`, `noun_mismatch`, `ambiguous_key`, `unknown_seam`, `unknown_skill`, and the per-command ones), the readout seam table in `internal/cli/readout.go`, and the golden files under `internal/cli/testdata`.

In memory it holds one invocation: a fresh root command, one tree load, one result. Nothing survives the process. A few package-level variables exist only as test seams.

The checkout-local files whose reading and writing the CLI drives are specified elsewhere: `docs/spec/artifact-schema.md`, "Local identity: `identity.yml`" and "Local settings: `local.yml`"; the ignore groups and scaffold in `docs/spec/cli-surface.md`, "Bootstrapping a project".

## Behavior to preserve

The grammar. Verb, then a required noun that is checked against the node's typed id, then an id or key. The four shapes that bend it, batch `approve`, and the path-addressed `design` noun are in `docs/spec/cli-surface.md`, "Grammar". Human output names a node as key path with the id in parentheses; `--json` is unchanged by that.

The envelope and exit codes. Every `--json` answer is `{ok, command, data | error}`; `0` is success, `1` a blocked business answer that keeps a `data` block, `2` misuse, `3` environment. Refusals a parser raises before a command runs keep the same envelope, with the `--json` request read from the raw arguments. Project discovery walks up to the nearest `.tiller/tiller.yml`. All of this is `docs/spec/cli-surface.md`, "Output and exit codes". In the code: `Envelope`, `classify`, `jsonRequested`, and the `emit*` family in `internal/cli/envelope.go`; `execute` and `renderParserFailure` in `internal/cli/root.go`. `classify` reaches exit `3` only through a typed error (`store.EnvError` or an error that carries its own exit code); an untyped error falls through to misuse, exit `2`, reason `error`. One observed rule the sources state only in part: in human mode a success prints to stdout, while a blocked answer and a failure print to stderr; under `--json` the envelope goes to stdout, while advisories and narration (the `requires_tiller` advisory, a readout-unavailable notice, gate progress) stay on stderr. The spec states the stderr line for parser refusals only.

Identity. `--contributor`, then `TILLER_CONTRIBUTOR`, then the gitignored `.tiller/identity.yml`, checked against the manifest roster, with a refusal instead of a guess and a spoken fallback (`acting as <id> (<source>)`). See `docs/spec/cli-surface.md`, "The actor model", and the `join` and roster paragraphs of "Bootstrapping a project" for `contributor_unset`, `contributor_unknown`, and `identity_unreadable`. The code is `resolveActingContributor` and its siblings in `internal/cli/add.go`; a command opts in through `addContributorFlag`, which is what makes its report carry an actor.

Readouts, next, and guide. The block the CLI renders at each seam and gate, the correction form, the `Up next` tail, and the terminal rendering: `docs/spec/cli-surface.md`, "The agent readout". The list they all end on: "Ranked next". The commands: "Guidance and next moves: guide, next, readout". Code: `internal/cli/readout.go`, `readout_gate.go`, `readout_render.go`, `next.go`, `next_render.go`, `guide.go`, and `tty.go`. The block text itself is built in `internal/views`.

Intake. One door for a document after `init`, as a file or a folder, opening one draft amendment, or none with `--no-amendment` before any epic exists: `docs/spec/cli-surface.md`, "Large changes: amendments" (the `tiller intake` paragraphs) and "Bootstrapping a project" (the founding design); `docs/tiller-commands.md`, "Amend (a reshape of the plan)". Code: `internal/cli/intake.go` and `import_docs.go`.

Maintenance commands. `migrate` and `upgrade`, their scopes, the shared result payload, protocol `1`, and the exit rules: `docs/spec/cli-surface.md`, "Migrating and upgrading: migrate, upgrade"; `docs/tiller-commands.md`, "Migrate" and "Upgrade". `compact`: "Compacting events: compact". `usage`: "Capturing agent usage: usage". `sync` with its two best-effort follow-ups: "The done crossing" and "Observing merges and reading holds: sync, status, ls". `serve` flags, bind policy, and browser opening: "tiller serve". Every command that reaches its run function also runs the advisory `requires_tiller` check first, on stderr only, never failing (`internal/cli/requires.go`); a parser-stage refusal, `--help`, and `--version` do not reach it.

The rest of the per-verb contracts, which later work must keep stable as flags, payload fields, and reason strings: `docs/spec/cli-surface.md`, "Proposal correction commands", "Design baseline commands", "Recording surfaces and dependencies", "Specifying a feature", "The work loop", "Gates, evals, and attestation", "Review grain", "Reading nodes: show, ls, status", "Rendering the tree and dependency order: tree, check readiness", "Detecting drift: drift", "Searching: search", and "Relationship queries". The settled rulings are listed under "Decisions settled so far".

Two general guarantees are easy to break by accident: a transition whose target equals the current state is a no-op success, and help text and refusals name only `tiller` commands or in-project paths, never a file of the Tiller source repo ("Output and exit codes").

## Edges

Inward, `internal/cli` calls nearly everything, and each of these is another page's subject: `internal/store` (tree load, commits, environment errors) and `internal/core/*` with `internal/workflow` (every mutation and lifecycle rule); `internal/gen` and `internal/skills` (orientation, shims, guide text); `internal/views`, `internal/search`, `internal/relationships`, and `internal/serve` (read projections, search, queries, the console); `internal/selfupdate` (binary replacement); `internal/vcs` (Git observation); `internal/usage` (capture). It reaches `internal/execution` only through `internal/readinput`. Third-party, in non-test code: cobra, pflag, and `golang.org/x/term`; `yaml.v3` is imported by tests only. It also imports the repository's top-level `ui` package, which carries the embedded console (`internal/cli/serve_cmd.go`).

Outward, it starts two kinds of process itself: the browser opener in `internal/cli/serve_cmd.go` and the selected Tiller executable as an upgrade child in `internal/cli/upgrade_child.go`. Git, the project's gate commands, and Homebrew are started by the packages it calls (`internal/vcs`, `internal/core/gates`, `internal/selfupdate`). It reads `TILLER_CONTRIBUTOR`, `BROWSER`, `NO_COLOR`, and the user's home directory, and passes the environment reader to usage capture.

The contracts across those edges are the envelope (agents and the upgrade parent parse it), the exit code, and the copyable command text other surfaces print. `docs/tiller-commands.md` is itself an edge: the docs site publishes it, so a flag change is a doc change in the same commit.

## Described paths

`internal/e2e` holds tests only: it builds the binary and drives it as a subprocess against scratch projects (`CONTRIBUTING.md`, "Set up").

- `cmd`
- `internal/cli`
- `internal/e2e`

## Discrepancies and gaps

Where `actor` sits. `docs/spec/cli-surface.md`, "The actor model", says the `--json` envelope carries a top-level `actor` field, always present, and `docs/tiller-commands.md`, "Conventions", says the same. The code merges it into the `data` block (`withActor` in `internal/cli/envelope.go`; the tests read `data["actor"]`), which agrees with the three-key envelope in "Output and exit codes". An exit `2` or `3` answer has no `data` block and so carries no actor.

Schema number. `docs/spec/cli-surface.md`, "Migrating and upgrading: migrate, upgrade", says `schema_version` is "currently 14". The same document's Status line says 15, and so does `CurrentSchemaVersion` in `internal/store/manifest.go`.

`grade` is missing from an index. `tiller grade` is registered (`internal/cli/grade.go`) and described in the body of `docs/tiller-commands.md`, but its "Command index" table has no row for it.

The no-node list is short. `docs/spec/cli-surface.md`, "Grammar", lists the project-level commands that take no node and calls `status` the one node command with a bare form. `usage` and `compact` take no node and are not in the list, and `tree` has both a bare form and a rooted `tree <epic|feature> <node>` form with `--status`, which `docs/tiller-commands.md`, "Reads", documents and the spec does not.

Hidden upgrade flags. The spec documents the hidden `upgrade --project-only --inspect`. The code also registers hidden `--upgrade-handoff` and `--expected-executable-sha256` (`internal/cli/upgrade.go`), which the parent passes to its child. No source names them.

What was checked: command registration, the envelope and exit mapping, parser refusals, actor resolution and placement, the readout seam table, and the `usage`, `sync`, `intake`, `guide`, `tree`, and `upgrade` flag sets. Not checked line by line: the per-verb refusal reasons and payload fields in the lifecycle command files, and the upgrade child protocol.
