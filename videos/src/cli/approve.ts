import { approveScript } from "../build/approve.js";
import { layoutFor } from "../build/layout.js";

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run approve -- <video-id>");
  process.exit(2);
}
try {
  const { hash, warnings } = approveScript(id, layoutFor());
  for (const w of warnings) console.warn(`warning: ${w}`);
  console.log(`approved ${id} (${hash.slice(0, 12)})`);
} catch (e) {
  console.error((e as Error).message);
  process.exit(1);
}
