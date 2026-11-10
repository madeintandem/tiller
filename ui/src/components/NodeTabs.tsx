// NodeTabs is the node detail body's tab shell: an underline rail over the
// per-noun panel set (spec/evals/tasks/dependencies/history for a feature, and
// so on), reusing Explore's own mode-switch chrome (explore.css's .mode-nav /
// .mode-nav a.active) rather than inventing a second tab idiom - the
// console's visual system is locked, and this change is structural, not
// visual. Explore's own version of this row is built from <Link>, since a
// mode switch is a real navigation the back button should walk; a node tab
// is a lighter switch (ui-surface.md never asks for a tab to be its own
// history entry), so this row is buttons plus a controlled `active` key and
// an `onSelect` callback, and NodeDetail is the one place that reads and
// writes the `?tab=` param those callbacks end up changing.
//
// Selection is not this component's own state: NodeDetail resolves `active`
// from the URL (falling back to the default tab itself, same shape Explore's
// own mode resolution takes for an unrecognized `?mode=`), and this component
// applies one more fallback of its own - an `active` key that names no tab at
// all (a stale link, a hand-edited URL) reads as the first tab rather than
// rendering no panel at all, so the page never goes blank over a bad param.

import { useId, useRef, type KeyboardEvent, type ReactElement, type ReactNode } from "react";
import { useScrolledPast } from "../lib/useScrolledPast";
import "../views/explore.css";

export interface NodeTab {
  key: string;
  label: string;
  count?: number;
  // "warning" tints the count for problems worth a look (Home's Branches
  // diagnostics); the default stays the muted tally.
  countTone?: "warning";
  render: () => ReactElement;
}

export default function NodeTabs({
  tabs,
  active,
  onSelect,
  label = "Node detail tab",
  pinned,
}: {
  tabs: NodeTab[];
  active: string;
  onSelect: (key: string) => void;
  // The rail's accessible name. The node page keeps the default; Home's
  // needs-you section uses the same rail for its decision kinds (2026-09-01).
  label?: string;
  // When given, the rail sits in a sticky .node-pin so it stays in view while
  // the reading pane scrolls, and this content (the node page's one-line
  // header) overlays the band above it once the rail has pinned. It never
  // takes space in flow, so showing it moves nothing. Home omits it.
  pinned?: ReactNode;
}) {
  const activeTab = tabs.find((tab) => tab.key === active) ?? tabs[0];
  // Real ARIA tabs (WAI-ARIA tabs pattern, automatic activation). One panel is
  // rendered at a time, so every tab's aria-controls names that one panel id,
  // and the panel is labelled by whichever tab is selected. Callers hand over
  // their bodies through each tab's render(); the component owns the wrapper.
  const baseId = useId();
  const panelId = `${baseId}-panel`;
  const tabId = (key: string) => `${baseId}-tab-${key}`;
  const tabRefs = useRef(new Map<string, HTMLButtonElement>());
  // Selection follows focus. onSelect is the surface's own navigation (a
  // replace, for every surface today), so arrowing never fills history. The
  // buttons are keyed by tab, so the target survives the re-render the URL
  // change causes and can be focused right away.
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
    const at = tabs.findIndex((tab) => tab.key === activeTab.key);
    let next: number;
    switch (event.key) {
      case "ArrowRight":
        next = (at + 1) % tabs.length;
        break;
      case "ArrowLeft":
        next = (at - 1 + tabs.length) % tabs.length;
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = tabs.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    onSelect(tabs[next].key);
    tabRefs.current.get(tabs[next].key)?.focus();
  };
  const panel = (
    <div role="tabpanel" id={panelId} aria-labelledby={tabId(activeTab.key)} tabIndex={0}>
      {activeTab.render()}
    </div>
  );
  // The sentinel sits one band above the rail's place in flow, so it leaves
  // the pane exactly when the rail pins (node-pin.css).
  const sentinelRef = useRef<HTMLSpanElement>(null);
  const railPinned = useScrolledPast(sentinelRef);
  const rail = (
    <>
      {railPinned && pinned}
      <div className="mode-nav" role="tablist" aria-label={label}>
        {tabs.map((tab) => {
          const isActive = tab.key === activeTab.key;
          return (
            <button
              key={tab.key}
              type="button"
              role="tab"
              id={tabId(tab.key)}
              ref={(el) => {
                if (el) tabRefs.current.set(tab.key, el);
                else tabRefs.current.delete(tab.key);
              }}
              className={isActive ? "active" : undefined}
              aria-selected={isActive}
              aria-controls={panelId}
              tabIndex={isActive ? 0 : -1}
              onKeyDown={onKeyDown}
              onClick={() => onSelect(tab.key)}
            >
              {tab.label}
              {/* The count is its own span so the rail can mute it (node
                  header pass, 2026-08-30); the button's accessible name is
                  the same "Label · n" it always was. The space stays OUTSIDE
                  the span on purpose: accessible-name computation trims each
                  inline element's own text, so a space inside the span would
                  read "Tasks· 4" to assistive tech and to every test. */}
              {tab.count !== undefined && (
                <>
                  {" "}
                  <span className={tab.countTone === "warning" ? "tab-count tab-count-warning" : "tab-count"}>
                    · {tab.count}
                  </span>
                </>
              )}
            </button>
          );
        })}
      </div>
    </>
  );
  if (pinned === undefined) {
    return (
      <>
        {rail}
        {panel}
      </>
    );
  }
  // The wrapper holds the panel too, so the pin stays stuck the whole way
  // down the panel, and it is the sentinel's positioning box.
  return (
    <div className="node-pinned">
      <span className="node-pin-sentinel" ref={sentinelRef} aria-hidden="true" />
      <div className="node-pin">{rail}</div>
      {panel}
    </div>
  );
}
