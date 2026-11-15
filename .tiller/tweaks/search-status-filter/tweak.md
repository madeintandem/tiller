# Search has no filter by status

## What

Add a way to narrow `tiller search` results by node status, alongside the existing `--kind`. At minimum, one flag that keeps only live work (not `done` or `cancelled`); a `--status <status>` that names one status is the fuller form.

## Why

The commonest reason to search is to check whether something is already tracked before adding it. Finished and cancelled nodes match the same words and crowd the answer. Observed on 2026-11-15: the first result for `rubric` was a `done` bug.

## Done when

- A search can be limited to live nodes, and done and cancelled nodes are left out of that result.
- The filter composes with `--kind` and with `--json`.
- Documents, which have no status, follow a rule stated in `docs/spec/cli-surface.md`, "Searching: search", and `docs/tiller-commands.md`.
