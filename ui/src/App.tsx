import { type RefObject, useLayoutEffect, useRef } from "react";
import { Link, Navigate, NavLink, Route, Routes, useLocation, useParams } from "react-router-dom";
import tillerLogo from "../../tiller-logo.svg";
import tillerLogoInverted from "../../tiller-logo-inverted.svg";
import type { ProjectSummary } from "./api";
import { statusPath, versionPath } from "./api";
import AsOf from "./components/AsOf";
import ThemeToggle from "./components/ThemeToggle";
import { titleOf, useDocumentTitle, useFetch } from "./hooks";
import { PeopleProvider } from "./people";
import Build from "./views/Build";
import Drift from "./views/Drift";
import Explore, { NoSelectionPane } from "./views/Explore";
import FileDetail from "./views/FileDetail";
import History from "./views/History";
import Home from "./views/Home";
import NodeDetail from "./views/NodeDetail";
import Report from "./views/Report";
import WorkItems from "./views/WorkItems";
import AmendmentsPane from "./views/explore/AmendmentsPane";
import QuestionsPane from "./views/explore/QuestionsPane";

// Explore, Build, and Fix size their frames to the viewport below the topbar
// with calc(100vh - var(--topbar-h)). The topbar's height is whatever its
// row lays out to, and it wraps taller on a narrow window, so the token
// follows the measured header rather than a guessed constant; otherwise the
// difference scrolls the page under the sticky header. ResizeObserver is
// absent in jsdom, so there the mount measure is all there is.
// The page-scrolled views, the ones whose content passes under the topbar.
export function scrollsUnderTopbar(pathname: string): boolean {
  return pathname === "/" || pathname === "/report" || pathname === "/history";
}

function useTopbarHeight(headerRef: RefObject<HTMLElement | null>) {
  useLayoutEffect(() => {
    const header = headerRef.current;
    if (header === null) {
      return;
    }
    const root = document.documentElement;
    const measure = () => {
      const height = header.getBoundingClientRect().height;
      if (height > 0) {
        root.style.setProperty("--topbar-h", `${height}px`);
      }
    };
    measure();
    if (typeof ResizeObserver === "undefined") {
      return () => root.style.removeProperty("--topbar-h");
    }
    const observer = new ResizeObserver(measure);
    observer.observe(header);
    return () => {
      observer.disconnect();
      root.style.removeProperty("--topbar-h");
    };
  }, [headerRef]);
}

export default function App() {
  const pathname = useLocation().pathname;
  const { data: summary, error: summaryError } = useFetch<ProjectSummary>(statusPath, { refreshKey: pathname });
  const { data: build } = useFetch<{ version: string }>(versionPath);
  // Node and source-file readers and the Questions and Amendments ledgers
  // all render inside Explore's shell (the layout route below), so the nav
  // marks Explore as where you are even though none of those paths starts
  // with /explore. NavLink's own isActive only knows its own `to`.
  const onExploreChild =
    pathname.startsWith("/node/") ||
    pathname.startsWith("/file/") ||
    pathname === "/questions" ||
    pathname === "/amendments";
  const headerRef = useRef<HTMLElement>(null);
  useTopbarHeight(headerRef);

  return (
    <PeopleProvider contributors={summary?.contributors}>
      {/* Home, Report, and History scroll the page under the sticky topbar,
          so only they take its shadow; Explore, Build, and Fix frame
          themselves below it and never scroll beneath it. */}
      <header className={scrollsUnderTopbar(pathname) ? "site-header site-header-shadow" : "site-header"} ref={headerRef}>
        <div className="header-row">
          <Link to="/" className="wordmark">
            <img className="wordmark-logo wordmark-logo-light" src={tillerLogo} alt="" width="747" height="453" />
            <img className="wordmark-logo wordmark-logo-dark" src={tillerLogoInverted} alt="" width="747" height="453" />
            Tiller
            {build !== null && <span className="wordmark-version">v{build.version}</span>}
          </Link>
          {summary !== null && <span className="mono project-key">{summary.project.key}</span>}
          <nav className="site-nav">
            {/* NavLink marks the current view; end keeps Home from claiming
                every route. Navigation labels carry no count badges. */}
            <NavLink to="/" end>
              Home
            </NavLink>
            <NavLink
              to="/explore"
              className={({ isActive }) => (isActive || onExploreChild ? "active" : undefined)}
            >
              Explore
            </NavLink>
            <NavLink to="/build">Build</NavLink>
            <NavLink to="/fix">Fix</NavLink>
            <NavLink to="/report">Report</NavLink>
            <NavLink to="/history">History</NavLink>
          </nav>
          {/* mock:235 - the topbar's right cluster: the freshness line beside
              the theme toggle, the console v2 phase 2 home for both (JC's
              ruling, 2026-08-10). */}
          <div className="topbar-right">
            <AsOf />
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Home summary={summary} summaryError={summaryError} />} />
          {/* The epic-selection board (console v2 phase 3, task-13-brief.md);
              Build.tsx owns its own /api/status fetch for the default
              selection rather than taking summary as a prop, the same
              every-view-fetches-its-own posture Explore and History already
              keep - Home is the one exception, since App already needs
              summary for the header identity band it renders regardless of
              route. */}
          <Route path="/build" element={<Build />} />
          <Route path="/fix" element={<WorkItems />} />
          {/* The pre-rename address of Fix (/bugs-chores, renamed 2026-10-06):
              old bookmarks and links carry ?attached= and ?terminal=, so the
              redirect keeps the whole query string. */}
          <Route path="/bugs-chores" element={<FixRedirect />} />
          {/* The client-facing metrics page (docs/spec/ui-surface.md,
              Report): numbers and charts only, no node links. */}
          <Route path="/report" element={<Report />} />
          {/* The window rides ?window=, so one route serves all three; the
              view reads the param and degrades an unknown one to visit. */}
          <Route path="/history" element={<History />} />
          {/* The pre-rename address (Recap.tsx renamed to History.tsx
              2026-08-10): a bookmark still lands on the same digest, and
              carries its window along, so a shared /recap?window=24h link
              does not silently drop the window it named. */}
          <Route path="/recap" element={<RecapRedirect />} />
          {/* Explore is a pathless LAYOUT route (JC, 2026-08-05): it owns the
              heading, mode nav, and navigator, while its outlet swaps among
              the no-selection pane, node reader, shared source-file reader,
              and questions ledger. Every child renders at one stable element
              position, which preserves navigator state across destinations. */}
          <Route element={<Explore />}>
            <Route path="/explore" element={<NoSelectionPane />} />
            {/* The questions ledger (Phase 4, task 16): a real child route
                of the same pathless layout /node/:noun/* already shares, so
                the navigator stays standing beside it and its pinned row can
                mark it current. Registered here rather than nested under
                /explore so the address is the plain /questions the brief
                names and a hard refresh renders it directly. */}
            <Route path="/questions" element={<QuestionsPane />} />
            {/* The amendments ledger takes the same standing-navigator shape. */}
            <Route path="/amendments" element={<AmendmentsPane />} />
            {/* A splat, not a :param, because a task's canonical address is a
                three-segment key whose slashes are real path segments. Both
                forms reach the same page: the console links by id, and a human
                pasting a key off the terminal gets the node the CLI just
                named. The API already resolves either (serve/node.go,
                nodePath). */}
            <Route path="/node/:noun/*" element={<NodeDetail />} />
            {/* Unowned search hits and local node provenance use the same
                standing Explore shell. The splat preserves every authored
                path segment; FileDetail passes the read to /api/prose. */}
            <Route path="/file/*" element={<FileDetail />} />
          </Route>
          {/* The retired selection route (task C2's /explore/:noun/:id):
              selection now rides the canonical /node/ address, so a
              pre-change bookmark redirects rather than 404s. */}
          <Route path="/explore/:noun/:id" element={<SelectionRedirect />} />
          {/* The header band's drift count links here (ui-surface.md, the
              header band sentence): the same candidates-and-uncovered answer
              `tiller drift` gives. Not in the top nav (Home, Explore,
              Build, Fix, Report, History): the band link is the
              discovery path, matching how the queue and node pages already
              surface without a standing nav entry of their own. */}
          <Route path="/drift" element={<Drift />} />
          {/* Without this, a path no route claims renders the chrome and an
              empty body: HTTP 200, header drawn, nothing inside, no error. A
              blank page is the one answer a router must never give. */}
          <Route path="*" element={<NoSuchPage />} />
        </Routes>
      </main>
    </PeopleProvider>
  );
}

// The old browse-selection URL, alive only as a forwarding address: replace,
// not push, so the back button never walks through the retired form.
function SelectionRedirect() {
  const { noun, id } = useParams();
  return <Navigate to={`/node/${noun}/${id}`} replace />;
}

// The pre-rename /recap URL, alive only as a forwarding address, same
// replace-not-push rule as SelectionRedirect above. The window rides the
// query string (?window=24h), and a fixed-window URL is a shareable read
// (History.tsx's own doc comment), so the redirect carries the search string
// along rather than dropping it at the door.
function RecapRedirect() {
  const location = useLocation();
  return <Navigate to={`/history${location.search}`} replace />;
}

// The pre-rename /bugs-chores URL, a forwarding address on the same
// replace-not-push rule. Its query string (?attached=, ?terminal=) and hash
// are the shareable state, so both ride along.
function FixRedirect() {
  const location = useLocation();
  return <Navigate to={`/fix${location.search}${location.hash}`} replace />;
}

// The address nothing claims. It is deliberately not the node page's "No such
// node", which answers a different question: that one means the tree has no
// such node, this one means the console has no such page.
function NoSuchPage() {
  useDocumentTitle(titleOf("Not found"));
  return (
    <section className="page-shell">
      <h1>No such page</h1>
      <p>
        Nothing in this console answers to <span className="mono">{useLocation().pathname}</span>.
      </p>
      <p className="muted">
        <Link to="/">Back to the desk</Link>
      </p>
    </section>
  );
}
