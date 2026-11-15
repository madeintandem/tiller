# Search prints a matching line in full, however long

## What

Cap the text `tiller search` prints for one matching line, in human output, and trim around the match so the matched term stays visible. Prose in this repository is written one paragraph per line, so a single hit can be hundreds of characters. Keep the full line available under `--json`, or mark truncation there, so a consumer can tell.

## Why

Search is the cheap way for an agent to find a node or a passage before loading a file. Observed on 2026-11-15: `tiller search "rubric"` printed lines of roughly 200 characters each and several per result, so a broad query costs far more context than the handful of results suggests.

## Done when

- A matching line longer than the cap prints a bounded excerpt that contains the match, with a visible mark where text was cut.
- A line under the cap prints unchanged.
- `--json` either carries the whole line or says the excerpt was truncated; the choice is stated in `docs/spec/cli-surface.md`, "Searching: search".
