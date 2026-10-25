---
name: mend-drift
description: Use when tiller drift reports a stale_spec, active_basis, answered_question, missing_surface, orphan_eval, stale_design, or shifted_design candidate. Proposes the smallest honest repair from tracked-work or working-tree evidence, then stops for human review and explicit named-doc confirmation.
---

# mend-drift

## Trigger

`tiller drift` reports a `stale_spec`, `active_basis`, `answered_question`, `missing_surface`, `orphan_eval`, `stale_design`, or `shifted_design` candidate. Detection is `tiller drift`'s; drafting the fix from here is yours. A sync `as-is docs touched` notice is only a prompt to run drift, not a candidate or permission to approve a doc.

## Reasoning

Read the candidate: its marker, the node or design-doc path, the files it names, and its recorded history anchors. Each marker names its own disagreement; design docs are path-addressed prose, never work nodes with invented ids.

**stale_spec.** A node is `done`, but its verified spec, eval, or code surface no longer matches what was verified at the done crossing: the doc content changed, or a file the completion recorded as touched is gone. The claim the node made when it finished no longer holds.

**active_basis.** Work is proceeding against a spec, an eval, or an implementation spec the human has changed since it was accepted: the feature's or task's accepted basis no longer matches the working-tree `feature.md`, `feature.eval.md`, or `task.md`. It is the one marker whose condition also refuses `start`, `check`, and `submit`, so you usually meet it first as a blocked verb and only later as a drift candidate; `tiller status <noun> <node>` names the same drift directly.

**answered_question.** The node carries a recorded assumption for a question that has since been answered, and the answer has not been reviewed into the node. The plan moved forward on a guess that has now resolved.

**missing_surface.** A path recorded as code the epic or the feature touches no longer exists in the working tree. This is planned work rather than completed work: the same vanished-file condition `stale_spec` checks on its own surface leg, caught before the work is built rather than re-examined after it is done. Recording a corrected path unions it in rather than replacing the old one, so a stale path keeps flagging until it is restored or the node leaves scope.

**orphan_eval.** A criterion's binding points at a test, or a `preserve` criterion's baseline artifact, that no longer exists on disk. The contract still asserts something the working tree can no longer produce evidence for.

**stale_design.** A confirmed as-is doc describes a surface on which tracked feature work crossed to done after confirmation, or a described path has disappeared. Read the doc, its ledger entry, every named feature's `feature.md` via the candidate's `features[].spec`, and the bounded tracked diff from `since` to `until` on the affected described paths. `since` is the confirmation's project commit and `until` is the latest relevant crossing, not an invitation to widen the diff to all later HEAD churn. For multiple features, read each feature's crossing and spec before proposing one coherent prose correction. A missing-path candidate may have no feature or upper anchor: inspect the removal and whether the description or its coverage needs changing, rather than inventing a crossing.

**shifted_design.** The opt-in working-tree comparison found changed bytes, modes, additions, or removals under a confirmed described path. There may be no tracked feature to explain it, and unrelated changes can share this noisy review surface. Read current code, `git status --short`, the stamp-to-HEAD history where available, staged and unstaged diffs, and relevant untracked files; HEAD alone omits the changes this marker detects. Confirmation fingerprinted working-tree bytes that may never have been committed, and it wrote no Git objects. The stamp's project commit is only a history anchor, not a guarantee those bytes are recoverable with `git show`. External reference trees may have no usable Git history at all, and this project's anchor is not their HEAD. State these limits and reconcile against the available current code without inventing historical content.

For either design marker, propose the smallest correction to the as-is prose and explain which observed behavior changed. If the design doc is a pointer page, follow it to the live source and propose the correction there; keep the pointer and its section-to-surface map accurate rather than copying a second live spec into `design/`. Existing `derived_from` blobs recover translated source documents, not necessarily the old code. External originals remain untouched unless the human explicitly authorizes work there. When current code is already described accurately, propose confirmation after showing that review, not a cosmetic edit.

Read the candidate's own files, anchors, features, and context before drafting. `files` names affected paths; `context` explains surrounding facts without itself being drift. An `uncovered` entry is not a candidate or a clean comparison: the sweep could not establish the facts. Report the exact reason and recovery prerequisite, whether an unconfirmed doc needs human review, a missing completion or history record needs recovery, or a path is unsafe or unreadable. Do not guess a semantic correction, treat a failed read as a deletion, or approve to suppress the warning. Restore access or correct the declared path safely, then rerun drift. A genuinely missing described path is instead a `stale_design` candidate: propose restoring it or correcting/removing its describes relation through `tiller link design` and `tiller unlink design`, never a ledger hand edit.

Draft the smallest honest fix that reconciles the record with the working tree, with the reasoning shown so the human can judge it in one read: a doc edit to `feature.md` or `task.md`, an eval update to `feature.eval.md`, a corrected binding reference, a surface re-recorded through `tiller link --surface` (with the moved or deleted path dropped through `tiller unlink --surface` once the current location is linked), an accepted basis refreshed, or an assumption reviewed in. Never auto-apply it. The model is the same one the schema states everywhere else: detect, propose, approve. `tiller drift` already detected; you propose; the human approves the diff, exactly as they approve any other change to the plan.

stale_spec carries one asymmetry worth saying to the human plainly. orphan_eval, and the doc or eval edits described above generally, clear on the next sweep, since the sweep re-reads the file fresh every run. stale_spec instead compares against the completion record pinned once at the done crossing, and which leg fired decides what clears it. On the eval leg of a done feature, and for an `orphan_eval` on a done feature, edit only the bindings (a moved or renamed test, a command, a baseline artifact path) in `feature.eval.md`, then open an amendment as `tiller guide amend` describes, stage `tiller amend <A-id> --correct-eval <feature> --reason "<why>"`, and stop for the human's `tiller approve amendment <A-id>`. Add `--retire <criterion-id>` for each check you deliberately removed, since a criterion may leave the contract only when named. The approval re-pins `verified_against.eval` to the corrected file, so the next `tiller drift` comes back clean on that leg. It refuses a change to any criterion's statement, polarity, required flag, or baseline surface, and it adds no criterion: changed acceptance is forward work, not a correction. The spec leg and the surface leg of `stale_spec` have no such command, so a forward-going correction to a done feature's `feature.md`, or a surface path that moved, does not clear the candidate on its own; only a byte-identical revert does, or the node reopening and re-crossing to done under new work. Draft that edit anyway, but say plainly that the candidate persists until one of those two things happens rather than promising the next `tiller drift` comes back clean.

Design markers clear differently: once the human has reviewed the proposed prose against current code, `tiller approve design <doc>...` records a fresh working-tree confirmation for exactly the named design-relative docs. A prose edit or repeated `link` alone does not reconfirm them. Changing describes invalidates confirmation, so review the repaired coverage before approving. Never infer approval from permission to investigate, draft, commit, or mend; only explicit human instruction naming the docs delegates the keystroke. There is no blanket approval and no automatic approval after a diff. Reconfirmation can clear a tracked stale candidate without an extra code commit; do not manufacture one merely to advance HEAD.

When the drift is not one candidate but a cascade from a single upstream change, a renamed column that stale-specs a dozen done features, say, surface the cascade to the developer as one finding rather than mending marker by marker: name every node it touches and stop there. A cascade is a plan reshape; hand it to the developer whole.

## Recovery

On any refusal, follow the recovery protocol in `.tiller/orientation.md`.

## Choreography

Run the sweep, read one candidate, draft its fix, and stop for the human; the clearing act differs by marker.

```
tiller drift                                              # read marker, node or doc path, files, and anchors
tiller drift --json                                       # design candidates also carry features and an upper anchor
# uncovered: name the failed comparison and its recovery prerequisite, not a guessed fix

# draft the fix for one candidate, active_basis on form-engine/nrca-form-loading (F-79dh3q):
#   read its files and since, then edit feature.md, task.md, or feature.eval.md
#   toward what the human actually wants, with the reasoning shown beside the diff
# stop. the human reviews the diff and the reasoning. on their explicit
# instruction naming the node you may run the clearing act yourself, stating
# what you are about to record; otherwise hand them the exact command. run the
# one that matches the candidate's own marker:
tiller approve feature form-engine/nrca-form-loading --refresh-basis                     # active_basis
tiller approve feature form-engine/nrca-form-loading --reviewed-assumption Q-h2n7m5      # answered_question
tiller link feature form-engine/rendering-modes --surface app/models/form_definition.rb  # missing_surface
tiller unlink feature form-engine/rendering-modes --surface app/models/form.rb           # missing_surface, after linking where the code moved
# orphan_eval on a non-done node clears by the eval edit alone, no further command.
# stale_spec or orphan_eval on a done feature's eval bindings: edit feature.eval.md,
# then stage the reviewed correction and stop for the human's approve:
tiller add amendment eval-fix --title "Correct the loader eval bindings"                  # then write its narrative
tiller amend eval-fix --correct-eval form-engine/nrca-form-loading \
  --reason "loader tests moved into two files" --retire C-4                              # --retire only for a check removed on purpose
#   the human runs: tiller approve amendment eval-fix   (re-pins verified_against.eval)
# stale_spec on the spec leg or the surface leg: the edit is still the right fix to
# draft, but say plainly that the candidate persists until a byte-identical revert
# or a reopen-and-re-cross, not a forward correction alone (the Reasoning above says why)
# stage only the reviewed prose and exact command-written state/event paths
git add -- .tiller/epics/form-engine/features/nrca-form-loading/feature.md
git commit -m "mend-drift: reconcile form-engine/nrca-form-loading"
# a mend that touches only .tiller/ commits directly on the integration branch, no PR
tiller drift                                              # once the fix lands, confirm the candidate cleared
```

The baseline arm uses a different review surface and clearing act:

```
tiller drift --json
# read design/as-is/workspaces.md, its ledger entry, and each features[].spec
# stale_design: git diff <since-sha> <until-sha> -- <affected-described-paths>
# use actual returned commit values without their git: prefix; quote literal paths
# shifted_design: inspect history where available AND staged/unstaged/untracked
# working-tree evidence; state dirty-confirmation and external-history limitations
# draft a prose correction, or correct the live source when the doc is a pointer
# missing described paths: propose restoration or CLI link/unlink coverage repair
# stop with the diff, reasoning, exact doc paths, and the command for human review
# ONLY after explicit human instruction naming this doc:
tiller approve design 'as-is/workspaces.md'
# commit only the reviewed prose/source and exact changed ledger/event paths
tiller drift                                              # report remaining candidates and uncovered entries honestly
```

## Hand-off

Up to the human, who reviews the drafted fix and its reasoning, then approves it; mend-drift never applies a fix on its own. Name a work node by its key path with the id in parentheses. Name a design candidate by `design/<doc>` and hand over the shell-quoted `tiller approve design <doc>` command, with the path relative to `.tiller/design/`, never a fake id or a work-node approval. Approval means the human reviewed that named doc against current code, including any live pointer source; it is never inferred from the request to repair drift. Out to craft when the reconciliation is real test-writing rather than a doc or eval correction: mend-drift drafts the reconciliation, and deep implementation goes to execute and the project's own `AGENTS.md` and `CLAUDE.md`, retaining execute's authorized publication and human merge boundaries.
