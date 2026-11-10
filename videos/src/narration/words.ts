// The pace assumed wherever there is no real audio to measure: the length estimate and fake
// narration. The narration voice measured 183 wpm over the first real v1 build (906 words in
// 296.6 s); this sits a little under it so both err long.
export const WPM = 175;

export type Word = { text: string; start: number; end: number };

// Group per-character timings into words: whitespace separates words and is dropped.
export function charsToWords(chars: string[], starts: number[], ends: number[]): Word[] {
  const words: Word[] = [];
  let cur: Word | undefined;
  for (let i = 0; i < chars.length; i++) {
    if (/^\s*$/.test(chars[i])) {
      cur = undefined;
      continue;
    }
    if (!cur) {
      cur = { text: "", start: starts[i], end: ends[i] };
      words.push(cur);
    }
    cur.text += chars[i];
    cur.end = ends[i];
  }
  return words;
}

// Space the transcript's words evenly across a duration (fake mode).
export function spreadWords(transcript: string, duration: number): Word[] {
  const texts = transcript.split(/\s+/).filter(Boolean);
  const step = duration / Math.max(texts.length, 1);
  return texts.map((text, i) => ({ text, start: i * step, end: (i + 1) * step }));
}
