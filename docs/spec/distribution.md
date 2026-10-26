# Tiller Distribution and Adoption

**Scope.** This document defines how Tiller reaches a developer and a project: how it is built, installed, updated, versioned, migrated, wired into a coding harness, adopted, and documented. It layers over the [artifact schema](./artifact-schema.md), canonical for the on-disk model, and the [CLI command surface](./cli-surface.md), canonical for commands. Where this document disagrees with either, they win and this one is fixed.

**Status.** Current reference at version stamp 0.21.2, schema 12, orientation generation 57, and eleven embedded skills. Installation, publication, and adoption of a build in a managed project remain separately authorized acts. The [roadmap](../roadmap.md) lists what is not built yet.

---


## Self-contained in the repo

Tiller is self-contained in the project repo. It neither reads nor writes global or user config: no `~/.config/tiller`, no machine-default contributor. The committed `contributors` roster in `.tiller/tiller.yml` says who can hold work; the gitignored `.tiller/identity.yml` says who I am on this checkout. A fresh clone carries the roster but no identity, so every checkout runs its own `tiller join`. The only thing Tiller depends on beyond the repo is the binary on `PATH`. The deferred launcher's engine cache under `~/.cache/tiller/versions/` would be a disposable projection of released binaries, not another source of truth.

The agent surface is self-contained the same way. Nothing Tiller puts at the agent and harness interface (CLI output and refusals, the skill text `tiller guide` serves and the shims delegate to, the generated `orientation.md`, the files `init` seeds) references a path or document that does not exist inside the project or come from the binary itself. Version skew is what makes this a correctness rule rather than a portability nicety: a developer's machine may hold a Tiller source checkout while the project's resolved binary is a different version entirely, so an out-of-project pointer is not just possibly absent on a teammate's machine, it is possibly wrong even where it resolves. The binary is the source of truth for guidance at its own version. `orientation.md` states the boundary to the agent directly: the project directory is the whole workspace and Tiller's guidance is complete inside it, so the agent goes outside only where the developer points.

---


## Managed-project independence

Tiller is a development workflow tool, never a dependency of the projects it manages. A managed project's setup, build, tests, CI, packaging, deployment, and runtime must work without Tiller installed, without access to Tiller releases, and without consuming `.tiller/` as an operational input. Project CI and merge eligibility must not depend on Tiller availability or Tiller state validation. This extends the harness-independence rule to the project itself.

Agents must not introduce Tiller package dependencies, build or test wrappers that call Tiller, setup hooks, workflow installation steps, Tiller release credentials, deployment prerequisites, runtime imports, or required host checks into a managed project. Installing and upgrading Tiller remain separate developer workflow choices, not prerequisites of ordinary project commands. Optional, advisory, or warning-only Tiller CI jobs are prohibited too.

The dependency direction is one-way. Local Tiller may run an existing project command such as `bin/ci` and record its result, but `bin/ci` must remain directly runnable and must not call Tiller. Committed `.tiller/` files remain planning and evidence records for Tiller and the agent workflow, not application inputs or CI gate inputs. Local `tiller check` and evidence recording remain intact; genuine project failures still need repair and must not be skipped or reported as success.

An existing or approved plan that introduces this dependency conflicts with the rule. The agent must surface the conflict and propose an independently runnable alternative, not silently implement the plan or work around it with credentials. This is guidance, not a claim that Tiller can enforce arbitrary repository edits.

The boundary applies to managed projects. Tiller's own implementation, tests, release automation, and disposable verification fixtures necessarily exercise Tiller and are exempt.

### Planning-only CI without Tiller

When a stable required check is needed, classify changes with ordinary repository tooling inside an always-running workflow. Application verification may be not applicable only when every change belongs to a narrow, confirmed set of non-executable, non-test-consumed planning paths. Looking at changed path names is not consuming Tiller state as a gate input.

Run the normal project checks for code, mixed changes, unknown paths, protected inputs, or uncertain history. Protected inputs include executable scripts, test fixtures, configuration, and anything the build or tests consume. Do not blanket-exempt `.tiller/**`, Markdown, or YAML. If the classifier cannot establish the planning-only case, it must select full project verification.

Report the selected route honestly and propagate failures from every applicable check. A planning-only result need not fabricate a replacement validation job. Never install, invoke, download, or provision credentials for Tiller in that managed-project workflow, including an optional advisory job. `tiller status` and `tiller check readiness` are local workflow tools, not replacement CI checks.

---


## What Tiller is built as

Tiller is a single, statically linked binary, written in Go, with the web console's assets and the skill bundle embedded inside it. One file on the `PATH`, no runtime to install, no interpreter, no dependency on whatever the project it manages happens to be written in.

That last point is the whole reason for the choice. Tiller's gates shell out to `bin/rails` and `bundle exec rspec`, but that is the managed project's runtime, configured in `tiller.yml`. Tiller itself has to run identically over a Rails repo, a Python service, and a TypeScript monorepo, and a self-contained binary is the only artifact with zero coupling to the host project's stack. It is the same agnosticism the agent surface already lives by, drawn one layer down: the CLI is model-and-harness agnostic above, and language-and-runtime agnostic below.

Embedding the console and skills in the binary buys the sync guarantee: CLI behavior, console behavior, generated orientation, and full skill guidance come from the same artifact. The current `PATH` executable is that binary. A stable launcher resolving a compatible engine per project remains unbuilt; its design is retained under Versioning.

### Third-party licenses

Tiller's own code is licensed under Apache 2.0, with attribution in `NOTICE` and the name and logo policy in `TRADEMARKS`. `THIRD_PARTY_NOTICES.txt` contains the full upstream copyright, license, and notice text for the Go runtime dependencies, the bundled console JavaScript dependency closure, and the Inter, Space Grotesk, and Space Mono fonts. Every release archive includes it alongside `LICENSE`, `NOTICE`, and `TRADEMARKS`. The embedded console serves the same text at `/THIRD_PARTY_NOTICES.txt`; the documentation website distributes its own notices at `/tiller/THIRD_PARTY_NOTICES.txt`. Font copies carry their full SIL Open Font License text in `ui/public/fonts/`, `site/src/fonts/`, and the website's public font-license directory.

After updating dependencies, run `npm ci` in `ui/` and `site/`, then `node scripts/third-party-notices.mjs` from the repository root. The generator inventories the executable's Go dependencies across all four release targets and copies actual license files, including upstream `NOTICE` files. Console and website builds then refresh the notices from their bundled module inventories. The website also includes notices for inline framework code and generated Pagefind search assets. Review and retain the generated source notices with the rebuilt console assets before packaging; the release build does not require Node or regenerate them. Missing upstream license text is a build failure, not an SPDX-only or URL-only fallback.

When an npm package omits license files, `third_party/npm-notices.json` records version-specific provenance and full upstream text. This also covers Pagefind's emitted JavaScript: Svelte and language-tag helpers, adequate-little-templates, mark.js, and its vendored fflate decompressor. These do not appear as separate modules in Tiller's build. The final website hook runs after Starlight generates Pagefind's assets and checks the emitted script inventory against the versioned upstream source and lockfiles. Unknown or missing scripts require an attribution review, rather than quietly shipping an incomplete notice. Unreviewed package versions do not inherit those notices.

The documentation site's Pagefind WebAssembly search incorporates GPL-licensed runtime code, separate from Tiller's Apache-2.0-licensed binary and console. The website distributes the full corresponding source archive, including the linked dependency sources, licenses, and upstream build scripts, beside its search assets. Its Project sidebar and third-party notices link to `/tiller/pagefind-1.5.2-corresponding-source.tar.gz`. Versioned WASM license provenance lives under `third_party/pagefind-wasm-1.5.2/`. The archive is not tracked in git. It is a GitHub Release asset under the tag `third-party/pagefind-1.5.2-source`, and every site build fetches it into `site/public/` and refuses bytes whose SHA-256 differs from `correspondingSource.sha256` in `third_party/pagefind-wasm-1.5.2/manifest.json`. Updating Pagefind requires building the new version's archive and inventory, publishing the archive as its own release asset, and pinning its name, release and checksum in the new manifest before rebuilding. The website build refuses a missing or mismatched archive and a version-specific license manifest without that pin.

---


## Install

Tiller installs the way developer CLIs install, and deliberately offers nothing exotic:

- A **Homebrew tap** for the Mac mainstream: `brew install --cask madeintandem/tap/tiller`. Casks are macOS-only, so Linux users take the installer.
- A **`curl … | sh` installer** for macOS and Linux, which fetches the right binary for the platform, verifies its checksum, and drops it on the `PATH`. This is a developer installation path, not a managed-project CI step.
- **GitHub Releases** with checksummed binaries per platform, the substrate both of the above sit on and the thing `tiller upgrade` reads.


Windows package managers (Scoop, winget) wait for real demand rather than shipping ahead of it. There is no installer beyond placing a binary, because there is nothing else to place: no runtime, no services, no system daemons.

---


## Update

`tiller upgrade` is the normal maintenance command. Inside a project it updates the installed binary through its installation owner, applies supported schema migrations, and refreshes the current resolved checkout's generated instructions, skill shims, harness wiring, and required ignore entries. Running from a subdirectory selects that same checkout. Outside a project it updates only the binary and explicitly reports that other checkouts were not touched. An inaccessible or malformed project marker is a refusal, not an outside-project success.

| Scope | Binary | Current checkout |
|---|---|---|
| `tiller upgrade` | Update through the installation owner | Migrate if needed, then refresh |
| `tiller upgrade --project-only` | Use the running binary; no network or package manager | Migrate and refresh, including already-current schema |
| `tiller upgrade --binary-only` | Update through the installation owner | Explicitly excluded |
| `tiller upgrade --version vX.Y.Z` | Select an exact self-managed release | Maintain only if that target supports this checkout |

Project-only and binary-only are mutually exclusive; project-only with `--version` is misuse. Project-only outside a project refuses before writes. `--contributor <id>` supplies ordinary registered-contributor attribution only when a real migration is needed, resolved before `TILLER_CONTRIBUTOR` and local identity. It neither registers a contributor nor selects checkout identity; schema-9 migration, and current-schema refresh of a checkout that pulled it, preserve the selected ID while renaming an existing legacy identity key. Current-schema artifact refresh needs no identity. Join remains onboarding or deliberate identity selection, not the maintenance recipe; standalone `tiller migrate` remains explicit local migration plus the same refresh.

For self-managed installations, unpinned upgrade resolves the latest GitHub Release without downgrading an already-ahead build. Already-current skips archive download and replacement but still maintains the checkout. An explicit pin preserves exact selection/reinstallation semantics. Downloaded archives are checksum-verified before use. For combined maintenance, the verified candidate inspects the checkout before replacement; a known migration blocker, unsupported inspection protocol, or newer project schema/orientation refuses without replacing the binary. `--binary-only --version ...` is the explicit binary-only downgrade path. No schema or guidance downgrade is allowed.

The handoff uses the selected executable's exact resolved path, not another `tiller` on PATH. Internal inspection protocol **1** is `upgrade --project-only --inspect --json`; it reports executing identity, migration path, schema/orientation observations, planned paths, blockers, and execution warnings without project, identity, harness, Git-index, or persistent-cache writes. Inspection is not a reservation: apply revalidates executable identity and source/destination snapshots. Binary-only and outside-project replacement need no project protocol.

### Installation ownership

For a verified Homebrew cask or legacy formula, upgrade delegates only to that installed Tiller package in its owning prefix, then resolves and verifies the resulting executable before project maintenance. Homebrew-looking paths without verified metadata refuse; they never fall back to manual replacement. Exact `--version` pinning refuses before invoking brew. Project-only bypasses brew entirely.

The delegated child alone receives `HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1` and `HOMEBREW_NO_INSTALL_CLEANUP=1`, preventing implicit dependent upgrades and broad cleanup. Never run an unqualified `brew upgrade`, overwrite the Cellar/Caskroom, add sudo, change credentials, or bypass security policy. Brew may still refresh metadata or install genuine dependencies of the selected package; this is not a promise that no other package files change. Current-binary inspection catches known blockers before delegation, but future target rules may be unavailable until brew finishes. Brew can therefore change the machine before target compatibility is known; failures report that boundary without manual-swap fallback or project rollback.

`GITHUB_TOKEN`, when set, authenticates self-managed release lookups and asset downloads, which avoids anonymous API rate limits. It does not supply a Homebrew authentication bypass. Release-service failure is a real default-upgrade failure, not proof that the binary is current; offer offline `--project-only` explicitly.

### Retention and drain recovery

Before replacement, retain and verify the actual outgoing executable under `~/.local/share/tiller/previous/<version>-<sha256>/tiller`, outside the installation owner's directories. Version comes from that executable and SHA-256 from its bytes. The private retained directory never overwrites conflicting or unverified files; identical verified copies may be reused. Retention failure stops replacement, and installation identity is rechecked before replacement. A refusal may leave a valid retained copy, reported separately. Distinct builds remain until the developer removes them; no automatic pruning, binary rollback, schema dispatch, or repository scan is introduced.

A retained executable is not automatically compatible with an old checkout. Use one only after verifying its schema support; otherwise obtain a compatible release before draining. Use the reported **directory**, not just a renamed executable, so gates that invoke bare `tiller eval` use the same build:

```sh
# In a temporary drain shell, replace this with the verified directory reported by upgrade.
export PATH="/exact/reported/retained/directory:$PATH"
tiller --version
# Finish authorized old-schema work, human acceptance/final merges, and sync.
exit
# Back outside the drain shell, invoke the upgraded executable explicitly.
/exact/path/to/current/tiller upgrade --project-only
```

Never use an older binary to mutate a migrated checkout. Maintenance never finishes, cancels, approves, merges, or syncs work to bypass a blocker. Retained-ref namespace collisions are execution warnings, not migration failure or permission to delete history; no remote fetch is performed for that check.

### Completion and durable boundaries

Binary replacement, source migration, and derived refresh are separate durable boundaries, not one machine-wide transaction. A real migration requires committed, clean existing transformation inputs and no unresolved conflicts; new event/output paths require absence guards, not nonexistent committed baselines. Unrelated dirty application files do not block. Refresh repairs stale or missing generated artifacts in a dirty checkout while preserving human text outside managed regions and unrelated skills. Unsafe symlinks, read failures, and malformed markers refuse. All refresh artifacts share one derived transaction and byte-identical outputs are skipped; obsolete shims are removed only with positive generated ownership evidence.

Exit 0 means every requested stage completed or was genuinely inapplicable. A successful binary replacement followed by migration failure is incomplete; a committed migration followed by refresh failure retains the new schema and event and is also incomplete. Confirmed rollback may establish unchanged destinations. `mutation_recovery_needed` instead reports `recovery_required`, preserves the journal, and requires existing transaction recovery before retry. A lost or malformed child result leaves unestablished project phases `unknown`, not assumed unchanged. Never delete `.tiller/.cache`, locks, or journals to make maintenance pass.

The CLI reports completed phases, actual paths and generations, blockers, warnings, and the exact failed phase and recovery. Known partial results survive nonzero child envelopes through the [CLI envelope contract](./cli-surface.md#output-and-exit-codes). After success, review/commit the reported diff with ordinary Git. Other branches, worktrees, teammates, and projects must incorporate it and run `tiller upgrade --project-only` themselves. Restart existing serve processes and reload harness instructions as needed; disk refresh does not reload user processes.


### Upgrading from an older binary

An already-running older process cannot acquire the current upgrade logic during its own replacement: it may still call join and return early when already current. Keep a compatible old executable before that install if any checkout needs it, then run the newly installed binary's `tiller upgrade --project-only`. Older binaries do not support the current flags, and join never migrates implicitly.

The deferred `upgrade --project` spelling belongs to an unbuilt project-pinned engine cache, not to `--project-only`. The launcher, cache, and opt-out update nudge remain unbuilt.

---

## What Tiller places on a machine

Before versioning, it helps to see the whole footprint, because the artifacts version very differently and most of them are disposable.

| Artifact | Where it lives | Committed? | Version-coupled? |
|---|---|---|---|
| Stable skill shims | `.claude/skills/`, per checkout | no | they delegate to the running binary's `tiller guide` |
| `orientation.md` | `.tiller/`, in the repo | yes | yes, generation-keyed; moves when its stated rules change |
| The pointer block | a managed region in `AGENTS.md` / `CLAUDE.md` | yes | barely; thin and stable |
| `.tiller/` data | the repo | yes | yes, hard; this is the schema |
| `.tiller/identity.yml` | the repo, per checkout | no; gitignored | no; the local "who I am here" |
| `.tiller/local.yml` | the repo, per checkout, optional | no; gitignored | no; hand-edited preferences, never written by Tiller |
| Retained executables | `~/.local/share/tiller/previous/<version>-<sha256>/tiller` | no | verified outgoing builds for explicit recovery; not automatic dispatch |
| Engine cache (unbuilt) | planned `~/.cache/tiller/versions/`, per machine | no | per required project line; rebuildable |
| Shell completions | the shell's completion dir, per machine | no | cosmetically |

Two rules organize that list:

- **Generated artifacts that must travel with the repo are committed; per-machine conveniences are not.** `orientation.md` is committed, so a fresh clone orients an agent before `tiller` is even installed. The installed skill files are stable shims, not the full versioned skill bodies: they carry enough metadata for harness discovery and then ask the project-resolved `tiller guide` for the full skill text. Shims and cached engines stay out of Git to avoid generated-file conflicts. Generated artifacts carry a "do not hand-edit" header.
- **The one artifact that is both shared and version-coupled is the `.tiller/` data itself.** That is what the versioning model below protects. The launcher and skill shims are conveniences around that boundary, not another source of truth.

### Generated-board retirement

The generated `.tiller/board.md` and its generation command are retired. Live project status comes from the existing CLI reads and local console, not a replacement file. This changes no ownership, acceptance, merge-observation, or lifecycle policy and requires no work drain or source-schema conversion.

Deploy the changed binary separately, then run `tiller upgrade --project-only` in each continuing checkout to refresh orientation and skill discovery instructions. Review the diff, reload agent instructions, and restart any console that should use the new binary. Keep orientation committed.

Tiller neither deletes leftovers nor changes the Git index, and maintenance never inspects a retired board to decide whether it can continue. Inspect and preserve unexpected hand edits before deliberately removing a tracked, unmodified generated file from the repository root:

```sh
git rm -- .tiller/board.md
```

Review and commit that deletion through ordinary Git, and incorporate it into continuing branches. Do not force removal. Ordinary local deletion of an ignored or untracked leftover is optional; do not recursively delete a directory obstruction or follow a symlink. Tiller no longer seeds the board ignore rule and does not purge existing human-owned ignore entries, which are harmless. An older binary may recreate the file; the new binary still excludes that exact retired coordinate from live document reads. Installation, project cleanup, commits, publication, and adoption remain separately authorized operations.



---


## Versioning

Tiller carries two version numbers, and keeping them apart is what makes upgrades livable.

The **binary version** (`1.7.2`) moves on every release. It carries the console, bug fixes, new flags, and new skill text. Upgrade it freely.

The **schema version** (a plain integer, stamped as `schema_version` in `.tiller/tiller.yml`) moves only when the on-disk format changes in a breaking way. It is a migration boundary, not a release number. Tiller `1.4` through `1.9` might all speak `schema_version: 3`; schema `4` arrives once, deliberately. Because the two are decoupled, the great majority of upgrades carry no schema change at all, so they need no migration and interoperate across a team without ceremony.

Changes sort into three classes, and naming them is half the design:

| Class | Example | Migration? |
|---|---|---|
| **1, binary-only** | console, bug fix, new flag, new skill text | none; different `1.x` interoperate |
| **2, additive schema** | a new optional field an older binary can ignore | required in v1; softens to none once lossless unknown-field preservation is built and tested |
| **3, breaking schema** | a renamed field, a moved directory, a changed status meaning | required |

The work is to make Class 1 invisible, Class 2 safe, and Class 3 a controlled, coordinated event, concentrating the real pain in Class 3 rather than spreading a little across every release. In v1, "Class 2 safe" means safe via migration, not via best-effort preservation. A field addition requires a migration until lossless unknown-field preservation is implemented and tested, so an additive change carries the same coordinated-migration discipline as Class 3 rather than relying on an older binary to round-trip the field. The reason is that exact YAML round-tripping is harder than it sounds, and silently losing an unknown field in a file-first tool whose data is the source of truth is worse than asking the team to run a migration. Once lossless preservation exists and is tested, Class 2 can soften back to a soft-or-none migration; until then it is treated as breaking.

**The safety check.** Each binary knows the schema range it reads and the single version it writes, for example "reads 2 through 4, writes 4." On every invocation it compares that against the tree's `schema_version`:

- In range and equal to what it writes: silent and perfect. Every Class 1 upgrade lands here.
- Older than what it writes: ordinary commands refuse and name `tiller upgrade --project-only` (or explicit `tiller migrate`) when the team is ready. Explicit upgrade authorizes registered migrations after their safety checks, not lifecycle approvals or merges.
- Newer than it can read: refuse and name `tiller upgrade`; an older binary must not corrupt a tree that already migrated ahead of it. Current tree loading requires the current schema, including ordinary read commands.


That third rule is what makes a mixed-version team safe rather than dangerous: a teammate who has not upgraded cannot write the newer tree, so the advisory drift warning escalates to a safe write-lockout instead of a silent mess. The v1 unknown-field policy is the same instinct applied to additive changes: rather than letting an older binary silently drop a field it does not recognize, an additive change is gated behind a migration, so the choice is again a clean stop over a quiet loss of data.

**Unbuilt launcher and cache design.** On every invocation the launcher would find the nearest `.tiller/tiller.yml`, read `schema_version`, `schema_epoch`, and `project.requires_tiller`, and choose an engine from `~/.cache/tiller/versions/`. If the bundled current engine can read and write the tree safely, it uses that. If the project needs a different cached engine, it execs that engine. If no compatible engine is present, it stops with a recoverable environment error naming `tiller upgrade --project`. Outside a Tiller repo, it uses its bundled current engine. Neither launcher resolution, the cache, nor `upgrade --project` ships in the current binary.

---


## Migration

`tiller migrate` moves a project's `.tiller/` data across a schema boundary, and it is a Rails migration in all but its target. Sequential, versioned, deterministic transforms; each binary carries the migrations from the lowest schema it reads up to the one it writes; the current version is stamped in the data. The two differences from a database migration both cut in the team's favor. It runs against a git working tree, so the "up" migration produces a reviewable diff a human reads on a branch before committing it, and the audit trail is git itself. It is forward-only in practice: the safety is "review it before you merge it," not a down-migration, and after a migration the team moves forward rather than back. The migration code is in the binary, so the operation is hermetic, with no network and no external files.

A Class 3 migration is a coordinated team event. One person runs upgrade on the adopting branch, the team reviews and commits/merges its diff, and teammates incorporate that change and run `tiller upgrade --project-only`. The schema stamp guards older binaries; `requires_tiller` remains advisory.

**Unbuilt schema-fence design.** A future Class 3 migration that transforms data advances both `schema_version` and `schema_epoch` and records the migration event. Once that migration is visible in a checkout, every mutating command checks the current branch before writing: local `.tiller/` changes made before the epoch without incorporating the migration cause a recoverable schema-fence refusal. Recovery is to merge or rebase the migration, rerun `tiller migrate` if old-shape files remain, then continue. A local tool cannot detect a migration commit it has never fetched; that remains the Git-latency boundary, not a reason to add Tiller to managed-project CI or branch protection. Current stamp-only migrations neither advance the epoch nor arm a fence.

**Rejected CI-emitter design.** `tiller ci-status` and Tiller-backed branch protection are retired, not deferred, optional, or candidates for a renamed integration. Making a CI runner interpret Tiller's exit code would still couple merge eligibility to Tiller and its state. Schema stamps, version floors, schema fences, drift markers, readiness, and recorded gates belong to the local Tiller workflow, not managed-project CI validation. The [project-independence contract](#managed-project-independence) defines the independent alternative.

**Schema discipline.** Before `1.0`, breaking changes are accepted when needed, and every boundary with real trees on both sides gets a forward migration. After `1.0`, schema bumps should be rare, announced, and batched into major releases. Schema 1 has no migration path and requires re-init. Supported older migrations compose forward to the current schema. Most boundaries are stamp-only or additive: the migration changes the manifest stamp (adding a field where one is new), records a migration event, preserves work, events, ownership, reviews, and active execution, and needs no work drain or ownership review. Older binaries refuse a newer stamp rather than silently dropping fields during YAML mutation, and ordinary commands refuse an older stamp until migration. Two boundaries carry extra requirements: crossing into schema 5 drains in-flight work, and crossing into schema 6 converts active ownership under human review (both below). Upgrade and migrate refresh only the current resolved checkout and never select or create checkout identity; they only rename a legacy identity key (next paragraphs).

An epic keyed `tweaks`, which a binary that predates the Tweaks shelf could create, refuses migration with `migration_reserved_epic_key` (exit 3) because its feature refs would read as Tweak refs: rename a draft one with the older binary (`tiller rename epic tweaks --key <new-key>`), commit, and retry; an epic past draft has no rename path, so that project stays on its older binary. Install a compatible binary before project maintenance; review and commit the refreshed instructions, reload agents, and restart consoles.

The migration that introduced contributors renamed the shared manifest roster from `owners:` to `contributors:` and checkout-local identity from `owner:` to `contributor:`. It preserves contributor IDs and names, the selected local identity, claims and lifecycle, branches, reviews, accepted bases, evidence, and historical event bytes. An absent identity stays absent. Local identity is gitignored, so a migration renames only its own checkout's file; after pulling the migrated manifest, each other checkout's `tiller upgrade --project-only` or bare `tiller join` renames a legacy `owner:` key in place, provided that ID is on the roster. Other commands refuse a legacy key as `identity_unreadable` and name that remedy. Roster events are `contributor.add` and `contributor.remove`; historical `owner.add` and `owner.remove` events are not rewritten. There are no compatibility command, flag, environment, or current-schema key aliases. Acting identity uses `--contributor`, `TILLER_CONTRIBUTOR`, then local identity; execution/question `owner` fields and scalar ownership filters are unchanged. Historical events with `owner:` remain readable without rewriting their bytes.



### Pre-release schema boundaries

These apply only to projects created before the current schema line; a project started on a current build never meets them. `tiller migrate` (or `tiller upgrade`) refuses a pre-schema-5 project while any task is `in_progress`, `check`, or `in_review`, or a nonterminal epic or feature carries a PR or review ref: finish and sync that work with the older binary, have the human perform final integration merges, and retry, without cancelling or hand-editing state to satisfy the check. A pre-schema-6 project with active owned work migrates only with a human-reviewed ownership file: `tiller migrate --dry-run --json` returns `data.ownership_review`, the human fills every `confirmed_owner`, and `tiller migrate --ownership-review <file>` (or `tiller upgrade --project-only --ownership-review <file>`) applies it; stale, incomplete, or conflicting input refuses before writes. Either migration preserves history, PRs, review refs, branches, accepted bases, and evidence. Coordinate the migration commit across the team: upgrade every participant and merge the migrated state into each continuing branch, never migrating stale branches independently.

A retained legacy ref such as `tiller/epic/feature` can block creation of `tiller/epic/feature/task`. Diagnose both ref names and preserve history. For unstarted work, a non-conflicting configured branch pattern avoids the collision; deliberate archival/renaming of retained refs is a human operation, not automatic migration cleanup.

The optional `project.requires_tiller` field in `tiller.yml` is advisory, not a write fence. Every command probes it through a lenient one-field read before running, even when the schema will refuse the tree. A binary below the floor prints one stderr line naming `tiller upgrade`; exit code and stdout are unchanged, including the JSON envelope. The floor grammar is `>= X.Y[.Z]`, with the `>=` and a `v` prefix optional. An invalid floor is reported on stderr rather than silently disabling the advisory. The schema check remains the hard boundary.

---


## Harness integration

This is the part with no precedent to copy, because Tiller has to make a harness "know about" it without the harness depending on it.

The dependency arrow points the safe way. **The harness never depends on Tiller.** It does not import it, require it to run, or call it. What depends on what is the other direction: Tiller depends on knowing each harness's file conventions, where it looks for skills, which instruction file it loads, and those are stable, public, file-based conventions, not an API. Integration is Tiller writing files into locations the harness already watches. Remove Tiller and the harness is unaffected: some inert markdown and a few skill files it is free to ignore.

The same independence applies to the managed project, not just its harness. Agents may choose to invoke Tiller through the harness for local workflow work; neither the harness's operation nor the project's ordinary commands may require it. See [Managed-project independence](#managed-project-independence) for the project contract and permitted local check direction.

**Each harness adapter places neutral files:** skill discovery files, the instruction-file pointer block, and any registration the harness requires. The neutral skills and pointer text are the source of truth. Claude Code is the only shipped discovery adapter. Other harnesses, including Codex, can use the committed instruction files, `.tiller/orientation.md`, and `tiller guide` without an adapter.

**Unbuilt multi-harness design.** Auto-detection from files such as `.claude/`, `CLAUDE.md`, and `AGENTS.md`, a `--harness` selector, and Codex, Gemini, or Copilot discovery adapters remain future work. Each adapter should stay limited to file placement rather than introduce a harness dependency or duplicate the skill bodies.

**`tiller join` wires a checkout to a project.** Onboarding is three standalone commands: `init`, a human's `tiller add contributor <id> --name "<name>"`, then `join --as <id>`. Init never calls either of the other two. Join selects identity after clone or on deliberate identity change and shares the same idempotent artifact refresh as maintenance. Routine maintenance uses upgrade, not re-join.

`join` selects the contributor for this checkout; it never registers one. It resolves `--as`, then `TILLER_CONTRIBUTOR`, then local `.tiller/identity.yml`, validating the selected id against the committed roster. Unknown ids refuse with `contributor_unknown` and name human registration through `tiller add contributor <id> --name "<name>"`. Join writes only local identity plus the shared artifact refresh; it does not migrate. Source mutations resolve `--contributor`, `TILLER_CONTRIBUTOR`, then local identity. A present but unreadable identity refuses with `identity_unreadable` only when attribution actually needs that fallback; the remedy remains `join --as <id>`. Explicit contributor/environment overrides bypass the identity file, and artifact-only maintenance never reads it for attribution. Read-only commands may degrade to no acting contributor. There is no global config or machine-default contributor.

### The pointer block

Join's onboarding and upgrade/migrate's shared refresh upsert a fenced managed region in the project's harness instruction file. Human text outside it is preserved; malformed, duplicate, or inverted markers refuse rather than guessing ownership. The legacy opening marker still says `managed by tiller join` for cross-generation recognition, not to prescribe re-join as maintenance. There is no separate pointer-generation registry; the orientation generation guard prevents older maintenance from overwriting newer guidance. The same guard covers `tiller init` reruns: a checkout whose orientation is newer than the binary's generation refuses with `orientation_newer` before init ensures the skeleton or refreshes orientation, so a rerun changes nothing. Schema 5's hard rule distinguishes authorized task-to-aggregate assembly from the human-only final integration merge.

```markdown
<!-- tiller:begin (managed by `tiller join`; edits inside are overwritten) -->
## Tiller

This project uses Tiller to run its SDLC workflow. Specs, decomposition, planning, and acceptance gates live in `.tiller/`, under git, beside the code they describe.

One hard rule: drive work to the edge of done and stop. You may propose, build, attach evidence, run gates, and open PRs. Authorized execution includes creating task branches and merging them into the recorded feature or epic aggregate, never into the configured integration branch. A gate is the human's decision, not the human's keyboard: run `approve` only on an explicit human instruction naming the nodes. You never attest a human gate and never cross work to done, even on instruction; the final integration merge is the human's, and Tiller records the crossing from it.

PRs are for code: a change that touches only `.tiller/` commits directly, with no pull request.

Read `.tiller/orientation.md` for the full rules and how to work. Run `tiller status` before each session to see where things stand.
<!-- tiller:end -->
```

The hard rule names every human-only crossing in the current verb scheme and carries the delegation model's split (the gate is the human's decision, not the human's keyboard, per [cli-surface.md](./cli-surface.md) and [skill-set.md](./skill-set.md)): `approve` is the start gate (approving a plan or a draft tree), the one keystroke the human can explicitly delegate by naming the nodes; attesting a human review gate, asserting a fast-forward crossing, and marking anything done are never the agent's to run, even on instruction, since everything above a task completes by rollup over the human's merges. The merge stays the one boundary that genuinely holds.

The hard rule stays inline, where it is always in the harness's loaded context, because it is the one thing that must never be missed. Everything else is delegated to `orientation.md`, which keeps the block small and stable while the detail lives in a file the agent reads on demand. A wording improvement to the pointer block is therefore a schema-era decision, not a patch-release convenience, because otherwise two developers on different binary patches would churn the same committed region back and forth.

### orientation.md

`.tiller/orientation.md` is a generated, committed, do-not-edit file, the agent's zero-dependency front door. It is generated because its content is Tiller's rules rather than the project's craft, so Tiller owns it; committed because a fresh clone must orient an agent before `tiller` is installed; and keyed to a generation counter rather than the binary patch, one that moves only when the rules the file states change, whether the driver is a schema bump or a deliberate revision of the rules text. Every binary in a generation emits a byte-identical file, so re-emitting is a no-op diff with no git churn until the stated rules deliberately change. Unlike the local board, orientation remains committed.

`tiller upgrade` refreshes this checkout's orientation after binary selection and migration. `tiller upgrade --project-only` refreshes it offline with the running binary, without joining or changing identity. Join's onboarding and standalone migrate reuse the same refresh. Readiness names a stamped generation mismatch: behind the binary, use project-only; ahead of the binary, use upgrade. Refresh refuses to overwrite higher on-disk generations rather than downgrading them. Byte-identical outputs are not rewritten.

The orientation generation is 57, with schema 12 and eleven embedded skills. The generation moves only when the rules the generated file states change, and older generated instructions must not overwrite a higher orientation generation. Individual claims, frozen execution ownership, contributor-versus-assignment semantics, transfer, shared closeout, baseline and design-brief guidance, project independence, and the human acceptance, final-merge, and fast-forward boundaries are stable across generations.

Use the reviewed project-only ownership migration above for schema-5 active work; only schemas 2 through 4 crossing schema 5 require old-work drain. Review and commit source and generated changes separately from binary installation outcomes. Current-schema refresh is identity-free, real migration needs a registered actor, and neither selects checkout identity. Every continuing branch/checkout must incorporate the published migration and refresh its own artifacts. Do not hand-edit installed shims or generated orientation.

Skill trigger descriptions live in embedded frontmatter; registry descriptions supply thin discovery shims and `tiller guide` supplies full bodies. Shims carry `<!-- tiller:generated-skill-shim -->` as stable ownership evidence; conservative legacy recognition never treats a directory prefix alone as permission to delete user files. Ambiguous legacy ownership is a named warning. Verify generated instructions by running the candidate binary's `tiller init` or `tiller upgrade --project-only` in a disposable project, then exercise the resulting guidance with controlled agents. This validates the candidate's generated surface without installing it or changing another project's generated files.

Instruction acceptance needs executable scenarios beyond golden or keyword checks: a real agent consumes the embedded guide in a disposable project with local Git and a temporary bare remote. Observe independent task branches, dependent coarse starts from assembled prerequisites, current-tip and review-head ancestry, task submission-state inclusion, same-PR reuse after requested changes, and the exact human acceptance/final-merge stops. The scenario driver supplies human decisions; Tiller does not orchestrate Git or host operations.

It carries about a page, in four parts:

- **What Tiller is:** the principle. Tiller holds the structure, the gates, and the state; the agent does the reasoning; the human approves at the gates; and Tiller guides rather than controls.
- **The rules you must follow:** drive lifecycle work to the edge of done and stop; mutate machine-owned state only through `tiller`, never by hand-editing it; normally record lifecycle reasoning through a command; the recovery protocol, where exit `1` is a business answer to fix and retry, `2` and `3` are a bad call to fix, and `tiller status <noun> <node>` names the legal moves; the node reference convention, naming a node to the human by its key path with the id in parentheses; and the workspace boundary, where the project directory is the whole workspace, Tiller's guidance is complete inside it, and the agent goes outside only where the developer points. `design-brief` instead returns one derived Markdown snapshot without a mutation or terminal readout. Its recovery permits only safe reads and explicit gaps, not source repair or state-writing commands; only an explicitly requested deliverable file may be written.
  Managed-project independence keeps ordinary project operations and CI separate from Tiller; local checks may invoke independently runnable project commands, never the reverse.
- **How to work:** `tiller status` to see state, `tiller next` or bare `tiller guide` for the ranked next moves, and lifecycle skills including `derive-baseline` between `orient` and `decompose`. Existing-code projects derive/reconcile docs and stop at human per-doc confirmation before ordinary ranked moves return; pending docs are reviewed, not regenerated. The list ranks human decisions holding the current feature and near-due questions first, then the reader's own unfinished work, then the current feature and epic, then recently touched work, then the rest in plan order. Approval never silently claims or starts work. Pure orient relays the list and ends the reply, starting the top agent move only after a reply that does not redirect; authorized execute prepares a verified draft before exact human acceptance, then rechecks, reuses/readies the same PR, formally submits, commits/pushes state, and reads it back. Reported merges continue only related acceptance, ordinary sync, and integration-branch publication/readback, without a new state-only PR. Missing formal submission does not prove no draft exists; pure reads establish no host facts. Explicit no-publish, genuine conflicts, unavailable host access, and actual offline work remain distinct boundaries. The agent uses Git/host tools; Tiller remains the recorder, and craft belongs to `AGENTS.md`.
  Baseline maintenance follows a touched-doc sync notice with `tiller drift`, never automatic repair or confirmation. `mend-drift` reads bounded tracked-work diffs and feature specs for `stale_design`, working-tree evidence for opt-in `shifted_design`, and states dirty or external history limitations. It follows pointer docs to live sources, distinguishes missing-path candidates from uncovered reads, and stops at explicit human confirmation of named docs.
  An explicit request for a UI/UX design brief or prototype handoff for an epic, feature, or task reaches `tiller guide design-brief` outside the ranked next list, even when the list names a gate. Merely entering the project, encountering a draft, or completing work does not trigger it. The export neither starts the named work nor changes any lifecycle gate.
  An explicit request for a code review of one epic, feature, task, bug, chore, or tweak, or naming rows of an existing review, reaches `tiller guide code-review` outside the ranked next list. The review writes only its file under `.tiller/reviews/`; a disposition records through `tiller` commands and one commit and approves nothing.
- **Where things live:** the `.tiller/` layout in brief, and `tiller.yml` for the project's archetype, contributors, and gates, with live specifics deferred to `tiller status` so the file itself stays stable.


The `orient` skill and this file are projections of the same binary, so a rich harness gets description-triggered orientation through the skill while a bare one reads the file, and the two cannot drift.

### Graceful degradation

The same mechanism runs at three fidelities. A rich harness auto-discovers the installed skills and triggers them by description. A harness with an instruction file but no skill system gets the pointer block, reads `orientation.md`, and reaches every skill through `tiller guide`. A bare shell with a local model reads `orientation.md` and runs `tiller guide` by hand. One story, degrading cleanly, and the committed `orientation.md` plus `tiller guide` are the floor that makes the no-dependency claim literally true.

### First pass: the Claude Code adapter

The shipped `tiller join` adapter is Claude Code. It requires a Tiller repo and refuses outside one with the canonical environment error, the same exit-3 posture every other command takes.

The current source catalog installs eleven embedded skills as discoverable Claude Code skills in the project, one shim per skill at `.claude/skills/tiller-<skill>/SKILL.md`: orient, derive-baseline, decompose, specify, execute, capture, mend-drift, amend, intake, design-brief, and code-review. Each shim is thin: YAML frontmatter carrying the tiller-prefixed `name` and `description` lifted from the embedded skill, then a short body that tells the agent to run `tiller guide <skill>` and follow exactly what it prints. The shim never inlines the skill body, because the project-resolved binary holds the version-correct text and a copied body would drift from it. Because the shims are per-machine projections of the binary and not source of truth, join adds `/.claude/skills/tiller-*/` to the repo `.gitignore`, along with `/artifacts/tiller/`, `/.tiller/.cache/`, `/.bin/`, `/.tiller/identity.yml`, and `/.tiller/local.yml`, all anchored to the repo root. It creates the file or appends only missing patterns. An existing anchored or unanchored twin, or a plain parent directory line that covers the pattern, counts as present. Human entries and other `.claude` content stay untouched. Legacy ignore files gain missing patterns without hiding the project's own nested directories.

It writes the managed pointer block into `AGENTS.md` at the repo root, creating the file with just the block when absent and otherwise upserting only the fenced `tiller:begin`..`tiller:end` region in place, never touching the human's content around it. The merge is byte-stable, so running `join` twice yields byte-identical shims and an identical `AGENTS.md`.

It also upserts a redirect into `CLAUDE.md` at the repo root: created with just the fenced region when absent, only the region replaced when present, human content untouched, byte-stable on rerun. The region points to `AGENTS.md`, not a copy of the pointer block, so `AGENTS.md` stays its canonical and only home. Claude Code auto-loads `CLAUDE.md`, not `AGENTS.md`; the redirect makes the pointer available in the session. Join reports the installed shims, wired instruction files, and next move.

Additional harness auto-detection and discovery adapters remain unbuilt. Join's identity selection and refusal behavior are specified above; registration stays solely with `add contributor`, never a side effect of wiring a harness.

---


## Identity

Identity lives in the repo, in two halves, and nowhere else. The committed `contributors` roster in `.tiller/tiller.yml` is the shared "who can hold work" on this project, maintained by `tiller add contributor <id> --name "<name>"` and `tiller remove contributor <id>`, deliberate human-gated acts; the gitignored `.tiller/identity.yml` (`contributor: <id>`) is the per-checkout "who I am here," written by `tiller join --as <id>`. `join` selects among the registered contributors; it never adds one, so an id absent from the roster is refused rather than fabricated onto it. A developer joins each checkout they work from, so a fresh clone, which carries the roster but no identity file, must run its own `join` against an id a human has already registered. From then on every Tiller command attributes to that owner, resolved from `--contributor`, then `TILLER_CONTRIBUTOR`, then `.tiller/identity.yml`, and a command that resolves the acting contributor from either fallback says so on its own output rather than staying silent about it. There is no profile in `~/.config/tiller`, no machine-default contributor, and no global config of any kind; keeping identity in a gitignored in-repo file rather than a shared global also dodges the version-skew trap of one `~/.config/tiller` straddling projects pinned to different schema lines. There is no separate agent identity: a developer's agent acts as that developer, so a claim, a proposal, and a finished task all attribute to the human, and two engineers stay distinct only because they are two people, not because a human and their agent are two actors.

On one machine the agent and human share a filesystem, shell, and Git credentials, so local identity cannot enforce a human-only gate. Skills and orientation carry that guidance; the human's merge is the boundary. Tiller state validation must not become a managed project's branch-protection check.

---


## Onboarding

The path from nothing to real work is short. The developer who starts a project:

```
brew install --cask madeintandem/tap/tiller   # once per machine (macOS); Linux: the curl installer
cd acme
tiller init --name "Acme" --key acme --kind brownfield
                                               # scaffold with an empty roster, an explicit surface root,
                                               # and an optional observation of this repository's HEAD
tiller add contributor robin --name "Robin Hale"    # register yourself on the committed contributors roster (a human act)
tiller join --as robin                         # select that identity for this checkout: write the gitignored
                                               # .tiller/identity.yml, wire the harness
tiller status                                  # the scaffold exists; readiness still needs design inputs
# reconcile existing docs against code, then move them under .tiller/design/
# or write a pointer page if another consumer needs the original location
# legacy-migration also needs an existing mode: reference root in tiller.yml
tiller check readiness
# once ready, point the agent at the repo and the decompose skill
```

A teammate joining a project that already exists:

```
git clone …                              # .tiller/, the pointer block, and orientation.md arrive with the repo
brew install --cask madeintandem/tap/tiller
cd acme
tiller add contributor dana --name "Dana Whitfield"   # only if dana is not already on the roster; a human runs this once
tiller join --as dana                    # wire this machine: install skills, reconcile the block, select identity
tiller check readiness
```

`tiller init` creates the project with an empty roster and no identity; a human's `tiller add contributor` registers who may hold work; `tiller join` selects a registered contributor and wires the checkout. Init never calls either of the other commands, and join registers no one. The proposed `tiller doctor` command is not implemented. It would check installation, harness wiring, configured identity, and whether gate commands are runnable, deferred, or broken. Launcher-resolution checks also belong to unbuilt design, not current onboarding. The agent proposes a decomposition and the human can inspect the tree in `tiller serve`.

Kind describes the relationship to code predating adoption, not which later feature changes are
legal. Fresh brownfield and legacy-migration projects declare `.` as `mode: surface`; legacy
readiness also requires an existing root explicitly tagged `mode: reference`. Every declared root
needs a legal explicit mode in every kind. These projects optionally record `project.adopted` when
HEAD can be observed, not a dirty-worktree fingerprint or an external legacy revision. A rerun
keeps the human-owned manifest and its recorded identity, kind, roots, and original or absent
anchor; `join` and `migrate` never backfill it. Greenfield starts with `code: []` and no anchor.

Design preparation remains separate from scaffold success. Move in-repo docs when Tiller takes
ownership, or use a pointer page for a source that must stay in place, then reconcile their claims
against code. External `--design` imports copy without changing their sources on first scaffold.
A valid rerun succeeds with a skipped-import notice; it never imports or appends provenance.
Contributors and any required reference declaration still need completing before readiness, even with
imported docs. Init leaves an existing `.gitignore` alone; join upserts the shared Tiller patterns.
The baseline ledger, named-doc confirmation gate, and the `derive-baseline` startup move are built. Fresh imports
refuse a root-level `baseline.yml`, including case variants, before scaffold writes; import prose,
not another project's confirmations. The current build uses schema 12, eleven skills, and orientation generation 57.
Fresh scaffolds include `stale_design` in drift checks; `shifted_design` remains opt-in. Init reruns, join, and migrate
never rewrite an existing manifest's checks list or confirm its baseline. Empty or invalid modes that
older binaries tolerated now need a human manifest correction; no migration rewrites them, and
`requires_tiller` remains advisory.

**Unbuilt diagnostic design.** `doctor` would report setup in three gate states rather than one. `runnable` means the configured command can be invoked now. `deferred` means the command is well-formed but the project scaffold it names does not exist yet, which is healthy for a fresh greenfield repo before the first code is generated. `broken` means the scaffold should exist for this project state but the command cannot run. Under this design, day-one greenfield could be clean with deferred gates; in-review work would still require runnable gates. These are local Tiller diagnostics, not managed-project CI prerequisites.

---


## Documentation

Tiller's docs live in the git repo, like everything else Tiller believes in, and v1 keeps them lean.

The **machine-readable** surface is the specs in `docs/spec/`, generated `orientation.md`, `tiller guide`, and `tiller status --json` with `--help`. The **human-readable** surface provides:

- A **README** as the front door, opening on the non-coercion principle (the best one-line answer to "what is Tiller"), the opinions in sixty seconds, the install one-liner, and a link onward.
- A **quickstart** that walks install to first decomposition to seeing it light up in `tiller serve`, the five-minute path to the aha.
- A **concepts page** carrying the mental model, where the diagrams live.


The visuals are designed here as an inventory and produced downstream, the way the console's visual design is its own step. Five earn their place, each teaching something prose labors at:

1. **The lifecycle ribbon:** a node from `draft` to `done`, with the human-gate crossings marked. The work loop and the edge-of-done stance in one picture.
2. **The tree as progressive elaboration:** epics to features to tasks, narrative to specification to implementation detail, the mental model most worth a picture.
3. **Reasoning, recording, approving:** the three-way split, now also the non-coercion principle made visible.
4. **The surface map:** the binary embedding the console and skills, `.tiller/` in the repo under git, the harness wired by block and orientation and skills, the dependency arrow pointing the safe way.
5. **The onboarding flow:** install, init, add contributor, join, decompose. "Productive in minutes," drawn.


The documentation site is built; further tutorials are not.

---


## Decisions settled so far

The design reasoning behind these contracts is in the [design history](../design-history.md). The sections above retain the current contracts and distinct future designs.


---


## What's next

Unbuilt distribution scope remains the multi-engine launcher and cache, `upgrade --project`, the opt-out update nudge, additional harness discovery adapters and selection, lossless unknown-field preservation, and schema fences. The [roadmap](../roadmap.md#not-built-yet) tracks implementation coverage; none is implied by the shipped self-update, stamp-only migration, or Claude Code wiring. The CI emitter is rejected by design and is not future scope.