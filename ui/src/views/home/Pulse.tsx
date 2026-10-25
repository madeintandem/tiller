// The desk's pulse: the Just happened, In flight, and Up next columns and
// their rows, moved out of Home.tsx whole (issue #96).

import { Link } from "react-router-dom";
import type { FetchState } from "../../hooks";
import type {
  BlockedItem,
  GatesItem,
  NextAnswer,
  NextCandidate,
  QuestionHoldItem,
  Recap,
  StreamItem,
  Streams,
} from "../../api";
import EmptyNote from "../../components/EmptyNote";
import { ProseInline } from "../../components/Prose";
import RefLink from "../../components/RefLink";
import SectionHead from "../../components/SectionHead";
import StateDot from "../../components/StateDot";
import { addrOf } from "../../lib/addr";
import { burstPhrase, burstWhen } from "../../lib/bursts";
import type { Burst } from "../../lib/bursts";
import { ROW_CAP } from "../../lib/disclosure";
import { has } from "../../lib/has";
import { sentence } from "../../lib/sentence";
import { PersonName, usePersonName } from "../../people";
import { composeJustHappened, formatClock, targetLabel } from "./justHappened";
import { nextLabel, nextReason } from "./nextRow";

// ---------- The pulse: just happened, in flight, up next ----------

// The console-wide row cap (lib/disclosure.ts's own idiom, "one number and
// one idiom everywhere"), reused for the pulse's bounded lists rather than
// separate hand-picked numbers: just happened's burst rows and in flight's
// blocked register. Up next needs none; the server already cuts it to five.
const PULSE_CAP = ROW_CAP;

export function JustHappenedColumn({ state }: { state: FetchState<Recap> }) {
  return (
    <div className="pcol">
      <SectionHead label="Just happened" headerExtra={<span className="muted mono pulse-window">last 24h</span>} />
      {state.error !== null && <p className="error">{state.error}</p>}
      {state.error === null && state.data === null && <p className="muted">Loading...</p>}
      {state.data !== null && <JustHappenedBody recap={state.data} />}
    </div>
  );
}

function JustHappenedBody({ recap }: { recap: Recap }) {
  const { bursts, moreCount } = composeJustHappened(recap, PULSE_CAP);
  if (bursts.length === 0) {
    return <EmptyNote>Nothing in the last 24 hours.</EmptyNote>;
  }
  return (
    <>
      {bursts.map((b, i) => (
        // Index keys are safe here: a burst has no id of its own, and the
        // list is rebuilt wholesale from a refetched payload.
        <JustHappenedRow key={i} burst={b} />
      ))}
      <p className="pfoot">
        {moreCount > 0 && <>{moreCount} more in this window · </>}
        <Link to="/history?window=24h">Full history →</Link>
      </p>
    </>
  );
}

// One collapsed-burst row, History.tsx's own grammar (burstPhrase/burstWhen,
// lib/bursts.ts) reshaped into the pulse's .prow markup: a clock time, what
// happened, who. No status dot: unlike the board streams below, a
// RecapEntry carries no status field to paint honestly, and inventing one
// from the event's verb would be exactly the narration ui-surface.md's
// "mechanical, never narrative" rule forbids.
//
// Round-1 review, Finding 1 (Critical): burstPhrase's own subject carries no
// numeral ("tasks done", not "4 tasks done") - History renders the count in
// its own separate .recap-burst-count column, which this row has no
// equivalent of, so a multi-entry burst here read as a bare noun phrase with
// the count silently dropped. n is now rendered explicitly for n > 1, the
// only case the count is not already implied (a lone entry names its own
// node instead of counting, and "1 task done" would be an odd thing to say
// about one node when the node's own name is right there).
function JustHappenedRow({ burst }: { burst: Burst }) {
  const n = burst.entries.length;
  const phrase = burstPhrase(burst.type, n);
  const lone = n === 1 ? burst.entries[0] : undefined;
  const subject = lone?.targets[0];
  return (
    <div className="prow">
      <span className="t">{burstWhen(burst.entries, formatClock)}</span>
      <span className="what">
        {subject !== undefined ? (
          <>
            <RefLink node={subject} label={targetLabel(subject)} className="" /> <b>{phrase.verb}</b>
          </>
        ) : n > 1 ? (
          <b>
            {n} {phrase.subject}
          </b>
        ) : (
          <b>{phrase.subject}</b>
        )}
      </span>
      <span className="who"><PersonName id={burst.contributor} /></span>
    </div>
  );
}

export function InFlightColumn({ state }: { state: FetchState<Streams> }) {
  return (
    <div className="pcol">
      <SectionHead label="In flight" />
      {state.error !== null && <p className="error">{state.error}</p>}
      {state.error === null && state.data === null && <p className="muted">Loading...</p>}
      {state.data !== null && <InFlightBody streams={state.data} />}
    </div>
  );
}

function InFlightBody({ streams }: { streams: Streams }) {
  // Historical contribution alone does not create a current assignment.
  const contributors = streams.contributors.filter(
    (o) => o.active.length + o.gates_in_review.length + o.blocked.length > 0,
  );
  const available = streams.unclaimed?.length ?? 0;
  if (contributors.length === 0 && available === 0) {
    return <EmptyNote>Nothing in flight.</EmptyNote>;
  }
  return (
    <>
      {/* The pool stays a single rollup; Up next ranks its claimable tasks
          among the reader's other moves. */}
      {available > 0 && (
        <p className="pool-line muted">
          <Link to="/build">
            {available} unclaimed {available === 1 ? "task" : "tasks"}
          </Link>
        </p>
      )}
      {contributors.map((o) => (
        <div className="contributor-group" key={o.contributor.id}>
          <div className="og-name"><PersonName id={o.contributor.id} name={o.contributor.name} /></div>
          {/* Render order is the brief's own: active, then gates in review,
              then the blocked register (task-10-brief.md). Settled decision
              6: only the blocked register caps, at ROW_CAP, with an "N more"
              foot carrying the full remaining count; active and in-review
              rows are motion and are never capped. */}
          {o.active.map((item) => (
            <ActiveRow key={`active:${item.noun}:${item.id}`} item={item} />
          ))}
          {o.gates_in_review.map((item) => (
            <GatesRow key={`gates:${item.noun}:${item.id}`} item={item} />
          ))}
          {o.blocked.slice(0, PULSE_CAP).map((item) => (
            <BlockedRow key={blockedRowKey(item)} item={item} />
          ))}
          {/* The foot always points at /build, even when a capped row is a
              shelf item, which lives on Fix instead: the
              remainder can mix all five work nouns, so no single page holds
              every row this foot stands for, and Build is where most of them
              are. */}
          {o.blocked.length > PULSE_CAP && (
            <p className="pfoot">
              {o.blocked.length - PULSE_CAP} more blocked · <Link to="/build">Build →</Link>
            </p>
          )}
        </div>
      ))}
    </>
  );
}

function ActiveRow({ item }: { item: StreamItem }) {
  return (
    <div className="prow">
      <StateDot status={item.status} />
      <NounChip noun={item.noun} />
      <span className="what">
        <b>
          <RefLink node={item} label={addrOf(item)} className="" />
        </b>{" "}
        · {item.display}
      </span>
    </div>
  );
}

// A shelf noun (bug, chore, tweak) self-labels with a small noun chip so it reads as
// such at a glance beside the epic/feature/task rows around it, which carry
// no chip at all.
function NounChip({ noun }: { noun: string }) {
  if (noun !== "bug" && noun !== "chore" && noun !== "tweak") return null;
  return <span className="noun-chip mono">{noun}</span>;
}

// GatesInReview holds tasks and shelf nodes alone, always at in_review by
// construction (a PR always represents a task or a shelf node, streams.go's
// own doc comment), so the status dot is a true fact rather than a guess.
function GatesRow({ item }: { item: GatesItem }) {
  return (
    <div className="prow">
      <StateDot status="in_review" />
      <NounChip noun={item.noun} />
      <span className="what">
        <b>
          <RefLink node={item} label={addrOf(item)} className="" />
        </b>{" "}
        · in review
      </span>
    </div>
  );
}

// Every blocked condition family paints the existing shared blocked register;
// the structured detail supplies the truthful explanation beside it.
function BlockedRow({ item }: { item: BlockedItem }) {
  return (
    <div className="prow">
      <StateDot status="blocked" />
      <NounChip noun={item.noun} />
      <span className="what">
        <b>
          <RefLink node={item} label={addrOf(item)} className="" />
        </b>{" "}
        · <BlockedWhy item={item} />
      </span>
    </div>
  );
}

function blockedRowKey(item: BlockedItem): string {
  const node = `blocked:${item.noun}:${item.id}`;
  if ((item.question_holds ?? []).length > 0) {
    return `${node}:question`;
  }
  if (item.held !== undefined) {
    return `${node}:held`;
  }
  return `${node}:manual`;
}

function questionHoldText(holds: QuestionHoldItem[]): string {
  const label = holds.length === 1 ? "question hold" : "question holds";
  return `${label}: ${holds.map((hold) => `${hold.title} (${hold.question})`).join(", ")}`;
}

function BlockedWhy({ item }: { item: BlockedItem }) {
  const questions = item.question_holds ?? [];
  if (questions.length > 0) {
    return <>{questionHoldText(questions)}</>;
  }
  if (item.held !== undefined) {
    return (
      <>
        held under {item.held.ref} ({item.held.state})
      </>
    );
  }
  const refs = item.depends_on_refs ?? [];
  if (refs.length === 0) {
    return <>blocked</>;
  }
  return (
    <>
      blocked on <span className="mono">{refs.join(", ")}</span>
    </>
  );
}

// Up next is the ranked next list for the served checkout's contributor
// (GET /api/next; ranked-next design, decisions 5 and 7), rendered as served:
// the server ranks and cuts to five, and the column never re-sorts, filters,
// or recomputes. The header names who it is ranked for, by roster display
// name the way In flight names people; with no identity the list is plan
// order and a muted line says so. It may overlap Needs you,
// which stays the complete inventory; there is no "show all" (decision 11).
export function UpNextColumn({ state }: { state: FetchState<NextAnswer> }) {
  const personName = usePersonName();
  const reader = state.data?.reader ?? "";
  return (
    <div className="pcol">
      <SectionHead label={reader === "" ? "Up next" : `Up next · for ${personName(reader)}`} />
      {state.error !== null && <p className="error">{state.error}</p>}
      {state.error === null && state.data === null && <p className="muted">Loading...</p>}
      {state.data !== null && <UpNextBody next={state.data} />}
    </div>
  );
}

function UpNextBody({ next }: { next: NextAnswer }) {
  return (
    <>
      {next.reader === "" && (
        <p className="pnote muted">
          <ProseInline markdown="Not personalized; run `tiller join`." />
        </p>
      )}
      {next.candidates.length === 0 && <EmptyNote>Caught up: nothing is legal to do right now.</EmptyNote>}
      {next.candidates.map((c, i) => (
        // A node never appears twice (decision 10), but a move such as
        // decompose names no node, so the index breaks the tie.
        <NextRow key={`${c.kind}:${c.node?.id ?? i}`} candidate={c} />
      ))}
      {has(next.hint) && (
        <p className="pnote hint muted">
          <ProseInline markdown={sentence(next.hint)} />
        </p>
      )}
    </>
  );
}

// One ranked move: its actor, then the action and a link to its node (a
// question by its title), then why it ranks here, worded as `tiller next`
// words it. The command stays off the row; Needs you and the CLI carry it.
function NextRow({ candidate }: { candidate: NextCandidate }) {
  const { node } = candidate;
  const reason = nextReason(candidate);
  return (
    <div className="prow">
      <span className="actor">{candidate.actor === "human" ? "You" : "Agent"}</span>
      <span className="what">
        {candidate.action}
        {node !== undefined && (
          <>
            {" "}
            <b>
              <RefLink node={node} label={nextLabel(node)} className="" />
            </b>
          </>
        )}
        {has(reason) && (
          <span className="why">
            <ProseInline markdown={reason} />
          </span>
        )}
      </span>
    </div>
  );
}
