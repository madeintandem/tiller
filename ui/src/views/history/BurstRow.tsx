import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import type { RecapEntry, TargetRef } from "../../api";
import type { Burst } from "../../lib/bursts";
import { leafKey } from "../../lib/bursts";
import { eventPhrase } from "../../lib/eventPhrase";
import { ROW_CAP } from "../../lib/disclosure";
import { has } from "../../lib/has";
import RefLink from "../../components/RefLink";
import Time from "../../components/Time";
import { PersonName } from "../../people";
import { historyPath, viewOf } from "./controls";

// One row per burst: the mock's `.hrow` anatomy (mono time, a dot, the bold
// "what", a trailing mono ref where the burst has a single leaf key to show,
// and the owner right-aligned), expanding in place for a run of more than
// one. The count is still always rendered, at every run length including
// one, because it is still the chart (see the top-of-file note): what varies
// at n=1 is that the SUBJECT names its own node instead of a bare count, and
// it has nothing to expand.
//
// The dot never paints a status: a RecapEntry carries none to paint honestly,
// and home/Pulse.tsx's JustHappenedRow already declined to invent one from the
// event's verb for the identical data. What it marks instead is ask, which
// the entry genuinely carries and which never goes stale the way a "current
// status" claim would - lit only when the burst holds one, same as the WORD
// beside it, so the fact is still never carried by color alone.
//
// `origin` is undefined in epic mode (the section head already says it) and
// always set in flat mode (FlatBody), where the row has no section head and
// must carry its epic/"project" label. The stream half is already the row's
// own `.who` below. Origin renders on every lone or multi row; unlike the
// trailing EntryAddresses slot, it is one fact per burst rather than one fact
// per expanded entry.
export default function BurstRow({
  burst,
  groupKey,
  origin,
}: {
  burst: Burst;
  groupKey: string;
  origin?: string;
}) {
  const [open, setOpen] = useState(false);
  const [params] = useSearchParams();
  const n = burst.entries.length;
  const phrase = eventPhrase(burst.type, n);
  const lone = n === 1 ? burst.entries[0] : undefined;
  const subject = lone?.targets[0];
  // A burst row shows the clock of its latest event (entries[0]); the day
  // lives in the day heading above it and the absolute time on hover.
  const when = burst.entries.length > 0 ? burst.entries[0].at : "";
  const hasAsk = burst.asks > 0;

  // The ask clause, written out in WORDS at every run length: a magenta dot
  // alone would carry the fact in color only, and ui-surface.md's own rule is
  // that a state is never color by itself. Its numeral is dropped once every
  // entry in the run asked (the row's own bold "what" already printed the
  // count once, to its left), and kept whenever only SOME did, because then
  // it says something the "what" phrase did not.
  // "asked you" is a link that filters the page to asks, keeping every other
  // parameter (historyPath composes them).
  const askTo = historyPath({ ...viewOf(params), kind: "asked" });
  const askClause = hasAsk && (
    <span className="recap-asks-count">
      <Link to={askTo} replace>
        {burst.asks === n ? "asked you" : `${burst.asks} asked you`}
      </Link>
    </span>
  );

  // The origin slot: reuses the same mono/muted ref treatment
  // .recap-burst-key already paints,
  // under its own class since it carries a different fact (which group, not
  // which leaf key) at a different cardinality (every row, not just a lone
  // one).
  const originSpan = origin !== undefined && (
    <span className="mono muted recap-burst-origin">{origin}</span>
  );

  // A run of one is already its own entry, so there is no third level to open
  // and no toggle to draw: it names its node instead of counting one.
  if (n === 1 && lone !== undefined) {
    return (
      <li className="recap-burst">
        <div className="hrow recap-burst-lone">
          <TimeCell iso={when} />
          {subject !== undefined ? (
            <>
              <span className={hasAsk ? "verb ask" : "verb"}>{phrase.verb}</span>
              <span className="what">
                <RefLink node={subject} label={targetLabel(subject)} className="recap-burst-node" />
                {askClause}
                <EntryAddresses entry={lone} groupKey={groupKey} className="recap-burst-key" />
                {originSpan}
              </span>
            </>
          ) : (
            <span className={hasAsk ? "what what-run ask" : "what what-run"}>
              <b>{phrase.subject}</b>
              {askClause}
              <EntryAddresses entry={lone} groupKey={groupKey} className="recap-burst-key" />
              {originSpan}
            </span>
          )}
          <span className="who"><PersonName id={burst.contributor} /></span>
        </div>
      </li>
    );
  }
  return (
    <li className="recap-burst">
      <details
        className="hrow-group"
        open={open}
        onToggle={(event) => {
          setOpen(event.currentTarget.open);
        }}
      >
        <summary className="hrow">
          <TimeCell iso={when} />
          <span className={hasAsk ? "what what-run ask" : "what what-run"}>
            <b>
              {n} {phrase.subject}
            </b>
            {askClause}
            {originSpan}
          </span>
          <span className="who"><PersonName id={burst.contributor} /></span>
          {/* The mock's own wording (mocks.html:1149), in place of the
              chevron the row used to draw: a run of more than one is the
              only row anatomy with something to open, so it is the only one
              that earns a control here. aria-hidden the same way the
              chevron it replaced was: a native <details> already announces
              its own expanded/collapsed state to assistive tech, so this
              text is a sighted affordance layered on top of semantics the
              element already carries, not new accessible content of its
              own. */}
          <span className="burst-toggle" aria-hidden="true">
            {open ? "– close" : "+ open"}
          </span>
        </summary>
        <div className="burst-body">
          <BurstEntries burst={burst} groupKey={groupKey} />
        </div>
      </details>
    </li>
  );
}

// The third and last level of disclosure, capped with the same "N more" the
// desk's cards use. No commands here: "every command copies" governs the
// commands a surface shows, and the recap shows none, because the actionable
// command is the queue's job and re-importing it is what this move removed.
function BurstEntries({ burst, groupKey }: { burst: Burst; groupKey: string }) {
  const [expanded, setExpanded] = useState(false);
  const shown = expanded ? burst.entries : capPreservingAsks(burst.entries, ROW_CAP);
  const hidden = burst.entries.length - shown.length;
  // The BORDER mark rides individual rows only where the burst is MIXED. In a
  // burst that is all asks the head already said it for every row, and
  // painting the same loud border 32 times is the repetition this pass
  // exists to kill.
  const markAsks = burst.asks > 0 && burst.asks < burst.entries.length;

  return (
    <>
      <ul className="recap-entries">
        {shown.map((e, i) => (
          <li key={i} className={markAsks && e.ask ? "recap-entry recap-entry-ask" : "recap-entry"}>
            {/* The DOT is not gated by markAsks the way the border above it
                is: it is a 6px circle, not a 2px colored border with its own
                negative margin, so 32 identical accent dots do not carry the
                same "same loud mark 32 times" problem the border-gating rule
                exists to avoid. It just states e.ask's own truth on every
                row - accent on every row of an all-ask burst (the head's own
                dot already promised that), muted on every row of a burst
                with none, and the one signal that actually differs
                row-to-row inside a mixed burst. Fixed 2026-08-11 (task-19
                review, finding 2): gating it on markAsks too painted a MUTED
                dot under an ACCENT head on an all-ask burst, the exact
                inversion the dot exists to prevent. */}
            <span className={e.ask ? "dot ask" : "dot"} aria-hidden="true" />
            {e.targets.map((t, j) => (
              <RefLink key={j} node={t} label={targetLabel(t)} className="recap-entry-title" />
            ))}
            <EntryAddresses entry={e} groupKey={groupKey} />
          </li>
        ))}
      </ul>
      {hidden > 0 && (
        <button type="button" className="recap-more" onClick={() => setExpanded(true)}>
          {hidden} more {hidden === 1 ? "row" : "rows"} ▸
        </button>
      )}
    </>
  );
}

// The client's own "N more" cap has to honor the same rule the server's own
// per-group cap already does (api.ts's RecapGroup.omitted doc: "asks are
// never capped, so an omitted count is always routine activity") - an entry
// that asked something is never among what a cap hides. A plain
// slice(0, cap) breaks that the moment an ask sits past position `cap` in the
// burst's own newest-first order: it rides behind "N more" until a click,
// which is exactly the drop the rule forbids, one level down from where it
// was already honest. Every ask ships in the initial view; the remaining cap
// slots, if any, fill with the newest routine entries, and the whole result
// re-sorts back to the burst's own newest-first order so nothing displaced
// out of position reads as if it happened later than it did.
function capPreservingAsks(entries: RecapEntry[], cap: number): RecapEntry[] {
  if (entries.length <= cap) {
    return entries;
  }
  const asks = entries.filter((e) => e.ask);
  const routineSlots = Math.max(cap - asks.length, 0);
  const shown = new Set(entries.filter((e) => !e.ask).slice(0, routineSlots));
  for (const e of asks) {
    shown.add(e);
  }
  return entries.filter((e) => shown.has(e));
}

// The trailing address slot: a keyed target contributes its leaf key, while a
// materialized question contributes its Q-id. Other keyless targets contribute
// nothing, and a dangling target already renders its bare ref as the leading
// label, so it is never duplicated here.
function EntryAddresses({
  entry,
  groupKey,
  className = "recap-entry-key",
}: {
  entry: RecapEntry;
  groupKey: string;
  className?: string;
}) {
  const addresses = entry.targets
    .map((target) =>
      has(target.key)
        ? leafKey(target.key, groupKey)
        : target.noun === "question" && has(target.title)
          ? target.id
          : "",
    )
    .filter((address) => address !== "");
  if (addresses.length === 0) {
    return null;
  }
  return <span className={`mono muted ${className}`}>{addresses.join(" · ")}</span>;
}

// A row leads with its title or key. A materialized question's Q-id follows in
// EntryAddresses; keyed nouns likewise trail their leaf key. Only a dangling
// id, which has neither title nor key, falls back to its ref as its sole label.
function targetLabel(t: TargetRef): string {
  if (has(t.title)) {
    return t.title;
  }
  return has(t.key) ? t.key : t.ref;
}

// The row's leading clock cell: the time of day only, the absolute time on
// hover (Time carries the title). A row never repeats its day.
export function TimeCell({ iso }: { iso: string }) {
  return (
    <span className="t">
      <Time iso={iso} mode="clock" />
    </span>
  );
}
