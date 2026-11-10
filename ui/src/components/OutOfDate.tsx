// A reread that fails keeps the last good content on screen (hooks.ts's
// useFetch carries it beside the error), and this says so above it rather
// than replacing it. The error is the visible text, so the warning is never
// color alone. Home's stat tiles carry their own smaller form of the same
// note (home/HeaderBand.tsx's StaleNote).
export default function OutOfDate({ error }: { error: string | null }) {
  if (error === null) {
    return null;
  }
  return (
    <p className="error" role="status">
      Out of date: {error}
    </p>
  );
}
