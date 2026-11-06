import { ffmpeg, probeDuration, run } from "./ffmpeg.js";

const NOISE = "-45dB";
const MIN_SILENCE = 0.3;

/** Find where speech starts and ends, from silencedetect on the audio track. */
async function speechWindow(src: string, total: number): Promise<{ start: number; end: number }> {
  const { stderr } = await run("ffmpeg", [
    "-hide_banner", "-nostats", "-i", src, "-vn", "-af", `silencedetect=noise=${NOISE}:d=${MIN_SILENCE}`, "-f", "null", "-",
  ]);
  const starts = [...stderr.matchAll(/silence_start: (-?[\d.]+)/g)].map((m) => Math.max(0, parseFloat(m[1])));
  const ends = [...stderr.matchAll(/silence_end: ([\d.]+)/g)].map((m) => parseFloat(m[1]));
  let start = 0;
  let end = total;
  if (starts.length && starts[0] <= 0.05 && ends.length) start = ends[0];
  // A trailing silence either has no end line or ends at (about) EOF.
  const last = starts.length - 1;
  if (last >= 0 && (ends.length <= last || ends[last] >= total - 0.1)) end = Math.max(start, starts[last]);
  return end > start ? { start, end } : { start: 0, end: total };
}

/**
 * Trim leading and trailing silence, then conform to 1920x1080 at 30 fps mp4.
 * A mismatched aspect ratio (portrait phone clips) is scaled to fit and padded
 * with black bars, never stretched or cropped.
 */
export async function prepareIntro(srcPath: string, outPath: string): Promise<{ path: string; duration: number }> {
  const total = await probeDuration(srcPath);
  const { start, end } = await speechWindow(srcPath, total);
  await ffmpeg([
    "-i", srcPath, "-ss", String(start), "-to", String(end),
    "-vf", "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2:black,setsar=1,fps=30,format=yuv420p",
    "-c:v", "libx264", "-profile:v", "high", "-crf", "18", "-preset", "medium",
    "-c:a", "aac", "-b:a", "256k", "-ar", "48000", "-ac", "2", "-movflags", "+faststart", outPath,
  ]);
  return { path: outPath, duration: await probeDuration(outPath) };
}
