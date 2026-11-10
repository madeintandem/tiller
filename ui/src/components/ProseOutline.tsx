import type { OutlineItem } from "../lib/outline";

// ProseOutline is the "On this page" list beside a long spec: navigation
// only, one in-page anchor per section heading, no state of its own. The
// ids it links to are the ones Prose puts on those headings.
export default function ProseOutline({ items }: { items: OutlineItem[] }) {
  return (
    <nav className="prose-outline" aria-label="On this page">
      <span className="prose-outline-head">On this page</span>
      <ul>
        {items.map((item) => (
          <li key={item.id} className={item.depth === 3 ? "sub" : undefined}>
            <a href={`#${item.id}`}>{item.text}</a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
