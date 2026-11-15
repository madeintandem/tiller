# Search does not reach the documents a baseline points at

## What

Let `tiller search` find text in the source documents the baseline ledger records. Today it matches node fields and Markdown under `.tiller/` only. When a project's as-is pages are pointers, the contracts they point at live outside `.tiller/` (in this repository, `docs/spec/`, `docs/tiller-commands.md`, `README.md`, and the others listed under `derived_from` in `.tiller/design/baseline.yml`), and search cannot see them. Extend the corpus to the files the ledger names as sources, at their current working-tree content, grouped in results the way unowned prose already is.

Whether to go further (every Markdown file under a declared root) is a decision for whoever picks this up; the ledger's sources are the bounded, already-recorded set.

## Why

A pointer page says where a rule lives, so the next step is always to open the source it names. An agent that searches for a term gets the pointer page and not the passage, and falls back to `grep` over the repository. Observed on 2026-11-15: `tiller search "rubric"` returned the as-is page's mention and nothing from `docs/spec/artifact-schema.md`, where the rule is.

## Done when

- A keyword that appears only in a ledger source document returns that file and its matching lines, in human and `--json` output.
- A source path the ledger records that no longer exists is skipped without failing the search.
- The existing refusals for an unsafe or unreadable corpus still hold, and no file outside the project is read.
- `docs/spec/cli-surface.md`, "Searching: search", describes the wider corpus.
