import { copyFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { loadConfig } from "../config.js";
import { VIDEOS_DIR } from "../script/load.js";
import { narrate } from "../narration/elevenlabs.js";

// About 150 words, roughly a minute at a natural pace.
const PARAGRAPH = `Most teams do not fail because the code is hard. They fail because nobody can say, with a straight face, what the thing is supposed to do. The plan lives in one head, the decisions live in a chat thread from March, and the review happens after the work is already done. So the first week goes to arguing about what was meant, and the second week goes to redoing what was built on a guess. I have watched this happen on projects of every size, and the fix is almost never more process. The fix is writing down the intent where everyone can see it, agreeing to it before the work starts, and keeping the record honest as the work changes. That is a small habit. It feels slow for about a day. After that, you stop having the same argument twice, and the work gets quieter in a way you start to appreciate.`;

const local = join(VIDEOS_DIR, ".local");
const cfg = loadConfig(join(local, "config.yaml"));
const r = await narrate(PARAGRAPH, cfg.elevenlabs, join(local, "narration-cache"));
mkdirSync(local, { recursive: true });
copyFileSync(r.audioPath, join(local, "voice-test.mp3"));
console.log(`wrote ${join(local, "voice-test.mp3")} (${r.duration.toFixed(1)}s, ${r.words.length} words)`);
