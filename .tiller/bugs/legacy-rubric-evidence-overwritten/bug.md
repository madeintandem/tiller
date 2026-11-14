# Rubric evidence recorded before content-addressed refs can still be overwritten

Imported from [GitHub issue 14](https://github.com/madeintandem/tiller/issues/14).

## Reproduction

Derived by reading the code at v0.24.0, not run.

1. Start from a project where a rubric criterion was recorded and attested with a binary older than v0.24.0. Its attestation's `evidence_ref` is the rubric packet's `output_path`, `artifacts/tiller/evals/<node>-<criterion>.json`.
2. With v0.24.0 or later, run `tiller eval <noun> <node> --emit-rubrics`. The packet's `output_path` for that criterion is the same path.
3. Write a new grade to that `output_path`, as the execute skill directs.

## Expected

`docs/spec/artifact-schema.md`, "Retained rubric evidence", states the rule: no recording can change the bytes behind a reference an attestation or an earlier criterion state holds. Evidence recorded from v0.24.0 on meets it, because it is retained under `artifacts/tiller/evals/<node>-<criterion>-out-<sha256>.json`.

## Actual

For a reference recorded before v0.24.0, step 3 replaces the bytes the older attestation points at, before Tiller runs at all, because the handoff path and the old evidence reference are the same file. The spec says so plainly and nothing rewrites old state. Severity is graded `important`: attested evidence is silently replaced, but only for references recorded before v0.24.0 and only when that criterion is graded again. Read strictly, the scale's "data loss or corruption" would make it a blocker; the approver may regrade it.

## Fixed when

- After a new grade is written to the packet's `output_path`, an attestation recorded before v0.24.0 still reads its original evidence bytes through `/api/evidence` and on disk.
- The packet's `output_path` is a location no recorded evidence reference uses, and the execute skill, the rubric packet golden, `docs/spec/skill-set.md`, and `docs/spec/artifact-schema.md` name the new location.
- No migration rewrites an existing `evidence_ref`.
