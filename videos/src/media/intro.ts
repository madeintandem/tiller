import { ffmpeg, hasFilter, loudnormFilter, probeColorTransfer, probeDuration, run } from "./ffmpeg.js";

const NOISE = "-45dB";
const MIN_SILENCE = 0.3;
// Kept before the first word and after the last, so the cut never clips a breath or a consonant.
const PAD = 0.12;

/** The voice level: the intro is normalized here, and the music bed sits relative to it. */
export const VOICE_LUFS = -16;
const VOICE = { I: VOICE_LUFS, TP: -1.5, LRA: 11 };

const HDR_TRANSFERS = new Set(["arib-std-b67", "smpte2084"]); // HLG, PQ

/**
 * The filters that bring an HDR (HLG or PQ) clip down to SDR BT.709, or "" for SDR. Without
 * zscale in this ffmpeg build there is no faithful tone map, so an HDR clip is refused.
 */
export function hdrFilter(transfer: string | undefined, zscale: boolean): string {
  if (!transfer || !HDR_TRANSFERS.has(transfer)) return "";
  if (!zscale) {
    throw new Error(
      `the intro clip is HDR (${transfer}) and this ffmpeg cannot tone-map it; ` +
        "turn off HDR Video in Camera settings (Settings > Camera > Formats) and film it again",
    );
  }
  return "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,";
}

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
  if (!(end > start)) return { start: 0, end: total };
  return { start: Math.max(0, start - PAD), end: Math.min(total, end + PAD) };
}

/**
 * Trim leading and trailing silence (keeping a short pad), bring the voice to -16 LUFS
 * (two-pass loudnorm), tone-map an HDR clip to SDR (or refuse it), then conform to
 * 1920x1080 at 30 fps mp4. A mismatched aspect ratio (portrait phone clips) is scaled to
 * fit and padded with black bars, never stretched or cropped.
 */
export async function prepareIntro(srcPath: string, outPath: string): Promise<{ path: string; duration: number }> {
  const transfer = await probeColorTransfer(srcPath);
  const hdr = hdrFilter(transfer, HDR_TRANSFERS.has(transfer ?? "") && (await hasFilter("zscale")));
  const total = await probeDuration(srcPath);
  const window = await speechWindow(srcPath, total);
  const voice = await loudnormFilter(srcPath, VOICE, window);
  await ffmpeg([
    "-i", srcPath, "-ss", String(window.start), "-to", String(window.end),
    "-vf",
    `${hdr}scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2:black,setsar=1,fps=30,` +
      "setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709,format=yuv420p",
    "-af", `${voice},aresample=48000`,
    "-c:v", "libx264", "-profile:v", "high", "-crf", "18", "-preset", "medium",
    "-c:a", "aac", "-b:a", "256k", "-ar", "48000", "-ac", "2", "-movflags", "+faststart", outPath,
  ]);
  return { path: outPath, duration: await probeDuration(outPath) };
}
