import { layoutFor, scriptPath } from "../build/layout.js";
import { previewMarkdown } from "../build/preview.js";
import { checkScript } from "../build/validate.js";
import { loadScript } from "../script/load.js";

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run preview -- <video-id>");
  process.exit(2);
}
try {
  const layout = layoutFor();
  const s = loadScript(scriptPath(layout, id));
  console.log(previewMarkdown(s, layout));
  // The same checks approve runs; a script that cannot build exits non-zero.
  if (checkScript(s, layout).problems.length) process.exit(1);
} catch (e) {
  console.error((e as Error).message);
  process.exit(1);
}
