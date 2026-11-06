import { join, resolve } from "node:path";
import { VIDEOS_DIR } from "../script/load.js";

// Where every input and output of a build lives, rooted at a videos/ directory.
// VIDEOS_SCRIPTS and VIDEOS_APPROVALS point the scripts and approvals elsewhere
// (the end-to-end test build uses them to reach test/fixtures without touching
// the committed approvals.json); TILLER overrides the binary.
export type Layout = {
  root: string;
  scripts: string;
  approvals: string;
  series: string;
  snapshots: string;
  marks: string;
  publicDir: string;
  local: string;
  config: string;
  intros: string;
  music: string;
  cache: string;
  builds: string;
  out: string;
  // What publish has uploaded: local only, since unlisted video IDs must not reach a public repo.
  published: string;
  tiller: string;
};

export function layoutFor(root: string = VIDEOS_DIR, env: NodeJS.ProcessEnv = process.env): Layout {
  const local = join(root, ".local");
  return {
    root,
    scripts: env.VIDEOS_SCRIPTS ? resolve(env.VIDEOS_SCRIPTS) : join(root, "scripts"),
    approvals: env.VIDEOS_APPROVALS ? resolve(env.VIDEOS_APPROVALS) : join(root, "approvals.json"),
    series: join(root, "series.yaml"),
    snapshots: join(root, "snapshots"),
    marks: join(root, "fixture", "marks"),
    publicDir: join(root, "public"),
    local,
    config: join(local, "config.yaml"),
    intros: join(local, "media", "intros"),
    music: join(local, "media", "music.mp3"),
    cache: join(local, "narration-cache"),
    builds: join(local, "build"),
    out: join(root, "out"),
    published: join(local, "published.json"),
    tiller: env.TILLER || "tiller",
  };
}

export const scriptPath = (l: Layout, id: string) => join(l.scripts, `${id}.yaml`);
export const introPath = (l: Layout, id: string) => join(l.intros, `${id}.mov`);
export const buildDir = (l: Layout, id: string) => join(l.builds, id);
