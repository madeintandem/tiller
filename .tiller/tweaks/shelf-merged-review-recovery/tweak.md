# A shelf item's already-merged PR cannot be recorded

## What

Extend historical merged-review recovery to bugs, chores, and tweaks, so a
shelf node whose fix merged before Tiller recorded its submission can be
closed accurately, with the reviewed head, the integration result, the PR
number, and a reason on record.

Today `tiller submit <feature|epic> --recover-merged` is the only recovery
path, and `docs/spec/cli-surface.md` says "Task and shelf recovery refuse".
Ordinary `submit bug` must run on the node's recorded branch, so after the
merge its state commits land on a branch that has already merged and never
reach the integration branch without a second merge.

## Why

Observed on Headway with Tiller 1.0.6 on 2026-11-19. Seven approved bugs were
fixed in PR #13 from a hand-named branch, with no `start`, `check`, or
`submit` recorded (see tweaks/several-shelf-items-one-review). The PR merged
to `main` at `e66e7f7`. The seven bugs still read `ready`, and the agent
reported that "Tiller 1.0.6 supports historical merge recovery for features
and epics, not bugs." The only honest options left are to redo the lifecycle
on throwaway branches or to leave fixed bugs in the queue.

A merge that outruns its record is the same accident on a shelf node as on a
feature, and the human needs the same audited way out.

## Done when

- A human-authorized recovery command, run from the integration checkout,
  records a merged review against a bug, chore, or tweak that is `ready`,
  `in_progress`, `check`, or `in_review`, taking the reviewed head, the merge
  commit, a reason, and the PR number when a remote is configured.
- Tiller verifies the local Git evidence the same way feature recovery does,
  and refuses when the head is not contained in the integration result.
- One PR can be recovered against several shelf nodes.
- After the recovery delta is committed, ordinary `tiller sync` crosses each
  recovered node to `done` with a basis that marks it as recovered, and the
  readout shows it as a recovered review.
- Recovery approves no gate and records the missing gate evidence as a
  warning, as feature recovery does.
- The seven Headway bugs can be closed with this command.
- `docs/spec/cli-surface.md`, `docs/tiller-commands.md`, and the execute
  guidance describe shelf recovery.
