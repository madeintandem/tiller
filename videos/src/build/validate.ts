import { existsSync } from "node:fs";
import { join } from "node:path";
import { readSnapshot, type Step } from "../check/diff.js";
import { WPM } from "../narration/words.js";
import type { Script } from "../script/schema.js";
import { BEAT_GAP, INTRO_GAP, OUTRO_SECONDS, ceilingFor, mss, termSteps } from "../timeline/build.js";
import type { Layout } from "./layout.js";
import { youtubeFieldProblems } from "./meta.js";

// Until a script's intro clip is filmed and trimmed, assume this long.
const INTRO_ESTIMATE = 20;
export const WARN_SECONDS = 7 * 60 + 15;

const words = (s: string) => s.split(/\s+/).filter(Boolean).length;

/** A rough program length: the narration at 175 wpm, the intro estimate (for a script with an intro clip), the gaps, and the outro. */
export function estimateSeconds(script: Script): number {
  const narration = (script.beats.reduce((n, b) => n + words(b.say), 0) / WPM) * 60;
  return narration + (script.intro ? INTRO_ESTIMATE + INTRO_GAP : 0) + BEAT_GAP * (script.beats.length - 1) + OUTRO_SECONDS;
}

/**
 * Everything about a script that can fail a build later and is free to check now: every term
 * beat's steps exist in the committed snapshots, every capture's fixture mark exists, the
 * title and description pass YouTube's rules, and the estimated length is under the ceiling
 * (a warning as it nears it). Approve, preview, and the build preflight all run this.
 */
export function checkScript(script: Script, layout: Layout): { problems: string[]; warnings: string[] } {
  const problems: string[] = [];
  const warnings: string[] = [];

  const snapshots: Record<string, Step[] | null> = {};
  for (const b of script.beats) {
    if (b.show.kind !== "term") continue;
    const stage = b.show.stage;
    if (!(stage in snapshots)) {
      try {
        snapshots[stage] = readSnapshot(stage, layout.snapshots);
      } catch {
        snapshots[stage] = null;
      }
    }
    const steps = snapshots[stage];
    if (!steps) {
      problems.push(`beat ${b.id}: no committed snapshot for stage "${stage}" in ${layout.snapshots}`);
      continue;
    }
    try {
      termSteps(b.id, b.show, { [stage]: steps });
    } catch (e) {
      problems.push((e as Error).message);
    }
  }

  for (const c of script.captures) {
    const mark = join(layout.marks, `${c.mark}.tar.gz`);
    if (!existsSync(mark)) problems.push(`capture ${c.id}: no fixture mark "${c.mark}" at ${mark}`);
  }

  problems.push(...youtubeFieldProblems({ title: script.title, description: script.description }));
  for (const b of script.beats) {
    const bad = [...new Set(b.chapter?.match(/[<>]/g) ?? [])];
    if (bad.length) problems.push(`chapter "${b.chapter}" (beat ${b.id}) contains ${bad.join(" and ")}, which YouTube rejects in a description`);
  }

  const est = estimateSeconds(script);
  const ceil = (s: number) => Math.ceil(s);
  // The warning sits 45 seconds under the ceiling, whichever ceiling the script runs under.
  const max = ceilingFor(script);
  const warn = max - (480 - WARN_SECONDS);
  if (est > max) {
    problems.push(`estimated length ${mss(est, ceil)} is over the ${mss(max)} ceiling (narration at ${WPM} wpm); cut words`);
  } else if (est > warn) {
    warnings.push(`estimated length ${mss(est, ceil)} is past ${mss(warn)}, close to the ${mss(max)} ceiling`);
  }
  return { problems, warnings };
}
