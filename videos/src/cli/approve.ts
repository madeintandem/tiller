import { loadScript, scriptHash, VIDEOS_DIR } from "../script/load.js";
import { approve } from "../script/approval.js";

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run approve -- <video-id>");
  process.exit(2);
}
const path = `${VIDEOS_DIR}scripts/${id}.yaml`;
try {
  loadScript(path);
} catch (e) {
  console.error((e as Error).message);
  process.exit(1);
}
const hash = scriptHash(path);
approve(id, hash);
console.log(`approved ${id} (${hash.slice(0, 12)})`);
