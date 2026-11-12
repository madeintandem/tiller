// AmendmentHoldFlag marks a draft an open amendment holds until it is approved
// or withdrawn. It always reads "In amendment" in words; the tooltip names the
// amendment. A held draft is a proposal waiting on a decision, not blocked
// work, so it takes the neutral drift-flag register and never the blocked one
// (HeldChip and the .flag.blocked span mean a retired ancestor). It is a
// different fact from `held`, which is why the wire name is amendment_hold.

import type { AmendmentHoldView } from "../api";

export function amendmentHoldTitle(by: string): string {
  return `Held by amendment ${by}`;
}

export default function AmendmentHoldFlag({ hold, by }: { hold?: AmendmentHoldView; by?: string }) {
  const name = hold?.title ?? by;
  if (name === undefined || name === "") {
    return null;
  }
  return (
    <span className="flag hold" title={amendmentHoldTitle(name)}>
      In amendment
    </span>
  );
}
