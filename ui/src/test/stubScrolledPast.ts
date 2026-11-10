// stubScrolledPast makes useScrolledPast report the rail's sentinel as
// scrolled out (or not), the way a real IntersectionObserver would once the
// reading pane scrolls. It returns the restore function.
export function stubScrolledPast(scrolledPast: boolean): () => void {
  const original = globalThis.IntersectionObserver;
  globalThis.IntersectionObserver = class {
    constructor(private cb: IntersectionObserverCallback) {}
    observe(target: Element) {
      this.cb([{ isIntersecting: !scrolledPast, target } as IntersectionObserverEntry], this as unknown as IntersectionObserver);
    }
    disconnect() {}
    unobserve() {}
    takeRecords() {
      return [];
    }
  } as unknown as typeof IntersectionObserver;
  return () => {
    globalThis.IntersectionObserver = original;
  };
}
