// Timing for the lower thirds, kept pure so it can be tested without rendering.

export const CHAPTER_SECONDS = 3;
const IN_FRAMES = 9; // 300 ms at 30 fps
const OUT_FRAMES = 9;

/** A chapter lower third runs 3 s (or `seconds`) from the narration start, but never past its card's slot. */
export function chapterFrames(from: number, slotEnd: number, fps: number, seconds: number = CHAPTER_SECONDS): number {
  return Math.max(1, Math.min(Math.round(seconds * fps), slotEnd - from));
}

/** Opacity of a lower third `frame` frames into a sequence of `duration`: in, hold, out. */
export function lowerThirdOpacity(frame: number, duration: number): number {
  // In a short sequence the ramps share it, so the peak is still reached.
  const ramp = Math.max(1, Math.min(IN_FRAMES, OUT_FRAMES, Math.floor(duration / 2)));
  const enter = Math.min(1, frame / ramp);
  const exit = Math.min(1, (duration - frame) / ramp);
  return Math.max(0, Math.min(enter, exit));
}
