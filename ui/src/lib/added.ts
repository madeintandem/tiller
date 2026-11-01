// "Added Oct 12", or "Added Oct 12, 2025" when the year is not now's, in the
// reader's local time (the way History.tsx labels days).
export function formatAdded(iso: string, now: Date): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return `Added ${iso}`;
  const sameYear = d.getFullYear() === now.getFullYear();
  const day = d.toLocaleDateString(
    undefined,
    sameYear ? { month: "short", day: "numeric" } : { month: "short", day: "numeric", year: "numeric" },
  );
  return `Added ${day}`;
}
