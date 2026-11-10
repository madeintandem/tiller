// useScrolledPast reports whether an element has left its scroll container's
// viewport: false while any part of it shows, true once it has scrolled out.
// The node page uses it to swap the full header for a one-line pinned one.
// Where IntersectionObserver is undefined (jsdom, very old browsers) it stays
// false, so the full header is the only header and nothing is lost.

import { useEffect, useState, type RefObject } from "react";

export function useScrolledPast(ref: RefObject<Element | null>): boolean {
  const [past, setPast] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (el === null || typeof IntersectionObserver === "undefined") {
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      const last = entries[entries.length - 1];
      if (last !== undefined) {
        setPast(!last.isIntersecting);
      }
    });
    observer.observe(el);
    return () => {
      observer.disconnect();
    };
  }, [ref]);
  return past;
}
