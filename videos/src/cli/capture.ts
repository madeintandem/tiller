// Usage: tsx src/cli/capture.ts <script-id>
import { join } from "node:path";
import { buildDir, layoutFor, scriptPath } from "../build/layout";
import { shoot } from "../capture/shoot";
import { loadScript } from "../script/load";

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run capture -- <script-id>");
  process.exit(2);
}
const layout = layoutFor();
const script = loadScript(scriptPath(layout, id));
const outDir = join(buildDir(layout, id), "captures");
for (const c of script.captures) console.log(`wrote ${await shoot(c, outDir)}`);
