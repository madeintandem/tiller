// Renders a late frame of TermSmoke fed by a real snapshot. Usage: tsx src/cli/term-still.ts <out.png> [stage] [frame]
import { resolve } from "node:path";
import { bundle } from "@remotion/bundler";
import { renderStill, selectComposition } from "@remotion/renderer";
import { readSnapshot } from "../check/diff";

const [out = "term.png", stage = "s1", frame = "400"] = process.argv.slice(2);
const inputProps = { steps: readSnapshot(stage).slice(0, 9) };
const serveUrl = await bundle({ entryPoint: resolve(import.meta.dirname, "../index.ts") });
const composition = await selectComposition({ serveUrl, id: "TermSmoke", inputProps });
await renderStill({ composition, serveUrl, inputProps, frame: Number(frame), output: resolve(out) });
