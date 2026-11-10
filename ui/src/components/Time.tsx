import { formatAbsolute, formatClock, formatPoint, formatWhen } from "../lib/time";

// Time renders one RFC3339 instant through the shared formatter, with the full
// instant on hover. "when" is the default relative-then-dated read, "clock"
// the bare time of day, and "point" a point in time that never reads as an
// age, for sentences such as "since <point>".
export default function Time({ iso, mode = "when" }: { iso: string; mode?: "when" | "clock" | "point" }) {
  const text =
    mode === "clock" ? formatClock(iso) : mode === "point" ? formatPoint(iso, new Date()) : formatWhen(iso, new Date());
  return <time dateTime={iso} title={formatAbsolute(iso)}>{text}</time>;
}
