import { readSnapshot, type Step } from "../check/diff.js";
import type { Beat, Script } from "../script/schema.js";
import { mss, termSteps } from "../timeline/build.js";
import type { Layout } from "./layout.js";
import { checkScript, estimateSeconds } from "./validate.js";

// What a beat puts on screen, spelled out so the words can be judged against it: a term
// beat's commands and say lines from the committed snapshot, a console beat's page.
function onScreen(b: Beat, script: Script, layout: Layout): string[] {
  const show = b.show;
  switch (show.kind) {
    case "card":
      return [`*Card: ${show.kicker} / ${show.heading}${show.body ? ` / ${show.body}` : ""}*`];
    case "term": {
      let steps: Step[];
      try {
        steps = termSteps(b.id, show, { [show.stage]: readSnapshot(show.stage, layout.snapshots) });
      } catch (e) {
        return [`*Terminal (${show.stage}): ${(e as Error).message}*`];
      }
      const lines = steps.map((s) => (s.kind === "cmd" ? `${s.actor.padEnd(5)} $ ${s.cmd}` : `${s.actor.padEnd(5)}   ${s.text}`));
      return [`*Terminal (${show.stage}):*`, "", "```", ...lines, "```"];
    }
    case "console": {
      const c = script.captures.find((x) => x.id === show.capture);
      if (!c) return [`*Console: unknown capture ${show.capture}*`];
      const actions = (c.actions ?? [])
        .map((a) =>
          a.wait !== undefined ? `wait ${a.wait} s`
          : a.scrollTo ? `scroll to ${a.scrollTo}`
          : a.scroll !== undefined ? `scroll ${a.scroll}px`
          : a.click ? `click ${a.click}`
          : `wait for ${a.waitFor}`,
        )
        .join(", ");
      return [`*Console: \`${c.path}\` at mark ${c.mark}, ${c.seconds} s${actions ? ` (${actions})` : ""}*`];
    }
    case "outro":
      return ["*Outro*"];
  }
}

/** The script as Markdown for review: chapters, narration, and what is on screen under each beat. */
export function previewMarkdown(s: Script, layout: Layout): string {
  const { problems, warnings } = checkScript(s, layout);
  const out: string[] = [`# ${s.title}`, ""];
  if (problems.length) out.push("## Problems (this script cannot build)", "", ...problems.map((p) => `- ${p}`), "");
  if (warnings.length) out.push(...warnings.map((w) => `> Warning: ${w}`), "");
  out.push(s.description, "", `Tags: ${s.tags.join(", ")}`, "", `Estimated length: ${mss(estimateSeconds(s), Math.ceil)}`, "");
  out.push("## Live intro (JC)", "", s.intro.transcript, "");
  for (const b of s.beats) {
    if (b.chapter) out.push(`## ${b.chapter}`, "");
    out.push(`**${b.id}:** ${b.say}`, "", ...onScreen(b, s, layout), "");
  }
  return out.join("\n");
}
