# Sync says nothing when a merge names shelf items that never started

## What

Make `tiller sync` and the readout report a warning when commits newly landed
on the integration branch name a bug, chore, or tweak id in their messages
while that node is still `ready`, `in_progress`, or `check` with no recorded
submission.

Today the shelf pass of sync observes only nodes in `in_review`
(`internal/core/mutate/sync_workitem.go`), so work that merged without a
recorded submission is invisible to it.

## Why

Observed on Headway with Tiller 1.0.6 on 2026-11-19. PR #13 merged at
`e66e7f7` with commit messages naming seven bug ids, for example
"B-rs3vxf Preserve Inbox capture order after reverted filing" and a commit
body listing "B-67kx27 B-wsll19 B-timrgi B-1ycsiq B-0ozrzn B-k14sjj". None of
the seven had been started. `tiller sync` printed:

```
Sync: no merged branches to record.
```

and the readout said "No crossings to record" with "queue: 7 bugs, 0 chores,
0 tweaks ready". Nothing told the developer that the merge they had just
reported was unrecorded, or why.

The orientation already requires every commit for shelf work to carry its
node id, so the evidence is in the history. A silent no-op reads as success.

## Done when

- After a merge whose commits name a shelf node id, with that node not
  `in_review`, `done`, or `cancelled`, `tiller sync` prints a warning naming
  the node, its status, and the commit, and the JSON output carries the same
  warning.
- The warning says the node has no recorded submission and names the next
  lawful step.
- The warning moves no status and records no crossing.
- A commit that only records state for the node, such as an approval, does
  not raise the warning.
- The warning stops once the node is recorded as submitted, done, or
  cancelled.
- The readout's needs-attention section shows the same finding.
