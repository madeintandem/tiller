export default function PageLoading({ title }: { title: string }) {
  return (
    <section className="page-shell">
      <h1>{title}</h1>
      <p className="muted" role="status">Loading…</p>
    </section>
  );
}
