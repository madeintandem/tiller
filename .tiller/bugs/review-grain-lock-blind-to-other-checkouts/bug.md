# The review-grain lock misses work started in another checkout

## Reproduction

Observed on 2026-11-15 with the release binary current that day (v1.0.1 or v1.0.2; the install
changed during the session) in a new greenfield project. It was not rerun in a scratch project, and
the cause below is derived by reading the session record and the spec.

1. On the integration branch in the primary checkout, an epic with approved features had its review
   grain set to `epic` and that change was committed.
2. In a separate worktree, three tasks of one feature were claimed, started, checked, and
   submitted. Their state commits were made on their task branches and merged into the aggregate
   branch `tiller/reviews/epics/<epic>`. None of that state was on the integration branch yet,
   which is the normal position before the review merges.
3. In the primary checkout, on the integration branch with a clean tree:
   `tiller configure feature <epic>/<feature> --review-grain feature`
4. The same command was then run in the worktree, on the aggregate branch.

## Expected

`docs/spec/cli-surface.md`, "Safe configure window", says: "An affected feature PR or review ref,
or an affected task in `in_progress`, `check`, or `in_review`, refuses with
`review_grain_work_in_flight`. A recorded aggregate branch locks every review grain change on that
feature". Three of the feature's tasks were `in_review` and its aggregate branch existed, so the
change should have been refused wherever it was run.

## Actual

In the primary checkout (step 3) the command exited `0`:

```
review grain: feature recorded on <epic>/<feature> (<id>).
resolved grain here is now: feature (feature explicit).
Commit this change to integration before starting tasks: work started in another checkout or
worktree resolves the grain it can see.
```

It wrote the feature's state file and one event. In the worktree (step 4) the same command exited
`1`:

```
Blocked: changing review grain on <epic>/<feature> (<id>) would retarget <task> (<id>), currently
in_review; finish and sync that review first.
```

The lock reads task state from the files in the current checkout. On the integration branch the
tasks still read `ready` and the feature records no aggregate, so the guard has nothing to refuse.
The agent noticed the disagreement and discarded the uncommitted change. Had it been committed,
integration would have recorded `feature` grain for a feature whose tasks ride an epic review. What
that does to the later submit and sync was not observed.

The printed reminder covers the opposite direction only: work started elsewhere after a grain
change. Nothing warns that the lock itself cannot see work already started elsewhere.

The aggregate branch `tiller/reviews/epics/<epic>` and the three canonical task branches existed as
local refs in the same repository when step 3 ran.

## Fixed when

- Step 3 refuses with `review_grain_work_in_flight` or `review_grain_branch_locked`, exit `1`, and
  writes no state or event, when the canonical aggregate branch for the affected feature or epic,
  or the canonical branch of an affected task, exists as a ref in the repository and carries state
  that would lock the change.
- The refusal names the branch where the in-flight state was found.
- A grain change with no such refs still succeeds as it does today.
- A regression test builds this two-checkout arrangement and asserts the refusal from the
  integration branch.
- `docs/spec/cli-surface.md`, "Safe configure window", says which refs the lock reads beyond the
  current checkout.
