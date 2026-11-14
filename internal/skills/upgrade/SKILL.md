---
name: upgrade
description: Use only when the developer explicitly asks to upgrade Tiller, migrate this checkout's schema, or refresh its generated guidance. Runs the maintenance the request names, reports each phase's outcome, and stops for the developer to review and commit the diff.
---

# upgrade

This skill assumes `.tiller/orientation.md`. Read `.tiller/orientation.md` first if it is not in context. If it lacks a section this skill names, it is stale: tell the developer the project needs `tiller upgrade --project-only`.

## Trigger

The developer explicitly asks to upgrade Tiller, migrate this checkout's schema, or refresh its generated guidance. Maintenance is separate from choosing lifecycle work. The request sits outside the ranked next list and authorizes only the maintenance it names: never claiming, starting, specifying, approving, or the next queued work.

Entering a project, finishing work, or asking what to do next does not trigger this skill. A stale-guidance notice from another skill is a reason to tell the developer the project needs `tiller upgrade --project-only`, not to run maintenance unasked.

## Reasoning

### Scopes

`tiller upgrade` updates the installed binary through its installation owner, then maintains the current checkout: its supported schema migrations and generated instructions and artifacts.

`tiller upgrade --project-only` maintains this checkout with the running binary, offline, with no network or package-manager calls and without Homebrew. Use it for stale generated guidance, other checkouts, and a second checkout after a machine update.

`tiller upgrade --binary-only` updates only the installed binary and deliberately excludes project maintenance. The scopes are mutually exclusive: `--binary-only` cannot combine with `--project-only`, and `--project-only` cannot select a version with `--version`.

`tiller migrate` applies this checkout's registered forward schema migrations and refreshes its generated instructions with the running binary; it never changes the installed binary.

One checkout's maintenance never updates other branches, worktrees, or projects. Those stay untouched; teammates incorporate the reviewed migration and maintain their own checkouts. Review and commit the reported project diff.

### Identity

Maintenance never selects a different checkout identity. `tiller join --as <you>` remains onboarding or deliberate identity selection, never the routine refresh remedy. Missing identity still calls for `tiller join --as <you>` after human registration.

Artifact-only refresh needs no identity. A real migration needs a registered actor, resolved through `--contributor`, then `TILLER_CONTRIBUTOR`, then local identity. `--contributor` names the migration's actor without changing the checkout's identity.

### Homebrew ownership

Verified Homebrew installations keep brew as their installation owner. `tiller upgrade` delegates only the matching Tiller package to its owning brew, suppressing implicit dependent upgrades and cleanup; genuine dependencies and metadata refresh remain brew's responsibility. Exact version pins refuse there. Never overwrite Cellar/Caskroom files or any other directory brew manages, and never bypass private-download authentication, including after a failure. Brew may already have changed the machine before the target's project compatibility is established; report that boundary rather than promising a global rollback.

### Team adoption

Adoption is coordinated Git work, not independent migrations in stale clones. Do not migrate stale branches independently.

First retain a compatible executable and a recoverable committed baseline. Pause team state mutations. Reconcile state and active owners on the designated integration branch. Inspect, review, and apply one migration once, in one designated checkout, then commit and publish it through normal Git.

Upgrade every participant, and merge the migrated state into each continuing work branch without discarding its task-specific state or task records. Before resuming, refresh each checkout, restart consoles, reload agent instructions, and check schema, identity, task owner, branch, and legal next action.

Adoption is not authorization or an evidence waiver: it is not permission to skip verification or to migrate a project.

A schema fence is local, not a distributed lock: it does not stop stale unmigrated clones, and old binaries in unmigrated clones can still conflict. Never mutate migrated state with an older binary. After adoption, recovery must preserve new work rather than blindly restore old schema files.

### Schema notes

Schema 6 freezes execution ownership, not a feature-wide claim. Inspect an old checkout with `tiller migrate --dry-run --json`; it writes nothing. Save the whole report or its `data.ownership_review` outside source state. The review lists every started, unfinished execution unit: each task, and each node on every shelf. A checkout older than schema 6 predates the Tweak shelf, so in practice its entries are tasks, bugs, and chores. Have the human fill every initially empty `confirmed_owner` with a registered contributor, leaving all other snapshot fields unchanged. Apply it once with `tiller migrate --ownership-review <file>` or `tiller upgrade --project-only --ownership-review <file>`; a stale or incomplete review refuses.

Schema-5 active work and open reviews keep their branches, PRs, statuses, and evidence. Only migrations crossing schemas 2 to 4 into schema 5 require draining old shared-branch work first, with a verified compatible old binary, including the human's final merges and sync. Keep that binary's directory on PATH for the temporary drain session only, then leave the session.

Schema 8 adds the Tweak shelf. The 7-to-8 migration changes only the manifest version: existing Bug and Chore records remain unchanged, and no work drain is required. Older binaries refuse schema 8; upgrade every participant before resuming state mutations.

Schema 9 renames the project roster to `contributors:` and the checkout's local identity to `contributor:`, preserving the selected ID while renaming its field. Use `tiller add contributor <id> --name "<name>"`, `tiller remove contributor <id>`, and `--contributor` or `TILLER_CONTRIBUTOR` for acting identity. Task, Bug, Chore, and Tweak claims remain `owner`; ownership filters remain `--owner`. The 8-to-9 migration preserves IDs, claims, active work, and history without a work drain or ownership review. Legacy command, flag, and environment names are not aliases.

## Recovery

On a refusal, follow `.tiller/orientation.md`.

Read phase outcomes even on failure: a binary replacement or source migration can have completed before refresh fails. With `--json`, the binary phase and the project's migration and artifacts phases each report their own status, and `complete` covers the whole requested scope. A successful binary update followed by a failed project phase is incomplete. Preserve the completed phases and their state, and review the reported diff; retry `tiller upgrade --project-only` only after the named cause is repaired.

A phase reported `recovery_required` carries `mutation_recovery_needed`: preserve the transaction journal and recover the transaction first, never delete the cache or journal. Unknown durability is not proof of no writes.

Never downgrade the schema or newer generated guidance, erase caches to bypass a refusal, or infer unchanged files from a lost child result.

Disk refresh does not reload running processes: restart existing serve processes and reload harness instructions as needed.

An already-running older binary does not acquire this upgrade logic mid-process, during its own replacement. Retain a compatible old executable before the first new install, then run the new binary's `tiller upgrade --project-only` explicitly afterward.

## Choreography

Run only the scope the developer named.

```
# before: what this machine runs and whether the guidance is stale
tiller --version                                         # the running binary's version
tiller check readiness                                   # names stale generated guidance and other gaps

# the scope the request names, one of:
tiller upgrade                                           # update the installed binary, then migrate and refresh this checkout
tiller upgrade --project-only                            # maintain this checkout with the running binary, offline
tiller upgrade --binary-only                             # update only the installed binary; no project writes
tiller upgrade --version vX.Y.Z                          # an exact self-managed release; refused for Homebrew installs
tiller migrate                                           # migrate and refresh this checkout; never touches the binary

# a real migration names a registered actor when the checkout has no identity
tiller upgrade --project-only --contributor <id>         # the migration's actor; checkout identity unchanged

# schema 6 adoption: inspect, the human confirms owners, apply once
tiller migrate --dry-run --json                          # writes nothing; save the report outside .tiller/
tiller migrate --ownership-review <file>                 # apply the human-reviewed mapping
tiller upgrade --project-only --ownership-review <file>  # the same apply through upgrade

# after: the reported project diff, for the developer to review and commit
git status --short
git diff
```

## Hand-off

Up to the developer. Report each phase's outcome, binary, migration, and artifacts, with the files it changed, its notes and warnings, and any failure by its reason. Name what remains: a serve process to restart, harness instructions to reload, and other checkouts and teammates that maintain their own. Then stop for the developer to review and commit the reported project diff.

There is no `tiller readout upgrade`: maintenance is not a lifecycle seam, so the reply ends with the phase report, without a readout block or the ranked list. Do not continue into the next queued work.
