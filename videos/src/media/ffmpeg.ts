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
