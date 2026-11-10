// FeatureCard is one Build board card and its expanded task rows, split out
// of Build.tsx (which keeps the page, the epic panel, and the columns).

import { Link, useNavigate } from "react-router-dom";
import type { BuildEpic, BuildFeature, RelatedNode } from "../../api";
import Contributors from "../../components/Contributors";
import StateBar from "../../components/StateBar";
import StateGlyph from "../../components/StateGlyph";
import { taskMix, taskMixCaption } from "./mix";

// Card body navigates to the feature's own node page; a task row inside the
// expand navigates to the task's; the expand chevron is its own click
// target and stops propagation so it never also fires the card's navigate
// (mock behavior - flagged as a small-target risk, accepted for now, the
// same call the brief itself already made). The outer element is a plain
// div rather than an anchor: a task row inside the expanded list is its own
// link, and an anchor cannot legally nest another one, so the whole-card
// click is a programmatic navigate instead, with role="link" and a keydown
// handler standing in for the anchor semantics an anchor would have given
// for free.
export default function FeatureCard({
  epic,
  showEpic,
  feature,
  askLabel,
  isDrift,
  expanded,
  onToggleExpand,
}: {
  epic: BuildEpic;
  // Whether this card opens a run of its epic in its column (showEpicEyebrow).
  showEpic: boolean;
  feature: BuildFeature;
  askLabel: string | undefined;
  isDrift: boolean;
  expanded: boolean;
  onToggleExpand: () => void;
}) {
  const navigate = useNavigate();
  const hasTasks = feature.tasks.length > 0;
  const caption = taskMixCaption(feature.tasks);

  function open() {
    navigate(`/node/feature/${feature.id}`);
  }

  const classes = ["fcard"];
  // An expandable card draws the absolutely positioned caret at its top
  // right; the class lets build.css reserve that width on the title and the
  // epic eyebrow so neither runs under it.
  if (hasTasks) {
    classes.push("expandable");
  }
  if (askLabel !== undefined) {
    classes.push("needs-you");
  }
  if (expanded) {
    classes.push("open");
  }

  return (
    <div
      className={classes.join(" ")}
      role="link"
      tabIndex={0}
      onClick={open}
      onKeyDown={(e) => {
        // A keydown bubbles up from whatever is actually focused inside the
        // card - the expand chevron, a task-row Link - before it ever
        // reaches this div, since neither of those stops keyboard events the
        // way their own onClick stops a mouse one. Space is already safe
        // (button activation fires on keyup, past this handler entirely),
        // but Enter on the chevron would otherwise navigate the card away
        // instead of expanding it, and Enter on a focused task row would
        // race the row's own Link navigation against this one. Acting only
        // when the event's own target IS the card (e.currentTarget) is the
        // same "was this key meant for me" check the click handler gets for
        // free from stopPropagation on those same two children.
        if (e.target !== e.currentTarget) {
          return;
        }
        if (e.key === "Enter") {
          open();
        }
      }}
    >
      {showEpic && <span className="f-epic">{epic.title ?? epic.key ?? epic.id}</span>}
      <div className="f-title">{feature.title ?? feature.ref}</div>
      {hasTasks && (
        <button
          type="button"
          className="f-expand"
          data-expand
          aria-expanded={expanded}
          aria-label={expanded ? "Collapse tasks" : "Expand tasks"}
          onClick={(e) => {
            e.stopPropagation();
            onToggleExpand();
          }}
        >
          ⌄
        </button>
      )}
      <div className="f-progress">
        {hasTasks ? (
          <>
            {/* The bar's accessible name is the caption, so the visible copy
                of the same words is hidden from assistive tech rather than
                read twice. */}
            <StateBar mix={taskMix(feature.tasks)} caption={caption} />
            <span className="taskcount" aria-hidden="true">
              {caption}
            </span>
          </>
        ) : (
          <span className="taskcount">no tasks yet</span>
        )}
      </div>
      <div className="f-meta">
        {feature.contributors.length > 0 && (
          <span className="contributors-chip">
            Contributors: <Contributors people={feature.contributors} />
          </span>
        )}
        <BugCountLink feature={feature} />
        {/* blocked (the manual flag) and held (a retired ancestor) are
            distinct conditions that must never fold into one:
            two independent sibling spans, either or both present, rather
            than one flag trying to say two things. */}
        {feature.blocked === true && <span className="flag blocked">blocked</span>}
        {feature.held !== undefined && (
          <span className="flag blocked" title={`held under ${feature.held.ref} (${feature.held.state})`}>
            held
          </span>
        )}
        {isDrift && <span className="flag drift">drift</span>}
        {askLabel !== undefined && <span className="flag ask">{askLabel}</span>}
      </div>
      {hasTasks && expanded && (
        <div className="tasklist">
          {feature.tasks.map((task) => (
            <CardTaskRow key={task.id} task={task} />
          ))}
        </div>
      )}
    </div>
  );
}

function BugCountLink({ feature }: { feature: BuildFeature }) {
  if (feature.open_work_items <= 0) return null;
  const label = `${String(feature.open_work_items)} open fixes attached`;
  return (
    <Link
      to={`/fix?attached=${encodeURIComponent(feature.id)}&terminal=all`}
      className="feature-work-items"
      aria-label={label}
      title={label}
      onClick={(e) => {
        e.stopPropagation();
      }}
    >
      <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
        <path d="M5 5.5h6v6H5zM6 3.5h4M3 7h2m6 0h2M3 10h2m6 0h2M6 12v2m4-2v2" />
      </svg>
      <span>{feature.open_work_items}</span>
    </Link>
  );
}

function CardTaskRow({ task }: { task: RelatedNode }) {
  const label = task.title !== undefined && task.title !== "" ? task.title : task.ref;
  // The status leads as a shape glyph whose accessible name and hover tooltip
  // are the status in plain words, so the title keeps the row's full width
  // and no trailing chip crowds it. A cancelled task has its own glyph shape
  // (StateGlyph) and its row steps back, as the Explore navigator's settled
  // rows do.
  const className = task.status === "cancelled" ? "trow is-cancelled" : "trow";
  const body = (
    <>
      {task.status !== "" && <StateGlyph status={task.status} size={12} />}
      <span className="trow-title">{label}</span>
    </>
  );
  // A dangling edge (RelatedNode's own doc comment: an id the tree no
  // longer knows degrades to an empty noun/id) renders as plain text, the
  // same guard RefLink applies everywhere else on the console.
  if (task.noun === "" || task.id === "") {
    return <div className={className}>{body}</div>;
  }
  return (
    <Link
      to={`/node/${task.noun}/${task.id}`}
      className={className}
      onClick={(e) => {
        e.stopPropagation();
      }}
    >
      {body}
    </Link>
  );
}
