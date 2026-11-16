# Historical review recovery refuses a manifest whose required gates name bug, chore, or tweak

## Reproduction

Observed with tiller 1.0.2 and reproduced with 1.0.3 (`ca5923e`) against a
managed project whose feature review merged before `tiller submit feature`
recorded it.

1. Start from a project with a configured remote whose `.tiller/tiller.yml`
   scopes a required command gate to the shelf nouns as well as feature and
   task:

   ```yaml
   gates:
     required:
       evals: { kind: command, run: "tiller eval ${node.noun} ${node.id}", on: [feature] }
       build: { kind: command, run: "scripts/build.sh", on: [feature, task, bug, chore, tweak] }
       tests: { kind: command, run: "scripts/test.sh",  on: [feature, task, bug, chore, tweak] }
   ```

2. Take a feature at feature review grain whose tasks are all `in_review`, with
   passing gates recorded under that manifest, and merge its aggregate review
   branch into the integration branch without ever running
   `tiller submit feature`.
3. On the integration checkout, run the recovery the execute readout names:

   ```text
   tiller submit feature <feature> --recover-merged --head <reviewed-oid> \
     --merge-commit <merge-oid> --reason "<explanation>" --pr <n> --json
   ```

## Expected

Recovery accepts the historical manifest and records the review binding, so
ordinary `tiller sync` can then observe the merge. The manifest is valid: the
loader accepts it, and `tiller check` ran and recorded gates under it for every
node in the review. `docs/spec/artifact-schema.md` says the `on` list is read
at task grain for all three shelf nouns, so `bug`, `chore`, and `tweak` entries
change nothing about which gates a feature or task must carry.

## Actual

Recovery refuses before reading any state:

```json
{
  "ok": false,
  "command": "submit",
  "data": {
    "node": "F-o5mumr",
    "noun": "feature",
    "prerequisite": "restore supported historical feature/task gate scopes",
    "reason": "historical_policy_invalid",
    "ref": "build"
  }
}
```

The check in `readRecoveryHistory`
(`internal/core/mutate/recover_review_history.go`) rejects any required gate
whose `on` list holds a noun other than `feature` or `task`. The prerequisite
asks the developer to restore a historical manifest, which would mean rewriting
merged history, so the refusal has no lawful recovery. Every review in such a
project that merges before its submit is stranded: the tasks stay `in_review`
with `review_missing`, the feature never rolls up, and a bug carried by the
review never closes. `tiller sync` records no crossings.

## Fixed when

- `tiller submit feature --recover-merged` and `tiller submit epic
  --recover-merged` succeed against a historical manifest whose required gates
  list `bug`, `chore`, or `tweak` in `on` alongside `feature` or `task`, and a
  following `tiller sync` records the task crossings and the rollup.
- A regression test covers that manifest shape and fails without the fix.
- A required gate whose `on` list holds a noun that is not `feature`, `task`,
  `bug`, `chore`, or `tweak` is still refused with `historical_policy_invalid`.
