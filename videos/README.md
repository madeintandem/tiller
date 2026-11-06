# videos

A standalone Node/TypeScript pipeline that turns a reviewed script into a how-to video about Tiller: narration in JC's voice, on-screen terminal and console footage from a real run of the pinned Tiller version, rendered with Remotion and uploaded to YouTube. It runs on JC's Mac only; there is no CI job.

Design: `docs/plans/2026-11-06-how-to-video-series-design.md`.

## The human track

1. Write or edit `videos/scripts/<id>.yaml`.
2. Read it with `npm run preview -- <id>`.
3. Approve it with `npm run approve -- <id>`. This records the script's content hash in `approvals.json`. Any wording change voids the approval; line endings and trailing whitespace do not.
4. Record the live intro clip into `videos/.local/` (never committed).

No script renders unless its current hash matches its approval.

## Setup

Node 26. From this directory: `npm install`. Playwright browsers are installed separately when capture work starts.

## npm scripts

- `npm test`: run the vitest suite.
- `npm run typecheck`: `tsc --noEmit`.
- `npm run approve -- <id>`: validate `scripts/<id>.yaml` and record its hash in `approvals.json`.
- `npm run preview -- <id>`: print the script as Markdown (chapters, narration, then what is on screen in italics).

- `npm run check`: verify `tiller --version` matches `series.yaml`, rebuild the fixture into a temp dir, and diff every step against the committed snapshots after masking IDs, SHAs, timestamps, durations, and temp paths.

- `npm run term-still -- <out.png> [stage] [frame]`: render one frame of the `TermSmoke` composition fed by a committed snapshot, to eyeball the terminal.

More scripts are added here as the pipeline tasks land.

## Layout

- `series.yaml`: the pinned Tiller version.
- `approvals.json`: script ID to approved content hash.
- `scripts/`: one YAML script per video.
- `src/script/`: schema, loader, hashing, approvals.
- `.local/`, `out/`, `node_modules/`: gitignored.
- `src/theme.ts`, `src/fonts.ts`, `src/components/`: MiT palette, fonts, and frame components (Frame, TitleCard, Kicker, LowerThird, Outro).
- `src/index.ts`, `src/Root.tsx`: Remotion entry and compositions (`Smoke` for now).
- `public/brand/`: MiT logo SVGs used by the frame.
- `fixture/`: the brownfield Acme Orders demo repo the videos are recorded from. `app/` is the existing app, `content/` the authored files the agent and the human write, `stage1.sh` to `stage3.sh` one script per video, `lib.sh` their helpers (`run`, `say`, `put`, `mark`).
- `snapshots/s1.jsonl` to `s3.jsonl`: every step each stage ran, one JSON object per line, with each Tiller command's real output.
- `fixture/marks/*.tar.gz`: the whole fixture checkout, `.git` included, saved at named moments (`s1-end`, `s2-end`, `s3-review`, `s3-end`, and the steps between).

## The fixture

`bash fixture/run.sh` rebuilds the Acme Orders repo from scratch with the `tiller` on `PATH` (or `TILLER=<binary>`), runs the three stages, and rewrites `snapshots/` and `fixture/marks/`. Commit both. `--out DIR` writes them under `DIR` instead; `--check` writes nothing and compares the fresh steps against the committed snapshots. `bash fixture/fixture_test.sh` builds Tiller from this repo and checks the whole run.
