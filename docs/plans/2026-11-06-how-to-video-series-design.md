# Tiller how-to video series: design

**Scope.** This document defines the first Tiller how-to video series for YouTube: what the three videos teach, how they look and sound, and the fully automated pipeline that produces and publishes them. The product docs (README, `site/src/content/docs/quickstart.md`, `source-quickstart.md`, `skills.md`) stay canonical for how Tiller behaves. Where they disagree with this document, they win and this one is fixed.

**Status.** Designed 2026-11-03 to 2026-11-06 with JC through a one-question-at-a-time brainstorm. Not built. Next step is an implementation plan.

## What this is

A short series of professional how-to videos that take a developer new to Tiller from nothing to a feature merged on their own repo. JC appears on camera for a few seconds at the start of each video. Everything after that is produced by code: a narration script in JC's words, spoken by an ElevenLabs clone of his voice, over a reenacted terminal and agent session and real captures of the Tiller console, framed in Made In Tandem styling, rendered, captioned, and uploaded to YouTube as unlisted.

What it refuses: it is not a screen recording of a live agent run, it is not a talking-head series, and it never publishes a video publicly on its own.

## Why

The first arc is the fastest path to seeing Tiller's end-to-end value: install it, let it understand an existing codebase, specify a first feature, build it, review it, fix what review finds, merge, and watch acceptance close. JC is not experienced with video and wants production 100% automated, so the series has to be cheap to make and cheap to remake when a Tiller release changes a command.

**Success:** after the series, a viewer can install Tiller on their own repo and ship a first simple feature.

## Goals, non-goals, anti-goals

**Goals**
- Three videos, 5 to 8 minutes each, with 8 minutes as a hard ceiling.
- Each video ends on a human decision, so the series teaches by its shape that the agent does the work and the human decides.
- One command rebuilds any video from source.
- Fully automated from approved script to unlisted YouTube upload, including title, description, chapters, thumbnail, and captions.

**Non-goals**
- Showing a real, live agent run. A reenactment is fine (JC: "a simulation is fine").
- Making the footage look "real" for its own sake. JC: "don't let the medium get in the way of the message." Clarity of what is taught outranks visual fidelity; the reenactment only has to be believable enough not to distract.
- Videos beyond this first arc. Later series reuse the pipeline but are designed separately.
- Creating or configuring the YouTube channel. Assume it exists.

**Anti-goals** (failure even if it technically works)
- Robotic narration, or anything that feels like AI slop. This is JC's stated wince.
- A video that shows a command or output the current Tiller binary does not produce.
- A video going public without JC's review.

## The three videos (settled with JC)

The split follows Tiller's real onboarding flow as of v0.22.0, checked against the README, the site quickstart, the skills doc, and `internal/views/zero_day.go`.

**Video 1: Set Tiller up on your repo.** Install with Homebrew. `tiller init --kind brownfield`, `tiller add contributor`, `tiller join`, commit. Tell the agent "Run `tiller guide` and follow what it says," which starts the `derive-baseline` skill. The agent writes as-is docs describing what the code does today and stops at its readout. The human reviews and runs `tiller approve design`. Ends in the console with the repo mapped. Payoff: Tiller understands what I already have. Ends on the human decision to confirm the baseline.

**Video 2: From idea to approved spec.** After the baseline is confirmed, Tiller reports that no work is planned yet and names how work enters. The human writes a one-page to-be design doc under `.tiller/design/`, which makes `decompose` due. The agent proposes the epic and feature; the human approves the shape (`tiller approve epic`). The agent runs `specify`: spec, acceptance contract (`feature.eval.md`), and tasks, then `tiller spec feature`. The human reads it and approves the plan (`tiller approve feature`). Payoff: I decided what gets built, and it is written down. Ends on the human decision to approve the plan.

The first feature enters through a design doc, not `capture`, because shelf work runs no eval and this arc needs one.

**Video 3: Build it and ship it.** The agent claims and starts the task, builds on its own branch, runs `tiller check task`, and submits. The human asks for a code review ("code review feature F-..."); the review finds a real bug; the human logs it and says "fix B1 on its branch"; the agent fixes it. The human merges. `tiller sync` observes the merge, `tiller check feature` runs the acceptance eval on main, a second sync rolls the feature up to done, and the console shows it. Payoff: the end-to-end value. Ends on the human decision to merge.

Note the real order: the feature eval runs on main after the merge, not before review. The videos follow the binary, not the shorthand "built, evaled, reviewed."

The harness on screen is Claude Code, since `join` installs its skill shims. Narration may note once that any harness reaches the same skills through `tiller guide`. (My call.)

## How it opens (settled with JC)

Every video opens with a very short live to-camera clip of JC, the only footage he films. Roughly: "Hi, I'm JC from Made In Tandem, and I'm gonna show you how to use Tiller..." In it he discloses that the rest of the video is "my words, in my voice, performed by AI."

Handoff (my call): JC drops one raw clip per video into a known folder. The pipeline trims leading and trailing silence, normalizes loudness, applies the MiT frame, and adds the transition into the narrated section. A video refuses to render without its intro clip.

## How it sounds

- **Voice (JC):** ElevenLabs clone of JC's voice, driven by its API.
- **Words (my call):** scripts are written in JC's voice using the `write-like-jc` skill, scene by scene, plain and specific, never narrator-speak.
- **Approval (my call, JC agreed):** JC approves each script before it renders. His intro says "my words," and that only holds if he signed off on them.
- **Music (my call):** a quiet licensed track under the intro and outro only, nothing under the narration.
- **Captions (my call):** built from the approved script text aligned to ElevenLabs timing, not from speech recognition, uploaded as a separate caption track, not burned in.

## How it looks

- **Frame (JC: "use MiT styling"; application is my call):** title cards, kickers, lower thirds, captions styling, intro and outro, and the thumbnail use the MiT style guide at `~/dev/mit-style-guide/mit-style-guide-2026-v1.html`: its 18-color palette and group rules, Space Grotesk display, Inter body, Space Mono mono, the olive-mono kicker, the soft edge, and the wordmark and sigil SVGs in `logos/`. The guide has no motion guidance; motion stays restrained and consistent with its tone.
- **Content inside the frame (my call):** the Tiller console and the terminal keep their own real look, so viewers see what they will get. MiT styling never repaints Tiller's UI.
- **Clean style (JC: "you pick a clean style"):** no reference video. Calm pacing, generous whitespace, one idea on screen at a time.

## How it's made (my calls; JC: "pick the best tool with the best automation capabilities")

- **Assembly:** Remotion, where each video is a React/TypeScript composition rendered to MP4. It matches the console's stack. Remotion requires a paid company license for organizations over three people; JC approved buying it.
- **Terminal and agent session:** rendered as React components inside Remotion, not screen-recorded, building on the homepage faux terminal (`site/src/pages/index.astro`, around line 247 and 834). Keystrokes and agent output are timed to narration word timings.
- **Console footage:** Playwright captures (stills and short motion) of a running `tiller serve` against the demo repo, in the lineage of `site/screenshots/shoot.mjs`.
- **Narration:** ElevenLabs API with word-level timestamps; those timestamps drive on-screen timing.
- **Encode:** ffmpeg for the final encode and audio mastering.
- **Where it lives:** in this repo under `videos/`, beside `site/`, so it sits next to the commands it demonstrates.
- **What is real and what is authored (my call):** every Tiller command and its output shown on screen comes from a real run of the pinned binary against the demo repo, captured to snapshots. The agent's side (the Claude Code session, its prose, and the files it writes: as-is docs, spec, `feature.eval.md`, the review finding, the fix) is hand-authored fixture content, committed into the demo repo by the stage scripts. That is the reenactment JC approved, and the "binary does not produce" anti-goal applies to Tiller's output, not the agent's lines.
- **Staying current (my call):** the series is pinned to one Tiller version, recorded per video. Bumping the pin re-runs every on-screen command against the demo repo and diffs the output against the snapshots. A renamed or removed command, or changed output, fails the build until the snapshot or the script is updated. A script change of any kind voids its approval (approval is recorded as a hash of the script per video) and needs JC to approve it again.
- **Where it runs (my call):** on JC's Mac, one command per video. No CI for v1; intro clips, cached narration, renders, and credentials stay local and out of git.
- **Spoken versus shown (my call):** the narration never reads command syntax, flags, IDs like `F-x8k2q`, or paths aloud. The screen shows them; the voice says what they do. Each script beat carries separate spoken text and on-screen text, captions follow the spoken words, and an ElevenLabs pronunciation dictionary covers the few words the clone gets wrong (Tiller, MiT).

## The demo repo (settled with JC)

An Acme stand-in app, so nothing real or private appears on screen. The existing `site/screenshots/acme.sh` builds a greenfield project caught mid-flight; this series needs a different fixture (my call on its shape): a small, believable brownfield Acme application with real code and no `.tiller/` yet, plus scripted stage snapshots so each video starts from the state the previous one ended in. Console captures run against real Tiller state built through the CLI. The code-review bug in video 3 is planted deliberately so the review finds something real and the fix is small and legible.

## Publishing (settled with JC)

Fully automated upload through the YouTube Data API: title, description, chapters (from scene boundaries), thumbnail, and captions. Every upload is **unlisted**. JC watches the final cut and flips it public himself. The channel exists; the pipeline takes its OAuth credentials as configuration and never commits them.

Two human checkpoints in total: script approval before render, final-cut review before public.

**Disclosure (JC):** every upload sets the API's `containsSyntheticMedia` flag, and the description repeats the intro's disclosure that the narration is JC's words in his voice, performed by AI. Nothing extra on screen beyond the intro.

**Until the audit passes (JC: no audited project exists):** YouTube locks API uploads from an unaudited Google Cloud project to private. The pipeline uploads as private until the audit clears, and JC flips each video to unlisted or public by hand. Once audited, the default becomes unlisted with no other change.

## Prerequisites with lead time (JC, 2026-11-06)

These gate the build, so the plan starts them first and runs them in parallel with pipeline work that doesn't depend on them.

- **Voice clone:** none exists. JC records clean samples and trains an ElevenLabs Professional Voice Clone, which takes days. The first deliverable to depend on it is a short test narration that JC judges against the "robotic or slop" anti-goal. Nothing that requires the final voice ships before it passes. Pipeline work can use a stock ElevenLabs voice as a stand-in until then.
- **YouTube API audit:** no audited project exists. Create the Google Cloud project, set up OAuth for the channel, and submit the YouTube API compliance audit right away. It takes weeks. Use the private-upload fallback above until it passes. Custom thumbnails need a verified channel too.
- **Remotion license:** JC approved buying the company license. Buy it before Remotion code is written.

## Decisions settled so far

- **First arc, three videos**, split at human decision points (JC).
- **Reenactment, not a live run**, judged by whether it serves the information (JC).
- **Cloned voice, no face** except a **short live intro** per video with an AI disclosure (JC).
- **ElevenLabs** for the voice (JC). **Remotion, React terminal, Playwright, ffmpeg** for the rest (my calls).
- **MiT styling on the frame**, Tiller's real look inside it (JC asked for MiT styling; the split is my call).
- **5 to 8 minutes**, never longer (JC).
- **Anti-goal: robotic or slop-feeling narration** (JC).
- **Acme stand-in demo repo** (JC).
- **Automated YouTube publish, unlisted first** (JC), private until the API audit passes; **synthetic-media flag and description disclosure** (JC).
- **Prerequisites:** train a Professional voice clone, start the YouTube API audit now, buy the Remotion license (JC).
- **Scripts in JC's voice, approved by JC before render**; music only at intro and outro; script-derived captions; pipeline in `videos/`; command check before render (my calls).
- **Real Tiller output, authored agent content; pinned version with snapshot diffs; script hash voids approval on change; local Mac only; narration never reads syntax aloud** (my calls, from the builder check).

## Left to the builder

The exact scene breakdown and script for each video, within the flow above and the length ceiling. The Acme app's language and domain. Remotion composition structure, the stage-snapshot mechanism, and caching of narration audio. Thumbnail layout within the MiT guide. Choice of music track and its license. The precise form of the command check. Where the intro clips, credentials, and rendered outputs live, with large media and secrets kept out of git.

## What's next

An implementation plan: start the three prerequisites first, then build the pipeline end to end on video 1 (using a stand-in voice until the clone passes), then videos 2 and 3.
