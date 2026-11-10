// TitleRef is a row's node reference in the console's one order: the title
// leads as the link, and the key follows in mono. A node with no title shows
// its key once, as the link, with no key span after it, so an untitled row
// never repeats its key or leaves a dangling separator.

import RefLink from "./RefLink";
import type { RefTarget } from "./RefLink";
import "./titleRef.css";

export default function TitleRef({ node, title, addr }: { node: RefTarget; title?: string; addr: string }) {
  if (title === undefined || title === "") {
    return <RefLink node={node} label={addr} />;
  }
  return (
    <>
      <RefLink node={node} label={title} className="" />
      <span className="mono title-ref-key">{addr}</span>
    </>
  );
}
