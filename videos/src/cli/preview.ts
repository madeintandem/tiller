import { layoutFor, scriptPath } from "../build/layout.js";
import { loadScript } from "../script/load.js";
import type { Show } from "../script/schema.js";

function onScreen(show: Show): string {
  switch (show.kind) {
    case "card":
      return `Card: ${show.kicker} / ${show.heading}${show.body ? ` / ${show.body}` : ""}`;
    case "term":
      return `Terminal (${show.stage}): ${show.from} to ${show.to}`;
    case "console":
      return `Console capture: ${show.capture}`;
    case "outro":
      return "Outro";
  }
}

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run preview -- <video-id>");
  process.exit(2);
}
try {
  const s = loadScript(scriptPath(layoutFor(), id));
  const out: string[] = [`# ${s.title}`, "", s.description, "", `Tags: ${s.tags.join(", ")}`, ""];
  out.push("## Live intro (JC)", "", s.intro.transcript, "");
  for (const b of s.beats) {
    if (b.chapter) out.push(`## ${b.chapter}`, "");
    out.push(`**${b.id}:** ${b.say}`, "", `*${onScreen(b.show)}*`, "");
  }
  console.log(out.join("\n"));
} catch (e) {
  console.error((e as Error).message);
  process.exit(1);
}
