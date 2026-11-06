import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ffmpeg, loudnormFilter, probeDuration } from "./ffmpeg.js";
import { VOICE_LUFS } from "./intro.js";

// The bed sits 18 LU under the voice (the intro is normalized to VOICE_LUFS); the final
// program normalization moves both by the same gain, so the gap holds. That needs loudnorm's
// linear mode, which it drops for dynamic compression when the measured loudness range
// exceeds the LRA target; a bed 18 LU under the voice gives the program a wide range, so the
// LRA targets here are set wide enough to stay linear.
export const MUSIC_UNDER_VOICE = 18;
const PROGRAM = { I: -14, TP: -1, LRA: 20 };
const MUSIC = { I: VOICE_LUFS - MUSIC_UNDER_VOICE, TP: -2, LRA: 20 };
const FADE = 1;

/** Music gain: 0 everywhere except 0..introEnd and outroStart..end, with FADE-second ramps at each edge. */
export function musicGain(introEnd: number, outroStart: number, end: number): string {
  const ramp = (a: number, b: number) => `clip(min((t-${a})/${FADE},(${b}-t)/${FADE}),0,1)`;
  return `${ramp(0, introEnd)}+${ramp(outroStart, end)}`;
}

/**
 * Mix music under the intro and outro only, loudness-normalize the whole program
 * (two-pass loudnorm), and encode H.264 High / AAC 320k with faststart.
 */
export async function master(
  renderPath: string,
  musicPath: string,
  outro: { start: number },
  introEnd: number,
  outPath: string,
): Promise<string> {
  const end = await probeDuration(renderPath);
  const dir = mkdtempSync(join(tmpdir(), "master-"));
  try {
    const mixed = join(dir, "mix.wav");
    const gain = musicGain(introEnd, outro.start, end);
    const music = await loudnormFilter(musicPath, MUSIC);
    await ffmpeg([
      "-i", renderPath, "-stream_loop", "-1", "-i", musicPath,
      "-filter_complex",
      `[1:a]${music},aformat=sample_rates=48000:channel_layouts=stereo,` +
        `volume='${gain}':eval=frame[m];` +
        `[0:a]aformat=sample_rates=48000:channel_layouts=stereo[v];` +
        `[v][m]amix=inputs=2:duration=first:normalize=0[a]`,
      "-map", "[a]", "-t", String(end), "-c:a", "pcm_f32le", mixed,
    ]);

    const pass2 = await loudnormFilter(mixed, PROGRAM);

    await ffmpeg([
      "-i", renderPath, "-i", mixed, "-map", "0:v:0", "-map", "1:a:0",
      "-af", `${pass2},aresample=48000`,
      "-c:v", "libx264", "-profile:v", "high", "-pix_fmt", "yuv420p", "-crf", "18", "-preset", "medium",
      "-c:a", "aac", "-b:a", "320k", "-movflags", "+faststart", "-t", String(end), outPath,
    ]);
    return outPath;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}
