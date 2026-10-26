// Explore is the drill lens (ui-surface.md, "Explore"): three lightweight
// modes over one corpus, browse, filter, and search, plus the shell that every
// visible destination renders inside. App mounts this as the pathless parent
// of /explore, /questions, /node/:noun/*, and /file/*, so the navigator,
// heading, and mode nav stay mounted while the outlet swaps among the
// no-selection pane, ledger, node reader, and loose-file reader. That stable
// element position preserves navigator expansion across destinations.
// Navigator reads refresh on pathname changes alongside the reading pane.
//
// The active mode lives in the URL (?mode=), so every view here is linkable
// and the back button walks real history instead of lost component state.
// Node, file, and questions addresses all read as browse mode: their outlet is
// already showing the split-shaped destination browse exists to carry, so a
// stray ?mode= never hides that destination. Filter and search remain
// reachable only at /explore.
//
// The selected node is the /node/:noun/* address itself (an id on every
// rendered link, a canonical key when a human pastes one), read here with
// matchPath rather than useParams because a layout's own params never include
// its children's. The old /explore/:noun/:id selection route is retired; App
// redirects it to /node/:noun/:id so pre-change bookmarks keep landing.
//
// Tree expansion (task 1, docs/spec/ui-surface.md) is
// owned here rather than by each navigator row: an explicit map
// (views/explore/expansion.ts) of which branches are held open, materialized
// by autoOpen whenever the selection changes - before paint, via
// useLayoutEffect, so a deep link renders its ancestry open on the first
// frame with no flash of collapsed rows. A selection can only ever add opens;
// nothing here ever closes a branch, which is what lets a branch opened by
// an earlier selection stay open once the reader moves on to something else.
//
// The split's own width (task 3, same plan) is a second, independent piece
// of state: navWidth drives explore.css's --explore-nav-w custom property,
// which the .explore-nav rule below reads for its actual width. A hand-
// rolled divider between the navigator and the outlet moves it three ways -
// a mouse drag (window-level mousemove/mouseup while dragging, wired only
// for the drag's own duration and torn down on mouseup or unmount), an
// ArrowLeft/ArrowRight nudge on the focused divider, and a double-click
// reset - every one of them folded through splitSize.ts's clampNavWidth so
// navWidth can never leave [NAV_WIDTH_MIN, NAV_WIDTH_MAX]. No touch
// handling: the desktop-first posture (ui-surface.md, 2026-08-11) owes none,
// and the divider is inert below the 48rem stack where the split un-stacks
// into one column.
//
// Both pieces of state survive a reload (task 4, same plan): the expansion
// map under a per-project localStorage key (views/explore/expansionStorage.ts),
// since it names actual project node ids and one project's open branches
// bleeding into another project's tab would be visibly wrong; the split
// width under one flat key (views/explore/splitStorage.ts), since a pane
// width is a plain viewport preference with nothing project-specific to
// leak. The width restores in its own useState initializer (no project key
// to wait on); the expansion map restores in a useLayoutEffect that retries
// until the project key resolves, mirroring Build.tsx's own restoredRef
// guard for its epic selection.
//
// /api/tree and /api/status are two independent fetches with no ordering
// guarantee between them, and the hand-closed rule above ("stays closed
// even when selected") has to hold no matter which one wins that race - two
// review rounds hardened this: the restore composes with whatever this
// session has already materialized or the reader has already clicked
// rather than replacing it outright (round 1); the auto-open effect itself
// holds until /api/status has settled, so the restore always gets its one
// chance to apply a stored hand-close before auto-open can ever open that
// same branch (round 2 - without this, whichever fetch happened to resolve
// first silently decided whether a remembered close survived). The split's
// first real render also waits for /api/status to settle before it ever
// paints, which is what makes both effects' declaration order actually
// load-bearing instead of merely timed right in the lucky case.

import type { CSSProperties, KeyboardEvent } from "react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link, matchPath, Outlet, useLocation, useSearchParams } from "react-router-dom";
import type { ListResult, ProjectSummary, TreeModel } from "../api";
import { listPath, statusPath, treePath } from "../api";
import Navigator, { isSelected, type NavigatorSelection } from "../components/Navigator";
import PageLoading from "../components/PageLoading";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import {
  autoOpen,
  closeAll,
  emptyExpansion,
  mergeRestored,
  openAll,
  toggle,
  type Expansion,
} from "./explore/expansion";
import { readExpansion, writeExpansion } from "./explore/expansionStorage";
import { expandableShelfIDs, selectedShelfBranch } from "./explore/shelves";
import { clampNavWidth, NAV_WIDTH_DEFAULT, NAV_WIDTH_MAX, NAV_WIDTH_MIN } from "./explore/splitSize";
import { readNavWidth, writeNavWidth } from "./explore/splitStorage";
import FilterMode from "./explore/FilterMode";
import MapMode from "./explore/MapMode";
import SearchMode from "./explore/SearchMode";
import "./explore.css";

// The nudge a single ArrowLeft/ArrowRight press moves the divider (task 3's
// locked ruling): large enough to feel like a deliberate step, small enough
// that reaching either bound takes a handful of presses rather than one.
const NAV_WIDTH_STEP = 16;

const modes = ["browse", "filter", "map"] as const;
type RailMode = (typeof modes)[number];
type Mode = RailMode | "search";
const modeLabels: Record<RailMode, string> = { browse: "Browse", filter: "Filter", map: "Map" };

export default function Explore() {
  const [params] = useSearchParams();
  const location = useLocation();
  const nodeMatch = matchPath("/node/:noun/*", location.pathname);
  const fileMatch = matchPath("/file/*", location.pathname);
  const questionsCurrent = location.pathname === "/questions";
  const amendmentsCurrent = location.pathname === "/amendments";
  const raw = params.get("mode");
  // Anything that is not a mode reads as browse, so a stale or hand-mangled
  // link degrades to the default view rather than a blank page. A node, file,
  // questions, or amendments address reads as browse regardless of a stray
  // ?mode= because its destination already occupies the split's reading pane.
  const mode: Mode =
    nodeMatch === null &&
    fileMatch === null &&
    !questionsCurrent &&
    !amendmentsCurrent &&
    (raw === "filter" || raw === "search" || raw === "map")
      ? raw
      : "browse";
  const selected: NavigatorSelection | null =
    nodeMatch !== null && nodeMatch.params.noun !== undefined
      ? { noun: nodeMatch.params.noun, addr: nodeMatch.params["*"] ?? "" }
      : null;
  const questionsActive = questionsCurrent || selected?.noun === "question";
  const amendmentsActive = amendmentsCurrent || selected?.noun === "amendment" ||
    (mode === "filter" && (params.get("noun") === "amendments" || params.get("noun") === "amendment"));

  // Tree and shelf reads idle outside Browse; question counts stay in the
  // navigation rail across every Explore mode.
  const split = mode === "browse";
  // Filter, Search, and Map replace the reading pane, so the layout names
  // the tab itself there. In Browse the pane's own destination (node, file,
  // questions, amendments, or the empty pane) owns the title.
  useDocumentTitle(split ? null : titleOf("Explore"));
  const navigatorOptions = { refreshKey: location.pathname, retainData: true };
  const tree = useFetch<TreeModel>(split ? treePath : null, navigatorOptions);
  const questions = useFetch<ListResult>(listPath("question"), navigatorOptions);
  const bugs = useFetch<ListResult>(split ? listPath("bug") : null, navigatorOptions);
  const chores = useFetch<ListResult>(split ? listPath("chore") : null, navigatorOptions);
  const tweaks = useFetch<ListResult>(split ? listPath("tweak") : null, navigatorOptions);
  const openQuestions = questions.data?.nodes.reduce((count, row) => count + Number(row.status === "open"), 0) ?? 0;

  // Task 4's own fetch: Explore had fetched no project identity before this
  // task (this file's own header comment used to say so), but the expansion
  // map's storage key is namespaced per project, the same every-view-
  // fetches-its-own posture Build.tsx already keeps for its own default
  // selection (App.tsx:66-72).
  const summary = useFetch<ProjectSummary>(statusPath, { retainData: true });
  const projectKey = summary.data?.project.key;
  // Round 2 fix (re-review): whether /api/status resolved to real project
  // data or to an error, "settled" is the one signal the auto-open effect
  // below waits on before it may touch `expansion` at all - the same signal
  // the split's own first real paint waits on (used again near the return,
  // below). Tied to settling, not to a successful restore alone: an
  // /api/status error must not wedge auto-open shut forever - there is
  // nothing to restore in that case (projectKey never resolves), but a deep
  // link's ancestry still has to open.
  const summarySettled = summary.data !== null || summary.error !== null;

  const [expansion, setExpansion] = useState<Expansion>(emptyExpansion);

  // Task 4's restore: mirrors Build.tsx:102-136's own restoredRef guard,
  // adapted from a URL param to plain state - retry (no latch) while
  // projectKey is still undefined, latch once the restore actually runs,
  // latch and do nothing when nothing is stored. useLayoutEffect, not
  // useEffect, for the same paint-timing reason Build's own comment gives:
  // the remembered map has to be what the tree's first visible frame
  // renders, not a fixed-up second frame after a flash of the empty-map
  // default. The render below also gates the split on summary having
  // settled, so this effect has always had its one chance to run by the
  // time the split is first painted at all.
  //
  // Fix round 1 (review): COMPOSE, never replace. A reviewer probe
  // confirmed that a flat setExpansion(stored) here could wipe out
  // whatever this session had already materialized or the reader had
  // already clicked, if either happened to land in an earlier commit.
  // mergeRestored (expansion.ts) is the pure composition itself - LIVE
  // (this session's own state) outranks STORED for any id both name, and
  // STORED only fills in what live does not yet know about, the same
  // precedence toggle's own hand-closed rule already gives an explicit
  // choice over an inferred one; its own doc comment and unit tests carry
  // the contract now, not this call site. Kept as defense in depth after
  // round 2's fix below (the auto-open effect no longer races ahead of
  // this one at all, so `prev` at merge time is ordinarily just this
  // session's own hand clicks, if any), rather than narrowed back to a
  // flat replace now that the one known race is closed a different way.
  const restoredRef = useRef(false);
  useLayoutEffect(() => {
    if (restoredRef.current) {
      return;
    }
    if (projectKey === undefined) {
      // Project key not resolved yet; retry on the next render rather than
      // giving up (the attempt is not "used" until it actually runs).
      return;
    }
    restoredRef.current = true;
    const stored = readExpansion(projectKey);
    if (stored === null) {
      // Nothing stored for this project; today's default (the empty map,
      // opened only by the auto-open effect below) applies, same as if this
      // effect never ran.
      return;
    }
    setExpansion((prev) => mergeRestored(stored, prev));
  }, [projectKey]);

  // Runs before paint (useLayoutEffect) so a deep link's ancestry is already
  // open on the tree's first rendered frame, the same timing the old derived
  // `open` expression got by construction. Depends on the selection's own
  // fields rather than the object (a fresh literal every render, see above)
  // and on the fetched tree data, since the ancestry cannot be computed
  // until both are known - re-running once the tree lands is what opens a
  // deep link's ancestry the instant it arrives rather than never. The
  // functional setExpansion((prev) => ...) form is load-bearing, not a
  // style choice: it reads whatever the LATEST committed map is (this
  // session's own clicks, or the restore effect's own composed merge) at
  // the moment this update actually applies, rather than closing over a
  // stale `expansion` captured at render time.
  //
  // Held until summarySettled (round 2 fix, re-review): /api/tree and
  // /api/status are independent fetches with no ordering guarantee, and
  // this effect's OWN dependency (tree.data) can resolve before the restore
  // effect above's (projectKey) does. A re-review probe confirmed that
  // running unguarded made the hand-closed rule arrival-order dependent -
  // when the tree won the race, this effect opened the ancestry before any
  // stored map existed to check, the restore's own compose then let that
  // "true" survive over a remembered "false", and the next persist rewrote
  // storage without the hand-close the reader had actually chosen. Gating
  // on summarySettled guarantees the restore effect above has always had
  // its one chance to run first (it fires in the very same commit that
  // flips summarySettled true, ahead of this effect in declaration order,
  // and React flushes every layout effect before the next one paints) - so
  // by the time this effect ever calls autoOpen, `prev` already reflects
  // whatever storage held, hand-closed entries included, regardless of
  // which fetch actually won the race. The dependency list adds
  // summarySettled so this effect re-fires the moment it flips (it does
  // not change on its own otherwise), which is what re-runs a tree that
  // had already landed while the effect was still being held.
  useLayoutEffect(() => {
    if (!summarySettled) {
      return;
    }
    const ids = selectionAncestryIds(tree.data, selected);
    if (ids.length === 0) {
      return;
    }
    setExpansion((prev) => autoOpen(prev, ids));
  }, [summarySettled, selected?.noun, selected?.addr, tree.data]);

  function handleToggleBranch(id: string, open: boolean) {
    setExpansion((prev) => toggle(prev, id, open));
  }

  // Task 2's two whole-tree commands. Open-all reads the CURRENT tree data
  // (not a cached id list) so it always covers what is actually rendered;
  // close-all needs no ids at all, since the empty map means "nothing held
  // open or closed" regardless of what used to be in it. Neither touches
  // `selected`, so the auto-open effect above (keyed on the selection's own
  // fields) does not re-run and does not immediately reopen the branch
  // close-all just collapsed.
  function handleOpenAll() {
    setExpansion((prev) =>
      openAll(prev, expandableIds(tree.data, bugs.data, chores.data, tweaks.data)),
    );
  }

  function handleCloseAll() {
    setExpansion(closeAll());
  }

  // Task 4's persist: every change to the map, once the project key is
  // known, so a reload restores exactly what the reader had open - a hand-
  // closed entry included, and the empty map close-all leaves behind
  // included too (expansionStorage.ts's own doc comment: the empty map is a
  // real remembered state, not "nothing stored"). A plain useEffect, not
  // useLayoutEffect: nothing here needs to beat paint the way the restore
  // above does.
  //
  // projectKeyRef, not projectKey itself, in the dependency array (fix
  // round, the reviewer's minor finding): keeping projectKey as a direct
  // dependency meant this effect fired the INSTANT it first resolved, even
  // on a first-ever visit where the restore effect above had nothing to
  // apply (stored === null) - `expansion` had not actually changed, but the
  // effect ran anyway (verified empirically: a dedicated test asserting the
  // storage key stays absent failed until this was fixed) and wrote that
  // untouched default straight back to storage, silently turning "nothing
  // stored" into "[]" and destroying the very distinction
  // expansionStorage.ts's own header comment defends. Depending on
  // `expansion` alone means this effect only ever re-runs when the map
  // itself actually changes - restore's own composed setExpansion call
  // included, since that always produces a new map when it has anything to
  // merge in - while the ref still hands the effect body a fresh
  // projectKey to write under without making its mere presence a trigger.
  // restoredRef.current stays as a second, explicit guard: `expansion` can
  // still change (auto-open materializing a deep link's ancestry) before
  // projectKey has resolved at all, and this effect must not write a value
  // that has not been through its own chance to compose with storage yet.
  //
  // expansion.ts's autoOpen/toggle are identity-stable when nothing actually
  // changes (Task 1 review fold-in): a no-op materialization returns the
  // SAME map reference, so it never reaches this effect as a change at all
  // rather than merely being ignored by a guard inside it.
  const projectKeyRef = useRef<string | undefined>(undefined);
  projectKeyRef.current = projectKey;
  useEffect(() => {
    if (!restoredRef.current || projectKeyRef.current === undefined) {
      return;
    }
    writeExpansion(projectKeyRef.current, expansion);
  }, [expansion]);

  // Task 3's resizable split: navWidth drives the --explore-nav-w custom
  // property explore.css's .explore-nav width reads (var(--explore-nav-w,
  // 300px)), so every source of a new width - a drag, an arrow key, a
  // double-click - funnels through this one piece of state rather than each
  // owning a DOM measurement of its own. splitRef is the drag handler's only
  // way to find "the split's own bounding rect" (the brief's own wording,
  // deliberately not the page's coordinate space).
  // Task 4's width restore: read in the useState INITIALIZER, not an
  // effect, because the storage key is flat (splitStorage.ts's own doc
  // comment: a pane width is a viewport preference, no project key to wait
  // on) - the same panelStorage.ts idiom Build.tsx:72 already uses for its
  // own flat-keyed panel state. No layout effect, no flash: the restored
  // width is what the very first render already sets.
  const [navWidth, setNavWidth] = useState<number>(() => readNavWidth());
  const [dragging, setDragging] = useState(false);
  const splitRef = useRef<HTMLDivElement>(null);

  // Task 4's width persist: unconditional (no project key to wait on,
  // matching the initializer above), but gated on `!dragging` (fix round,
  // small finding) so a mouse drag writes once at its own end rather than
  // once per mousemove - the drag effect below updates navWidth on every
  // pointer move, and without this gate each of those intermediate pixels
  // would hit localStorage too. Keyboard nudges and the double-click reset
  // never set dragging, so they still persist immediately, one write each.
  useEffect(() => {
    if (dragging) {
      return;
    }
    writeNavWidth(navWidth);
  }, [navWidth, dragging]);

  // Wired only while dragging is true, and torn down the moment it is not -
  // on mouseup (dragging flips false, this effect's own cleanup runs before
  // the next one) and on unmount (React runs the same cleanup regardless of
  // why the component is going away). That is what keeps a drag that
  // outlives the component (a navigation away mid-drag) from leaking a
  // window-level listener forever.
  useEffect(() => {
    if (!dragging) {
      return;
    }
    function onMove(event: MouseEvent) {
      // Wedge guard: a mouseup that fires outside the browser window (the
      // reader released the button over another app, or off-screen) never
      // reaches this window's listeners, so dragging would otherwise stay
      // armed until the next click. event.buttons reports which buttons are
      // actually held at the moment of THIS move - 0 means none are, which
      // can only mean the button already let go somewhere this window never
      // saw - so treat that the same as the mouseup that should have
      // arrived, before ever touching the width.
      if (event.buttons === 0) {
        setDragging(false);
        return;
      }
      const rect = splitRef.current?.getBoundingClientRect();
      if (rect === undefined) {
        return;
      }
      setNavWidth(clampNavWidth(event.clientX - rect.left));
    }
    function onUp() {
      setDragging(false);
    }
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
  }, [dragging]);

  function handleDividerKeyDown(event: KeyboardEvent) {
    if (event.key === "ArrowLeft") {
      event.preventDefault();
      setNavWidth((w) => clampNavWidth(w - NAV_WIDTH_STEP));
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      setNavWidth((w) => clampNavWidth(w + NAV_WIDTH_STEP));
    }
  }

  // Fix round (review, Important finding): the split's first REAL render
  // waits on summarySettled (computed above, alongside projectKey) - data
  // or error alike - mirroring Build.tsx:104-107's own "needsDefault &&
  // summary.data/error" gate for its default epic selection. Two things
  // ride on this, together: it is what makes the restore effect's
  // declaration-order argument actually load-bearing (the effect is
  // guaranteed to have run, for good or for nothing, before the split the
  // reader can see and click ever paints - no more click-in-the-first-few-
  // hundred-ms race, because there is nothing rendered yet to click), and
  // it is the no-flash guarantee the brief promised in the first place, now
  // actually held to it instead of merely timed right in the lucky case. An
  // error settles the gate too: the split renders anyway, with persistence
  // silently off (projectKey stays undefined, so the restore and persist
  // effects above both stay inert) rather than trading a broken
  // /api/status for a broken Explore.

  if (split && !summarySettled) {
    return <PageLoading title="Explore" />;
  }

  return (
    <section className="explore">
      <nav className="mode-nav" aria-label="Explore mode">
        {modes.map((m) => (
          <ModeLink key={m} target={m} current={questionsActive || amendmentsActive ? null : mode} params={params} />
        ))}
        <span className="explore-nav-separator" aria-hidden="true" />
        <Link
          to="/questions"
          className={questionsActive ? "explore-ledger-link active" : "explore-ledger-link"}
          aria-current={questionsActive ? "page" : undefined}
          aria-busy={questions.loading}
          title={questions.error ?? undefined}
        >
          Questions
          {openQuestions > 0 && <span className="count-badge">{openQuestions} open</span>}
        </Link>
        <Link
          to="/amendments"
          className={amendmentsActive ? "active" : undefined}
          aria-current={amendmentsActive ? "page" : undefined}
        >
          Amendments
        </Link>
      </nav>
      {split && (
        <div
          ref={splitRef}
          className={dragging ? "explore-split dragging" : "explore-split"}
          style={{ "--explore-nav-w": `${navWidth}px` } as CSSProperties}
        >
          <Navigator
            tree={tree}
            questions={questions}
            bugs={bugs}
            chores={chores}
            tweaks={tweaks}
            summary={summary}
            selected={selected}
            expansion={expansion}
            onToggleBranch={handleToggleBranch}
            onOpenAll={handleOpenAll}
            onCloseAll={handleCloseAll}
          />
          {/* The resizable divider (task 3): a mousedown starts a drag (the
              effect above wires window mousemove/mouseup while dragging is
              true), ArrowLeft/ArrowRight nudge by NAV_WIDTH_STEP through the
              same clamp a drag uses, and a double-click resets to the
              default. No touch handlers - desktop-first posture
              (ui-surface.md, 2026-08-11), and the divider is hidden outright
              below the 48rem stack (explore.css) where there is no
              side-by-side split left to resize. */}
          <div
            className="explore-divider"
            role="separator"
            aria-orientation="vertical"
            aria-label="Resize navigator"
            aria-valuenow={navWidth}
            aria-valuemin={NAV_WIDTH_MIN}
            aria-valuemax={NAV_WIDTH_MAX}
            tabIndex={0}
            onMouseDown={() => setDragging(true)}
            onDoubleClick={() => setNavWidth(NAV_WIDTH_DEFAULT)}
            onKeyDown={handleDividerKeyDown}
          />
          <Outlet />
        </div>
      )}
      {!split && (
        // Filter, search, and map ride the same fixed frame the split above
        // does (task 25): .explore-modebody is their one scroll area, since
        // none of them has a navigator beside it to split the frame with.
        // Filter and search need no project identity at all, so neither
        // waits on summarySettled. The map does: its open set and viewport
        // are remembered per project, and materializing them before the key
        // has settled would ignore what this project had stored and then
        // overwrite it (fix round 0). It takes that same gate as a prop
        // rather than blanking this whole body, so the mode rail and the
        // map's own loading line stay put while /api/status is out. Map is
        // also the one mode that owns its pan and zoom, so its body drops
        // the reading padding and the scrollbar (explore.css's
        // .explore-modebody-map).
        <div className={mode === "map" ? "explore-modebody explore-modebody-map" : "explore-modebody"}>
          {mode === "filter" && <FilterMode />}
          {mode === "search" && <SearchMode />}
          {mode === "map" && (
            <MapMode projectKey={projectKey} projectName={summary.data?.project.name} ready={summarySettled} />
          )}
        </div>
      )}
    </section>
  );
}

// Collects the node ids whose branch a selection should hold open: the epic
// that is the selection or contains it, and, when the selection sits inside
// a feature (as the feature itself or one of its tasks), that feature too -
// the same reach the old per-row `selectedHere || containsSelection`
// expression covered, computed once here instead of at every row's own
// render. A task carries no chevron of its own, so its id is never in the
// list; only its two ancestors ever need to be pushed open for it to become
// visible.
function selectionAncestryIds(tree: TreeModel | null, selected: NavigatorSelection | null): string[] {
  const shelf = selectedShelfBranch(selected?.noun);
  if (shelf.length > 0) {
    return shelf;
  }
  if (tree === null || selected === null) {
    return [];
  }
  const ids: string[] = [];
  for (const epic of tree.nodes) {
    let epicHolds = isSelected(selected, "epic", epic);
    for (const feature of epic.children) {
      const featureHolds =
        isSelected(selected, "feature", feature) ||
        feature.children.some((task) => isSelected(selected, "task", task));
      if (featureHolds) {
        ids.push(feature.id);
        epicHolds = true;
      }
    }
    if (epicHolds) {
      ids.push(epic.id);
    }
  }
  return ids;
}

// Every id open-all should set true (task 2): every epic (EpicRow always
// draws a chevron, even over zero features) plus every feature that carries
// at least one task (FeatureRow draws a chevron only then; a childless
// feature gets a spacer instead and so has nothing of its own to open). A
// leaf feature and every task get no entry, the same set Navigator.tsx's own
// rows already agree has a disclosure control at all.
function expandableIds(
  tree: TreeModel | null,
  bugs: ListResult | null,
  chores: ListResult | null,
  tweaks: ListResult | null,
): string[] {
  const shelfIDs = expandableShelfIDs(bugs, chores, tweaks);
  if (tree === null) {
    return shelfIDs;
  }
  const ids: string[] = [];
  for (const epic of tree.nodes) {
    ids.push(epic.id);
    for (const feature of epic.children) {
      if (feature.children.length > 0) {
        ids.push(feature.id);
      }
    }
  }
  return [...ids, ...shelfIDs];
}

// The reading pane with nothing selected (/explore in browse mode): it says
// so plainly rather than guessing a default node to show. Mounted by App as
// the /explore child route's element, beside the navigator the layout above
// keeps standing.
export function NoSelectionPane() {
  // In Browse the layout above owns no title: a node or file destination in
  // the reading pane names the tab, and the empty pane says "Explore".
  useDocumentTitle(titleOf("Explore"));
  return (
    <div className="explore-detail">
      <p className="muted">Select a node from the navigator to see its detail.</p>
    </div>
  );
}

// A mode switch keeps the other modes' params: stepping into search and back
// must not lose a half-built filter. Browse is the default, so its link drops
// the mode param and keeps the canonical /explore URL clean. Since it always
// targets plain /explore, switching modes also leaves any node or file
// destination behind, exactly as leaving a reading pane should read.
function ModeLink({
  target,
  current,
  params,
}: {
  target: RailMode;
  current: Mode | null;
  params: URLSearchParams;
}) {
  const next = new URLSearchParams(params);
  if (target === "browse") {
    next.delete("mode");
  } else {
    next.set("mode", target);
  }
  // A node detail tab selection has no meaning outside that page; without
  // this a stray `?tab=` picked up from a node page (the outlet mounts the
  // identical NodeDetail instrument) would ride along into filter or
  // search's URL for no reason at all.
  next.delete("tab");
  const search = next.toString();
  const active = target === current;
  return (
    <Link
      to={search === "" ? "/explore" : `/explore?${search}`}
      className={active ? "active" : undefined}
      aria-current={active ? "page" : undefined}
    >
      {modeLabels[target]}
    </Link>
  );
}
