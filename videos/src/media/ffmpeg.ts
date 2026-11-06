import { execFile } from "node:child_process";

export const ffEnv = () => ({ ...process.env, PATH: `${process.env.PATH ?? ""}:/opt/homebrew/bin:/usr/local/bin` });

/** Run ffmpeg or ffprobe; resolves with stdout and stderr (ffmpeg filters report on stderr). */
export function run(bin: "ffmpeg" | "ffprobe", args: string[]): Promise<{ stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    execFile(bin, args, { env: ffEnv(), maxBuffer: 64 * 1024 * 1024 }, (err, stdout, stderr) => {
      if (err) {
        if ((err as NodeJS.ErrnoException).code === "ENOENT") return reject(new Error("ffmpeg not found; brew install ffmpeg"));
        return reject(new Error(`${bin} failed: ${err.message}\n${stderr}`));
      }
      resolve({ stdout, stderr });
    });
  });
}

export const ffmpeg = (args: string[]) => run("ffmpeg", ["-y", "-hide_banner", "-loglevel", "error", ...args]);

export async function probeDuration(path: string): Promise<number> {
  const { stdout } = await run("ffprobe", ["-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", path]);
  return parseFloat(stdout);
}

/** The first video stream's color transfer (e.g. bt709, arib-std-b67 for HLG, smpte2084 for PQ), if tagged. */
export async function probeColorTransfer(path: string): Promise<string | undefined> {
  const { stdout } = await run("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries", "stream=color_transfer", "-of", "default=nw=1:nk=1", path]);
  const t = stdout.trim();
  return t === "" || t === "unknown" ? undefined : t;
}

let filterList: Promise<string> | undefined;

/** Whether this ffmpeg build has a filter (zscale is optional in Homebrew's ffmpeg). */
export async function hasFilter(name: string): Promise<boolean> {
  filterList ??= run("ffmpeg", ["-hide_banner", "-filters"]).then((r) => r.stdout);
  return new RegExp(`^\\s*\\S+\\s+${name}\\s`, "m").test(await filterList);
}

export type LoudnessTarget = { I: number; TP: number; LRA: number };

/**
 * Pass one of a two-pass loudnorm: measure `path` (optionally only `window`) and return the
 * pass-two filter that brings it to `target` with one linear gain where the peaks allow.
 */
export async function loudnormFilter(path: string, target: LoudnessTarget, window?: { start: number; end: number }): Promise<string> {
  const norm = `loudnorm=I=${target.I}:TP=${target.TP}:LRA=${target.LRA}`;
  const cut = window ? ["-ss", String(window.start), "-to", String(window.end)] : [];
  const { stderr } = await run("ffmpeg", ["-hide_banner", "-nostats", "-i", path, ...cut, "-vn", "-af", `${norm}:print_format=json`, "-f", "null", "-"]);
  const m = JSON.parse(stderr.slice(stderr.lastIndexOf("{"), stderr.lastIndexOf("}") + 1));
  return (
    `${norm}:measured_I=${m.input_i}:measured_TP=${m.input_tp}:measured_LRA=${m.input_lra}:` +
    `measured_thresh=${m.input_thresh}:offset=${m.target_offset}:linear=true`
  );
}
