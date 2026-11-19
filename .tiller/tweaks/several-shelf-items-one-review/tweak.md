# Several bugs, chores, or tweaks cannot share one review

## What

Add a batch noun for shelf work, so the instruction "fix these n bugs under
one PR" has a lawful path: a batch names several approved bugs, chores, or
tweaks, owns one branch and one pull request for them, and crosses every
member to `done` when that one merge lands.

The batch is a first-class record with its own id, key, branch, gates, and
review surface. Its members stay ordinary shelf nodes that point at the batch
reviewing them. No member acts as a lead for the others, and the existing
carrier pointer (`tiller resolve --by`) is not the mechanism.

Today each shelf node owns its own branch and its own PR at every review grain
(`docs/spec/artifact-schema.md`: "a bug is one unit with one PR at every
setting"), and the execute guidance has no section for several shelf nodes
under one review.

## Why

Observed on Headway with Tiller 1.0.6 on 2026-11-19. The developer told the
agent: "There are 7 proposed bugs. Approve them all. Fix them all under one
PR." The agent approved the seven bugs, found no batch guidance in
`tiller guide execute`, and then worked outside the lifecycle: a hand-named
branch `fix/core-model-seven-bugs`, one PR, and no `claim`, `start`, `check`,
or `submit` for any bug. After the PR merged, `tiller sync` printed "Sync: no
merged branches to record." and all seven bugs still read `ready`.

Batching small related fixes into one review is a common request. When Tiller
has no path for it, the agent must choose between the developer's instruction
and the lifecycle, and either choice loses something.

Developer ruling, 2026-11-19: the path is a batch noun. Extending the carrier
so one "lead" bug carries the rest was considered and rejected as a hack,
because it makes one member's record stand in for a review that belongs to
all of them.

## Done when

- A batch can be created from two or more approved shelf nodes, mixing bugs,
  chores, and tweaks, and carries its own id, key, and status.
- An agent may create a batch on a developer instruction that names the
  members and asks for one review. It needs no instruction naming a lead.
- Starting the batch records one canonical branch for it, and that branch,
  its gates, and its PR are recorded on the batch, never on one member.
- A member of a live batch is not an execute target on its own: `start`,
  `check`, and `submit` on it refuse and name the batch.
- `tiller show` and `tiller status` on any member name the batch, its branch,
  and its PR, and the same views on the batch list every member and its state.
- Every commit on the batch branch carries the id of the member it serves.
  Each bug in the batch still requires its own regression test, and the gates
  recorded on the batch cover the whole branch.
- One `tiller sync` after the merge crosses the batch and every member to
  `done`, with each member's done basis naming the batch.
- Requested changes resume the batch on its existing branch and PR.
- Removing a member before the merge returns it to its own queue. Cancelling
  a member, or cancelling the batch, each has a defined outcome that strands
  no node.
- A node cannot join two live batches, and a node that already has a live
  carrier or its own started work cannot join one.
- The queue, the readout, `tiller next`, and the console show a batch as one
  unit of work with its members.
- `tiller guide execute` carries a batch section with choreography, and says
  that an agent asked for a shared review of shelf work must use a batch
  rather than a hand-named branch.
- `docs/spec/artifact-schema.md`, `docs/spec/cli-surface.md`, and
  `docs/tiller-commands.md` describe the batch noun and its lifecycle.
