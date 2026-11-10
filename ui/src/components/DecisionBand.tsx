// DecisionBand is the desk's burst form (home burst redesign, settled
// decisions 1-4): one kind's queue entries past ROW_CAP, every member
// visible as a row, the repeated chrome stated once in the head, and the
// batch command (where cli-surface.md has the verb) in the foot, phrased
// after-review and never as the headline. A row shows no command text; its
// Copy control copies that one entry's own command for a reviewer who has
// already decided.
import type { QueueEntry } from "../api";
import { has } from "../lib/has";
import { batchCommand, queueEntryKey, sharedWhy } from "../views/home/bands";
import type { QueueCardGroup } from "../views/home/queueGroups";
import CopyCmd, { CopyButton } from "./CopyCmd";
import { kindLabel, rowQualifier, rowTitle } from "./DecisionCard";
import RefLink from "./RefLink";

// tabbed: the band sits under a tab that already names its kind and count
// (Home's decision-kind rail, 2026-09-01), so the head keeps only the
// shared why-line, the one thing the tab did not say. The tab also carries the
// batch's Copy all, so a tabbed band has no foot repeating it.
export default function DecisionBand({ group, tabbed = false }: { group: QueueCardGroup; tabbed?: boolean }) {
  const why = sharedWhy(group.entries);
  const batch = batchCommand(group.kind, group.entries);
  return (
    <div className="dband" data-kind={group.kind}>
      {(!tabbed || why !== "") && (
        <div className="dband-head">
          {!tabbed && <span className="kind">{kindLabel(group.kind)}</span>}
          {!tabbed && <span className="dband-count">{group.entries.length}</span>}
          {why !== "" && <p className="why">{why}</p>}
        </div>
      )}
      {group.entries.map((entry) => (
        <BandRow key={queueEntryKey(entry)} entry={entry} shownWhy={why} />
      ))}
      {batch !== null && !tabbed && (
        <div className="dband-foot">
          <span className="muted">approve all {group.entries.length} after review</span>
          <CopyCmd cmd={batch} accent />
        </div>
      )}
    </div>
  );
}

function BandRow({ entry, shownWhy }: { entry: QueueEntry; shownWhy: string }) {
  const isQuestion = entry.noun === "question";
  const title = rowTitle(entry);
  const qualifier = rowQualifier(entry);
  // Two lines: the title, then its address and annotations beneath. The row
  // lives in a column a third of the page wide, and a chore title beside its
  // key did not fit on one line (JC, 2026-08-31).
  return (
    <div className="dband-row">
      <div className="dband-row-title">
        <RefLink node={entry} label={has(title) ? title : entry.ref} className="" />
        {has(entry.command) && (
          <CopyButton
            cmd={entry.command}
            label="Copy"
            ariaLabel={`Copy command for ${has(title) ? title : entry.ref}`}
            className="copy-btn dband-row-copy"
          />
        )}
      </div>
      <div className="dband-row-addr">
        {isQuestion ? (
          <span className="mono muted">{entry.id}</span>
        ) : (
          has(title) && has(entry.key) && <span className="mono muted">{entry.key}</span>
        )}
        {qualifier !== "" && <span className="mono muted"> · {qualifier}</span>}
        {shownWhy === "" && has(entry.due) && <span className="muted"> · due {entry.due}</span>}
      </div>
    </div>
  );
}
