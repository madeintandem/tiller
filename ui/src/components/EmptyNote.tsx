// EmptyNote is the "(none)" register the P1 streams introduced: a muted note
// where a list would be, so an empty section still shows it was consulted
// rather than forgotten.

import type { ReactNode } from "react";

export default function EmptyNote({ children }: { children?: ReactNode }) {
  return <p className="none">{children ?? "(none)"}</p>;
}
