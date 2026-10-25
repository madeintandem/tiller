// RefLink renders a node's precomputed human reference and links it to its
// node page. Links always address by id: a choice, not a route constraint,
// since the /node/:noun/* splat route resolves a key just as well; ids stay
// the target because they are stable under a rename, and the API resolves
// either. A dangling edge (the API degrades one to a bare-id row with an
// empty noun) renders as plain text, because a link to nowhere is worse than
// no link.
//
// label overrides the text without touching the addressing, for the surfaces
// that take ui-surface.md's console-scoped carve-out further and keep the id
// off a rendered row (the recap's one-line rows pass the node's key). The
// link and the guard stay identical, so the registers cannot drift apart.
//
// className swaps the visual register for the rows that lead with a TITLE
// rather than a reference (the relationship hub's related rows), where mono is
// the wrong voice because the text is prose, not a key. Those rows come
// through here anyway rather than building their own Link precisely so the
// dangling-edge guard has one implementation.

import { Link } from "react-router-dom";

// The addressing triple every payload row shares (a RelatedNode, a TargetRef,
// a QueueEntry, a StreamItem, a search hit's node): structural typing lets any
// of them pass straight through as the node prop. The payload's ref rides
// inside the object because React reserves `ref` as a prop name.
export interface RefTarget {
  noun: string;
  id: string;
  ref: string;
}

export default function RefLink({
  node,
  label,
  className = "mono",
}: {
  node: RefTarget;
  label?: string;
  className?: string;
}) {
  const text = label !== undefined && label !== "" ? label : node.ref;
  if (node.noun === "" || node.id === "") {
    return <span className={className}>{text}</span>;
  }
  return (
    <Link to={`/node/${node.noun}/${node.id}`} className={className}>
      {text}
    </Link>
  );
}
