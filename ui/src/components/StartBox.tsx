import type { NodeStart } from "../api";
import CopyCmd from "./CopyCmd";

// StartBox is the attention stack's box for a ready task: how to begin it.
// The server sends the commands (views.StartOf, the same claim command Up Next
// prints); this only lays them out. An unclaimed task shows the claim command
// and the start command that follows it, a claimed one only the start.
export default function StartBox({ start }: { start: NodeStart }) {
  const claimFirst = start.then !== undefined && start.then !== "";
  return (
    <div className="abox warn">
      <span className="kind">Next · {start.actor === "human" ? "Human" : "Agent"}</span>
      <div className="why">{claimFirst ? "Ready and unclaimed. Claim it, then start it." : "Ready. Start it."}</div>
      <CopyCmd cmd={start.command} />
      {claimFirst && <CopyCmd cmd={start.then as string} />}
    </div>
  );
}
