// Usage: tsx src/cli/capture.ts <script-id>
import { resolve } from "node:path";
import { shoot } from "../capture/shoot";
import { loadScript } from "../script/load";

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run capture -- <script-id>");
  process.exit(2);
}
const root = resolve(import.meta.dirname, "../..");
const script = loadScript(resolve(root, "scripts", `${id}.yaml`));
const outDir = resolve(root, ".local/build", id, "captures");
for (const c of script.captures) console.log(`wrote ${await shoot(c, outDir)}`);
