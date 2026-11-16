import { useEffect, type MouseEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";

import type { OutlineItem } from "../lib/outline";
import { IN_PAGE_CHANGE } from "../lib/useArrivalKey";

// ProseOutline is the "On this page" list beside a long spec: navigation
// only, one link per section heading, no state of its own. The ids it links
// to are the ones Prose puts on those headings.
//
// The console follows a link itself instead of leaving the bare #id anchor to
// the browser, which reloaded the whole console in Safari (B-iqc14u). A click
// scrolls the heading into view, where its scroll margin stops it below the
// pinned header and rail (prose.css), and names the heading in the address as
// the page's own change: it replaces the entry and reads nothing. The href
// stays, so a modified click still opens the heading in a new tab.
export default function ProseOutline({ items }: { items: OutlineItem[] }) {
  const navigate = useNavigate();
  const location = useLocation();
  const follow = (event: MouseEvent<HTMLAnchorElement>, id: string) => {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    scrollToHeading(id);
    navigate(
      { pathname: location.pathname, search: location.search, hash: `#${id}` },
      { replace: true, state: IN_PAGE_CHANGE, preventScrollReset: true },
    );
  };
  return (
    <nav className="prose-outline" aria-label="On this page">
      <span className="prose-outline-head">On this page</span>
      <ul>
        {items.map((item) => (
          <li key={item.id} className={item.depth === 3 ? "sub" : undefined}>
            <a href={`#${item.id}`} onClick={(event) => follow(event, item.id)}>
              {item.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function scrollToHeading(id: string): void {
  document.getElementById(id)?.scrollIntoView?.({ block: "start" });
}

// useHeadingLanding lands a page that opens with a heading's id in its
// address, a reload or a link someone shared, on that heading. The browser
// cannot do it: the spec is not in the document yet when it looks for the
// fragment. It runs once, when the spec first renders, and only for an id
// this spec's own headings carry, so a fragment that means something else
// (a source line on a file page) is left alone.
export function useHeadingLanding(items: OutlineItem[]): void {
  const { hash } = useLocation();
  useEffect(() => {
    let id: string;
    try {
      id = decodeURIComponent(hash.slice(1));
    } catch {
      return;
    }
    if (id !== "" && items.some((item) => item.id === id)) {
      scrollToHeading(id);
    }
    // Once per mount: later address changes are the outline's own clicks,
    // which have already scrolled.
  }, []);
}
