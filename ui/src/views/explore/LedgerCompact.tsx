// LedgerCompact is a ledger pane's one-line header (Questions, Amendments),
// shown in the band above the pinned tab rail once the page header has
// scrolled away: the ledger's name and the tally that needs attention. It
// reuses the node page's band (node-pin.css) and, like NodeCompact, is hidden
// from assistive tech because the full header is still in the document.

export default function LedgerCompact({ title, tally }: { title: string; tally: string }) {
  return (
    <div className="node-compact" aria-hidden="true">
      <span className="node-compact-title">{title}</span>
      <span className="muted">{tally}</span>
    </div>
  );
}
