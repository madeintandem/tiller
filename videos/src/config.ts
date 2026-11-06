import { readFileSync } from "node:fs";
import { parse } from "yaml";

export type Config = {
  elevenlabs: {
    apiKey: string;
    voiceId: string;
    modelId: string;
    dictionaryId?: string;
    dictionaryVersion?: string;
    // JC's confirmed clone. Publish refuses a build narrated in any other voice.
    approvedVoiceId?: string;
  };
  youtube: { privacy: "private" | "unlisted"; categoryId: string };
};

// Highest-quality ElevenLabs TTS model per https://elevenlabs.io/docs/overview/models
export const DEFAULT_MODEL_ID = "eleven_v4";

export function loadConfig(path = "videos/.local/config.yaml"): Config {
  let raw: any;
  try {
    raw = parse(readFileSync(path, "utf8")) ?? {};
  } catch (e) {
    throw new Error(`cannot read config ${path}: ${(e as Error).message}`);
  }
  const fake = process.env.NARRATION_FAKE === "1";
  const el = raw.elevenlabs ?? {};
  const missing: string[] = [];
  if (!fake) {
    for (const k of ["apiKey", "voiceId"]) {
      if (typeof el[k] !== "string" || el[k] === "") missing.push(`elevenlabs.${k}`);
    }
  }
  if (missing.length) throw new Error(`config ${path} is missing: ${missing.join(", ")}`);

  const yt = raw.youtube ?? {};
  const privacy = yt.privacy ?? "private";
  if (privacy !== "private" && privacy !== "unlisted") {
    throw new Error(`config ${path}: youtube.privacy must be "private" or "unlisted", got ${JSON.stringify(privacy)}`);
  }
  const opt = (v: unknown) => (v === undefined || v === null || v === "" ? undefined : String(v));
  return {
    elevenlabs: {
      apiKey: el.apiKey ?? "",
      voiceId: el.voiceId ?? "",
      modelId: opt(el.modelId) ?? DEFAULT_MODEL_ID,
      dictionaryId: opt(el.dictionaryId),
      dictionaryVersion: opt(el.dictionaryVersion),
      approvedVoiceId: opt(el.approvedVoiceId),
    },
    youtube: { privacy, categoryId: opt(yt.categoryId) ?? "28" },
  };
}
