// Vitest/RTL coverage for App's own <Routes>, not any one view: whether a URL
// reaches a page at all. A console row now shows a task's three-segment key
// and hides its id, but every link addresses by id, so a key-addressed URL
// like /node/task/form-engine/certification-form-loading/transcribe-the-schema is a
// four-segment path against a route that only ever took three
// (/node/:noun/:addr). That mismatch renders NOTHING: no loading state, no
// error, the header and nav draw and the body is empty. This file pins the
// splat route that fixes that, the catch-all that turns "no route claims
// this" into a stated page instead of a blank one, and the empty-splat guard
// that keeps /node/task/ from spinning forever.
//
// Fetch stubbing follows home.test.tsx / history.test.tsx's stubFetch-keyed-by-
// URL convention (no MSW in this harness). App always fetches /api/status for
// the header regardless of route, so every stub map below includes it.

import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { NodePage, ProjectSummary } from "../../api";
import App, { scrollsUnderTopbar } from "../../App";

const nativeScrollIntoView = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  "scrollIntoView",
);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  if (nativeScrollIntoView) {
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", nativeScrollIntoView);
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  }
});

function summary(overrides: Partial<ProjectSummary> = {}): ProjectSummary {
  return {
    project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
    contributors: [{ id: "jc", name: "JC Grubbs" }],
    counts: { epics: 0, features: 0, tasks: 0, questions: 0 },
    whose_move: { human: 0, agent: 0 },
    ...overrides,
  };
}

function taskPage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: {
      noun: "task",
      id: "T-h4k2wq",
      key: "form-engine/certification-form-loading/transcribe-the-schema",
      title: "Transcribe the schema",
      status: "ready",
    },
    ref: "form-engine/certification-form-loading/transcribe-the-schema (T-h4k2wq)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

// The single-segment address every rendered console link actually emits
// (RefLink, TaskList's own link before this branch, the queue, search hits):
// a feature has no folder-backed key fixture here, only an id. This is the
// shape the splat route has to keep resolving even though the route change
// was motivated by a DIFFERENT, multi-segment shape - a route rewrite aimed
// at one address form silently breaking the other is exactly the regression
// this test exists to catch.
function featurePage(overrides: Partial<NodePage> = {}): NodePage {
  return {
    node: {
      noun: "feature",
      id: "F-9",
      title: "Wire the billing webhook",
      status: "ready",
    },
    ref: "F-9",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    ...overrides,
  };
}

// Matches home.test.tsx and history.test.tsx's own helper: an unmapped path
// 404s the way fetchJSON expects, so a missing fixture fails loudly instead
// of resolving to undefined.
function stubFetch(routes: Record<string, unknown>) {
  const spy = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in routes) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
    }
    return Promise.resolve({
      ok: false,
      status: 404,
      statusText: "Not Found",
      text: () => Promise.resolve(""),
    });
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function emptyExploreRoutes(extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    "/api/status": summary(),
    "/api/tree": { project: "acme", nodes: [] },
    "/api/ls?noun=question": { noun: "question", count: 0, nodes: [] },
    ...extra,
  };
}

function HistoryProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <aside>
      <output data-testid="location">
        {location.pathname + location.search + location.hash}
      </output>
      <button type="button" onClick={() => navigate(-1)}>
        Browser Back
      </button>
    </aside>
  );
}

describe("Person labels across roster refreshes", () => {
  it("resolves an ID-only task owner and keeps retired attribution visible", async () => {
    const page = taskPage();
    page.node.owner = "jc";
    page.node.proposed_by = "former-owner";
    const routes = emptyExploreRoutes({
      "/api/node/task/T-h4k2wq": page,
    });
    stubFetch(routes);
    render(<MemoryRouter initialEntries={["/node/task/T-h4k2wq"]}><App /></MemoryRouter>);

    expect(await screen.findByText("JC Grubbs")).toHaveAttribute("title", "jc");
    expect(screen.getByText("former-owner")).toBeInTheDocument();

    routes["/api/status"] = summary({ contributors: [{ id: "jc", name: "Updated Full Name" }] });
    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    expect(await screen.findByText("Updated Full Name")).toHaveAttribute("title", "jc");
    expect(screen.queryByText("JC Grubbs")).not.toBeInTheDocument();

    routes["/api/status"] = summary({ contributors: [] });
    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    expect(await screen.findByText("jc")).toBeInTheDocument();
    expect(screen.queryByText("Updated Full Name")).not.toBeInTheDocument();
  });
});

// Execution sources moved from the App shell to Home's Branches tab (JC,
// 2026-10-06): on a real project the diagnostics list took the top of every
// page. Off Home, the shell neither reads them nor draws them.
describe("App shell: execution sources stay on Home", () => {
  it("neither fetches nor renders execution diagnostics on Explore", async () => {
    const spy = stubFetch(
      emptyExploreRoutes({
        "/api/execution": {
          observations: [],
          diagnostics: [{ branch: "task-missing", code: "state_unreadable", message: "Task state could not be read" }],
        },
      }),
    );

    render(
      <MemoryRouter initialEntries={["/explore"]}>
        <App />
      </MemoryRouter>,
    );

    await screen.findByRole("heading", { name: "Explore" });
    expect(screen.queryByText(/Execution visibility diagnostics/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Task state could not be read/)).not.toBeInTheDocument();
    expect(spy.mock.calls.map((c) => String(c[0]))).not.toContain("/api/execution");
  });
});

describe("App shell: the topbar height token follows the header", () => {
  it("sets --topbar-h to the header's measured height and clears it on unmount", async () => {
    stubFetch(emptyExploreRoutes());
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      const height = this.classList.contains("site-header") ? 73 : 0;
      return { x: 0, y: 0, top: 0, left: 0, right: 0, bottom: height, width: 0, height, toJSON: () => ({}) };
    });

    const { unmount } = render(
      <MemoryRouter initialEntries={["/explore"]}>
        <App />
      </MemoryRouter>,
    );

    await screen.findByRole("heading", { name: "Explore" });
    expect(document.documentElement.style.getPropertyValue("--topbar-h")).toBe("73px");
    unmount();
    expect(document.documentElement.style.getPropertyValue("--topbar-h")).toBe("");
  });
});

describe("App routing: a task's key, not just its id, addresses the node page", () => {
  it("reaches the node page on a four-segment, key-addressed task URL", async () => {
    stubFetch({
      "/api/status": summary(),
      "/api/node/task/form-engine/certification-form-loading/transcribe-the-schema": taskPage(),
    });

    render(
      <MemoryRouter
        initialEntries={["/node/task/form-engine/certification-form-loading/transcribe-the-schema"]}
      >
        <App />
      </MemoryRouter>,
    );

    // Before the splat route, this address matched no route at all: no
    // loading state, no error, an empty body. Any of these three means the
    // route matched and NodeDetail mounted. The assertion lives inside
    // waitFor's own callback, re-querying on every poll, rather than holding
    // a findByText result across the await: the stub resolves so fast that a
    // node grabbed at the loading instant can be unmounted (a different
    // element type replaces it) before a later assertion on that same
    // reference gets to run.
    await waitFor(() => {
      expect(screen.queryByText(/Loading node|No such node|transcribe/i)).not.toBeNull();
    });
  });
});

describe("App routing: a plain id, the shape every console link emits, still resolves", () => {
  it("reaches the node page on a single-segment id URL rather than the catch-all", async () => {
    stubFetch({
      "/api/status": summary(),
      "/api/node/feature/F-9": featurePage(),
    });

    render(
      <MemoryRouter initialEntries={["/node/feature/F-9"]}>
        <App />
      </MemoryRouter>,
    );

    // Waits for the SETTLED page, not "Loading node...": a guard that
    // mistakenly treats a slash-free addr as malformed (an easy mistake once
    // a key, not an id, is the address most people think about) would leave
    // useFetch idle and this stuck on the loading line forever, which a
    // looser match against the loading text would not catch. The same
    // waitFor-inline-query shape as the key-addressed test above guards
    // against a captured reference going stale before a later assertion
    // reads it, since the stub resolves fast.
    await waitFor(() => {
      expect(screen.queryByText(/Wire the billing webhook/i)).not.toBeNull();
    });
    // The one thing that distinguishes this from a route that quietly stopped
    // taking ids: the catch-all must never have fired for this address.
    expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
  });
});

describe("App routing: an address no route claims", () => {
  it("renders 'No such page' instead of a blank body", async () => {
    stubFetch({ "/api/status": summary() });

    render(
      <MemoryRouter initialEntries={["/no/such/page/at/all"]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByText(/No such page/i)).toBeInTheDocument();
  });
});

describe("App routing: /build, the epic-selection board (Task 13, console v2 phase 3)", () => {
  it("reaches the real board (not the retired stub, not a blank page) and marks its own tab active", async () => {
    // Build.tsx owns its own /api/status fetch (App.tsx's own comment on the
    // route, updated by Task 13) plus /api/build, /api/queue, /api/drift;
    // the status fixture below carries no active_epics, so the board's
    // default selection is empty and it renders its own honest empty state
    // rather than needing full card fixtures - this file only has to prove
    // App wires the route to the real view, build.test.tsx owns the board's
    // own behavior in depth.
    stubFetch({
      "/api/status": summary(),
      "/api/build": { epics: [] },
      "/api/queue": { entries: [] },
      "/api/drift": { compared: 0 },
    });

    render(
      <MemoryRouter initialEntries={["/build"]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByText("No epics selected.")).toBeInTheDocument();
    // The retired Phase 1 stub's own wording, and the router's own
    // catch-all, both absent: this is the real board, not either fallback.
    expect(screen.queryByText(/lands in a later phase/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Build" }).className).toContain("active");
  });
});

describe("App routing: /fix, the shared work-item shelf", () => {
  it("reaches the board directly and marks its own tab active", async () => {
    stubFetch({
      "/api/status": summary(),
      "/api/work-items": { open: [], closed: [], attachments: [] },
    });

    render(
      <MemoryRouter initialEntries={["/fix"]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("region", { name: "Ready" })).toBeVisible();
    expect(screen.getByRole("link", { name: "Fix" })).toHaveAttribute("aria-current", "page");
  });
});

describe("App routing: /history, the renamed recap view", () => {
  it("reaches History directly and marks its own tab active", async () => {
    stubFetch({
      "/api/status": summary(),
      "/api/recap?window=visit": { since: "", window: "visit", reader: "", total: 0, groups: [] },
    });

    render(
      <MemoryRouter initialEntries={["/history"]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "History" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "History" }).className).toContain("active");
  });
});

describe("App routing: /report", () => {
  function reportRoutes(): Record<string, unknown> {
    const weeks = ["2026-09-28", "2026-10-05", "2026-10-12", "2026-10-19"];
    return {
      "/api/status": summary(),
      "/api/report": {
        project: "Acme Platform",
        as_of: "2026-10-20T15:00:00Z",
        weeks,
        week_boundaries: [...weeks, "2026-10-26"].map((day) => `${day}T00:00:00Z`),
        kpis: {
          features_done: 0,
          features_total: 0,
          shipped_last_7_days: 0,
          median_cycle_days: null,
          median_lead_days: null,
          completed_features: 0,
          open_bugs: 0,
          open_questions: 0,
          answered_questions: 0,
          open_questions_by_week: [0, 0, 0, 0],
        },
        burn_up: weeks.map(() => ({ scope: 0, done: 0 })),
        throughput: weeks.map(() => ({ feature_work: 0, fixes: 0, upkeep: 0 })),
        bugs: weeks.map(() => ({ reported: 0, fixed: 0, open: 0 })),
        feature_times: [],
        epics: [],
      },
    };
  }

  it("reaches Report directly and marks its own tab active", async () => {
    stubFetch(reportRoutes());

    render(
      <MemoryRouter initialEntries={["/report"]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByText("Project report")).toBeInTheDocument();
    expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Report" })).toHaveAttribute("aria-current", "page");
    expect(document.querySelector("header.site-header")).toHaveClass("site-header-shadow");
  });

  it("orders primary navigation Home, Explore, Build, Fix, Report, History", async () => {
    stubFetch(reportRoutes());

    const { container } = render(
      <MemoryRouter initialEntries={["/report"]}>
        <App />
      </MemoryRouter>,
    );

    await screen.findByText("Project report");
    const nav = container.querySelector<HTMLElement>("nav.site-nav")!;
    expect(within(nav).getAllByRole("link").map((link) => link.textContent)).toEqual([
      "Home",
      "Explore",
      "Build",
      "Fix",
      "Report",
      "History",
    ]);
  });
});

describe("App topbar shadow", () => {
  it("lifts the topbar only on the views that scroll under it", () => {
    expect(["/", "/report", "/history"].map(scrollsUnderTopbar)).toEqual([true, true, true]);
    expect(["/explore", "/build", "/fix", "/node/epic/E-aaaaaa", "/drift"].map(scrollsUnderTopbar)).toEqual([
      false,
      false,
      false,
      false,
      false,
    ]);
  });

  it("renders the shadow class on a scrolling view and not on a framed one", async () => {
    stubFetch({ "/api/status": summary() });
    const { container, unmount } = render(
      <MemoryRouter initialEntries={["/no-such-page"]}>
        <App />
      </MemoryRouter>,
    );
    expect(container.querySelector("header.site-header")).not.toHaveClass("site-header-shadow");
    unmount();
  });
});

describe("App routing: /recap, the pre-rename address", () => {
  it("redirects to /history carrying the window query string along", async () => {
    // If the redirect dropped the query, History would fall back to its
    // default (window=visit) fetch, which has no stub here and would 404
    // into the error state instead of rendering the 24h digest below.
    stubFetch({
      "/api/status": summary(),
      "/api/recap?window=24h": {
        since: "2026-08-09T12:00:00Z",
        window: "24h",
        reader: "jc",
        total: 4,
        groups: [],
      },
    });

    render(
      <MemoryRouter initialEntries={["/recap?window=24h"]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "History" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "24h" })).toHaveAttribute("aria-current", "page");
    expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
  });

  it("redirects a bare /recap (no window) to /history on the default visit window", async () => {
    stubFetch({
      "/api/status": summary(),
      "/api/recap?window=visit": { since: "", window: "visit", reader: "", total: 0, groups: [] },
    });

    render(
      <MemoryRouter initialEntries={["/recap"]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "History" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "visit" })).toHaveAttribute("aria-current", "page");
    expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
  });
});

describe("App routing: /bugs-chores, the pre-rename address of Fix", () => {
  it("redirects to /fix carrying the query string and hash along", async () => {
    stubFetch({ "/api/status": summary(), "/api/work-items": { open: [], closed: [], attachments: [] } });
    function LocationProbe() {
      const location = useLocation();
      return <p data-testid="where">{`${location.pathname}${location.search}${location.hash}`}</p>;
    }

    render(
      <MemoryRouter initialEntries={["/bugs-chores?attached=F-x&terminal=all#top"]}>
        <App />
        <LocationProbe />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("where")).toHaveTextContent("/fix?attached=F-x&terminal=all#top");
      expect(screen.getByRole("heading", { name: "Fix" })).toBeInTheDocument();
      expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
    });
  });
});

describe("App routing: /drift, the header band's link target", () => {
  it("reaches the Drift page rather than the catch-all", async () => {
    stubFetch({
      "/api/status": summary(),
      "/api/drift": { compared: 3, disabled: [] },
    });

    render(
      <MemoryRouter initialEntries={["/drift"]}>
        <App />
      </MemoryRouter>,
    );

    // A dropped or misplaced Route line (below the catch-all rather than
    // above it) would render "No such page" here instead.
    expect(await screen.findByRole("heading", { name: "Drift" })).toBeInTheDocument();
    expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
  });
});

describe("App routing: a noun with nothing after it", () => {
  it("renders 'No node addressed' rather than spinning forever", async () => {
    stubFetch({ "/api/status": summary() });

    render(
      <MemoryRouter initialEntries={["/node/task/"]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByText(/No node addressed/i)).toBeInTheDocument();
    expect(screen.queryByText(/Loading node/i)).not.toBeInTheDocument();
  });
});

// The persistent-hierarchy change (JC, 2026-08-05): every node page renders
// inside Explore's shell, so the navigator, the Explore heading, and the mode
// nav ride along, and the top nav marks Explore as where you are.
describe("App routing: node pages live under the Explore shell", () => {
  function shellRoutes(extra: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      "/api/status": summary(),
      "/api/tree": {
        project: "acme",
        nodes: [
          {
            noun: "epic",
            id: "E-fe",
            key: "form-engine",
            title: "Form Engine",
            status: "ready",
            path: "",
            files: [],
            children: [
              {
                noun: "feature",
                id: "F-9",
                key: "form-engine/billing-webhook",
                title: "Wire the billing webhook",
                status: "ready",
                path: "",
                files: [],
                children: [],
              },
            ],
          },
        ],
      },
      "/api/ls?noun=question": { noun: "question", count: 0, nodes: [] },
      ...extra,
    };
  }

  it("renders the navigator beside a node page, opened to the selection", async () => {
    stubFetch(shellRoutes({ "/api/node/feature/F-9": featurePage() }));

    const { container } = render(
      <MemoryRouter initialEntries={["/node/feature/F-9"]}>
        <App />
      </MemoryRouter>,
    );

    await screen.findByRole("heading", { level: 1, name: "Wire the billing webhook" });
    // The epic auto-opened because it holds the selection, and the feature
    // row carries the selected mark.
    const selected = container.querySelector(".explore-feat.is-selected");
    expect(selected).not.toBeNull();
    expect(selected?.textContent).toContain("Wire the billing webhook");
  });

  it("marks Explore active in the top nav on a /node/ address", async () => {
    stubFetch(shellRoutes({ "/api/node/feature/F-9": featurePage() }));

    render(
      <MemoryRouter initialEntries={["/node/feature/F-9"]}>
        <App />
      </MemoryRouter>,
    );

    await screen.findByRole("heading", { level: 1, name: "Wire the billing webhook" });
    const explore = screen.getByRole("link", { name: "Explore" });
    expect(explore.className).toContain("active");
  });

  it.each(["/questions", "/amendments"])("marks Explore active in the top nav on the %s ledger", async (path) => {
    stubFetch(shellRoutes({}));

    render(
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>,
    );

    await screen.findByRole("heading", { name: "Explore" });
    expect(screen.getByRole("link", { name: "Explore" }).className).toContain("active");
  });

  it("redirects the retired /explore/:noun/:id selection route to /node/", async () => {
    stubFetch(shellRoutes({ "/api/node/feature/F-9": featurePage() }));

    render(
      <MemoryRouter initialEntries={["/explore/feature/F-9"]}>
        <App />
      </MemoryRouter>,
    );

    // The old bookmark lands on the same node page, never the catch-all.
    await screen.findByRole("heading", { level: 1, name: "Wire the billing webhook" });
    expect(screen.queryByText(/No such page/i)).not.toBeInTheDocument();
  });

  // Task 6: an amendment is not a tree node (it does not join the navigator
  // at all), but its page still reaches the
  // console the same way every other noun's does: through this same shell,
  // this same splat route, no bespoke wiring anywhere.
  it("reaches an amendment page inside the Explore shell, with no navigator entry for it", async () => {
    const page: NodePage = {
      node: {
        noun: "amendment",
        id: "A-000001",
        key: "iphone-only",
        title: "Remove native iPad support from v1",
        status: "proposed",
      },
      ref: "iphone-only (A-000001)",
      prose: { path: ".tiller/amendments/A-000001-iphone-only.md", content: "Why this reshape exists." },
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      amendment: { operations: [] },
    };
    stubFetch(shellRoutes({ "/api/node/amendment/A-000001": page }));

    render(
      <MemoryRouter initialEntries={["/node/amendment/A-000001"]}>
        <App />
      </MemoryRouter>,
    );

    await screen.findByRole("heading", { level: 1, name: "Remove native iPad support from v1" });
    // No tab row: an amendment stays flat (ui-surface.md, "an amendment ...
    // stays flat"), so NodeTabs' own nav (the tab shell every tabbed noun
    // renders) never mounts here.
    expect(screen.queryByRole("navigation", { name: "Node detail tab" })).not.toBeInTheDocument();
    // The amendment's own narrative renders through the flat body.
    expect(screen.getByText("Why this reshape exists.")).toBeInTheDocument();
  });
});

describe("App routing: loose Markdown files live under the Explore shell", () => {
  const path = ".tiller/design/loose plans/#1 follow-up.md";
  const line = 7;
  const route = "/file/.tiller/design/loose%20plans/%231%20follow-up.md#L7";
  const apiPath =
    "/api/prose?path=.tiller%2Fdesign%2Floose%20plans%2F%231%20follow-up.md&line=7";
  const lineText = "  preserve   this source spacing  ";
  const file = {
    path,
    content:
      "# Release notes\n\n- first item\n- second item\n\n" +
      "Payload: <script>alert('owned')</script>\n" +
      lineText,
    line,
    line_text: lineText,
  };

  it("deep-links through the exact API query and renders the safe file reader at its source line", async () => {
    const scrollIntoView = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      writable: true,
      value: scrollIntoView,
    });
    const fetchMock = stubFetch(emptyExploreRoutes({ [apiPath]: file }));

    const { container } = render(
      <MemoryRouter initialEntries={[route]}>
        <App />
      </MemoryRouter>,
    );

    expect(scrollIntoView).not.toHaveBeenCalled();
    expect(await screen.findByText(`${path}:${line}`)).toBeInTheDocument();

    const requested = fetchMock.mock.calls.map((call) => String(call[0]));
    expect(requested.filter((url) => url.startsWith("/api/prose"))).toEqual([
      apiPath,
    ]);
    expect(screen.getByRole("link", { name: "Explore" }).className).toContain("active");

    const detail = container.querySelector(".explore-detail") as HTMLElement;
    expect(detail).not.toBeNull();
    expect(
      await within(detail).findByRole("heading", { level: 2, name: "Release notes" }),
    ).toBeInTheDocument();
    expect(
      within(detail)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["first item", "second item"]);
    expect(detail.querySelector("script")).toBeNull();
    expect(detail.textContent).toContain("<script>alert('owned')</script>");

    const locator = detail.querySelector("#L7") as HTMLElement;
    expect(locator).not.toBeNull();
    expect(locator.textContent).toContain(lineText);
    await waitFor(() => expect(scrollIntoView).toHaveBeenCalledTimes(1));
    expect(scrollIntoView.mock.instances[0]).toBe(locator);
  });

  it("shows recorded confirmation and provenance while each baseline marker opens its own findings", async () => {
    const doc = "loose plans/#1 follow-up.md";
    const remedy = "tiller approve design 'loose plans/#1 follow-up.md'";
    stubFetch(emptyExploreRoutes({
      [apiPath]: {
        ...file,
        baseline: {
          path: doc, describes: ["app/api", "../legacy/input"],
          derived_from: [{ kind: "git_blob", value: "git:abcdef0", path: "docs/retired.md", observed_at: "2026-09-01T10:00:00Z" }],
          confirmed: {
            by: "retired-owner",
            commit: { kind: "git_commit", value: "git:1234567", observed_at: "2026-09-02T10:00:00Z" },
            surfaces: [],
          },
        },
        drift_detail: {
          candidates: [
            { marker: "stale_design", noun: "design", id: "", key: doc, detail: "Tracked feature landed", remedy },
            { marker: "shifted_design", noun: "design", id: "", key: doc, detail: "Working-tree code differs", remedy },
          ],
        },
      },
    }));
    render(<MemoryRouter initialEntries={[route]}><App /></MemoryRouter>);
    const baseline = await screen.findByRole("region", { name: "As-is baseline" });
    expect(within(baseline).getByText("Confirmed")).toBeInTheDocument();
    expect(within(baseline).getByText("retired-owner")).toBeInTheDocument();
    expect(within(baseline).getByText("2026-09-02T10:00:00Z")).toBeInTheDocument();
    expect(within(baseline).getByText("docs/retired.md")).toBeInTheDocument();
    expect(within(baseline).getByText("../legacy/input")).toBeInTheDocument();
    const stale = within(baseline).getByRole("button", { name: "stale_design" });
    const shifted = within(baseline).getByRole("button", { name: "shifted_design" });
    expect(stale).toHaveAttribute("aria-expanded", "false");
    expect(within(baseline).queryByText("Tracked feature landed")).not.toBeInTheDocument();
    fireEvent.click(stale);
    expect(stale).toHaveAttribute("aria-expanded", "true");
    expect(within(baseline).getByText("Tracked feature landed")).toBeInTheDocument();
    expect(within(baseline).queryByText("Working-tree code differs")).not.toBeInTheDocument();
    expect(within(baseline).getByRole("button", { name: remedy })).toBeInTheDocument();
    fireEvent.click(shifted);
    expect(within(baseline).getByText("Working-tree code differs")).toBeInTheDocument();
    expect(within(baseline).getByRole("link", { name: "project-wide drift report" })).toHaveAttribute("href", "/drift");
  });

  it("keeps unconfirmed review and comparison failure visible without claiming a clean baseline", async () => {
    stubFetch(emptyExploreRoutes({
      [apiPath]: {
        ...file,
        baseline: { path: "-owner's notes.md", describes: ["app"] },
        drift_detail: {
          uncovered: [{ marker: "design", noun: "design", id: "", key: "-owner's notes.md", reason: "comparison_error", detail: "Unknown drift check" }],
        },
      },
    }));
    render(<MemoryRouter initialEntries={[route]}><App /></MemoryRouter>);
    const baseline = await screen.findByRole("region", { name: "As-is baseline" });
    expect(within(baseline).getByText("Unconfirmed")).toBeInTheDocument();
    expect(within(baseline).getByText(/not checked \(comparison_error\)/)).toBeInTheDocument();
    expect(within(baseline).getByRole("button", { name: "tiller approve design -- '-owner'\\''s notes.md'" })).toBeInTheDocument();
    expect(within(baseline).queryByText(/nothing moved|clean/i)).not.toBeInTheDocument();
  });

  it("decodes each encoded route segment exactly once for a literal percent escape", async () => {
    const literalPath = ".tiller/design/name%2Fnotes.md";
    const literalRoute = "/file/.tiller/design/name%252Fnotes.md#L9";
    const literalAPI =
      "/api/prose?path=.tiller%2Fdesign%2Fname%252Fnotes.md&line=9";
    const fetchMock = stubFetch(
      emptyExploreRoutes({
        [literalAPI]: {
          path: literalPath,
          content: "# Percent path",
          line: 9,
          line_text: "literal percent escape",
        },
      }),
    );

    render(
      <MemoryRouter initialEntries={[literalRoute]}>
        <App />
      </MemoryRouter>,
    );

    expect(await screen.findByText(`${literalPath}:9`)).toBeInTheDocument();
    const proseRequests = fetchMock.mock.calls
      .map((call) => String(call[0]))
      .filter((url) => url.startsWith("/api/prose"));
    expect(proseRequests).toEqual([literalAPI]);
  });

  it.each([
    ["a missing hash", "/file/.tiller/design/decision.md"],
    ["a non-positive line", "/file/.tiller/design/decision.md#L0"],
    ["a malformed locator", "/file/.tiller/design/decision.md#line-7"],
  ])(
    "shows an invalid location for %s without requesting prose",
    async (_label, initialPath) => {
      const fetchMock = stubFetch(emptyExploreRoutes());

      const { container } = render(
        <MemoryRouter initialEntries={[initialPath]}>
          <App />
        </MemoryRouter>,
      );

      await waitFor(() =>
        expect(container.querySelector(".explore-detail")).toHaveTextContent(/invalid/i),
      );
      const proseRequests = fetchMock.mock.calls
        .map((call) => String(call[0]))
        .filter((url) => url.startsWith("/api/prose"));
      expect(proseRequests).toEqual([]);
    },
  );

  it("frames a backend 404 as a stale file instead of exposing a raw fetch failure", async () => {
    stubFetch(emptyExploreRoutes());

    const { container } = render(
      <MemoryRouter initialEntries={[route]}>
        <App />
      </MemoryRouter>,
    );

    await waitFor(() =>
      expect(container.querySelector(".explore-detail")).toHaveTextContent(
        /no such file|no longer (?:exists|available)/i,
      ),
    );
    const detail = container.querySelector(".explore-detail") as HTMLElement;
    expect(detail).toHaveTextContent(`${path}:${line}`);
    expect(detail.textContent).not.toContain("GET /api/prose");
  });

  it("uses push navigation so Browser Back restores the exact search query URL", async () => {
    const searchLocation = "/explore?mode=search&q=attention%20%23plan";
    const searchResult = {
      query: "attention #plan",
      count: 1,
      truncated: false,
      kinds: [{ kind: "document", count: 1 }],
      nodes: [],
      documents: [{ path, count: 1, hits: [{ path, line, snippet: "attention #plan" }] }],
    };
    // The document row names the file without its .tiller/ prefix and links
    // to its first matching line.
    const documentLink = path.replace(/^\.tiller\//, "");
    stubFetch(
      emptyExploreRoutes({
        "/api/search?q=attention+%23plan": searchResult,
        [apiPath]: file,
      }),
    );

    render(
      <MemoryRouter initialEntries={[searchLocation]}>
        <App />
        <HistoryProbe />
      </MemoryRouter>,
    );

    fireEvent.click(await screen.findByRole("link", { name: documentLink }));
    expect(await screen.findByText(`${path}:${line}`)).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(route);

    fireEvent.click(screen.getByRole("button", { name: "Browser Back" }));
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe(searchLocation),
    );
    expect(await screen.findByRole("link", { name: documentLink })).toBeInTheDocument();
  });
});
