import { layoutFor, scriptPath } from "../build/layout.js";
import { loadScript, scriptHash } from "../script/load.js";
import { approve } from "../script/approval.js";

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run approve -- <video-id>");
  process.exit(2);
}
const layout = layoutFor();
const path = scriptPath(layout, id);
try {
  loadScript(path);
} catch (e) {
  console.error((e as Error).message);
  process.exit(1);
}
const hash = scriptHash(path);
approve(id, hash, layout.approvals);
console.log(`approved ${id} (${hash.slice(0, 12)})`);
