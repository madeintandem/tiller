---
name: derive-baseline
description: Use when a brownfield or legacy-migration project has no confirmed as-is design doc, or the developer asks to derive or reconcile its baseline, or to scan existing sources for unbuilt work. Reads existing docs and code, derives or translates module-grain descriptions, records recoverable provenance and one question per unbuilt capability, and stops for human confirmation of named docs. Re-entry reviews pending docs rather than regenerating them.
---

# derive-baseline

## Trigger

A `brownfield` or `legacy-migration` project has no confirmed doc in `.tiller/design/baseline.yml`, even if work is already recorded. This seam ranks ahead of decompose. A developer may also explicitly request baseline work. A newly declared uncovered root does not automatically trigger this seam in this build.

Read the ledger before deciding that documents must be written. If pending docs already exist, follow **Re-entry and corrections** below: inspect and reconcile those docs or hand the committed proposal back for review. A missing confirmation is not evidence of missing prose. Do not create another set merely because guide still names this seam.

A developer may also ask, on a project whose baseline is already confirmed, to scan existing sources for unbuilt work. That request runs only **Record forward work** below, through its standalone arm. Nothing in recorded state names this arm; never start it on your own.

## Reasoning

### Read before writing

Read `.tiller/orientation.md`, `.tiller/tiller.yml`, the existing ledger if present, all relevant docs under `.tiller/design/`, and the repository's own documentation, including its README, agent instructions, and in-repo specs. Imported docs are inputs, not proof that a baseline matches code. Read the relevant code before drafting, and preserve existing current-state work rather than starting a parallel source of truth.

For brownfield, understand the declared `mode: surface` roots, the existing code this project can change. For legacy-migration, understand the declared `mode: reference` roots, including explicitly declared external roots: they are the old system's behavior, not an implementation target. Every code root needs an explicit mode. Never edit reference code or relabel it as surface to get a command through. The project's surface code may be a new implementation, not the old system you are describing. No reference declaration is a separate legacy readiness gap, even after a design import.

Read `project.adopted` when present. It records this project's historical HEAD at adoption, not an external reference repository's HEAD, not a baseline fingerprint, and not a claim that the worktree was clean. If its commit resolves here, use the relevant historical code as context and identify material differences from the current working tree. Reconcile the description against current code too, including changed, added, and removed files, because approval fingerprints the working tree. For external references, inspect their actual current contents; the project adoption commit cannot stand in for their history. When adoption is absent, the object is unavailable, or a relevant path did not exist there, say exactly which history is missing or unusable. Never invent an adoption point or silently substitute HEAD as one.

Inventory the inputs you actually read and their destination modules. This inventory is reasoning for the run's notes, not a new state file. It lets the human check source conservation, which the ledger's path links alone cannot prove.

### Derive or translate at module grain

Where no documentation describes the code, **derive** the current-state doc from code. Explain observed behavior, not an intended rewrite. Mark genuine uncertainty and record a question rather than fill it with an invention.

Where documentation exists, **translate** it into the house format below, reconciling it with code in the same pass. This is a refactor, not a rewrite: move and regroup content, retain source wording as closely as the structure allows, and do not summarize away requirements or embellish behavior. When source and code disagree, describe the observed code, identify the source claim and exact paths, and open a question about the disagreement. Distinguish as-is facts from intended future changes instead of silently turning the latter into existing behavior.

The grain is a coherent module or surface, not one doc per file and not an undifferentiated whole-repository summary. Split a monolith along meaningful ownership and behavior boundaries. A module can describe several files or directories. If the human chooses to retain a coarse monolith, explain that its broad path coverage makes later comparison and review less selective; do not silently choose it for convenience.

Use this house format for each `.tiller/design/as-is/<module>.md`:

- **What this surface is:** its responsibility, boundaries, and current users. Say what it does today, not the feature you want to build next.
- **Data it owns:** persisted and in-memory state, invariants, identity, lifetime, and ownership. State when it owns no data rather than inventing a model.
- **Behavior to preserve:** observable rules, workflows, compatibility, failure behavior, and important edge cases downstream work must respect. Separate evidence from uncertainty.
- **Edges:** dependencies, callers, external services, interfaces, and the contracts across those boundaries.
- **Described paths:** the exact files or directories supporting this account, in the same coordinates registered with `--describes`.

These are human-owned Markdown headings, not frontmatter that the CLI interprets. Only a ledger entry makes the doc an as-is doc to Tiller. `describes` paths must exist at linking and lie at or beneath a declared code root. Reference roots are valid for this descriptive relation, unlike a feature's writable `--surface`. Use project-relative, forward-slash paths; an external reference may use `../legacy/...` only through its declared root. Do not broaden a path to a parent that contains but is outside the declared root. Do not traverse symlinks or special files to evade path safety.

### Record forward work

Sources often describe more than the code the as-is docs describe: surface roots for brownfield, reference roots for legacy-migration. Compare against that code, never against a new implementation that has barely begun. Typical leads: a spec whose feature never landed, an unbuilt section of an otherwise built spec, a roadmap. The as-is doc keeps its "not implemented" note, because that describes current behavior. Whether to build it is the human's decision, so record it as a question: not as run notes that vanish with this conversation, and not as planned work.

Open one question per unbuilt capability, not per file. Cite every source that describes it in `--source`. A code comment or TODO counts only when a document corroborates it; on its own it is not a lead. An old implementation plan counts only where it describes a capability no spec covers. Read the question ledger, proposed amendments, and the shelves first, and skip capabilities already built, already in the tree, or covered by any question, open or answered, by a proposed amendment, or by a bug, chore, or tweak. An answered question is a decision, including "leave it": do not reopen it under new wording. Re-adding an identical statement is a no-op, but a reworded duplicate is a second question.

```
tiller add question "<what the sources describe, what the code has, and the decision: build it, or leave it as history>" --title "<capability>: unbuilt" --source "<path>#<section>"
```

Repeat `<path>#<section>` inside the one `--source` value, comma-separated, when several sources describe the same capability.

A defect-shaped finding is a question too, never a captured bug. This seam reads code and docs; it does not reproduce anything, and a source/code disagreement may be a deliberate change. Deciding that current behavior is wrong is the human's call. Do not run `tiller add bug`, `tiller add chore`, or `tiller add tweak` from this seam.

The human's answer is the recorded decision. When the human answers in conversation, record it with `tiller answer question <Q-id> --answer "<text>"` on their instruction, so the ledger and later scans see it. "Build it" leads to `intake` on the cited source, or to a to-be design under `.tiller/design/`, which makes decompose due; intake needs the file on disk, so recover a retired source with `git show <blob-object-id>` before intake, using the blob the ledger recorded for it. "It's a bug" leads to `capture`. "Leave it" closes the lead. Name that next move and stop for the human's go; this seam runs neither intake nor capture.

**Standalone arm.** On an already-confirmed baseline, read the as-is docs' "not implemented" notes and their recorded `--from` provenance, recover retired sources with their recorded blob refs, reread sources that still live in the repository, and read the question ledger, proposed amendments, and the shelves. Open only the missing questions, commit exactly the question and event files, and end with the readout. The arm writes no as-is prose, links nothing, retires nothing, and approves nothing.

### Choose ownership once: move or pointer

Before translating existing sources, ask once whether the human is transferring ownership into Tiller or retaining another live consumer at the old location. If they already made that choice explicitly, use it without asking again. Offer **move** as the default: the translated module docs replace the originals. Use **pointer** when a published site, README link, or another consumer genuinely needs the original to remain authoritative. For a mixed collection, settle which exact paths fall into each choice in the same question. Do not silently leave two live copies of the same specification.

In the move arm, prepare the destination prose first so it exists for linking, record every source ref, then retire only the exact originals the translated module docs replace. Do not move or remove an input before its provenance is recorded. The committed result has one live source of truth, not both old specs and translated copies. Preserve assets and content still used elsewhere. Repoint in-repo links and agent-instruction references that named a retired path as part of the same scoped change. Do not delete a directory wholesale because one spec moved out of it.

In the pointer arm, write short module-grain pages under `.tiller/design/as-is/` that name the authoritative source and its exact sections, map those sections to the house-format concerns, and state the described paths. Do not duplicate the body. Record discrepancies and missing sections explicitly; a pointer does not certify an inaccurate source. Reconcile the authoritative prose in place only within the human's authorization; otherwise leave the uncertainty visible for review.

External `--design` imports remain source-preserving. Never move or retire files outside this project. Translate and, for a move within the project, retire the imported project-local input, not its external original. Record provenance from the committed local import. If an external source remains authoritative, use the pointer arm and avoid retaining a second live local spec. A source outside this project's HEAD cannot be passed as `--from`: first obtain a project-local import under the agreed ownership choice and commit those exact input bytes for recoverability. Do not invent an external blob ref or a working-tree hash when that prerequisite is unavailable; name the missing recoverable input and stop before retirement.

### Make provenance recoverable before retirement

Resolve `git.integration_branch` from the manifest and this project's Git conventions; do not assume `main`, `master`, or the current branch. Work directly on that integration branch for the authorized baseline move-and-split, not a feature branch or a merged review branch. Inspect the current branch, index, and working-tree changes before switching or staging. Preserve unrelated staged and unstaged changes, including unrelated `.tiller/` changes. Never stash, reset, discard, or commit someone else's work to make the run look clean. If a safe branch switch or an isolated commit is not possible, report that concrete prerequisite instead of working around it destructively.

`--from` resolves a regular source file at this project's current HEAD. It does not save untracked files, uncommitted edits, an external repository's objects, or a path that only exists in the index. Before translating or retiring a source, compare the bytes you read with its HEAD version. Commit any untracked imports or changed source inputs first, using only those exact source paths and the project's commit conventions. An unborn or non-Git project needs a real project Git HEAD before recoverable provenance and approval; do not manufacture a stamp. Describes-only linking does not require Git, but that is not permission to retire an unrecoverable source.

Observe HEAD explicitly after the input commit, verify each source exists there with `git show <head>:<source-path>`, and then record `tiller link design <doc> --describes <path>` and `tiller link design <doc> --from <source-path>` for every translated destination and each input it uses. Repeat the flags for multiple literal paths. A source split into several docs appears on each destination. A merged doc names every input. Verify the ledger's source paths and blob refs are the ones you read; `git show <blob-object-id>` recovers the original bytes even after retirement. Strip the `git:` display prefix before passing the object id to Git. Do not retire an input until all its destination refs are recorded and recoverable.

A repeat link is safe: existing membership is a no-op. A real `describes` change clears the doc's confirmation; a provenance-only update does not. Recording a new HEAD blob for the same source path updates that provenance, so do not blindly relink old retired inputs during re-entry. Never hand-edit `baseline.yml` or a confirmation block.

## Recovery

On refusal, follow `.tiller/orientation.md`'s recovery protocol. A failed link leaves source retirement pending, not authorized. Keep the source intact and repair the named path, actor, or provenance problem before continuing. Report a committed-state warning as committed state with a failed derived refresh, not as a rollback. Approval is a human gate, not a command failure to work around.

## Choreography

Read; settle move versus pointer; commit any input bytes that need recovery; observe HEAD; derive or translate; link described paths and committed source refs; record questions, including one per unbuilt capability; retire only the agreed recoverable local sources; commit the scoped result; render the readout and stop. Run only the applicable arm. The example paths below illustrate a translated module, not a required directory layout:

```
tiller check readiness
# Read manifest, ledger, existing design and repo docs, then relevant code.
# Inspect branch and all staged/unstaged work. Safely select the configured integration branch.
# If docs/product/editor.md is new or modified, commit only that input first.
git rev-parse HEAD
# Use the observed commit below, not a guessed adoption ref.
git show <observed-head>:docs/product/editor.md
# Write .tiller/design/as-is/editor.md in house format after reconciling code.
tiller link design as-is/editor.md --describes src/editor --describes src/shared/document.go
tiller link design as-is/editor.md --from docs/product/editor.md
# Inspect the ledger and recover the recorded source with git show <blob-object-id>.
# Open only questions actually found, with a short title:
tiller add question "<specific source/code disagreement or uncertainty>" --title "<short title>"
# One question per unbuilt capability, citing every source that describes it:
tiller add question "<what the sources describe, what the code has, and the decision: build it, or leave it as history>" --title "<capability>: unbuilt" --source "<path>#<section>"
# Move arm only, after every destination has its source ref. Never use force.
git rm -- docs/product/editor.md
# Fix only links that named that source. Pointer arm leaves the source in place.
# Commit the exact translated docs, retired paths, ledger, event/question files,
# and link repairs from this run, not a whole directory.
tiller readout derive-baseline
```

For a derive-only module, write the same house format and link `--describes`; omit `--from` because there is no source document. Do not create a fictitious source ref for code. For a pointer, retain the local authoritative source and replace the retirement step with the short pointer page and its section mapping. For an imported source, use its committed `.tiller/design/...` path in `--from`, never the external original.

The standalone forward-work arm runs only the question step: read the as-is docs, their sources, the question ledger, proposed amendments, and the shelves; add the missing questions; commit exactly the question and event files; and end with `tiller readout derive-baseline`.

Stage exact paths, not `git add -A` or a blanket `git add .tiller docs`. Inspect the intended patch and account for every file in it. Where unrelated work is already staged, a path-limited `git commit --only -- <exact-paths>` can keep it out of this commit; if an affected file also contains unrelated edits, isolate only the authorized hunks or stop for the conflicting work rather than include them. Do not run an unrestricted commit against an index you have not scoped. Commit the move/split, ledger, questions, emitted events, and necessary pointer repairs together so the review has the complete source-to-doc transition. Record the resulting commit for a later correction readout. Tiller writes state and never commits it for you.

## Readout

End by running `tiller readout derive-baseline` and reproducing its entire block verbatim as the last thing in your reply. Do not shorten its coverage map, collapse the fourth source or doc into an ellipsis, or write your own replacement table. The block exposes every recorded source-revision-to-doc mapping and every ledger doc with described paths and confirmation facts. Different blobs for the same source path are different source revisions. These links are not proof that every input's meaning survived translation. The block ends on its **Next:** ranked list; reproduce the list as written and never add a list of your own after it.

Before the block, add **Notes from this run** for what the ledger cannot know:

- The actual move versus pointer choice and any deliberately coarse module boundary.
- Exact source material dropped, merged, or left unplaceable, and its destination or reason. Do not fabricate a dropped count, including zero, from the ledger.
- Source/code disagreements, the paths and evidence behind them, and any missing or unusable adoption history. Say whether you reconciled current working-tree code, and name anything you could not inspect.
- Questions actually opened during this run, by short title and Q-id, and any unresolved uncertainty not recorded as a question. List forward-work questions separately, each with its capability and cited sources. The readout's **Open questions** is the current ledger count, not a count opened by this run.
- Any exact source or code prerequisite that prevented completion, without claiming a successful complete translation.

Keep notes factual, not boilerplate claims that everything was preserved. When naming work nodes, use key path with id in parentheses, never a bare id; baseline docs have path identities, not invented node ids. Nothing follows the readout, not a second Next line or an agent-authored approval summary.

## Re-entry and corrections

Read the existing pending docs, their ledger entries, the last committed proposal, and current described code before changing anything. Recover retired source bytes with their recorded blob refs; do not try to read them from HEAD after the retirement commit, recreate the old live specs, or rerun the translation into new filenames. If the proposal is complete and no correction is requested, present it for human review and stop. Do not self-approve or derive duplicates to escape the baseline move the ranked list names.

Explicit human feedback on pending docs stays within this seam, not an amendment to a work-node plan. Correct human-owned Markdown in place. Add or remove only mistaken relationships through `tiller link design <doc> --describes <path> --from <source-path>` or `tiller unlink design <doc> --describes <path> --from <source-path>`, including only the flags actually needed. Removing the final describes path unregisters the doc; it does not delete its prose. Unlink can repair recorded paths after a source or doc vanishes. Restore a missing doc from the proposal or deliberately unregister it under the requested correction, rather than hiding the readiness gap.

Reconcile corrected docs with current code, preserve valid provenance, commit only the correction, and run `tiller readout derive-baseline --since <sha>`, where `<sha>` is the commit named in the last block you reported. Reproduce that correction block verbatim and last, with observed notes before it, and return to the same named-doc human gate. A confirmed doc refreshed later also needs the human's explicit confirmation; a prose edit does not automatically update a stamp, and a provenance-only change does not make code current.

## Hand-off

The human reviews each doc against its source blobs and current described code, corrects the human-owned prose, or explicitly rejects the scoped translation. A reversal must preserve unrelated and subsequent work; never automatically revert or discard a mixed commit. The human confirms only the docs that match with `tiller approve design <doc>...`, for example `tiller approve design as-is/editor.md`. There is no blanket `--all`. This fingerprints the described working-tree code and records this project's HEAD and the acting owner; it does not edit Markdown or certify semantic source conservation.

The skill does not approve its own result. Run approval only on an explicit human instruction naming the exact docs, after stating what will be recorded; then report actual confirmations or unchanged docs and commit the command's scoped state delta. Neither a request to derive a baseline nor successful linking supplies that approval authority.

One confirmed doc releases the existing-code startup gate, not an all-doc gate. Remaining unconfirmed docs still await review but do not globally block the project. Rerun `tiller check readiness` for independent gaps such as missing design prose, contributors, reference root, root mode, or orientation; confirmation alone does not promise readiness. Then follow `tiller guide`: an empty tree reaches decompose only when a to-be design doc exists; otherwise guide reports that no work is planned, with forward-work questions inside its open-question count, while an existing work tree resumes its normal ranked list. Do not automatically cross into decompose or another seam. There are no automatic baseline drift markers, touched-doc sync reports, or new console approval actions in this build.
