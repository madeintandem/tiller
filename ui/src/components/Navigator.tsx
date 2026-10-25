// Navigator is Explore's left-hand tree pane (task C2): the
// epics-features-tasks tree from /api/tree, which arrives in dependency order
// (A1) and is rendered exactly as served - never re-sorted client-side,
// alphabetically or otherwise. Each section is its own fetch, passed down as
// FetchState, so a slow or failed resource never blanks another section.
//
// The tree runs three tiers because the work tree does: ui-surface.md's browse
// mode is "the work tree, epics to features to tasks", and a task is a folder
// node with a key and a page of its own, so leaving it out would make the
// navigator the one surface that cannot reach a third of the corpus. A feature
// carries its tasks the same way an epic carries its features, chevron and all,
// and a feature with none draws no chevron rather than an empty disclosure -
// it still reserves the chevron's own width with a spacer, so every row's
// title lines up on the same left edge whether or not it can expand (the
// mock's own alignment, an empty .tw span on a leaf row).
//
// Interaction grammar (Global Constraint): the title navigates (a real
// react-router Link, so the selection is a route and deep links work), the
// chevron expands in place. An epic's own StateGlyph, title, feature count,
// and child-mix StateBar are always visible, collapsed or not - only the
// feature rows themselves hide - which is why this is plain divs rather than
// a native <details>/<summary> (details hides everything but the summary
// when closed, and the bar has to survive collapse). A row's own openness
// reads from the `expansion` map Explore.tsx owns and passes down
// (views/explore/expansion.ts's `isOpen`); the chevron writes through the
// `onToggleBranch` callback rather than holding any state of its own. That
// map survives selection navigation because Explore.tsx's state does: the
// layout route renders this navigator at one stable element position across
// /explore, /questions, and every /node/:noun/* child, so React preserves
// Explore's state across a selection (pinned by explore.test.tsx's
// expansion-survival tests).
// The navigator never shows a bare id, and never falls back to one where a key
// is absent: a key appears as the row's hover title attribute only, the
// full reference living in the reading pane instead. A task row's hover title
// is its LOCAL key segment, the folder name, since the two segments above it
// are the rows this one already sits under.
//
// Row links address /node/:noun/:id, the console's one canonical node URL
// (JC, 2026-08-05): every other link site already emits it, and the node
// route renders inside the same layout as this navigator, so a row click
// swaps only the reading pane.
//
// Restyle (Phase 4, task 16, docs/plans/2026-08-09-console-v2-mocks.html's
// EXPLORE navigator): chevrons sit left of the dot (not trailing), indent
// guide lines run down each nested tier via .explore-feats/.explore-tasks's
// own border-left, rows are denser, and both an epic's and a feature's child
// count render in the mock's mono .tc register. None of this touches the
// behavior documented above - same two click targets, same held-open state,
// same served order - only how a row paints.

import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import type { ListResult, ProjectSummary, TreeModel, TreeNode } from "../api";
import type { FetchState } from "../hooks";
import { composeStat } from "../views/home/stat";
import type { Expansion } from "../views/explore/expansion";
import { isOpen } from "../views/explore/expansion";
import { readHideCancelled, visibleRows, writeHideCancelled } from "../views/explore/hideCancelled";
import { shelfBranchID } from "../views/explore/shelves";
import EmptyNote from "./EmptyNote";
import StateBar from "./StateBar";
import StateGlyph from "./StateGlyph";

// The current node's address as the URL carries it: an id on every rendered
// console link, but a canonical key when a human pastes one off the terminal
// (the /node/:noun/* splat takes either, and the API resolves both). The
// navigator matches rows against whichever form arrived, so a key-addressed
// page still lights its row.
export interface NavigatorSelection {
  noun: string;
  addr: string;
}

export default function Navigator({
  tree,
  questions,
  bugs,
  chores,
  tweaks,
  summary,
  selected,
  expansion,
  onToggleBranch,
  onOpenAll,
  onCloseAll,
}: {
  tree: FetchState<TreeModel>;
  questions: FetchState<ListResult>;
  bugs: FetchState<ListResult>;
  chores: FetchState<ListResult>;
  tweaks: FetchState<ListResult>;
  summary: FetchState<ProjectSummary>;
  selected: NavigatorSelection | null;
  // The lifted expansion map (views/explore/expansion.ts) and the one write
  // path into it a chevron ever needs: Explore.tsx owns the state, this
  // component and its rows only read and request changes.
  expansion: Expansion;
  onToggleBranch: (id: string, open: boolean) => void;
  // Task 2's whole-tree commands, plain writes into the same lifted map
  // (Explore.tsx computes the id list and the empty map, this component only
  // wires the two clicks to them - presentational, same as onToggleBranch).
  onOpenAll: () => void;
  onCloseAll: () => void;
}) {
  // The toggle is a reading preference this pane owns outright: Explore's
  // layout keeps the navigator mounted across selections, and storage
  // carries it across a reload.
  const [hideCancelled, setHideCancelled] = useState(readHideCancelled);
  function toggleHideCancelled() {
    writeHideCancelled(!hideCancelled);
    setHideCancelled(!hideCancelled);
  }
  return (
    <nav className="explore-nav" aria-label="Definition navigator">
      {/* Search and tree controls stay outside the tree's scroll area. */}
      <div className="explore-nav-head">
        <NavSearch />
        <TreeActions
          onOpenAll={onOpenAll}
          onCloseAll={onCloseAll}
          hideCancelled={hideCancelled}
          onToggleHideCancelled={toggleHideCancelled}
        />
        <div className="explore-nav-status" role="status" aria-label="Navigator refresh">
          <ResourceStatus label="Tree" state={tree} />
          <ResourceStatus label="Questions" state={questions} />
          <ResourceStatus label="Bugs" state={bugs} />
          <ResourceStatus label="Chores" state={chores} />
          <ResourceStatus label="Tweaks" state={tweaks} />
          <ResourceStatus label="Project summary" state={summary} />
        </div>
      </div>
      <div className="explore-nav-tree" aria-busy={tree.loading || bugs.loading || chores.loading || tweaks.loading}>
        <DefinitionBody
          tree={tree}
          selected={selected}
          expansion={expansion}
          onToggleBranch={onToggleBranch}
          hideCancelled={hideCancelled}
        />
        {/* Shelf work reads apart from the definition tree above it. */}
        <div className="explore-shelves">
          <ShelfBranch
            noun="bug"
            label="Bugs"
            state={bugs}
            selected={selected}
            expansion={expansion}
            onToggleBranch={onToggleBranch}
            hideCancelled={hideCancelled}
          />
          <ShelfBranch
            noun="chore"
            label="Chores"
            state={chores}
            selected={selected}
            expansion={expansion}
            onToggleBranch={onToggleBranch}
            hideCancelled={hideCancelled}
          />
          <ShelfBranch
            noun="tweak"
            label="Tweaks"
            state={tweaks}
            selected={selected}
            expansion={expansion}
            onToggleBranch={onToggleBranch}
            hideCancelled={hideCancelled}
          />
        </div>
      </div>
    </nav>
  );
}

function ResourceStatus({ label, state }: { label: string; state: FetchState<unknown> }) {
  if (state.error !== null) {
    return (
      <span className="error" title={state.error}>
        {label}: {state.data === null ? "load failed" : "refresh failed"}
        {state.loading && " (retrying)"}
      </span>
    );
  }
  if (state.loading) {
    return <span>{label}: {state.data === null ? "loading" : "refreshing"}</span>;
  }
  return null;
}

// TreeActions puts the whole tree under one click either way (task 2,
// docs/plans/2026-08-14-explore-tree-state.md): "Open all" and "Close all",
// deliberately worded apart from a chevron's own load-bearing
// "Expand"/"Collapse" aria-label (Chevron, below) so none of the existing
// name-pattern queries for those can ever match these two. "Hide cancelled"
// (JC, 2026-10-18) keeps one label and reports its state through
// aria-pressed, the standard toggle-button pattern, rather than flipping
// between two labels that could each read as the current state.
function TreeActions({
  onOpenAll,
  onCloseAll,
  hideCancelled,
  onToggleHideCancelled,
}: {
  onOpenAll: () => void;
  onCloseAll: () => void;
  hideCancelled: boolean;
  onToggleHideCancelled: () => void;
}) {
  return (
    <div className="explore-nav-actions">
      <button type="button" onClick={onOpenAll}>
        Open all
      </button>
      <button type="button" onClick={onCloseAll}>
        Close all
      </button>
      <button type="button" aria-pressed={hideCancelled} onClick={onToggleHideCancelled}>
        Hide cancelled
      </button>
    </div>
  );
}

// NavSearch is the unified bar's one persistent control (plan.md Phase 4
// item 3, "search and filter presentation merges into one persistent bar
// over the navigator"): a single input pinned above the tree, matching the
// mock's .nav-search pixel-for-pixel. It re-dresses rather than replaces the
// existing three-mode split (Explore.tsx's mode=browse|filter|search,
// unchanged): submitting a keyword here drives the SAME transition the
// mode-nav's own Search entry already does (?mode=search&q=...), just from a
// second, always-visible entry point rather than only the mode switcher -
// zero new endpoint, zero new parsing. Filter's own axis form (noun, status,
// owner, feature, epic - four independent fields, not one string) has no
// honest single-input equivalent short of inventing a free-text filter
// grammar, which is rewriting mode logic rather than re-dressing it; it
// stays reachable through the unchanged mode-nav entry. See task-16-report.md
// for the fuller account of that tension.
function NavSearch() {
  const navigate = useNavigate();
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = new FormData(event.currentTarget).get("q");
    if (typeof value !== "string" || value.trim() === "") {
      return;
    }
    const next = new URLSearchParams();
    next.set("mode", "search");
    next.set("q", value.trim());
    navigate(`/explore?${next.toString()}`);
  }
  return (
    <form onSubmit={submit}>
      <input
        className="nav-search"
        type="text"
        name="q"
        placeholder="Search or filter…"
        aria-label="Search"
      />
    </form>
  );
}


// DefinitionBody surfaces a failed tree fetch as an honest error - never as
// an empty navigator, which would misread as "this project has no epics."
function DefinitionBody({
  tree,
  selected,
  expansion,
  onToggleBranch,
  hideCancelled,
}: {
  tree: FetchState<TreeModel>;
  selected: NavigatorSelection | null;
  expansion: Expansion;
  onToggleBranch: (id: string, open: boolean) => void;
  hideCancelled: boolean;
}) {
  if (tree.data === null && tree.error !== null) {
    return <p className="error">{tree.error}</p>;
  }
  if (tree.data === null) {
    return <p className="muted">Loading tree...</p>;
  }
  if (tree.data.nodes.length === 0) {
    return <EmptyNote>No epics yet.</EmptyNote>;
  }
  return (
    <>
      {visibleRows(tree.data.nodes, hideCancelled).map((epic) => (
        <EpicRow
          key={epic.id}
          epic={epic}
          selected={selected}
          expansion={expansion}
          onToggleBranch={onToggleBranch}
          hideCancelled={hideCancelled}
        />
      ))}
    </>
  );
}

function ShelfBranch({
  noun,
  label,
  state,
  selected,
  expansion,
  onToggleBranch,
  hideCancelled,
}: {
  noun: "bug" | "chore" | "tweak";
  label: "Bugs" | "Chores" | "Tweaks";
  state: FetchState<ListResult>;
  selected: NavigatorSelection | null;
  expansion: Expansion;
  onToggleBranch: (id: string, open: boolean) => void;
  hideCancelled: boolean;
}) {
  const branchID = shelfBranchID[noun];
  const allRows = state.data?.nodes ?? [];
  const rows = visibleRows(allRows, hideCancelled);
  // The served count less whatever the toggle hid, so the number always
  // matches the rows a reader can open.
  const count = (state.data?.count ?? 0) - (allRows.length - rows.length);
  const open = isOpen(expansion, branchID);
  return (
    <section className="explore-shelf" aria-label={label} aria-busy={state.loading}>
      <div className="explore-shelf-row">
        {rows.length > 0 ? (
          <Chevron
            open={open}
            label={label}
            onToggle={() => onToggleBranch(branchID, !open)}
          />
        ) : (
          <span className="explore-chevron-spacer" />
        )}
        <Link to={`/fix?noun=${noun}`}>{label}</Link>
        <span className="tc mono">{count}</span>
      </div>
      {state.data === null && state.error !== null && <p className="error">{state.error}</p>}
      {state.data === null && state.error === null && (
        <p className="muted">Loading {label.toLowerCase()}...</p>
      )}
      {/* A shelf row's glyph sits inside its link, so the key's hover title
          rides the title text rather than the link: hovering the glyph shows
          only the glyph's own status tooltip, never the key's on top of it. */}
      {open &&
        rows.map((row) => (
          <Link
            key={row.id}
            to={`/node/${noun}/${row.id}`}
            className={rowClass("explore-shelf-node", row.status, isSelected(selected, noun, row) && "current")}
          >
            <StateGlyph status={row.status} />
            <span title={row.key}>{row.title ?? row.key ?? row.id}</span>
          </Link>
        ))}
    </section>
  );
}

// Chevron: a static glyph rotated by CSS off aria-expanded (explore.css),
// rather than swapping the character in JS, so open/close is one visual rule
// instead of two glyph literals to keep in sync. Shared by the epic and
// feature tiers, which is the whole disclosure control every other row in
// the tree already agrees on.
function Chevron({
  open,
  label,
  onToggle,
}: {
  open: boolean;
  label: string;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      className="explore-chevron"
      aria-expanded={open}
      aria-label={`${open ? "Collapse" : "Expand"} ${label}`}
      onClick={onToggle}
    >
      <span className="chevron-glyph" aria-hidden="true">
        ▸
      </span>
    </button>
  );
}

function ChevronSpacer() {
  return <span className="explore-chevron-spacer" aria-hidden="true" />;
}

function EpicRow({
  epic,
  selected,
  expansion,
  onToggleBranch,
  hideCancelled,
}: {
  epic: TreeNode;
  selected: NavigatorSelection | null;
  expansion: Expansion;
  onToggleBranch: (id: string, open: boolean) => void;
  hideCancelled: boolean;
}) {
  // Openness reads from the lifted map (isOpen), never derived from the
  // current selection at render time: Explore.tsx is the one place a
  // selection materializes into an open entry (autoOpen), and a chevron
  // click is the one place a hand toggle does (onToggleBranch).
  const open = isOpen(expansion, epic.id);
  const selectedHere = isSelected(selected, "epic", epic);
  const label = epic.title ?? epic.key ?? epic.id;
  // The bar keeps the full child mix (JC: the bar already reads well); only
  // the rows and their count follow the toggle.
  const { mix, caption } = composeStat(epic.children);
  const features = visibleRows(epic.children, hideCancelled);

  return (
    <div className={selectedHere ? "explore-epic is-selected" : "explore-epic"}>
      <div className={rowClass("explore-epic-row", epic.status)}>
        <Chevron
          open={open}
          label={label}
          onToggle={() => {
            onToggleBranch(epic.id, !open);
          }}
        />
        <StateGlyph status={epic.status} size={15} />
        <Link to={`/node/epic/${epic.id}`} className="explore-epic-title" title={epic.key}>
          {label}
        </Link>
        <span className="explore-epic-count">{features.length}</span>
      </div>
      <div className="explore-epic-bar">
        <StateBar mix={mix} caption={epic.children.length > 0 ? caption : undefined} />
      </div>
      {open && (
        <div className="explore-feats">
          {epic.children.length === 0 ? (
            <EmptyNote>(no features)</EmptyNote>
          ) : features.length === 0 ? (
            <EmptyNote>(only cancelled features)</EmptyNote>
          ) : (
            features.map((feature) => (
              <FeatureRow
                key={feature.id}
                feature={feature}
                selected={selected}
                expansion={expansion}
                onToggleBranch={onToggleBranch}
                hideCancelled={hideCancelled}
              />
            ))
          )}
        </div>
      )}
    </div>
  );
}

// localKey is the last segment of a canonical key, the node's own folder name
// (artifact-schema.md, Keys and paths). A key the tree does not carry stays
// undefined, so the row's title attribute is absent rather than empty.
function localKey(key: string | undefined): string | undefined {
  if (key === undefined || key === "") {
    return undefined;
  }
  const i = key.lastIndexOf("/");
  return i >= 0 ? key.slice(i + 1) : key;
}

// FeatureRow is the one-line row the brief names: a dot, a title, nothing
// else visible beyond its own child count - the key rides the hover title
// attribute rather than crowding the 300px column, and the full reference
// lives one click away in the reading pane. It grows its own chevron when it
// has tasks beneath it, and a same-width spacer when it has not (the mock's
// alignment rule: every row's title starts at the same left edge whether or
// not it can expand).
function FeatureRow({
  feature,
  selected,
  expansion,
  onToggleBranch,
  hideCancelled,
}: {
  feature: TreeNode;
  selected: NavigatorSelection | null;
  expansion: Expansion;
  onToggleBranch: (id: string, open: boolean) => void;
  hideCancelled: boolean;
}) {
  // Same lifted-map idiom EpicRow keeps one tier up: openness reads from
  // isOpen, the chevron writes through onToggleBranch, no state of its own.
  const open = isOpen(expansion, feature.id);
  const selectedHere = isSelected(selected, "feature", feature);
  const label = feature.title ?? feature.key ?? feature.id;
  const tasks = visibleRows(feature.children, hideCancelled);
  const hasChildren = tasks.length > 0;
  return (
    <>
      <div className={rowClass("explore-feat", feature.status, selectedHere && "is-selected")}>
        {hasChildren ? (
          <Chevron
            open={open}
            label={label}
            onToggle={() => {
              onToggleBranch(feature.id, !open);
            }}
          />
        ) : (
          <ChevronSpacer />
        )}
        <StateGlyph status={feature.status} />
        <Link to={`/node/feature/${feature.id}`} className="explore-feat-title" title={feature.key}>
          {label}
        </Link>
        {hasChildren && <span className="explore-feat-count">{tasks.length}</span>}
      </div>
      {open && hasChildren && (
        <div className="explore-tasks">
          {tasks.map((task) => (
            <TaskRow key={task.id} task={task} selected={selected} />
          ))}
        </div>
      )}
    </>
  );
}

// TaskRow is the third tier, the same one-line grammar a feature row keeps.
// A task's own title leads; its local key segment rides the hover title,
// since the segments above it are the two rows this one sits under. It
// never carries a chevron (a task has no children of its own), so it always
// draws the same spacer a childless feature row does, keeping every tier's
// title flush with the ones above it.
function TaskRow({ task, selected }: { task: TreeNode; selected: NavigatorSelection | null }) {
  const selectedHere = isSelected(selected, "task", task);
  const label = task.title ?? localKey(task.key) ?? task.id;
  return (
    <div className={rowClass("explore-task", task.status, selectedHere && "is-selected")}>
      <ChevronSpacer />
      <StateGlyph status={task.status} />
      <Link to={`/node/task/${task.id}`} className="explore-feat-title" title={localKey(task.key)}>
        {label}
      </Link>
    </div>
  );
}

// The statuses whose work is finished one way or another. Their rows step
// back (explore.css's .is-settled) so the unfinished rows are what a scan
// lands on; a blocked row is the opposite case and steps forward.
const SETTLED_STATUSES: ReadonlySet<string> = new Set(["done", "answered", "cancelled", "archived"]);

// rowClass composes a row's base class with its status emphasis and, when
// the row is the selection, the selected class that row kind already uses.
function rowClass(base: string, status: string, selectedClass: string | false = false): string {
  const classes = [base];
  if (SETTLED_STATUSES.has(status)) {
    classes.push("is-settled");
  } else if (status === "blocked") {
    classes.push("is-blocked");
  }
  if (selectedClass) {
    classes.push(selectedClass);
  }
  return classes.join(" ");
}

// Exported so Explore.tsx can match the same address (an id on every
// rendered link, a canonical key when a human pastes one) against the same
// tree payload when it derives a selection's ancestor ids for the lifted
// expansion map - one definition of "does this node match the address",
// not two that could drift apart.
export function isSelected(
  selected: NavigatorSelection | null,
  noun: string,
  node: Pick<TreeNode, "id" | "key">,
): boolean {
  if (selected === null || selected.noun !== noun) {
    return false;
  }
  return selected.addr === node.id || (node.key !== undefined && selected.addr === node.key);
}
