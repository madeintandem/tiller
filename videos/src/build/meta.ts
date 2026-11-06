import type { Script } from "../script/schema.js";

export const DISCLOSURE = "The narration is my words, in my voice, performed by AI.";

export type Meta = { title: string; description: string; tags: string[]; chapters: string[]; durationSeconds: number;
  // Provenance the publish step checks: a silent fake render, a stale build, or a build in
  // any voice but the approved clone must never reach YouTube.
  fake: boolean;
  scriptHash: string;
  voiceId: string;
};

/** The voice a build was narrated in: the configured voice ID, or "fake" for a NARRATION_FAKE build. */
export const voiceProvenance = (fake: boolean, voiceId: string): string => (fake ? "fake" : voiceId);

export const TITLE_MAX = 100;

/** YouTube rejects a title over 100 characters and either angle bracket in a title or description. */
export function youtubeFieldProblems(fields: { title: string; description: string }): string[] {
  const problems: string[] = [];
  if ([...fields.title].length > TITLE_MAX) problems.push(`title is ${[...fields.title].length} characters; YouTube allows ${TITLE_MAX}`);
  for (const [name, text] of [["title", fields.title], ["description", fields.description]] as const) {
    const bad = [...new Set(text.match(/[<>]/g) ?? [])];
    if (bad.length) problems.push(`${name} contains ${bad.join(" and ")}, which YouTube rejects`);
  }
  return problems;
}

// The upload metadata: the description is the script's, then the chapter list, then the AI disclosure.
export function metadata(
  script: Script,
  chapters: string[],
  durationSeconds: number,
  provenance: { fake: boolean; scriptHash: string; voiceId: string },
): Meta {
  return {
    title: script.title,
    description: [script.description.trim(), chapters.join("\n"), DISCLOSURE].join("\n\n"),
    tags: script.tags,
    chapters,
    durationSeconds: Math.round(durationSeconds * 1000) / 1000,
    fake: provenance.fake,
    scriptHash: provenance.scriptHash,
    voiceId: provenance.voiceId,
  };
}
