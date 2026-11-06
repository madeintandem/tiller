export type Actor = "agent" | "you";

export type Show =
  | { kind: "card"; kicker: string; heading: string; body?: string }
  | { kind: "term"; stage: string; from: string; to: string }
  | { kind: "console"; capture: string }
  | { kind: "outro" };

export type Beat = { id: string; chapter?: string; say: string; show: Show };

// One step of a capture, run in order after the page loads. `wait` holds the picture for that
// many seconds; `scroll` (pixels) and `scrollTo` (a selector) glide the page's scrolling content.
export type CaptureAction = { click?: string; waitFor?: string; scroll?: number; scrollTo?: string; wait?: number };

export type Capture = {
  id: string;
  mark: string;
  path: string;
  actions?: CaptureAction[];
  seconds: number;
};

export type Script = {
  id: string;
  title: string;
  description: string;
  tags: string[];
  intro: { transcript: string };
  captures: Capture[];
  beats: Beat[];
};

export class ScriptError extends Error {
  constructor(
    message: string,
    public readonly beatId?: string,
  ) {
    super(beatId ? `beat ${beatId}: ${message}` : message);
    this.name = "ScriptError";
  }
}

// Narration must never read commands, flags, node IDs, or paths aloud.
const FORBIDDEN_SAY: [RegExp, string][] = [
  [/\btiller [a-z]/, "a command"],
  [/--[a-z]/, "a flag"],
  [/\b[EFTB]-[a-z0-9]{6}\b/, "a node ID"],
  [/\.tiller\//, "a path"],
];

type Obj = Record<string, unknown>;

function isObj(v: unknown): v is Obj {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function str(o: Obj, key: string, where: string, beatId?: string): string {
  const v = o[key];
  if (typeof v !== "string" || v.trim() === "") {
    throw new ScriptError(`${where} needs a non-empty string "${key}"`, beatId);
  }
  return v;
}

function validateShow(raw: unknown, beatId: string): Show {
  if (!isObj(raw)) throw new ScriptError("show must be a mapping", beatId);
  switch (raw.kind) {
    case "card": {
      const show: Show = {
        kind: "card",
        kicker: str(raw, "kicker", "card show", beatId),
        heading: str(raw, "heading", "card show", beatId),
      };
      if (raw.body !== undefined) show.body = str(raw, "body", "card show", beatId);
      return show;
    }
    case "term":
      return {
        kind: "term",
        stage: str(raw, "stage", "term show", beatId),
        from: str(raw, "from", "term show", beatId),
        to: str(raw, "to", "term show", beatId),
      };
    case "console":
      return { kind: "console", capture: str(raw, "capture", "console show", beatId) };
    case "outro":
      return { kind: "outro" };
    default:
      throw new ScriptError(`unknown show kind "${String(raw.kind)}"`, beatId);
  }
}

function validateBeat(raw: unknown, index: number): Beat {
  if (!isObj(raw)) throw new ScriptError(`beat #${index + 1} must be a mapping`);
  const id = str(raw, "id", `beat #${index + 1}`);
  const say = str(raw, "say", "beat", id);
  for (const [re, what] of FORBIDDEN_SAY) {
    if (re.test(say)) throw new ScriptError(`say must not contain ${what} (matched ${re})`, id);
  }
  const beat: Beat = { id, say, show: validateShow(raw.show, id) };
  if (raw.chapter !== undefined) {
    beat.chapter = str(raw, "chapter", "beat", id);
    // The chapter lower third would cover a terminal's newest line or a capture; cards only.
    if (beat.show.kind !== "card") throw new ScriptError("chapter is only allowed on a card beat", id);
  }
  return beat;
}

function validateCapture(raw: unknown, index: number): Capture {
  const where = `capture #${index + 1}`;
  if (!isObj(raw)) throw new ScriptError(`${where} must be a mapping`);
  const id = str(raw, "id", where);
  const mark = str(raw, "mark", `capture ${id}`);
  const path = str(raw, "path", `capture ${id}`);
  const seconds = raw.seconds;
  if (typeof seconds !== "number" || !(seconds > 0)) {
    throw new ScriptError(`capture ${id} needs positive "seconds"`);
  }
  const capture: Capture = { id, mark, path, seconds };
  if (raw.actions !== undefined) {
    if (!Array.isArray(raw.actions)) throw new ScriptError(`capture ${id} actions must be a list`);
    capture.actions = raw.actions.map((a) => {
      if (!isObj(a)) throw new ScriptError(`capture ${id} action must be a mapping`);
      const action: CaptureAction = {};
      if (a.click !== undefined) action.click = str(a, "click", `capture ${id} action`);
      if (a.waitFor !== undefined) action.waitFor = str(a, "waitFor", `capture ${id} action`);
      if (a.scrollTo !== undefined) action.scrollTo = str(a, "scrollTo", `capture ${id} action`);
      if (a.scroll !== undefined) {
        if (typeof a.scroll !== "number") throw new ScriptError(`capture ${id} scroll must be a number`);
        action.scroll = a.scroll;
      }
      if (a.wait !== undefined) {
        if (typeof a.wait !== "number" || !(a.wait > 0)) throw new ScriptError(`capture ${id} wait must be a positive number of seconds`);
        action.wait = a.wait;
      }
      if (Object.keys(action).length === 0) throw new ScriptError(`capture ${id} has an action with no known key`);
      return action;
    });
  }
  return capture;
}

export function validateScript(raw: unknown): Script {
  if (!isObj(raw)) throw new ScriptError("script must be a mapping");
  const id = str(raw, "id", "script");
  const title = str(raw, "title", "script");
  const description = str(raw, "description", "script");
  if (!Array.isArray(raw.tags) || raw.tags.some((t) => typeof t !== "string")) {
    throw new ScriptError("script tags must be a list of strings");
  }
  if (!isObj(raw.intro)) throw new ScriptError("script needs intro.transcript");
  const intro = { transcript: str(raw.intro, "transcript", "intro") };
  if (raw.captures !== undefined && !Array.isArray(raw.captures)) {
    throw new ScriptError("script captures must be a list");
  }
  const captures = ((raw.captures as unknown[] | undefined) ?? []).map(validateCapture);
  if (!Array.isArray(raw.beats) || raw.beats.length === 0) {
    throw new ScriptError("script needs a non-empty beats list");
  }
  const beats = raw.beats.map(validateBeat);
  const seen = new Set<string>();
  for (const b of beats) {
    if (seen.has(b.id)) throw new ScriptError("duplicate beat id", b.id);
    seen.add(b.id);
  }
  const captureIds = new Set(captures.map((c) => c.id));
  for (const b of beats) {
    if (b.show.kind === "console" && !captureIds.has(b.show.capture)) {
      throw new ScriptError(`console show references unknown capture "${b.show.capture}"`, b.id);
    }
  }
  return { id, title, description, tags: raw.tags as string[], intro, captures, beats };
}
