# Feature prerequisites inside an epic-grain epic deadlock, and nothing warns before the grain locks

## Reproduction

The silent half was run with the v1.0.2 release binary on 2026-11-15, in an empty directory after
`git init` and one empty commit:

```
tiller init --key demo --kind greenfield --name Demo
tiller add contributor jc --name "J" && tiller join --as jc
tiller add epic foundation --title "Foundation"
tiller add feature foundation/skeleton --title "Skeleton"
tiller add feature foundation/spike --title "Spike" --depends-on foundation/skeleton
tiller configure epic foundation --review-grain epic
tiller check readiness
tiller add feature foundation/spike2 --title "Spike2"
tiller link feature foundation/spike2 --depends-on foundation/skeleton
```

The start refusal was observed the same day in the first epic of a new greenfield project, driven
by an agent following the shipped skills. It was not rerun in a scratch project. The sequence there
was:

1. Decompose and specify recorded `depends_on` from three features to a fourth feature in the same
   epic. The prerequisite was real: the fourth feature creates the build and test scripts the
   others' gates run.
2. All features were approved and the developer ran
   `tiller configure epic <epic> --review-grain epic`.
3. The prerequisite feature's three tasks were started, checked, submitted, and merged into the
   aggregate branch `tiller/reviews/epics/<epic>`.
4. `tiller start task <first task of a dependent feature> --json` was run from the aggregate.

## Expected

`docs/spec/cli-surface.md` says "Readiness reports genuine prerequisite loops as
`dependency_cycle` gaps" and that "mixed epic/feature completion deadlocks block". Tiller therefore
treats this arrangement as a known deadlock. A command that creates a known deadlock should refuse
it or say so in its own output, at the moment it can still be undone.

Ordering features inside one epic review is also an ordinary thing to want. Task prerequisites
already support it: in `internal/core/mutate/task_branches.go`, `taskStartBase` lets a task start
once a same-feature prerequisite is `in_review` and included in the aggregate, and bases the new
task on the aggregate. The execute skill describes this as "A dependent coarse start points at the
aggregate containing prerequisites". A developer reasonably expects a feature prerequisite inside
the same epic-grain epic to be satisfied the same way.

## Actual

`tiller configure epic foundation --review-grain epic` exits `0` and prints only the recorded
grain, the commit reminder, and a Next block. It does not mention that the change turned an
existing prerequisite into a loop. `tiller link feature ... --depends-on` on an epic already at
epic grain also exits `0` and prints only the new edge. `tiller next` afterwards lists the epic
approval and no cycle.

Only `tiller check readiness` reports it:

```
epic/feature completion obligations for foundation, foundation/skeleton, foundation/spike form a
dependency cycle; ... epic review for foundation completes riding features only after aggregate
merge; submission requires all riding tasks submitted and independent task/feature-grain overrides
done; review the recorded prerequisites and break the execution loop
```

Nothing in the specify or execute flow runs readiness at this point, so the agent in the observed
project learned of the loop only after it had built and submitted the whole prerequisite feature.

The start then refused, exit `1`:

```
"dependency": "<prerequisite feature id>", "dependency_status": "active",
"reason": "task_dependency_unfinished"
```

`taskStartBase` accepts a feature or epic prerequisite only when it is `done`, `cancelled`, or
`archived`. At epic grain a riding feature reaches `done` only when the epic's review merges, and
the epic cannot be submitted until every riding task is submitted, so the dependent tasks can never
start.

By then the exits were closed. Changing the prerequisite feature to its own review refused with
"would retarget ... currently in_review; finish and sync that review first", as the safe configure
window requires. The only way forward was `tiller unlink` on every edge, which removed true
prerequisites from state and left the order to soft order alone.

## Fixed when

- In an epic whose resolved review grain is `epic`, a task under a feature that depends on another
  feature of the same epic starts once every non-cancelled task of the prerequisite feature is
  `in_review` and included in the epic's aggregate branch. Its recorded `base_branch` is the
  aggregate.
- A prerequisite feature in the same epic whose tasks are not all submitted into the aggregate
  still refuses, and the refusal names what to submit and merge, as the task-prerequisite refusal
  does.
- With that rule in place, the reproduction above no longer makes `tiller check readiness` report a
  `dependency_cycle` for these edges, because they no longer deadlock.
- For any arrangement that readiness still classifies as a blocking cycle, the command that creates
  it (`configure --review-grain`, `approve --review-grain`, `link --depends-on`, or
  `add --depends-on`) prints the same diagnosis in its own human output and `--json` envelope.
- Regression tests cover the dependent start from the aggregate, the refusal while the prerequisite
  feature is incomplete, and the diagnosis printed by `configure` and `link`.
- `docs/spec/cli-surface.md` states the same-epic feature prerequisite rule next to the task one,
  and the execute skill's passage on dependent coarse starts covers it.
