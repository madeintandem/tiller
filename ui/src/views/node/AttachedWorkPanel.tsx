import type { RelatedNode } from "../../api";
import InstrSection from "../../components/InstrSection";
import RelatedList from "./RelatedList";

export default function AttachedWorkPanel({ items }: { items: RelatedNode[] }) {
  return (
    <InstrSection label="Fix" caption={`${items.length}`}>
      <RelatedList items={items} />
    </InstrSection>
  );
}
