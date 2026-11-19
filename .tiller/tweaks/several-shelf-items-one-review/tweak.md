# Several bugs, chores, or tweaks cannot share one review

## What

Give Tiller a lawful path for the instruction "fix these n bugs under one PR":
several approved bugs, chores, or tweaks worked on one branch, reviewed in one
pull request, and crossed to `done` together when that one merge lands.

Today each shelf node owns its own branch and its own PR at every review grain
(`docs/spec/artifact-schema.md`: "a bug is one unit with one PR at every
setting"), and `tiller resolve --by` accepts only an epic, feature, or task as
a carrier, so one shelf node cannot carry another. The execute guidance has no
section for a batch of shelf nodes and does not say what to do when the
developer asks for one.

## Why

Observed on Headway with Tiller 1.0.6 on 2026-11-19. The developer told the
agent: "There are 7 proposed bugs. Approve them all. Fix them all under one
PR." The agent approved the seven bugs, found no batch guidance in
`tiller guide execute`, and then worked outside the lifecycle: a hand-named
branch `fix/core-model-seven-bugs`, one PR, and no `claim`, `start`, `check`,
or `submit` for any bug. After the PR merged, `tiller sync` printed "Sync: no
merged branches to record." and all seven bugs still read `ready`.

Batching small related fixes into one review is an ordinary request. When
Tiller has no path for it, the agent must choose between the developer's
instruction and the lifecycle, and either choice loses something.

## Done when

- A developer instruction naming several approved shelf nodes and one review
  can be carried out with documented Tiller commands, on one branch with one
  PR, with no step that works outside the lifecycle.
- Every node in the batch records the shared review, so `tiller show` on any
  of them names the branch and PR that fixed it.
- One `tiller sync` after the merge crosses every node in the batch to `done`.
- A bug in the batch still requires its own regression test, and the gates
  recorded for the review cover the whole batch.
- Requested changes, cancellation of one node in the batch, and a batch mixing
  bugs, chores, and tweaks each have a defined outcome.
- `tiller guide execute` carries a batch section with choreography, and says
  that an agent asked for a shared review must use it rather than a
  hand-named branch.
- `docs/spec/artifact-schema.md` and `docs/spec/cli-surface.md` describe the
  batch path.
