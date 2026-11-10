import { useState } from "react";
import type { RecapGroup } from "../../api";
import { burstsOf } from "../../lib/bursts";
import { has } from "../../lib/has";
import RefLink from "../../components/RefLink";
import { PersonName } from "../../people";
import { TieredBursts } from "./BookkeepingRow";
import BurstRow from "./BurstRow";
import { burstAt, dayGroupsOf } from "./days";

// A group is one (epic, stream) cell of the digest. Epic is null for the
// project-level groups (a question, a dangling id), which the API sorts
// last; those render under a plain "project" label rather than a link.
// The head leads with the epic's TITLE and lets the key follow as muted mono
// (ui-surface.md's console-scoped carve-out), carries the cell's TRUE count
// (what was served plus what the API's cap omitted), and starts open.
export default function RecapGroupBlock({
  group,
  showStream,
  showAll,
}: {
  group: RecapGroup;
  showStream: boolean;
  showAll: boolean;
}) {
  const [open, setOpen] = useState(true);
  const total = group.entries.length + group.omitted;
  const groupKey = group.epic === null ? "" : group.epic.key;
  const bursts = burstsOf(group.entries);
  const days = dayGroupsOf(bursts, burstAt);
  // days is empty only for a group with no entries, which renders no rows.

  return (
    // open is bound, not just defaulted: the group starts open, so React would
    // otherwise have to be trusted never to re-assert a static `open` prop on
    // a re-render, which would slam a closed group back open.
    <details
      className="recap-group"
      open={open}
      onToggle={(event) => {
        setOpen(event.currentTarget.open);
      }}
    >
      <summary className="recap-group-head">
        {group.epic !== null ? (
          <>
            <RefLink
              node={{ noun: "epic", id: group.epic.id, ref: group.epic.ref }}
              label={has(group.epic.title) ? group.epic.title : group.epic.key}
              className="recap-group-title"
            />
            <span className="mono muted recap-group-key">{group.epic.key}</span>
          </>
        ) : (
          <span className="recap-group-title muted">project</span>
        )}
        {showStream && group.stream !== "" && (
          <span className="muted recap-group-stream">· <PersonName id={group.stream} /></span>
        )}
        {/* The rule-line: the same trailing hairline the day subheads below
            draw (mock's .day-h::after), so a section head reads as part of
            the same eyebrow family rather than a leftover pre-restyle
            treatment. Decorative only - the count beside it already carries
            the number a reader scans for. */}
        <span className="head-rule" aria-hidden="true" />
        <span className="recap-group-count muted">
          {total} {total === 1 ? "event" : "events"}
        </span>
        <span className="chevron-glyph" aria-hidden="true">
          {open ? "▾" : "▸"}
        </span>
      </summary>
      {/* Every day gets its heading, a one-day group included: rows carry the
          clock only, so the date has to live somewhere. Index keys are safe
          here: a burst has no id of its own, the list is rebuilt wholesale
          from a refetched payload, and its order never changes in place. */}
      {days.map((d) => (
        <div className="day" key={d.key}>
          <div className="day-h">
            <span className="eyebrow">{d.label}</span>
          </div>
          <ul className="recap-bursts">
            <TieredBursts
              bursts={d.bursts}
              showAll={showAll}
              renderBurst={(b, i) => <BurstRow key={i} burst={b} groupKey={groupKey} />}
            />
          </ul>
        </div>
      ))}
      {group.omitted > 0 && (
        <p className="recap-omitted muted">{group.omitted} older not carried in this digest</p>
      )}
    </details>
  );
}
