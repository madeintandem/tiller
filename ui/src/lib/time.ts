// One timestamp voice for the console. Every RFC3339 value the API sends is
// shown through these three functions, in the reader's local time. A value that
// does not parse is returned unchanged: showing the raw field beats hiding it.

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

// Newer ICU builds put a narrow no-break space before AM/PM; plain spaces keep
// the output identical everywhere.
function plain(s: string): string {
  return s.replace(/[  ]/g, " ");
}

function parse(iso: string): Date | null {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? null : d;
}

// formatWhen is the default read: relative inside a day, then a dated stamp.
export function formatWhen(iso: string, now: Date): string {
  const d = parse(iso);
  if (d === null) {
    return iso;
  }
  const age = now.getTime() - d.getTime();
  if (age < MINUTE) {
    return "just now";
  }
  if (age < HOUR) {
    return `${Math.floor(age / MINUTE)}m ago`;
  }
  if (age < DAY) {
    return `${Math.floor(age / HOUR)}h ago`;
  }
  const sameYear = d.getFullYear() === now.getFullYear();
  return plain(
    d.toLocaleString("en-US", {
      month: "short",
      day: "numeric",
      ...(sameYear ? {} : { year: "numeric" }),
      hour: "numeric",
      minute: "2-digit",
    }),
  );
}

// formatClock is a bare clock reading, for columns whose window is a day.
export function formatClock(iso: string): string {
  const d = parse(iso);
  if (d === null) {
    return iso;
  }
  return plain(d.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" }));
}

// formatAbsolute is the full instant, used as the hover title.
export function formatAbsolute(iso: string): string {
  const d = parse(iso);
  if (d === null) {
    return iso;
  }
  return plain(
    d.toLocaleString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
      hour: "numeric",
      minute: "2-digit",
      second: "2-digit",
    }),
  );
}
