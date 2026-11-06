import type { Script } from "../script/schema.js";
import { mss, type Timeline } from "./build.js";

export const MIN_CHAPTERS = 3;
export const MIN_CHAPTER_SECONDS = 10;

// YouTube chapter lines for the description. YouTube silently drops the whole list
// unless the first is 0:00, there are at least three, and each runs 10 s or more,
// so those rules are enforced here on the whole seconds YouTube will read.
export function chapters(timeline: Timeline, script: Script): string[] {
  const marks: { at: number; name: string }[] = [{ at: 0, name: "Intro" }];
  for (const beat of script.beats) {
    if (!beat.chapter) continue;
    const seg = timeline.segments.find((s) => s.beatId === beat.id);
    if (!seg) throw new Error(`beat ${beat.id}: not on the timeline`);
    marks.push({ at: Math.floor(seg.start), name: beat.chapter });
  }
  if (marks.length < MIN_CHAPTERS) {
    throw new Error(`YouTube needs at least ${MIN_CHAPTERS} chapters, script ${script.id} has ${marks.length}`);
  }
  const stops = [...marks.slice(1), { at: Math.floor(timeline.duration), name: "the end" }];
  marks.forEach((m, i) => {
    const next = stops[i];
    if (next.at - m.at < MIN_CHAPTER_SECONDS) {
      const label = (c: { at: number; name: string }) => `"${c.name}" (${mss(c.at)})`;
      throw new Error(`chapters ${label(m)} and ${label(next)} are under ${MIN_CHAPTER_SECONDS} s apart`);
    }
  });
  return marks.map((m) => `${mss(m.at)} ${m.name}`);
}
