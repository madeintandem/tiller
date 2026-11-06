import { existsSync, readFileSync } from "node:fs";
import { basename, join } from "node:path";
import type { youtube_v3 } from "googleapis";
import { DISCLOSURE, youtubeFieldProblems, type Meta } from "../build/meta.js";
import { scriptPath, type Layout } from "../build/layout.js";
import { outputPaths } from "../build/outputs.js";
import type { Config } from "../config.js";
import { isApproved } from "../script/approval.js";
import { scriptHash } from "../script/load.js";

// The YouTube upload resource for a built video. Never public, always flagged as
// altered or synthetic content, never made for kids.
export function videoResource(meta: Meta, cfg: Config): youtube_v3.Schema$Video {
  const privacy = cfg.youtube.privacy as string;
  if (privacy !== "private" && privacy !== "unlisted") {
    throw new Error(`youtube.privacy must be "private" or "unlisted", got ${JSON.stringify(privacy)}`);
  }
  if (!meta.description.includes(DISCLOSURE)) {
    throw new Error(`the description is missing the AI disclosure line: ${DISCLOSURE}`);
  }
  const fields = youtubeFieldProblems(meta);
  if (fields.length) throw new Error(`YouTube would reject this upload: ${fields.join("; ")}`);
  return {
    snippet: {
      title: meta.title,
      description: meta.description,
      tags: meta.tags,
      categoryId: cfg.youtube.categoryId || "28",
    },
    status: { privacyStatus: privacy, containsSyntheticMedia: true, selfDeclaredMadeForKids: false },
  };
}

export type Artifacts = { mp4: string; srt: string; jpg: string; meta: Meta };

// Refuses, before any API call, unless the build outputs are whole, real (not a fake
// render), narrated in the approved voice, current (the script has not changed since), and
// approved in the committed approvals.json. VIDEOS_APPROVALS is deliberately not consulted here.
export function loadPublishable(id: string, layout: Layout, cfg: Config): Artifacts {
  const files = outputPaths(layout, id);
  const missing = Object.values(files).filter((p) => !existsSync(p));
  if (missing.length) {
    throw new Error(`cannot publish ${id}: missing ${missing.map((p) => basename(p)).join(", ")} in ${layout.out} (run: npm run build -- ${id})`);
  }
  const meta = JSON.parse(readFileSync(files.meta, "utf8")) as Meta;
  if (meta.fake) {
    throw new Error(`cannot publish ${id}: it was built with NARRATION_FAKE=1 (silent fake narration); rebuild with real narration`);
  }
  // The description says "in my voice": only JC's confirmed clone may go up.
  const approvedVoice = cfg.elevenlabs.approvedVoiceId;
  if (!meta.voiceId) {
    throw new Error(`cannot publish ${id}: the build records no narration voice; rebuild it (run: npm run build -- ${id})`);
  }
  if (!approvedVoice) {
    throw new Error(`cannot publish ${id}: set elevenlabs.approvedVoiceId in ${layout.config} to JC's confirmed voice clone first`);
  }
  if (meta.voiceId !== approvedVoice) {
    throw new Error(
      `cannot publish ${id}: it was narrated in voice ${meta.voiceId}, not elevenlabs.approvedVoiceId (${approvedVoice}); ` +
        `set elevenlabs.voiceId to the approved clone and rebuild`,
    );
  }
  const path = scriptPath(layout, id);
  const hash = scriptHash(path);
  if (hash !== meta.scriptHash) {
    throw new Error(`cannot publish ${id}: stale build, the script has changed since it was built (run: npm run build -- ${id})`);
  }
  if (!isApproved(id, hash, join(layout.root, "approvals.json"))) {
    throw new Error(`cannot publish ${id}: script is not approved at its current content in approvals.json (run: npm run approve -- ${id})`);
  }
  return { mp4: files.mp4, srt: files.srt, jpg: files.jpg, meta };
}
