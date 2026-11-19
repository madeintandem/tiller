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

This tweak delivers the store record, lifecycle, sync, CLI, queue, readout,
`tiller next`, execute guidance, and specs. The console is a separate tweak,
tweaks/console-shows-shelf-batches.

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

## Settled decisions

Developer rulings, 2026-11-19.

1. The path is a batch noun. Extending the carrier so one "lead" bug carries
   the rest was rejected as a hack, because it makes one member's record stand
   in for a review that belongs to all of them.
2. The batch alone carries the lifecycle: `ready`, `in_progress`, `check`,
   `in_review`, `done`, or `cancelled`. A member keeps its stored status and
   reads a derived phase, `batched`, the way a carried node reads `carried`.
   Sync crosses each member to `done` with `done_basis: batched`.
3. A batch needs no approval of its own and is born `ready`. It may be created
   while some members are still `proposed`, but `start batch` refuses until
   every member is approved and `ready`. Creating or editing a batch sits in
   the same delegation class as `resolve`: a human runs it, or an agent does on
   an explicit instruction that names the members and asks for one review.
4. Membership is editable while the batch is `ready`, `in_progress`, or
   `check`, and refuses in `in_review` until the batch is resumed. A change
   after `check` stales the batch's gates. A removed member returns to its own
   queue, with a warning that commits already made for it on the batch branch
   are the developer's to revert. Cancelling a member removes it from the
   batch. Cancelling the batch releases every member to its own queue and
   cancels none of them. A batch that loses its last member is cancelled.
5. The record lives at `.tiller/batches/<key>/batch.yml` with a `BT-` id, a
   title, and no contract file: the members' own contracts are the contract.
   Its branch is `tiller/batches/<key>`.
6. `tiller add batch <key> --title <text> <member>...` creates it and
   `tiller batch <key> --add <member>` or `--remove <member>` edits it. The
   existing `claim`, `start`, `check`, `submit`, `resume`, `cancel`, `show`,
   `status`, and `ls` take `batch` as a noun.
7. A batch has one owner through `claim batch`. A member another contributor
   has claimed refuses to join.
8. The batch runs the gate set a single shelf node runs. Bugs, chores, and
   tweaks may be mixed, and a batch displays its highest member severity.
9. The record is folded into schema 16, which is merged and unreleased. The
   orientation generation is bumped because the guidance changes.

## Done when

- A batch can be created from two or more shelf nodes, mixing bugs, chores,
  and tweaks, and carries its own id, key, and status.
- `start batch` refuses, naming the member, while any member is not approved
  and `ready`.
- Starting the batch records one canonical branch for it, and that branch,
  its gates, and its PR are recorded on the batch, never on one member.
- A member of a live batch is not an execute target on its own: `claim`,
  `start`, `check`, and `submit` on it refuse and name the batch.
- `tiller show` and `tiller status` on any member name the batch, its branch,
  and its PR, and the same views on the batch list every member and its state.
- One `tiller sync` after the merge crosses the batch and every member to
  `done`, with each member's done basis `batched`.
- Requested changes resume the batch on its existing branch and PR.
- Adding, removing, and cancelling behave as settled decision 4 describes.
- A node cannot join two live batches, and a node that has a live carrier,
  its own started work, or a recovered review cannot join one.
- The queue, the readout, and `tiller next` show a batch as one unit of work
  and do not offer its members separately.
- The unrecorded-merge warning treats a batched member as accounted for.
- `tiller guide execute` carries a batch section with choreography. It says
  every commit on the batch branch carries the id of the member it serves,
  that each bug in the batch still needs its own regression test, and that an
  agent asked for a shared review of shelf work must use a batch rather than a
  hand-named branch.
- `docs/spec/artifact-schema.md`, `docs/spec/cli-surface.md`, and
  `docs/tiller-commands.md` describe the batch noun and its lifecycle.
