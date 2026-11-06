import { copyFileSync, existsSync, mkdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { loadConfig } from "../config.js";
import { VIDEOS_DIR } from "../script/load.js";
import { narrate } from "../narration/elevenlabs.js";

// The paragraph lives in a committed file JC can replace.
const textPath = join(VIDEOS_DIR, "voice-test.txt");
if (!existsSync(textPath)) {
  console.error(`voice-test needs ${textPath}: the text to narrate (committed, replaceable).`);
  process.exit(1);
}
const PARAGRAPH = readFileSync(textPath, "utf8").trim();

const local = join(VIDEOS_DIR, ".local");
const cfg = loadConfig(join(local, "config.yaml"));
const r = await narrate(PARAGRAPH, cfg.elevenlabs, join(local, "narration-cache"));
mkdirSync(local, { recursive: true });
copyFileSync(r.audioPath, join(local, "voice-test.mp3"));
console.log(`wrote ${join(local, "voice-test.mp3")} (${r.duration.toFixed(1)}s, ${r.words.length} words)`);
