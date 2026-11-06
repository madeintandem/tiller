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

- `npm run voice-test`: narrate `voice-test.txt` (two paragraphs from the root README, about 60 seconds; JC can replace the file with his own text) to `.local/voice-test.mp3` with the configured voice. Needs `.local/config.yaml` (or `NARRATION_FAKE=1` plus a config file, which yields silent audio).

- `npm run capture -- <id>`: record the script's console captures (see Console captures).

- `npm run build -- <id>`: turn an approved script into `out/<id>.mp4`, `out/<id>.srt`, `out/<id>.jpg` (thumbnail), and `out/<id>.meta.json` (see Building).
- `npm run publish -- <id>`: upload the built video, thumbnail, and captions to YouTube, never publicly (see Publishing).

More scripts are added here as the pipeline tasks land.

## Layout

- `series.yaml`: the pinned Tiller version.
- `approvals.json`: script ID to approved content hash.
- `scripts/`: one YAML script per video.
- `src/script/`: schema, loader, hashing, approvals.
- `.local/`, `out/`, `node_modules/`: gitignored.
- `src/theme.ts`, `src/fonts.ts`, `src/components/`: MiT palette, fonts, and frame components (Frame, TitleCard, Kicker, LowerThird, Outro).
- `src/index.ts`, `src/Root.tsx`: Remotion entry and compositions (`Smoke`, `TermSmoke`, and `Video`, whose length comes from the timeline passed as input props).
- `src/timeline/`: `buildTimeline` (intro, 0.4 s gap, beats 0.35 s apart, 6 s outro, refused past 8:00), `toSrt` (captions from the spoken words only), `chapters` (YouTube chapter lines, checked against YouTube's rules). Asset paths in a timeline are relative to the build directory, which the render step serves as Remotion's public directory.
- `src/components/Video.tsx`: the whole program: intro full bleed with a name lower third, then each beat's narration over its card, terminal, or console capture inside the frame, then the outro.
- `src/build/`: `layoutFor` (every build path), `preflight`, `renderVideo` (Remotion bundle and render), `metadata` (the upload metadata and disclosure line); `src/cli/build.ts` runs the phases.
- `src/check/run.ts`: `checkSnapshots` and `tillerVersion`, shared by `npm run check` and the build.
- `public/brand/`: MiT logo SVGs used by the frame.
- `fixture/`: the brownfield Acme Orders demo repo the videos are recorded from. `app/` is the existing app, `content/` the authored files the agent and the human write, `stage1.sh` to `stage3.sh` one script per video, `lib.sh` their helpers (`run`, `say`, `put`, `mark`).
- `snapshots/s1.jsonl` to `s3.jsonl`: every step each stage ran, one JSON object per line, with each Tiller command's real output.
- `fixture/marks/*.tar.gz`: the whole fixture checkout, `.git` included, saved at named moments (`s1-end`, `s2-end`, `s3-review`, `s3-end`, and the steps between).

## Narration

Config lives in `videos/.local/config.yaml` (gitignored). `config.example.yaml` shows every key with a comment. Narration is cached under `.local/narration-cache/` keyed by voice ID, model ID, dictionary ID and version, and the text, so editing one beat's words or switching voice regenerates only what changed.

- Text to speech: `POST /v1/text-to-speech/{voice_id}/with-timestamps?output_format=mp3_44100_128`. The response carries `audio_base64` and `alignment` with `characters`, `character_start_times_seconds`, `character_end_times_seconds`, which `charsToWords` groups into words. Docs: https://elevenlabs.io/docs/api-reference/text-to-speech/convert-with-timestamps
- Forced alignment (intro clip): `POST /v1/forced-alignment`, multipart `file` and `text`, response `words[]` of `{text, start, end, loss}`. Docs: https://elevenlabs.io/docs/api-reference/forced-alignment/create
- Default model: `eleven_v4`, listed as the newest and highest-quality model at https://elevenlabs.io/docs/overview/models (checked 2026-11-06). Not yet confirmed live that this model supports `with-timestamps`; if the first real call rejects it, set `modelId: eleven_multilingual_v2` in the config.

Fake mode: with `NARRATION_FAKE=1`, `narrate` and `align` make no network request. Narration is silent mp3 (44.1 kHz) lasting words / 150 wpm (minimum 1 s) with evenly spaced word timings, cached separately from real audio. `loadConfig` then does not require `elevenlabs.apiKey` or `voiceId`. This lets the pipeline build and render end to end without a key.

## The fixture

`bash fixture/run.sh` rebuilds the Acme Orders repo from scratch with the `tiller` on `PATH` (or `TILLER=<binary>`), runs the three stages, and rewrites `snapshots/` and `fixture/marks/`. Commit both. `--out DIR` writes them under `DIR` instead; `--check` writes nothing and compares the fresh steps against the committed snapshots. `bash fixture/fixture_test.sh` builds Tiller from this repo and checks the whole run.

## Console captures

`npm run capture -- <script-id>` loads `scripts/<id>.yaml` and records every capture it names into `.local/build/<id>/captures/<capture-id>.webm`. Each capture extracts its fixture mark to a temp dir, runs `tiller serve` there on a free port (`$TILLER` overrides the binary), and records the real console in Chromium at 1920x1080, scale 1, light theme. The page is held until `seconds` have passed since it loaded. The server and the temp dir are cleaned up even when a capture throws.

One-time setup: `npx playwright install chromium` (run from `videos/`).

## Building

`npm run build -- <id>` prints one line per phase with its elapsed time:

1. **preflight**: everything checkable locally and for free, all failures reported at once, before any paid API call or long render: the script exists and validates; its hash matches `approvals.json`; `.local/config.yaml` loads (with `NARRATION_FAKE=1` it needs no ElevenLabs keys); `tiller --version` matches `series.yaml`; `.local/media/intros/<id>.mov` exists; `.local/media/music.mp3` exists; ffmpeg runs.
2. **check**: the same snapshot check as `npm run check`.
3. **intro**: trim and conform the intro clip, then align its transcript for captions.
4. **narration**: each beat's `say`, cached (see Narration).
5. **captures**: every console capture the script names.
6. **timeline**: lay out the program and validate the chapters against YouTube's rules (and the 8:00 ceiling) before rendering.
7. **render**: Remotion `Video` composition, H.264 1920x1080 at 30 fps, to `.local/build/<id>/render.mp4`.
8. **master**: music under the intro and outro, loudness to -14 LUFS, to `out/<id>.mp4`.
9. **outputs**: `out/<id>.srt` and `out/<id>.meta.json` (`title`, `description`, `tags`, `chapters`, `durationSeconds`, `fake` (built with `NARRATION_FAKE=1`), `scriptHash` (the approved hash the render used); the description is the script's, a blank line, the chapter list, a blank line, then "The narration is my words, in my voice, performed by AI.").

10. **thumbnail**: the `Thumbnail` still (title in Space Grotesk on the MiT dark indigo field, with the sigil) to `out/<id>.jpg`, 1280x720, under 2 MB.

`.local/build/<id>/` is the build directory and Remotion's public directory: the prepared `intro.mp4`, `narration/<beat>.mp3`, `captures/<capture>.webm`, the brand assets, and `timeline.json` all sit there, and the timeline's paths are relative to it.

Overrides: `VIDEOS_SCRIPTS=<dir>` and `VIDEOS_APPROVALS=<file>` point `approve`, `preview`, `capture`, and `build` at another scripts directory and approvals file; `TILLER=<binary>` picks the binary.

End-to-end test build, with no ElevenLabs key and without touching the committed `approvals.json`:

```
# a generated intro (test pattern, a tone with half a second of silence at each end) and music
ffmpeg -f lavfi -i testsrc2=size=1920x1080:rate=30:duration=6 -f lavfi -i "sine=frequency=440:duration=5,adelay=500,apad=whole_dur=6" -c:v libx264 -pix_fmt yuv420p -c:a aac -t 6 .local/media/intros/v-test.mov
ffmpeg -f lavfi -i sine=frequency=220:duration=20 .local/media/music.mp3
export VIDEOS_SCRIPTS=test/fixtures VIDEOS_APPROVALS=.local/approvals.json NARRATION_FAKE=1
npm run approve -- v-test
npm run build -- v-test
```

Fake mode still needs a `.local/config.yaml`; one holding only the `youtube:` block will do.

## Publishing

`npm run publish -- <id>` uploads `out/<id>.mp4` with its thumbnail and `.srt` captions (language `en`, name "English") through the YouTube Data API, with `status.privacyStatus` from `youtube.privacy` (`private` or `unlisted`, never public), `containsSyntheticMedia: true`, and not made for kids. The video goes up by YouTube's resumable protocol (8 MiB chunks, resume from the reported offset after a network error or 5xx, bounded retries). It records `{ youtubeId, uploadedAt, privacy, thumbnail, captions }` in `published.json` right after the video uploads; if the thumbnail or captions then fail, rerun the same command and only the missing steps run. A complete entry is refused: delete the video in YouTube Studio first if you truly want a new upload.

It refuses, before any API call and naming the reason, when: the build was a fake render (`meta.fake`); the script changed since the build (`meta.scriptHash`); the script's current hash is not approved in the committed `approvals.json` (`VIDEOS_APPROVALS` is ignored); or `out/<id>.mp4`, `.srt`, `.jpg`, or `.meta.json` is missing.

Credentials: put a Google "Desktop app" OAuth client in `.local/youtube-client.json`. The first run opens the browser for consent and caches the token in `.local/youtube-token.json`.
