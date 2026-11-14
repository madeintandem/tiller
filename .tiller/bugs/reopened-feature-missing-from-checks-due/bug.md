# A reopened feature with stale gates is missing from checks_due

Imported from [GitHub issue 19](https://github.com/madeintandem/tiller/issues/19).

## Reproduction

Observed on a build of main during the late-work rollup fix on 2026-11-14 (commit `a670d57`, shipped in v0.24.0). Not re-run for this capture.

1. Take a feature to `done` with every task merged and the feature's gates green.
2. Add a task under the done feature, approve it, complete it, and merge it, with no `tiller sync` in between.
3. Run `tiller sync`. It crosses the late task and reopens the feature to `active` with its gates and criteria marked `stale`.
4. Read `checks_due` in the sync or readout output.

## Expected

A feature whose every task is done and whose required gates are not green is due a feature check. `checks_due` lists a feature whose gates were never run; a feature whose gates went stale on a reopen is in the same position and should be listed on the same terms.

## Actual

The reopened feature is absent from `checks_due`, because its gates are stale and not missing. The readout does route to `tiller check feature <id>` and `tiller status` shows `gate evals: stale`, so a person finds it. A consumer that reads `checks_due` alone sees nothing to do until the next sync records the crossing with policy violations.

## Fixed when

- After step 3, `checks_due` names the reopened feature with the `tiller check feature <id>` command.
- The Home desk and the Build board show the same feature as awaiting its check.
- `docs/spec/cli-surface.md` states that `checks_due` covers stale required gates as well as missing ones.
