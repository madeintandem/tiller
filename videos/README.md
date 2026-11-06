# videos

A standalone Node/TypeScript pipeline that turns a reviewed script into a how-to video about Tiller: narration in JC's voice, on-screen terminal and console footage from a real run of the pinned Tiller version, rendered with Remotion and uploaded to YouTube. It runs on JC's Mac only; there is no CI job.

Design spec: `docs/plans/2026-11-06-how-to-video-series-design.md`. The spec and the implementation plan live in `docs/plans/`; JC commits them separately from this pipeline.

## The human track

These steps are JC's. Each one gates something the pipeline cannot do alone.

**H1: Voice clone.** Record 30 minutes or more of clean speech (quiet room, one mic, a natural read of your own writing) and train an ElevenLabs Professional Voice Clone. Copy `config.example.yaml` to `.local/config.yaml` and put the clone's voice ID in `elevenlabs.voiceId` (a stock voice ID works there in the meantime, for test builds).

**H2: Voice test.** Run `npm run voice-test` and listen to `.local/voice-test.mp3` in the clone. Pass or fail it against "robotic or AI slop." This first real call also confirms the model: the default `eleven_v4` must accept the `with-timestamps` endpoint; if ElevenLabs rejects it, set `elevenlabs.modelId: eleven_multilingual_v2` and run it again. When the voice passes, set `elevenlabs.approvedVoiceId` to the same ID. Publish refuses any build whose narration voice is not `approvedVoiceId`, so a stock-voice build can never go up saying "in my voice."

**H3: YouTube API.** Create a Google Cloud project, enable the YouTube Data API v3, and create an OAuth client of type "Desktop app" for the channel's Google account. Download its JSON to `.local/youtube-client.json`. Submit the YouTube API Services audit right away; it takes weeks. Until it passes, YouTube keeps API uploads private, so leave `youtube.privacy: private`. Verify the channel (by phone) so custom thumbnails work. When the audit passes, set `youtube.privacy: unlisted`. While the Google project's OAuth consent screen is in Testing status, Google expires its refresh tokens after 7 days; when that happens publish says so (see Publishing).

**H4: Remotion license.** Buy the Remotion company license; it covers the rendering this pipeline does.

**H5: Music.** Pick one licensed track for under the intro and outro. Save it as `.local/media/music.mp3` and record the track, its source, and its license in `videos/MUSIC.md` (committed).

**H6: Intro clips.** Film one short to-camera intro per video once its script is approved, and save it as `.local/media/intros/<id>.mov` (`v1.mov`, `v2.mov`, `v3.mov`). On an iPhone, turn HDR off first (Settings > Camera > Formats > HDR Video): an HDR clip is refused unless this ffmpeg can tone-map it. The build trims the silence at each end (keeping 120 ms of pad), brings the voice to -16 LUFS, and conforms the picture to 1920x1080 at 30 fps.

### Order for each video

1. Read the script: `npm run preview -- <id>`. It shows the narration beside what is on screen and lists anything that would stop a build.
2. Approve it: `npm run approve -- <id>`.
3. Film the intro (H6).
4. Replace `intro.transcript` in `scripts/<id>.yaml` with what you actually said on camera; the captions align it to the clip.
5. Approve again (the transcript edit voids the first approval): `npm run approve -- <id>`.
6. Build: `npm run build -- <id>`.
7. Publish: `npm run publish -- <id>`, then review the upload in YouTube Studio.
8. Flip it public in YouTube Studio yourself. The pipeline never does.

No script renders unless its current hash matches its approval, and approve refuses a script that cannot build.

## Setup

Node 26 and ffmpeg (`brew install ffmpeg`). From this directory: `npm install`, then `npx playwright install chromium` for the console captures.

## npm scripts

- `npm test`: run the vitest suite.
- `npm run typecheck`: `tsc --noEmit`.
- `npm run preview -- <id>`: print the script as Markdown: chapters, narration, and under each beat what is on screen (a term beat's commands as `$ command` and its say lines, from the committed snapshot; a console beat's page, mark, and actions), with the estimated length and any problem that would stop a build (exits non-zero if there is one).
- `npm run approve -- <id>`: record `scripts/<id>.yaml`'s content hash in `approvals.json`, after checking that the script validates, that its `id` matches the file name, and that it can build (see Script checks). Any wording change voids the approval; line endings, trailing whitespace, and trailing blank lines do not.
- `npm run check`: verify `tiller --version` matches `series.yaml`, rebuild the fixture into a temp dir, and diff every step against the committed snapshots after masking IDs, SHAs, timestamps, durations, and temp paths.
- `npm run term-still -- <out.png> [stage] [frame]`: render one frame of the `TermSmoke` composition fed by a committed snapshot, to eyeball the terminal.
- `npm run voice-test`: narrate `voice-test.txt` (two paragraphs from the root README, about 60 seconds; replace the file with your own text if you like) to `.local/voice-test.mp3` with the configured voice. Needs `.local/config.yaml` (or `NARRATION_FAKE=1` plus a config file, which yields silent audio).
- `npm run capture -- <id>`: record the script's console captures (see Console captures).
- `npm run build -- <id>`: turn an approved script into `out/<id>.mp4`, `out/<id>.srt`, `out/<id>.jpg` (thumbnail), and `out/<id>.meta.json` (see Building).
- `npm run publish -- <id>`: upload the built video, thumbnail, and captions to YouTube, never publicly (see Publishing).

## Layout

- `series.yaml`: the pinned Tiller version.
- `approvals.json`: script ID to approved content hash.
- `scripts/`: one YAML script per video.
- `config.example.yaml`: every config key, commented; copy it to `.local/config.yaml`.
- `.local/`, `out/`, `node_modules/`: gitignored. `.local/` holds the config, credentials, media, narration cache, build directories, and `published.json`.
- `src/script/`: schema, loader, hashing, approvals.
- `src/theme.ts`, `src/fonts.ts`, `src/components/`: MiT palette, fonts, and frame components (Frame, TitleCard, Kicker, LowerThird, Outro, Terminal, Thumbnail).
- `src/index.ts`, `src/Root.tsx`: Remotion entry and compositions (`Smoke`, `TermSmoke`, `Thumbnail`, and `Video`, whose length comes from the timeline passed as input props).
- `src/term/`: what the terminal shows at each frame: commands type at 40 characters a second, then their output comes out a line at a time over the rest of the step's share, with the command held in view above it.
- `src/timeline/`: `buildTimeline` (intro, 0.4 s gap, beats 0.35 s apart, 6 s outro, refused past 8:00), `toSrt` (captions from the spoken words only), `chapters` (YouTube chapter lines, checked against YouTube's rules). Asset paths in a timeline are relative to the build directory, which the render step serves as Remotion's public directory.
- `src/components/Video.tsx`: the whole program: intro full bleed with a name lower third, then each beat's narration over its card, terminal, or console capture inside the frame, then the outro.
- `src/build/`: `layoutFor` (every build path), `validate` (the script checks), `preflight`, `approve`, `preview`, `outputs`, `renderVideo` (Remotion bundle and render), `metadata` (the upload metadata and disclosure line); `src/cli/build.ts` runs the phases.
- `src/capture/`: `serveMark` and `shoot` (console captures).
- `src/media/`: ffmpeg helpers, `prepareIntro`, and `master`.
- `src/narration/`: ElevenLabs narration and forced alignment, with the cache.
- `src/publish/`: the YouTube upload.
- `src/check/run.ts`: `checkSnapshots` and `tillerVersion`, shared by `npm run check` and the build.
- `public/brand/`: MiT logo SVGs used by the frame.
- `fixture/`: the brownfield Acme Orders demo repo the videos are recorded from. `app/` is the existing app, `content/` the authored files the agent and the human write, `stage1.sh` to `stage3.sh` one script per video, `lib.sh` their helpers (`run`, `say`, `put`, `mark`).
- `snapshots/s1.jsonl` to `s3.jsonl`: every step each stage ran, one JSON object per line, with each Tiller command's real output.
- `fixture/marks/*.tar.gz`: the whole fixture checkout, `.git` included, saved at named moments (`s1-end`, `s2-end`, `s3-review`, `s3-end`, and the steps between).

## Script checks

`preview`, `approve`, and the build's preflight run the same checks on a script, all free and local:

- every term beat's `from` and `to` steps exist, in order, in the committed snapshot for its stage;
- every capture's fixture mark exists in `fixture/marks/`;
- the title is at most 100 characters, and neither the title, the description, nor a chapter name contains `<` or `>` (YouTube rejects them);
- the estimated length (narration words at 150 wpm, plus a 20 s intro estimate, the gaps, and the 6 s outro) warns past 7:15 and fails past 8:00.

## Narration

Config lives in `.local/config.yaml` (gitignored). `config.example.yaml` shows every key with a comment. Narration is cached under `.local/narration-cache/` keyed by voice ID, model ID, dictionary ID and version, and the text, so editing one beat's words or switching voice regenerates only what changed.

- Text to speech: `POST /v1/text-to-speech/{voice_id}/with-timestamps?output_format=mp3_44100_128`. The response carries `audio_base64` and `alignment` with `characters`, `character_start_times_seconds`, `character_end_times_seconds`, which `charsToWords` groups into words. Docs: https://elevenlabs.io/docs/api-reference/text-to-speech/convert-with-timestamps
- Forced alignment (intro clip): `POST /v1/forced-alignment`, multipart `file` and `text`, response `words[]` of `{text, start, end, loss}`. Docs: https://elevenlabs.io/docs/api-reference/forced-alignment/create
- Default model: `eleven_v4`, listed as the newest and highest-quality model at https://elevenlabs.io/docs/overview/models (checked 2026-11-06). Not yet confirmed live that this model supports `with-timestamps`; H2 settles it (fallback: `eleven_multilingual_v2`).

Fake mode: with `NARRATION_FAKE=1`, `narrate` and `align` make no network request. Narration is silent mp3 (44.1 kHz) lasting words / 150 wpm (minimum 1 s) with evenly spaced word timings, cached separately from real audio. `loadConfig` then does not require `elevenlabs.apiKey` or `voiceId`. This lets the pipeline build and render end to end without a key; such a build is marked fake and publish refuses it.

## The fixture

`bash fixture/run.sh` rebuilds the Acme Orders repo from scratch with the `tiller` on `PATH` (or `TILLER=<binary>`), runs the three stages, and rewrites `snapshots/` and `fixture/marks/`. Commit both. `--out DIR` writes them under `DIR` instead; `--check` writes nothing and compares the fresh steps against the committed snapshots. `bash fixture/fixture_test.sh` builds Tiller from this repo and checks the whole run.

## Console captures

`npm run capture -- <script-id>` loads `scripts/<id>.yaml` and records every capture it names into `.local/build/<id>/captures/<capture-id>.mp4`. Each capture extracts its fixture mark to a temp dir, runs `tiller serve` there on a free port (`$TILLER` overrides the binary), and shoots the real console in Chromium at a 1280x720 viewport, device scale 2, light theme, so its text reads at laptop size inside the frame. Every frame is a 2560x1440 screenshot laid on a 30 fps clock, encoded to H.264, exactly `seconds` long. The server and the temp dir are cleaned up even when a capture throws.

A capture's `actions` run in order after the page loads:

- `waitFor: <selector>`: wait until it appears (takes no time on screen);
- `wait: <seconds>`: hold the picture;
- `scrollTo: <selector>`: glide the page's scrolling content (the element that actually scrolls, sideways too) until the selector sits near the top, over 1.2 s;
- `scroll: <pixels>`: glide that far down, over 1.2 s;
- `click: <selector>`: click it and cut to the settled page.

The last picture holds to the end. Actions that need more than `seconds` are refused.

## Building

`npm run build -- <id>` prints one line per phase with its elapsed time:

1. **preflight**: everything checkable locally and for free, all failures reported at once, before any paid API call or long render: the script exists and passes the script checks (warnings are printed); its `id` matches the file; its hash matches `approvals.json`; `.local/config.yaml` loads (with `NARRATION_FAKE=1` it needs no ElevenLabs keys); `tiller --version` matches `series.yaml`; `.local/media/intros/<id>.mov` exists; `.local/media/music.mp3` exists; ffmpeg runs. The script and its hash come from one read of the file, and the build renders and records exactly those. Then `out/<id>.*` from any earlier build is deleted.
2. **check**: the same snapshot check as `npm run check`.
3. **captures**: every console capture the script names (local and free, so before anything paid).
4. **intro**: refuse or tone-map HDR, trim the silence (keeping 120 ms of pad), normalize the voice to -16 LUFS, conform the clip, then align its transcript for captions.
5. **narration**: each beat's `say`, cached (see Narration).
6. **timeline**: lay out the program and validate the chapters against YouTube's rules (and the 8:00 ceiling) before rendering.
7. **render**: Remotion `Video` composition, H.264 1920x1080 at 30 fps, to `.local/build/<id>/render.mp4`.
8. **master**: music under the intro and outro only, 18 LU under the voice, then the whole program to -14 LUFS, to `out/<id>.mp4`.
9. **outputs**: `out/<id>.srt` and `out/<id>.meta.json` (`title`, `description`, `tags`, `chapters`, `durationSeconds`, `fake` (built with `NARRATION_FAKE=1`), `scriptHash` (the approved hash the render used), `voiceId` (the narration voice, or `fake`); the description is the script's, a blank line, the chapter list, a blank line, then "The narration is my words, in my voice, performed by AI.").
10. **thumbnail**: the `Thumbnail` still (title in Space Grotesk on the MiT dark indigo field, with the sigil) to `out/<id>.jpg`, 1280x720, under 2 MB.

`.local/build/<id>/` is the build directory and Remotion's public directory: the prepared `intro.mp4`, `narration/<beat>.mp3`, `captures/<capture>.mp4`, the brand assets, and `timeline.json` all sit there, and the timeline's paths are relative to it.

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

`npm run publish -- <id>` uploads `out/<id>.mp4` with its thumbnail and `.srt` captions (language `en`, name "English") through the YouTube Data API, with `status.privacyStatus` from `youtube.privacy` (`private` or `unlisted`, never public), `containsSyntheticMedia: true`, and not made for kids. The video goes up by YouTube's resumable protocol (8 MiB chunks, resume from the reported offset after a network error or 5xx, bounded retries). It records `{ youtubeId, uploadedAt, privacy, thumbnail, captions }` in `.local/published.json` (kept out of git, since unlisted video IDs must not reach a public repo) right after the video uploads. The thumbnail and the captions are each attempted and recorded, and every failure is reported together; rerun the same command and only the missing steps run. A complete entry is refused: delete the video in YouTube Studio first if you truly want a new upload.

It refuses, before any API call and naming the reason, when: the build was a fake render (`meta.fake`); the build's narration voice (`meta.voiceId`) is not `elevenlabs.approvedVoiceId`, or that key is unset; the script changed since the build (`meta.scriptHash`); the script's current hash is not approved in the committed `approvals.json` (`VIDEOS_APPROVALS` is ignored); the title or description breaks YouTube's rules; or `out/<id>.mp4`, `.srt`, `.jpg`, or `.meta.json` is missing.

Credentials: the "Desktop app" OAuth client from H3 in `.local/youtube-client.json`. The first run opens the browser for consent and caches the token in `.local/youtube-token.json`. If Google later refuses that token (`invalid_grant`: expired, which a project in Testing status does after 7 days, or revoked) or the file is corrupt, publish says to delete `.local/youtube-token.json` and rerun `npm run publish -- <id>`, which asks for consent again.
