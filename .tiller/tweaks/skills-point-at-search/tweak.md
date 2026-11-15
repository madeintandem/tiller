# Skills tell the agent to read the shelves where one search would do

## What

Point the skills' "read before adding" steps at `tiller search`. `capture` says to read `tiller ls bugs`, `tiller ls chores`, `tiller ls tweaks`, `tiller ls questions`, and the proposed amendments before adding; `derive-baseline` says to read the question ledger, proposed amendments, and the shelves before opening a forward-work question; `decompose` carries the same duplicate check for shelf items found in a document. Each should name `tiller search <keyword>` (with `--kind` where it helps) as the first move for finding an existing item, and keep the listing commands for when the search is not enough.

Today `tiller search` is named once in the generated orientation (under "How to work") and once in `orient`. No other skill mentions it.

## Why

An agent follows the commands a skill names. Listing three shelves and a 28-question ledger, then opening files to compare, costs thousands of tokens and still misses an item worded differently. In the session that set this project up on 2026-11-14 and 2026-11-15, the agent ran captures and a baseline scan and never used `tiller search`.

## Done when

- `capture`, `derive-baseline`, and `decompose` each name `tiller search` in their duplicate-check step.
- The instruction-contract tests in `internal/skills` cover the new wording where they pin these steps, and `docs/spec/skill-set.md` matches.
- The orientation generation moves only if the stated rules in the generated orientation change.
