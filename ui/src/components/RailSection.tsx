// RailSection is the desk rail's shared chrome (the mock's .sit/.shd): one
// collapsible panel with a letterspaced eyebrow and an optional right-aligned
// header extra, the chevron and the collapse state living here once rather
// than in the sections themselves.
//
// It has no caller today and that is deliberate. ui-surface.md's rail
// decision: the recap moved to its own view and the effort axis took the
// rail's remaining tenant with it, so the rail has no standing tenant and
// Home draws one full-width column instead, while "the collapsible chrome
// stays specified for the first future tenant." That is this file.
// Collapse persists in localStorage per
// section id (task C1's brief: "collapse state in localStorage, per-section
// chevron"), read synchronously on mount so a returning visit renders
// already collapsed rather than flashing open first. The header itself never
// collapses: only the body does, so the eyebrow, the chevron, and any header
// extra stay reachable while a section is closed.

import type { ReactNode } from "react";
import { useState } from "react";

const STORAGE_PREFIX = "tiller.home.rail.";

function readPersisted(id: string, fallback: boolean): boolean {
  try {
    const raw = window.localStorage.getItem(STORAGE_PREFIX + id);
    if (raw === null) {
      return fallback;
    }
    return raw !== "0";
  } catch {
    return fallback;
  }
}

function writePersisted(id: string, open: boolean) {
  try {
    window.localStorage.setItem(STORAGE_PREFIX + id, open ? "1" : "0");
  } catch {
    // A refused write (private mode, a full quota) still leaves the toggle
    // working for this session; only persistence across a reload is lost.
  }
}

export default function RailSection({
  id,
  title,
  headerExtra,
  defaultOpen = true,
  children,
}: {
  id: string;
  title: string;
  headerExtra?: ReactNode;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(() => readPersisted(id, defaultOpen));

  function toggle() {
    setOpen((prev) => {
      const next = !prev;
      writePersisted(id, next);
      return next;
    });
  }

  return (
    <section className="rail-section" data-rail-id={id}>
      <div className="rail-section-head">
        <button type="button" className="rail-section-toggle" onClick={toggle} aria-expanded={open}>
          <span className="chevron-glyph" aria-hidden="true">
            {open ? "▾" : "▸"}
          </span>
          <span className="eyebrow">{title}</span>
        </button>
        {headerExtra}
      </div>
      {open && <div className="rail-section-body">{children}</div>}
    </section>
  );
}
