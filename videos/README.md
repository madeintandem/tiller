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
