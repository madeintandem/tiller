import type { NodeStart } from "../api";
import { PersonName } from "../people";
import CopyCmd from "./CopyCmd";

// StartBox is the attention stack's box for a ready task: how to begin it.
// The server sends the commands (views.StartOf, the same claim command Up Next
// prints); this only lays them out. An unclaimed task shows the claim command
// and the start command that follows it. A claimed one shows only the start,
// framed as the owner's next step so no other reader is told to run it.
export default function StartBox({ start, owner, ownerName }: { start: NodeStart; owner?: string; ownerName?: string }) {
  const claimFirst = start.then !== undefined && start.then !== "";
  return (
    <div className="abox warn">
      <span className="kind">Next · {start.actor === "human" ? "Human" : "Agent"}</span>
      <div className="why">
        {claimFirst ? (
          "Ready and unclaimed. Claim it, then start it."
        ) : owner !== undefined && owner !== "" ? (
          <>
            Claimed by <PersonName id={owner} name={ownerName} />. Their next step:
          </>
        ) : (
          "Claimed. Next step:"
        )}
      </div>
      <CopyCmd cmd={start.command} />
      {claimFirst && <CopyCmd cmd={start.then as string} />}
    </div>
  );
}
