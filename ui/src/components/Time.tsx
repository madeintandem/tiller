import { formatAbsolute, formatClock, formatWhen } from "../lib/time";

// Time renders one RFC3339 instant through the shared formatter, with the full
// instant on hover.
export default function Time({ iso, mode = "when" }: { iso: string; mode?: "when" | "clock" }) {
  const text = mode === "clock" ? formatClock(iso) : formatWhen(iso, new Date());
  return <time dateTime={iso} title={formatAbsolute(iso)}>{text}</time>;
}
