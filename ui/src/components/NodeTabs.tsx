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

import type { ReactElement } from "react";
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
}: {
  tabs: NodeTab[];
  active: string;
  onSelect: (key: string) => void;
  // The rail's accessible name. The node page keeps the default; Home's
  // needs-you section uses the same rail for its decision kinds (2026-09-01).
  label?: string;
}) {
  const activeTab = tabs.find((tab) => tab.key === active) ?? tabs[0];
  return (
    <>
      <nav className="mode-nav" aria-label={label}>
        {tabs.map((tab) => {
          const isActive = tab.key === activeTab.key;
          return (
            <button
              key={tab.key}
              type="button"
              className={isActive ? "active" : undefined}
              aria-current={isActive ? "true" : undefined}
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
      </nav>
      {activeTab.render()}
    </>
  );
}
