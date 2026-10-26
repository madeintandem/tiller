---
name: capture
description: Use when an investigation or the developer's description identifies a concrete bug, housekeeping chore, or minor tweak that should enter a shelf for triage. Writes one complete shelf contract, records provenance and attachment through tiller add, commits the proposal, and stops before approval.
---

# capture

## Trigger

An investigation has reproduced a defect, or the developer has described a concrete defect or maintenance obligation that should remain visible beyond this conversation. Capture records the finding. It does not triage it. A human answer to a forward-work question that names a defect is that description.

## Reasoning

For authored prose, follow **External references** in `tiller guide orient`.

Choose `bug` when observed behavior violates expected product or system behavior. Choose `chore` for housekeeping without code side effects, such as documentation or operational upkeep. This is classification guidance, not policing or validation of Chores. Choose `tweak` for a minor adjustment that may change code; choose a Task for substantive planned work. Done work stays done: capture forward adjustments as new work rather than reopening completed records. If the work needs its own feature acceptance contract or changes the work tree, route to `decompose`, `specify`, or `amend` instead.

Write one contract per independently triageable outcome. A bug contract records the reproduction, expected behavior, actual behavior, and the observable fixed-when condition. A chore or tweak contract records what must be done, why it matters, and the observable done-when condition. Do not write an implementation plan here.

Use `--attach` only when an epic, feature, or task is the honest place the finding came from. Attachment is provenance and reading context, not parenthood or a gate. Use `--source` when the finding came from an investigation note, review, issue, log, or other durable artifact; cite the narrowest useful path and line range. Record shelf dependencies only when another bug, chore, or tweak must settle first.

The result remains `proposed`. Triage decides whether the contract is a bug, chore, or tweak, a duplicate, invalid, or ready to work. This skill must never approve, claim, start, resolve, or cancel the node.

## Recovery

On a refusal, follow `.tiller/orientation.md`. Fix an invalid local key, unresolved attachment, or invalid shelf dependency and rerun `tiller add`. Re-adding the same key is an idempotent readback, not a second finding.

## Choreography

```text
tiller add bug blank-on-reload --title "Form is blank after reload" \
  --attach form-engine/certification-form-loading \
  --source docs/investigations/2026-08-29-reload.md#L12-L44
# replace .tiller/bugs/blank-on-reload/bug.md with the complete contract:
# ## Reproduction
# ## Expected
# ## Actual
# ## Fixed when

tiller add chore refresh-runbook --title "Refresh the operations runbook" \
  --source docs/operations/toolchain-review.md#L8-L16
# replace .tiller/chores/refresh-runbook/chore.md with the complete contract:
# ## What
# ## Why
# ## Done when

tiller show bug bugs/blank-on-reload
tiller add tweak button-spacing --title "Adjust button spacing"
# replace .tiller/tweaks/button-spacing/tweak.md with the complete contract:
# ## What
# ## Why
# ## Done when

git add .tiller && git commit -m "capture: blank-on-reload"
# STOP. The status remains `proposed`; never approve it in capture.
```

## Hand-off

Up to the human for triage. Hand them the full shelf reference, the source and attachment you recorded, and the exact `tiller approve bug|chore|tweak <node>` command without running it. If they reject or redirect the finding, `cancel` remains human-only. `resolve` is delegable only on an explicit human instruction naming the shelf node and either `--clear` or the replacement carrier target.
