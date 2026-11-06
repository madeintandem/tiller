import type { Script } from "../script/schema.js";

export const DISCLOSURE = "The narration is my words, in my voice, performed by AI.";

export type Meta = { title: string; description: string; tags: string[]; chapters: string[]; durationSeconds: number };

// The upload metadata: the description is the script's, then the chapter list, then the AI disclosure.
export function metadata(script: Script, chapters: string[], durationSeconds: number): Meta {
  return {
    title: script.title,
    description: [script.description.trim(), chapters.join("\n"), DISCLOSURE].join("\n\n"),
    tags: script.tags,
    chapters,
    durationSeconds: Math.round(durationSeconds * 1000) / 1000,
  };
}
