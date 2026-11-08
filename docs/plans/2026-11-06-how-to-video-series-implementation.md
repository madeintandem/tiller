# Tiller how-to video series Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `videos/` pipeline in this repo that turns an approved script into a narrated, MiT-styled, captioned how-to video and uploads it to YouTube, then the three first-arc videos built with it.

**Architecture:** Each video is a YAML script of beats (spoken words plus what is on screen). A brownfield Acme fixture is driven through the real pinned `tiller` binary by stage scripts that record every command's real output and save the repo state at named marks. A build command checks approval and the snapshots, generates narration through ElevenLabs, captures the console from saved marks with Playwright, lays out a timeline from the narration timings, renders it with Remotion, masters it with ffmpeg, and writes captions, chapters, and a thumbnail. A separate publish command uploads through the YouTube Data API.

**Tech Stack:** TypeScript run with `tsx`, Remotion 4 (`remotion`, `@remotion/bundler`, `@remotion/renderer`, `@remotion/google-fonts`), React 19, `yaml`, Playwright (Chromium), ffmpeg/ffprobe from Homebrew, ElevenLabs REST API over `fetch`, `googleapis`, vitest. Fixture scripts in bash, following `site/screenshots/lib.sh`.

**Spec:** `docs/plans/2026-11-06-how-to-video-series-design.md`

## Global Constraints

- Every video renders to 8:00 or less; target 5 to 8 minutes. The build fails past 8:00.
- Every video opens with JC's live intro clip; no intro clip, no render.
- No narration text reads command syntax, flags, node IDs, or paths aloud; those appear only in on-screen text.
- No script renders unless its current content hash matches its recorded approval.
- Tiller is pinned in `videos/series.yaml`; the build refuses a `tiller --version` that differs from the pin.
- Every on-screen Tiller command and its output comes from a real run of the pinned binary (committed snapshots). Agent-side prose and files are authored.
- MiT styling on the frame only (`~/dev/mit-style-guide/mit-style-guide-2026-v1.html`): Space Grotesk display, Inter body, Space Mono mono, the 18 palette colors with their exact hex values. Tiller console and terminal keep their own look.
- Music under the intro and outro only, never under narration.
- Captions come from script text plus ElevenLabs timings, uploaded as a caption track, never burned in.
- Uploads set `status.containsSyntheticMedia: true`; the description carries the disclosure "The narration is my words, in my voice, performed by AI." Privacy comes from config: `private` until the YouTube API audit passes, then `unlisted`. The pipeline never sets `public`.
- Secrets, intro clips, music, narration cache, and renders live under `videos/.local/` (gitignored) and never enter git.
- Runs on JC's Mac only. No CI job.
- House rules: work in a worktree, commits signed off (`git commit -s`) with the session trailer, do not fast-forward main until JC says so. No em dashes in prose.

## Review Focus

- Running publish twice for the same video must not upload a second copy; it refuses and names the existing YouTube ID (Task 11).
- Switching from the stand-in voice to JC's clone, or editing one beat's words, must regenerate exactly the affected narration and nothing stale (Task 6).
- A script saved with CRLF line endings or trailing whitespace on an untouched line must not void its approval, while any wording change must (Task 1).
- Missing credentials, a missing intro clip, or an unapproved script must fail before any paid API call or long render starts (Task 10).
- Chapters must satisfy YouTube's rules (first at 0:00, at least three, each at least 10 seconds), or YouTube silently drops them (Task 9).

---

## Human track (start now, in parallel with Tasks 1 to 11)

These are JC's steps. Each gates a later task, named in brackets.

- [ ] **H1: Voice clone.** Record 30+ minutes of clean speech (quiet room, one mic, natural reading of your own writing). Create an ElevenLabs Professional Voice Clone. When ready, put its voice ID in `videos/.local/config.yaml` as `elevenlabs.voiceId`. [gates final renders in Task 12; Tasks 6 to 12 use a stock voice until then]
- [ ] **H2: Voice test.** Listen to `npm run voice-test` output (Task 6) in your clone. Pass or fail against "robotic or AI slop." [gates Task 12 final render]
- [ ] **H3: YouTube API.** Create a Google Cloud project, enable YouTube Data API v3, create an OAuth desktop client for the channel's Google account, download its JSON to `videos/.local/youtube-client.json`, and submit the YouTube API Services audit. Verify the channel (phone) so custom thumbnails work. When the audit passes, set `youtube.privacy: unlisted` in `videos/.local/config.yaml`. [gates Task 11 live run]
- [ ] **H4: Remotion license.** Buy the Remotion company license before Task 2 starts.
- [ ] **H5: Music.** Pick one licensed track; save it as `videos/.local/media/music.mp3` and record its license in `videos/MUSIC.md`. [gates Task 8 live run]
- [ ] **H6: Intro clips.** Film one short intro per video once its script is approved; save as `videos/.local/media/intros/v1.mov` (v2, v3). [gates Tasks 12 to 14 renders]

---

### Task 1: Package, script format, and approvals

**Files:**
- Create: `videos/package.json`, `videos/tsconfig.json`, `videos/README.md`, `videos/series.yaml`, `videos/approvals.json`
- Create: `videos/src/script/schema.ts`, `videos/src/script/load.ts`, `videos/src/script/approval.ts`, `videos/src/cli/approve.ts`, `videos/src/cli/preview.ts`
- Modify: `.gitignore` (add `/videos/.local/`, `/videos/node_modules/`, `/videos/out/`)
- Test: `videos/test/script.test.ts`, `videos/test/fixtures/v-test.yaml`

**Interfaces:**
- Produces:
  - `type Actor = "agent" | "you"`
  - `type Show = { kind: "card"; kicker: string; heading: string; body?: string } | { kind: "term"; stage: string; from: string; to: string } | { kind: "console"; capture: string } | { kind: "outro" }`
  - `type Beat = { id: string; chapter?: string; say: string; show: Show }`
  - `type Script = { id: string; title: string; description: string; tags: string[]; intro: { transcript: string }; beats: Beat[] }`
  - `type Capture = { id: string; mark: string; path: string; actions?: { click?: string; waitFor?: string; scroll?: number }[]; seconds: number }` (in the script file as top-level `captures: Capture[]`; add `captures` to `Script`)
  - `loadScript(path: string): Script` (throws `ScriptError` with the beat ID for any schema violation)
  - `scriptHash(path: string): string` (sha256 hex)
  - `isApproved(id: string, hash: string, approvalsPath?: string): boolean`
  - `series: { tiller: string }` loaded by `loadSeries(path = "videos/series.yaml")`
  - npm scripts: `test` (vitest run), `typecheck` (tsc --noEmit), `approve`, `preview`

- [ ] **Step 1: Scaffold.** `videos/package.json` (private, `type: module`), dev deps listed in Tech Stack, scripts `test`, `typecheck`, `approve: tsx src/cli/approve.ts`, `preview: tsx src/cli/preview.ts`. `series.yaml` is `tiller: 0.22.0`. `approvals.json` is `{}`. README states what `videos/` is, the human track, and every npm script (fill in as tasks land).

- [ ] **Step 2: Write the failing tests** in `videos/test/script.test.ts`:
  - `loads a valid script`: `loadScript("test/fixtures/v-test.yaml").beats.length === 3`, first beat `show.kind === "card"`.
  - `rejects a beat whose say contains a command`: a `say` matching `/\btiller [a-z]/`, `/--[a-z]/`, `/\b[EFTB]-[a-z0-9]{6}\b/`, or `/\.tiller\//` throws `ScriptError` naming the beat ID.
  - `rejects duplicate beat ids` and `rejects a term show missing from/to`.
  - `hash ignores CRLF and trailing whitespace`: the fixture rewritten with `\r\n` and trailing spaces hashes equal to the original.
  - `hash changes on a word change`: replacing one word in one `say` changes the hash.
  - `approval round-trip`: `approve` writes `{ "v-test": hash }`; `isApproved("v-test", hash)` is true; after a word edit, false.

- [ ] **Step 3: Run** `cd videos && npm test`. Expected: FAIL, modules not found.

- [ ] **Step 4: Implement** `schema.ts` (hand-written validator, no schema library), `load.ts` (parse with `yaml`), `approval.ts`. Hashing normalizes `\r\n` to `\n` and strips trailing whitespace per line, then sha256. `cli/approve.ts <id>` loads `videos/scripts/<id>.yaml`, validates it, writes the hash into `approvals.json` (sorted keys, trailing newline). `cli/preview.ts <id>` prints the script as readable Markdown (chapter headings, each beat's words, then what is on screen in italics) so JC reads it before approving.

- [ ] **Step 5: Run** `npm test && npm run typecheck`. Expected: PASS.

- [ ] **Step 6: Commit** `git commit -s -m "Videos: script format, approvals, and preview"`.

### Task 2: MiT theme and frame components

Requires H4.

**Files:**
- Create: `videos/src/theme.ts`, `videos/src/fonts.ts`, `videos/src/components/Frame.tsx`, `videos/src/components/TitleCard.tsx`, `videos/src/components/Kicker.tsx`, `videos/src/components/LowerThird.tsx`, `videos/src/components/Outro.tsx`, `videos/src/index.ts`, `videos/src/Root.tsx`
- Create: `videos/public/brand/` (copy `mit-logo-*.svg` and `mit-sigil-*.svg` actually used from `~/dev/mit-style-guide/logos/`)
- Test: `videos/test/theme.test.ts`, `videos/test/render-smoke.test.ts`

**Interfaces:**
- Produces:
  - `theme.colors`: the 18 tokens named as in the guide in camelCase (`websiteWhite: "#FBFBF5"`, `darkIndigo: "#001129"`, `mediumIndigo: "#0A2C49"`, `sharpieBlack: "#191819"`, `gracefulOlive: "#6F7347"`, `parchment: "#FFF4E6"`, and the other twelve from the guide's `--color-*` list)
  - `theme.fonts = { display: "Space Grotesk", body: "Inter", mono: "Space Mono" }`
  - `theme.video = { width: 1920, height: 1080, fps: 30 }`
  - `<Frame>{children}</Frame>`: MiT background, safe-area padding, wordmark corner
  - `<TitleCard kicker heading body? />`, `<Kicker text />` (olive mono kicker per the guide), `<LowerThird text />`, `<Outro />`
  - `Root.tsx` registers a `Smoke` composition (5 seconds of a TitleCard)

- [ ] **Step 1: Write the failing tests.** `theme.test.ts`: every hex in `theme.colors` equals the matching `--color-*` value parsed from `~/dev/mit-style-guide/mit-style-guide-2026-v1.html` (skip with a clear message if the file is absent); there are exactly 18. `render-smoke.test.ts` (timeout 120 s): bundle `src/index.ts`, `renderStill` the `Smoke` composition at frame 60 to a temp PNG; file exists and is 1920x1080.

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement.** Read the guide's sections on kicker, soft edge, wordmark, and color group rules and follow them; fonts load through `@remotion/google-fonts`. Motion is restrained: fades and short slides only, under 400 ms.

- [ ] **Step 4: Run** `npm test && npm run typecheck`. Expected: PASS. Open the PNG and check it against the guide by eye.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: MiT theme and frame components"`.

### Task 3: Brownfield Acme fixture and stage scripts

**Files:**
- Create: `videos/fixture/app/` (the Acme Orders app source, copied into a fresh repo by stage 1): `package.json`, `src/server.js`, `src/orders.js`, `test/orders.test.js`, `bin/test`, `README.md`, `docs/orders.md`
- Create: `videos/fixture/content/` (authored agent and human files each stage copies in: as-is docs, the to-be design doc, spec, eval, task, review file, fix diff)
- Create: `videos/fixture/lib.sh`, `videos/fixture/stage1.sh`, `videos/fixture/stage2.sh`, `videos/fixture/stage3.sh`, `videos/fixture/run.sh`
- Create: `videos/snapshots/s1.jsonl`, `s2.jsonl`, `s3.jsonl`, `videos/fixture/marks/*.tar.gz` (generated, committed)
- Test: `videos/fixture/fixture_test.sh`

**Interfaces:**
- Produces:
  - `lib.sh` helpers, used inside the fixture checkout with `TILLER` naming the binary and `SNAP` the stage's JSONL:
    - `run <step-id> <actor> <command...>`: runs the command, captures combined stdout/stderr, appends `{"id","actor","kind":"cmd","cmd","out":[lines],"exit"}` to `$SNAP`; fails the stage on non-zero exit unless the step ID ends in `-expect-fail`
    - `say <step-id> <actor> "<text>"`: appends `{"id","actor","kind":"say","text"}`
    - `put <content-path> <repo-path>`: copies an authored file into the repo (the agent or human writing it)
    - `mark <name>`: writes `videos/fixture/marks/<name>.tar.gz` of the whole checkout including `.git`
  - `run.sh [--out DIR] [--check]`: builds a fresh fixture under a temp dir, runs stage1 to stage3 in order, writes snapshots and marks (to `DIR`, default the committed locations)
  - Marks at least: `s1-end` (baseline confirmed), `s2-end` (plan approved), `s3-review` (code review written), `s3-end` (feature done)

The app: a dependency-free Node HTTP service, "Acme Orders", with `GET /orders` and `GET /orders/:id`, an in-memory store, `node --test` tests, and `bin/test` running them. `docs/orders.md` is an existing in-repo doc so derive-baseline has something to translate.

The feature (videos 2 and 3): "Filter orders by status" (`GET /orders?status=shipped`). The planted bug: an unknown status returns every order instead of a 400. The review finds it; the fix returns 400 with a message and adds a test. The eval criterion binds to `bin/test`.

- [ ] **Step 1: Write the failing test** `fixture_test.sh` (pattern: `site/screenshots/acme_test.sh`): build the binary from the repo root, run `run.sh --out "$TMP"`, then assert: the three JSONL files exist and every line parses with `jq`; `s1.jsonl` contains steps for `init --kind brownfield`, `add contributor`, `join`, `readout derive-baseline`, `approve design`; `s2.jsonl` contains `approve epic` and `approve feature`; `s3.jsonl` contains `check task`, `submit task`, `sync`, `check feature`; extracting `s3-end` and running `tiller status` there shows the feature done; every `cmd` step's `exit` is 0 except IDs ending `-expect-fail`.

- [ ] **Step 2: Run** `bash videos/fixture/fixture_test.sh`. Expected: FAIL.

- [ ] **Step 3: Write the app and stage scripts.** Stage 1 follows the README's "Start on the repo you already have" and `tiller guide derive-baseline`; stage 2 follows `tiller guide decompose` and `tiller guide specify`; stage 3 follows `tiller guide execute` and `tiller guide code-review` for the exact log-and-fix commands, then the quickstart's merge, `sync`, `check feature`, `sync` order. Configure the test gate with `bin/test` as the source quickstart shows. The `say` lines are the agent's and human's short in-terminal notes (homepage style, `site/src/pages/index.astro` around line 834). Authored content is written plainly and believably; it is what a good agent would produce for this app.

- [ ] **Step 4: Run** the test. Expected: PASS. Then `bash videos/fixture/run.sh` to write the committed snapshots and marks.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: brownfield Acme fixture and stage scripts"` (include `snapshots/` and `marks/`).

### Task 4: Pin and snapshot check

**Files:**
- Create: `videos/src/check/normalize.ts`, `videos/src/check/diff.ts`, `videos/src/cli/check.ts`
- Test: `videos/test/check.test.ts`

**Interfaces:**
- Consumes: snapshot JSONL from Task 3, `loadSeries` from Task 1
- Produces:
  - `type Step = { id: string; actor: Actor; kind: "cmd" | "say"; cmd?: string; out?: string[]; text?: string; exit?: number }`
  - `readSnapshot(stage: string, dir?: string): Step[]`
  - `normalize(line: string): string`: replaces node IDs `/\b[A-Z]{1,2}-[a-z0-9]{6}\b/` with `<ID>`, 7 to 40 char hex SHAs with `<SHA>`, ISO timestamps with `<TIME>`, durations like `(0s)`/`(1.2s)` with `(<DUR>)`, absolute temp paths with `<TMP>`
  - `diffSnapshots(committed: Step[], fresh: Step[]): string[]` (human-readable differences; empty means match)
  - `npm run check`: verifies `tiller --version` matches `series.tiller` (exit 1 naming both versions), runs `fixture/run.sh --out <tmp>`, diffs all three stages, exits 1 listing differences

- [ ] **Step 1: Write the failing tests:** normalize replaces each class above and leaves ordinary words alone (`"Created feature orders/status-filter (F-jn1wyk) in **draft**."` becomes `"... (<ID>) in **draft**."`); `diffSnapshots` returns empty for snapshots differing only in IDs and SHAs; reports a changed command line; reports a missing step by ID; reports changed output text.

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement.** Add `check` to npm scripts.

- [ ] **Step 4: Run** `npm test` then `npm run check` with the pinned binary. Expected: PASS and "snapshots match tiller 0.22.0".

- [ ] **Step 5: Commit** `git commit -s -m "Videos: pin and snapshot check"`.

### Task 5: Terminal component

**Files:**
- Create: `videos/src/term/state.ts`, `videos/src/components/Terminal.tsx`
- Test: `videos/test/term.test.ts`

**Interfaces:**
- Consumes: `Step` from Task 4
- Produces:
  - `type TermLine = { actor: Actor; kind: "cmd" | "out" | "say"; text: string; typed: number }` (`typed` is visible character count for the line being typed)
  - `termState(steps: Step[], startFrame: number, endFrame: number, frame: number, fps: number): TermLine[]`
  - `<Terminal steps startFrame endFrame />` rendering the homepage terminal's look: dark terminal background, `agent`/`you` gutter labels in their two colors, dim output, `**bold**`, `!!fail!!`, `++pass++` markup as on the homepage, a header reading "Claude Code" for agent lanes. Lines scroll so the newest stays visible.

Timing rule: steps share the beat window in proportion to their text length; a `cmd` types at 40 characters per second (capped to its share), then its output appears whole; a `say` appears whole. Everything for the beat is visible by `endFrame`.

- [ ] **Step 1: Write the failing tests:** at `startFrame` the result is empty; at `endFrame` every step's lines are present and fully typed; at the midpoint of a beat with two equal-length commands, the first is complete with output and the second is partly typed; markup survives into `text` for the renderer.

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement**, porting the homepage terminal CSS values into the component (font size scaled for 1080p legibility: at least 26 px mono).

- [ ] **Step 4: Run** `npm test && npm run typecheck`. Expected: PASS. Add a `TermSmoke` composition fed by `s1.jsonl` and render a still to eyeball it.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: terminal component"`.

### Task 6: Narration

**Files:**
- Create: `videos/src/config.ts`, `videos/src/narration/elevenlabs.ts`, `videos/src/narration/cache.ts`, `videos/src/narration/words.ts`, `videos/src/cli/voice-test.ts`
- Test: `videos/test/narration.test.ts`

**Interfaces:**
- Produces:
  - `loadConfig(path = "videos/.local/config.yaml"): Config` where `Config = { elevenlabs: { apiKey: string; voiceId: string; modelId: string; dictionaryId?: string; dictionaryVersion?: string }; youtube: { privacy: "private" | "unlisted"; categoryId: string } }`; throws naming each missing key
  - `type Word = { text: string; start: number; end: number }` (seconds)
  - `narrate(text: string, cfg: Config["elevenlabs"], cacheDir: string): Promise<{ audioPath: string; words: Word[]; duration: number }>` calling `POST /v1/text-to-speech/{voiceId}/with-timestamps` (output `mp3_44100_128`, `pronunciation_dictionary_locators` when configured)
  - `align(audioPath: string, transcript: string, apiKey: string, cacheDir: string): Promise<Word[]>` calling `POST /v1/forced-alignment` (for the intro clip)
  - `charsToWords(chars: string[], starts: number[], ends: number[]): Word[]`
  - `cacheKey(parts: string[]): string` (sha256 of the parts joined with `\0`)
  - `npm run voice-test`: narrates a fixed 60-second paragraph from JC's own writing to `videos/.local/voice-test.mp3`

The cache key for `narrate` is `[voiceId, modelId, dictionaryId ?? "", dictionaryVersion ?? "", text]`. A hit reads the cached mp3 and words JSON and makes no request.

- [ ] **Step 1: Write the failing tests** (mock `fetch`): `charsToWords` groups characters into words with the first char's start and last char's end, dropping whitespace; a second `narrate` with identical inputs makes zero requests; changing `voiceId` or one word of `text` makes exactly one new request; a non-200 response throws with the status and body text; `loadConfig` on a file missing `apiKey` throws naming `elevenlabs.apiKey`.

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement.** Default `modelId` is ElevenLabs' current highest-quality multilingual model; check the API docs at build time and record the chosen ID in `README.md`.

- [ ] **Step 4: Run** `npm test`, then `npm run voice-test` with the stock voice. Expected: PASS, playable mp3.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: ElevenLabs narration with cache"`.

### Task 7: Console captures

**Files:**
- Create: `videos/src/capture/serve.ts`, `videos/src/capture/shoot.ts`, `videos/src/cli/capture.ts`
- Test: `videos/test/capture.test.ts`

**Interfaces:**
- Consumes: marks from Task 3, `Capture` from Task 1
- Produces:
  - `serveMark(mark: string): Promise<{ url: string; stop(): Promise<void> }>`: extracts `fixture/marks/<mark>.tar.gz` to a temp dir, runs `tiller serve --port <free port>` there, resolves when the URL answers 200
  - `shoot(capture: Capture, outDir: string): Promise<string>`: Chromium at 1920x1080, device scale 1, records a video of loading `path` then performing `actions` over `seconds`; returns the `.webm` path
  - `npm run capture -- <script-id>`: shoots every capture the script names into `videos/.local/build/<id>/captures/`

- [ ] **Step 1: Write the failing test** (timeout 120 s, real binary): `serveMark("s1-end")` serves `/` with status 200; `shoot({ id: "t", mark: "s1-end", path: "/", seconds: 2 })` produces a webm whose ffprobe duration is between 1.5 and 3 seconds; the server stops and the temp dir is removed.

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement**, borrowing the server start and wait logic from `site/screenshots/shoot.mjs`. Light theme only.

- [ ] **Step 4: Run** `npm test`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: console captures from fixture marks"`.

### Task 8: Intro, music, and mastering

**Files:**
- Create: `videos/src/media/ffmpeg.ts`, `videos/src/media/intro.ts`, `videos/src/media/master.ts`
- Test: `videos/test/media.test.ts`

**Interfaces:**
- Produces:
  - `prepareIntro(srcPath: string, outPath: string): Promise<{ path: string; duration: number }>`: trims leading and trailing silence (below -45 dB for 0.3 s), scales to 1920x1080 at 30 fps, outputs mp4
  - `master(renderPath: string, musicPath: string, outro: { start: number }, introEnd: number, outPath: string): Promise<string>`: mixes music under `0..introEnd` and `outro.start..end` only, fading over 1 s at each edge, music at -24 LUFS beneath voice; whole program loudness-normalized to -14 LUFS integrated, -1 dBTP; H.264 High, yuv420p, AAC 320 kbps, `+faststart`
  - `probeDuration(path: string): Promise<number>`

- [ ] **Step 1: Write the failing tests** using generated media (ffmpeg `lavfi`: 1 s silence + 3 s tone + 1 s silence over a color source): `prepareIntro` output duration is 3 s plus or minus 0.15; `master` output measures -14 LUFS plus or minus 1 via `ffmpeg -af ebur128`; a probe of `master` output shows one h264 and one aac stream.

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement** with `child_process.execFile` on `ffmpeg`/`ffprobe`; two-pass `loudnorm`. Fail with "ffmpeg not found; brew install ffmpeg" when absent.

- [ ] **Step 4: Run** `npm test`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: intro preparation and mastering"`.

### Task 9: Timeline, captions, and chapters

**Files:**
- Create: `videos/src/timeline/build.ts`, `videos/src/timeline/captions.ts`, `videos/src/timeline/chapters.ts`, `videos/src/components/Video.tsx`
- Modify: `videos/src/Root.tsx` (register `Video` with `calculateMetadata` reading `timeline.json`)
- Test: `videos/test/timeline.test.ts`

**Interfaces:**
- Consumes: `Script`, `Word`, `Step`, components from Tasks 2 and 5
- Produces:
  - `type Segment = { beatId: string; start: number; end: number; audio: string; words: Word[]; show: Show; capturePath?: string; steps?: Step[] }`
  - `type Timeline = { id: string; fps: number; intro: { path: string; duration: number; words: Word[] }; segments: Segment[]; outro: { start: number; end: number }; duration: number }`
  - `buildTimeline(script, introInfo, narrations, captures, snapshots): Timeline`: intro, then 0.4 s gap, then each beat back to back with 0.35 s between beats, then a 6 s outro; throws `TooLongError` with the duration as `m:ss` when over 480 s
  - `toSrt(timeline: Timeline): string`: cues of at most 2 lines of 42 characters and at most 6 s, never spanning a beat boundary
  - `chapters(timeline: Timeline, script: Script): string[]` (`"0:00 Intro"` first, then each beat with `chapter` at its start)
  - `<Video timeline />`: intro clip, then per segment the narration audio over `TitleCard` / `Terminal` / console `OffthreadVideo` / `Outro` inside `Frame`, `LowerThird` showing the chapter name for 3 s at each chapter start

- [ ] **Step 1: Write the failing tests:** durations add up (intro + gaps + segments + outro equals `duration`); a script totaling 481 s throws `TooLongError` with "8:01"; SRT cue text concatenated equals the spoken text (intro transcript plus beats) with whitespace normalized; no cue exceeds 42 characters per line or 6 s; chapter list starts at `0:00`, has at least 3 entries, and `chapters` throws when any two are under 10 s apart.

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run** `npm test && npm run typecheck`. Expected: PASS.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: timeline, captions, and chapters"`.

### Task 10: Build command

**Files:**
- Create: `videos/src/cli/build.ts`, `videos/src/build/preflight.ts`, `videos/src/build/render.ts`
- Test: `videos/test/preflight.test.ts`

**Interfaces:**
- Consumes: everything above
- Produces:
  - `preflight(id: string): PreflightResult` where `PreflightResult = { ok: true } | { ok: false; problems: string[] }`, checking in this order and reporting all failures at once: script exists and validates; approval hash matches; `config.yaml` loads; `tiller --version` matches the pin; `videos/.local/media/intros/<id>.mov` exists; `videos/.local/media/music.mp3` exists; ffmpeg present
  - `npm run build -- <id>`: preflight, then `check`, then intro prep and alignment, narration per beat, captures, `buildTimeline`, Remotion `renderMedia` (h264, 1920x1080, 30 fps) to `.local/build/<id>/render.mp4`, `master` to `videos/out/<id>.mp4`, `videos/out/<id>.srt`, `videos/out/<id>.meta.json` (`{ title, description, tags, chapters, durationSeconds }`; description is the script description, a blank line, the chapter list, a blank line, the disclosure line)
  - `renderThumbnail(id)` is added in Task 11; build calls it when present

- [ ] **Step 1: Write the failing tests** (no network: `fetch` stubbed to throw if called): an unapproved script fails preflight and `fetch` was never called; a missing intro and a missing config both appear in one `problems` list; a fully prepared temp layout passes.

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement.** Print one line per phase with elapsed time.

- [ ] **Step 4: Run** `npm test`, then an end-to-end build of `test/fixtures/v-test.yaml` (approve it, a generated intro, stock voice). Expected: `videos/out/v-test.mp4` plays with intro, cards, terminal, console, outro, and the SRT lines up.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: build command"`.

### Task 11: Thumbnail and YouTube publish

Live run requires H3.

**Files:**
- Create: `videos/src/components/Thumbnail.tsx`, `videos/src/publish/metadata.ts`, `videos/src/publish/youtube.ts`, `videos/src/cli/publish.ts`, `videos/published.json`
- Modify: `videos/src/Root.tsx` (register `Thumbnail` still at 1280x720), `videos/src/cli/build.ts` (render `videos/out/<id>.jpg`, under 2 MB)
- Test: `videos/test/publish.test.ts`

**Interfaces:**
- Produces:
  - `videoResource(meta, cfg): youtube_v3.Schema$Video` with `snippet.title`, `snippet.description`, `snippet.tags`, `snippet.categoryId` (`cfg.youtube.categoryId`, default `"28"`), `status.privacyStatus = cfg.youtube.privacy`, `status.containsSyntheticMedia = true`, `status.selfDeclaredMadeForKids = false`
  - `publish(id: string): Promise<string>` (YouTube video ID): refuses when `published.json` has `id`; OAuth with the client in `.local/youtube-client.json`, token cached in `.local/youtube-token.json` (first run opens the browser consent); `videos.insert` (resumable), `thumbnails.set`, `captions.insert` (language `en`, name "English"); writes `{ [id]: { youtubeId, uploadedAt, privacy } }` to `published.json`
  - `npm run publish -- <id>`

- [ ] **Step 1: Write the failing tests:** `videoResource` sets `containsSyntheticMedia: true` and the configured privacy; it throws if privacy is anything but `private` or `unlisted`; the description contains the disclosure line exactly; `publish` with an existing `published.json` entry throws naming the existing YouTube ID and makes no API call (API client mocked).

- [ ] **Step 2: Run** `npm test`. Expected: FAIL.

- [ ] **Step 3: Implement.** The thumbnail is the video's title in Space Grotesk on an MiT field with the sigil, legible at 320x180.

- [ ] **Step 4: Run** `npm test`. Expected: PASS. After H3, publish `v-test` once, confirm in YouTube Studio that it is private, flagged altered content, captioned, thumbnailed, and chaptered, then delete it there and remove its `published.json` entry.

- [ ] **Step 5: Commit** `git commit -s -m "Videos: thumbnail and YouTube publish"`.

### Task 12: Video 1, Set Tiller up on your repo

**Files:**
- Create: `videos/scripts/v1.yaml`
- Modify: `videos/fixture/content/` and `videos/fixture/stage1.sh` only if the script needs a different on-screen moment (then re-run `fixture/run.sh` and commit the new snapshots)

- [ ] **Step 1: Draft the script** with the `write-like-jc` skill. Flow from the design: install, init brownfield, register, join, commit, "run tiller guide", the agent derives the baseline, the readout, the human confirms, the console. Chapters: Intro, Install, Join your repo, The baseline, Confirm it, See it. The intro transcript is a draft for JC to say in his own words; JC replaces it with what he actually said after filming. Target 6 minutes of narration at roughly 150 words per minute.
- [ ] **Step 2: Run** `npm run preview -- v1` and hand the Markdown to JC. Stop until JC approves or edits.
- [ ] **Step 3: JC runs** `npm run approve -- v1`, films the intro (H6).
- [ ] **Step 4: Run** `npm run build -- v1`. Expected: `videos/out/v1.mp4` between 5:00 and 8:00. Watch it end to end against the anti-goals; fix and re-approve if needed.
- [ ] **Step 5: After H1 and H2**, rebuild with the clone voice, then `npm run publish -- v1`. JC reviews the private or unlisted upload and flips it public himself.
- [ ] **Step 6: Commit** `git commit -s -m "Videos: video 1, Set Tiller up on your repo"`.

### Task 13: Video 2, From idea to approved spec

Same steps as Task 12 with `v2`. Flow: no work planned yet, the human writes a one-page design doc, decompose, approve the shape, specify (spec, eval, tasks), approve the plan. Chapters: Intro, No work yet, Write the idea down, The agent proposes, Approve the shape, The spec, Approve the plan. Commit `"Videos: video 2, From idea to approved spec"`.

### Task 14: Video 3, Build it and ship it

Same steps as Task 12 with `v3`. Flow: claim and start, build on a branch, task checks, submit, code review finds the unknown-status bug, log it, fix it on its branch, merge, sync, the feature eval on main, rollup to done in the console. Chapters: Intro, Build, Review, Fix, Merge, Done. Commit `"Videos: video 3, Build it and ship it"`.
