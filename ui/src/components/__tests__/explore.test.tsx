// Vitest/RTL coverage for task C2, Explore rebuilt as navigator plus reading
// pane (the retired console mock, "Explore,
// refreshed with #27 order"). Written red-first per the branch's TDD
// discipline.
//
// The brief's own required scenarios: epics render collapsed in served
// (dependency) order, never re-sorted alphabetically, against a fixture
// shaped like the real Acme graph (#27's Foundation/Ops&Security-lead,
// Audit-Engine-trails shape); expanding an epic reveals its features as
// one-line rows; selecting a node changes the route and renders the reading
// pane through the exact same components/NodeDetail instrument C3 built
// (never a second bespoke render of node content); and search hits keep
// their <mark> highlight after the row restyle. Plus one of the branch's
// other binding rules: a failed tree fetch surfaces its error honestly
// rather than rendering an empty navigator.
//
// The navigator's Effort section left with the effort axis, and its tests
// with it. Fix loop 1's surviving additions: manual epic expansion survives
// the first selection navigation (the Explore layout route stays mounted
// across /explore and every /node/:noun/* page, so navigator state is
// preserved - pinned here so a future route change that breaks it fails
// loudly); and the reading pane frames a 404 (a stale deep link) through
// views/NodeDetail's own not-found surface while non-404 failures keep the
// honest raw error.
//
// These render through <App/> rather than a hand-built route table: the
// persistent-hierarchy change (JC, 2026-08-05) made the route structure
// itself the contract under test - a pathless layout owning the navigator
// with /explore and /node/:noun/* as its children - and a copy of that
// structure here would let the copy pass while App drifts.

import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  Facet,
  FacetOption,
  ListResult,
  ListRow,
  NodePage,
  ProjectSummary,
  SearchResult,
  TreeModel,
  TreeNode,
} from "../../api";
import App from "../../App";
import RelatedList from "../../views/node/RelatedList";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  // Task 4 (docs/spec/ui-surface.md) makes Explore
  // persist the expansion map and the split width to localStorage, keyed by
  // project ("acme" throughout this file's fixtures) and flat
  // respectively - without this clear, a later test in this file would
  // restore whatever an earlier test left behind instead of starting from
  // today's real-world default (nothing stored), the same pollution
  // build.test.tsx's own afterEach already guards against for its sibling
  // persistence.
  window.localStorage.clear();
});

const QUESTION_TITLE = "Audit selection unit";
const QUESTION_STATEMENT =
  "Should the audit select one record per project or one record per project phase?\n\n" +
  "The answer changes how multi-year projects are counted in review queues and exported reports.";

// A fetch stub keyed by URL, matching home.test.tsx's own established
// pattern: an unmapped path 404s the way fetchJSON expects, so a missing
// fixture fails loudly instead of silently returning undefined. Returns the
// mock so a test can assert which URLs were (or were never) requested.
function stubFetch(routes: Record<string, unknown>, errors: Record<string, string> = {}) {
  const fetchMock = vi.fn((input: unknown) => {
    const url = String(input);
    if (url in errors) {
      return Promise.reject(new Error(errors[url]));
    }
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
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

// The same stub, but any path named in `delayedPaths` resolves on a real
// setTimeout instead of an already-resolved promise. stubFetch alone cannot
// produce a real arrival-order difference between two routes: every route
// there resolves via Promise.resolve, so both land in the very same
// microtask drain and Explore's own /api/tree and /api/status fetches
// always settle together, in the same commit - the one arrangement that
// happens to hide a Critical bug (a same-commit restore and
// auto-open compose or clobber in whatever order they were declared, and
// declaration order alone was enough to paper over it). Delaying one route
// for real is what actually separates the two fetches into distinct
// commits, the arrangement the bug needs to reproduce and the fix needs to
// survive.
function stubFetchWithDelay(routes: Record<string, unknown>, delayedPaths: string[], delayMs = 20) {
  const fetchMock = vi.fn((input: unknown) => {
    const url = String(input);
    if (!(url in routes)) {
      return Promise.resolve({
        ok: false,
        status: 404,
        statusText: "Not Found",
        text: () => Promise.resolve(""),
      });
    }
    const response = { ok: true, json: () => Promise.resolve(routes[url]) };
    if (delayedPaths.includes(url)) {
      return new Promise((resolve) => {
        setTimeout(() => resolve(response), delayMs);
      });
    }
    return Promise.resolve(response);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function emptyList(noun: string): ListResult {
  return { noun, count: 0, nodes: [] };
}

const bugRow: ListRow = {
  id: "B-a",
  key: "bugs/a",
  title: "Blank on reload",
  status: "ready",
  ref: "bugs/a (B-a)",
};

const choreRow: ListRow = {
  id: "CH-a",
  key: "chores/a",
  title: "Bump Go",
  status: "proposed",
  ref: "chores/a (CH-a)",
};

// A shelf row's glyph sits inside its link, so the link's accessible name
// leads with the status word (StateGlyph's own label): match on the title.
function shelfRow(title: string | undefined): RegExp {
  return new RegExp(`${title}$`);
}

function listResult(
  noun: string,
  rows: Array<Pick<ListRow, "id" | "status"> & Partial<Omit<ListRow, "id" | "status">>>,
): ListResult {
  const nodes = rows.map((row) => ({ ref: row.key ?? row.id, ...row }));
  return { noun, count: nodes.length, nodes };
}

function featureNode(overrides: Partial<TreeNode> = {}): TreeNode {
  return {
    noun: "feature",
    id: "F-x",
    status: "draft",
    path: "",
    files: [],
    children: [],
    ...overrides,
  };
}

function taskNode(overrides: Partial<TreeNode> = {}): TreeNode {
  return {
    noun: "task",
    id: "T-x",
    status: "proposed",
    path: "",
    files: [],
    children: [],
    ...overrides,
  };
}

function epicNode(overrides: Partial<TreeNode> = {}): TreeNode {
  return {
    noun: "epic",
    id: "E-x",
    status: "ready",
    path: "",
    files: [],
    children: [],
    ...overrides,
  };
}

// Foundation's own seven features, the exact Acme shape the later
// mock renders: one ready first-mover (App Skeleton) and six specced behind
// it, in the served dependency order the identity spine implies.
function foundationFeatures(): TreeNode[] {
  return [
    // The one feature carrying a task breakdown, in the served sibling order
    // gen.OrderTasks ranks: boot before its health check, which depends on it.
    featureNode({
      id: "F-pueftm",
      key: "foundation/app-skeleton-ci-boot",
      title: "App Skeleton & Green CI Boot",
      status: "ready",
      children: [
        taskNode({
          id: "T-boot",
          key: "foundation/app-skeleton-ci-boot/boot-the-app",
          title: "Boot the app",
          status: "in_progress",
        }),
        taskNode({
          id: "T-health",
          key: "foundation/app-skeleton-ci-boot/serve-the-health-check",
          title: "Serve the health check",
        }),
      ],
    }),
    featureNode({
      id: "F-2",
      key: "foundation/identity-roles-program-model",
      title: "Identity, Roles & Program Model",
      status: "specced",
    }),
    featureNode({
      id: "F-3",
      key: "foundation/multiprogram-tenancy-isolation",
      title: "Multi-Program Tenancy & DB Isolation",
      status: "specced",
    }),
    featureNode({
      id: "F-4",
      key: "foundation/authentication-devise-mfa",
      title: "Devise Authentication & TOTP MFA",
      status: "specced",
    }),
    featureNode({
      id: "F-5",
      key: "foundation/authorization-pundit-policies",
      title: "Pundit Authorization & Namespace Gating",
      status: "specced",
    }),
    featureNode({
      id: "F-6",
      key: "foundation/design-system-layout-shell",
      title: "Design System & Five-Namespace Shell",
      status: "specced",
    }),
    featureNode({
      id: "F-7",
      key: "foundation/program-settings-surface",
      title: "Acme Program Settings",
      status: "specced",
    }),
  ];
}

// Served (dependency) order per #27 and the mock's own illustration:
// Foundation and Ops & Security lead as the two roots; Audit Engine sits
// much later, after five other epics in the real run (trimmed here to the
// three that matter for the assertion). Alphabetically "Audit Engine" would
// sort before "Foundation", so asserting Foundation renders first proves
// real served-order pass-through rather than a lucky or client-re-sorted
// alphabetical pass.
function acmeTree(): TreeModel {
  return {
    project: "acme",
    nodes: [
      epicNode({
        id: "E-u2w9wv",
        key: "foundation",
        title: "Foundation",
        status: "ready",
        children: foundationFeatures(),
      }),
      epicNode({
        id: "E-ops1",
        key: "ops-security",
        title: "Ops & Security",
        status: "ready",
        children: [featureNode({ id: "F-ops1", key: "ops-security/first", title: "First Ops Feature" })],
      }),
      epicNode({
        id: "E-audit1",
        key: "audit-engine",
        title: "Audit Engine",
        status: "draft",
        children: [featureNode({ id: "F-aud1", key: "audit-engine/first", title: "First Audit Feature" })],
      }),
    ],
  };
}

function projectSummary(): ProjectSummary {
  return {
    project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
    contributors: [{ id: "jc", name: "JC Grubbs" }],
    counts: { epics: 3, features: 9, tasks: 2, questions: 0 },
    whose_move: { human: 0, agent: 0 },
  };
}

function baseRoutes(extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    "/api/status": projectSummary(),
    "/api/tree": acmeTree(),
    "/api/ls?noun=question": emptyList("question"),
    "/api/ls?noun=bug": emptyList("bug"),
    "/api/ls?noun=chore": emptyList("chore"),
    "/api/ls?noun=tweak": emptyList("tweak"),
    ...extra,
  };
}

interface ExploreRenderOptions {
  route?: string;
  bugs?: ListResult;
  chores?: ListResult;
  tweaks?: ListResult;
  bugsError?: string;
}

function renderExplore(input: string | ExploreRenderOptions = "/explore") {
  const options = typeof input === "string" ? null : input;
  if (options !== null) {
    stubFetch(
      baseRoutes({
        "/api/ls?noun=bug": options.bugs ?? emptyList("bug"),
        "/api/ls?noun=chore": options.chores ?? emptyList("chore"),
        "/api/ls?noun=tweak": options.tweaks ?? emptyList("tweak"),
      }),
      options.bugsError === undefined ? {} : { "/api/ls?noun=bug": options.bugsError },
    );
  }
  const initialPath = options?.route ?? (typeof input === "string" ? input : "/explore");
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <App />
    </MemoryRouter>,
  );
}

describe("Navigator: Bugs and Chores shelf branches", () => {
  it("opens Tweak shelf work and links TW addresses independently", async () => {
    renderExplore({ tweaks: listResult("tweak", [{ id: "TW-abc123", key: "tweaks/button", title: "Adjust button spacing", status: "ready" }]) });
    const nav = within(await screen.findByRole("navigation", { name: "Definition navigator" }));
    const expand = await nav.findByRole("button", { name: "Expand Tweaks" });
    expect(nav.getByRole("link", { name: "Tweaks" })).toHaveAttribute("href", "/fix?noun=tweak");
    expect(nav.queryByRole("link", { name: shelfRow("Adjust button spacing") })).not.toBeInTheDocument();
    fireEvent.click(expand);
    expect(nav.getByRole("link", { name: shelfRow("Adjust button spacing") })).toHaveAttribute("href", "/node/tweak/TW-abc123");
    fireEvent.click(nav.getByRole("button", { name: "Close all" }));
    expect(nav.queryByRole("link", { name: shelfRow("Adjust button spacing") })).not.toBeInTheDocument();
    fireEvent.click(nav.getByRole("button", { name: "Open all" }));
    expect(nav.getByRole("link", { name: shelfRow("Adjust button spacing") })).toBeVisible();
  });
  it("renders Bugs and Chores as independent top-level branches", async () => {
    renderExplore({
      bugs: listResult("bug", [
        { id: "B-a", key: "bugs/a", title: "Blank on reload", status: "ready" },
      ]),
      chores: listResult("chore", [
        { id: "CH-a", key: "chores/a", title: "Bump Go", status: "proposed" },
      ]),
    });

    const bugs = await screen.findByRole("link", { name: "Bugs" });
    const chores = screen.getByRole("link", { name: "Chores" });
    expect(bugs).toHaveAttribute("href", "/fix?noun=bug");
    expect(chores).toHaveAttribute("href", "/fix?noun=chore");

    fireEvent.click(screen.getByRole("button", { name: "Expand Bugs" }));
    expect(screen.getByRole("link", { name: shelfRow("Blank on reload") })).toHaveAttribute(
      "href",
      "/node/bug/B-a",
    );
  });


  it("keeps the tree and other shelf visible when one shelf fetch fails", async () => {
    renderExplore({ bugsError: "bugs unreadable", chores: listResult("chore", [choreRow]) });

    expect(await screen.findByText("bugs unreadable")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Foundation" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Chores" })).toBeInTheDocument();
  });

  it("auto-opens the owning shelf for a deep-linked bug", async () => {
    renderExplore({ route: "/node/bug/B-a", bugs: listResult("bug", [bugRow]) });

    expect(await screen.findByRole("button", { name: "Collapse Bugs" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: shelfRow("Blank on reload") })).toHaveClass("current");
  });
  it("auto-opens the owning shelf for a deep-linked TW address", async () => {
    renderExplore({ route: "/node/tweak/TW-abc123", tweaks: listResult("tweak", [{ id: "TW-abc123", key: "tweaks/button", title: "Adjust button spacing", status: "ready" }]) });
    expect(await screen.findByRole("button", { name: "Collapse Tweaks" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: shelfRow("Adjust button spacing") })).toHaveClass("current");
  });

  it("filters Tweak work and preserves TW node navigation", async () => {
    const tweak = { id: "TW-abc123", key: "tweaks/button", title: "Adjust button spacing", status: "ready", ref: "tweaks/button (TW-abc123)" };
    const fetchMock = stubFetch(baseRoutes({ "/api/ls?noun=tweak&status=ready": listResult("tweak", [tweak]) }));
    renderExplore("/explore?mode=filter&noun=tweak&status=ready");
    expect(await screen.findByRole("link", { name: /Adjust button spacing/ })).toHaveAttribute("href", "/node/tweak/TW-abc123");
    expect(screen.getByRole("combobox", { name: "Noun" })).toHaveValue("tweak");
    expect(fetchMock.mock.calls.map((call) => String(call[0]))).toContain("/api/ls?noun=tweak&status=ready");
  });
});

describe("Navigator: epics render collapsed, in served (dependency) order", () => {
  it("places the shelf branches below the epics", async () => {
    stubFetch(baseRoutes());

    renderExplore();

    const auditEngine = await screen.findByText("Audit Engine");
    for (const name of ["Bugs", "Chores", "Tweaks"]) {
      const shelf = await screen.findByRole("link", { name });
      expect(Boolean(auditEngine.compareDocumentPosition(shelf) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(true);
    }
  });

  it("renders epic titles in served order, not alphabetical, with feature rows collapsed", async () => {
    stubFetch(baseRoutes());

    renderExplore();

    const foundation = await screen.findByText("Foundation");
    const auditEngine = await screen.findByText("Audit Engine");
    // Served order puts Foundation first; alphabetical order would put Audit
    // Engine first. A pass here proves real served-order pass-through.
    expect(
      Boolean(foundation.compareDocumentPosition(auditEngine) & Node.DOCUMENT_POSITION_FOLLOWING),
    ).toBe(true);

    // Collapsed by default: no feature title is in the document yet.
    expect(screen.queryByText("App Skeleton & Green CI Boot")).not.toBeInTheDocument();
    expect(screen.queryByText("Acme Program Settings")).not.toBeInTheDocument();

    // Nothing selected yet: the reading pane says so rather than sitting
    // blank or guessing a default.
    expect(screen.getByText(/select a node/i)).toBeInTheDocument();
  });
});

describe("Navigator: expanding an epic shows one-line feature rows", () => {
  it("reveals every feature title, in served order, after the chevron toggles open", async () => {
    stubFetch(baseRoutes());

    renderExplore();

    await screen.findByText("Foundation");
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));

    const first = await screen.findByText("App Skeleton & Green CI Boot");
    const last = screen.getByText("Acme Program Settings");
    expect(screen.getByText("Identity, Roles & Program Model")).toBeInTheDocument();
    expect(Boolean(first.compareDocumentPosition(last) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(
      true,
    );

    // Collapsing again hides the rows without losing the epic row itself.
    fireEvent.click(screen.getByRole("button", { name: /Collapse Foundation/i }));
    expect(screen.queryByText("App Skeleton & Green CI Boot")).not.toBeInTheDocument();
    expect(screen.getByText("Foundation")).toBeInTheDocument();
  });
});

describe("Navigator + reading pane: selecting a node routes and renders NodeDetail", () => {
  it("navigates to /node/feature/:id and renders the selected node through NodeDetail", async () => {
    const page: NodePage = {
      node: {
        noun: "feature",
        id: "F-pueftm",
        key: "foundation/app-skeleton-ci-boot",
        title: "App Skeleton & Green CI Boot",
        status: "ready",
      },
      ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/feature/F-pueftm": page }));

    renderExplore();

    await screen.findByText("Foundation");
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    fireEvent.click(await screen.findByText("App Skeleton & Green CI Boot"));

    // The reading pane renders the fetched NodePage through NodeDetail's own
    // header (a single h1), not a second bespoke render of the same fact -
    // disambiguated from the navigator's own row, which is a plain link.
    expect(
      await screen.findByRole("heading", { level: 1, name: "App Skeleton & Green CI Boot" }),
    ).toBeInTheDocument();
    expect(screen.getByText("foundation/app-skeleton-ci-boot (F-pueftm)")).toBeInTheDocument();
  });

  it("selects an epic directly from its title, with no expansion required first", async () => {
    const page: NodePage = {
      node: { noun: "epic", id: "E-u2w9wv", key: "foundation", title: "Foundation", status: "ready" },
      ref: "foundation (E-u2w9wv)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/epic/E-u2w9wv": page }));

    renderExplore();

    fireEvent.click(await screen.findByText("Foundation"));

    expect(await screen.findByRole("heading", { level: 1, name: "Foundation" })).toBeInTheDocument();
    expect(screen.getByText("foundation (E-u2w9wv)")).toBeInTheDocument();
  });

  it("renders the selection directly from a deep-linked URL", async () => {
    const page: NodePage = {
      node: {
        noun: "feature",
        id: "F-pueftm",
        key: "foundation/app-skeleton-ci-boot",
        title: "App Skeleton & Green CI Boot",
        status: "ready",
      },
      ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/feature/F-pueftm": page }));

    renderExplore("/node/feature/F-pueftm");

    expect(
      await screen.findByRole("heading", { level: 1, name: "App Skeleton & Green CI Boot" }),
    ).toBeInTheDocument();
  });
});

describe("Navigator: the third tier, tasks under their feature", () => {
  // ui-surface.md's browse mode is "the work tree, epics to features to tasks",
  // and a task is a folder node with a key and a page of its own now, so the
  // navigator has to reach it. The rows arrive in the served sibling order and
  // are never re-sorted here, the same rule the two tiers above keep.
  it("reveals a feature's tasks in served order behind its own chevron", async () => {
    stubFetch(baseRoutes());

    renderExplore();

    await screen.findByText("Foundation");
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    await screen.findByText("App Skeleton & Green CI Boot");

    // Collapsed by default: a feature's tasks are one more disclosure down.
    expect(screen.queryByText("Boot the app")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Expand App Skeleton & Green CI Boot/i }));

    const first = await screen.findByText("Boot the app");
    const second = screen.getByText("Serve the health check");
    expect(Boolean(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(
      true,
    );

    fireEvent.click(screen.getByRole("button", { name: /Collapse App Skeleton & Green CI Boot/i }));
    expect(screen.queryByText("Boot the app")).not.toBeInTheDocument();
    expect(screen.getByText("App Skeleton & Green CI Boot")).toBeInTheDocument();
  });

  it("draws no chevron on a feature with no tasks", async () => {
    stubFetch(baseRoutes());

    renderExplore();

    await screen.findByText("Foundation");
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    await screen.findByText("Acme Program Settings");

    // A disclosure that opens onto nothing is furniture; the other six
    // Foundation features carry no tasks and so carry no chevron.
    expect(
      screen.queryByRole("button", { name: /Expand Acme Program Settings/i }),
    ).not.toBeInTheDocument();
  });

  it("selects a task and renders it through the same NodeDetail instrument", async () => {
    const page: NodePage = {
      node: {
        noun: "task",
        id: "T-boot",
        key: "foundation/app-skeleton-ci-boot/boot-the-app",
        title: "Boot the app",
        status: "in_progress",
      },
      ref: "foundation/app-skeleton-ci-boot/boot-the-app (T-boot)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/task/T-boot": page }));

    renderExplore();

    await screen.findByText("Foundation");
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    await screen.findByText("App Skeleton & Green CI Boot");
    fireEvent.click(screen.getByRole("button", { name: /Expand App Skeleton & Green CI Boot/i }));
    fireEvent.click(await screen.findByText("Boot the app"));

    expect(await screen.findByRole("heading", { level: 1, name: "Boot the app" })).toBeInTheDocument();
    expect(
      screen.getByText("foundation/app-skeleton-ci-boot/boot-the-app (T-boot)"),
    ).toBeInTheDocument();
  });

  it("opens both tiers above a deep-linked task, so the selection is visible", async () => {
    const page: NodePage = {
      node: {
        noun: "task",
        id: "T-health",
        key: "foundation/app-skeleton-ci-boot/serve-the-health-check",
        title: "Serve the health check",
        status: "proposed",
      },
      ref: "foundation/app-skeleton-ci-boot/serve-the-health-check (T-health)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/task/T-health": page }));

    const { container } = renderExplore("/node/task/T-health");

    // The epic opened because it holds the feature that holds the selection,
    // and the feature opened because it holds the selection itself. The
    // assertion is the navigator ROW, not the title: the reading pane renders
    // the same words in its own h1, so matching text alone would pass with no
    // task tier at all.
    expect(await screen.findByText("App Skeleton & Green CI Boot")).toBeInTheDocument();
    const selected = container.querySelector(".explore-task.is-selected");
    expect(selected).not.toBeNull();
    expect(selected?.textContent).toContain("Serve the health check");
  });
});

describe("Navigator: status freshness across selection", () => {
  it("refreshes tree statuses alongside the feature's task list without closing branches", async () => {
    const tree = acmeTree();
    const feature = tree.nodes[0].children[0];
    const page: NodePage = {
      node: {
        noun: "feature", id: feature.id, key: feature.key,
        title: feature.title, status: "done",
      },
      ref: `${feature.key} (${feature.id})`,
      prose: null,
      relationships: {
        tasks: feature.children.map((task) => ({
          noun: "task", id: task.id, key: task.key, title: task.title,
          status: "done", ref: `${task.key} (${task.id})`,
        })),
      },
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    const routes = baseRoutes({
      "/api/node/feature/F-pueftm": page,
      "/api/ls?noun=bug": listResult("bug", [bugRow]),
      "/api/ls?noun=chore": listResult("chore", [choreRow]),
    });
    const fetchMock = stubFetch(routes);
    renderExplore();
    const navigator = within(await screen.findByRole("navigation", { name: "Definition navigator" }));
    await navigator.findByText("Foundation");
    fireEvent.click(navigator.getByRole("button", { name: "Expand Audit Engine" }));
    fireEvent.click(navigator.getByRole("button", { name: "Expand Foundation" }));
    fireEvent.click(navigator.getByRole("button", { name: `Expand ${feature.title}` }));
    fireEvent.click(navigator.getByRole("button", { name: "Expand Bugs" }));
    fireEvent.click(navigator.getByRole("button", { name: "Expand Chores" }));
    expect(navigator.getByText("Boot the app").parentElement?.querySelector(".state-glyph"))
      .toHaveAttribute("data-status", "in_progress");

    // The working tree changes while Explore remains mounted.
    feature.status = "done";
    for (const task of feature.children) task.status = "done";
    routes["/api/tree"] = tree;
    const pending: Array<() => void> = [];
    const immediate = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation((input: unknown) => {
      const url = String(input);
      if (url === "/api/tree" || url.startsWith("/api/ls?")) {
        const request = Promise.withResolvers<Response>();
        pending.push(() => request.resolve(new Response(JSON.stringify(routes[url]))));
        return request.promise;
      }
      return immediate(input);
    });
    fireEvent.click(navigator.getByRole("link", { name: feature.title }));
    await screen.findByRole("heading", { level: 1, name: feature.title });
    // Detail has finished, but the navigator requests are still pending.
    expect(navigator.getByRole("link", { name: "Boot the app" })).toBeVisible();
    expect(navigator.getByRole("link", { name: shelfRow(bugRow.title) })).toBeVisible();
    expect(navigator.getByRole("link", { name: shelfRow(choreRow.title) })).toBeVisible();
    expect(navigator.getByRole("status", { name: "Navigator refresh" })).toHaveTextContent("Tree: refreshing");
    expect(screen.getByRole("link", { name: /Questions/ })).toHaveAttribute("aria-busy", "true");
    expect(navigator.getByRole("link", { name: feature.title }).parentElement).toHaveClass("is-selected");
    fireEvent.click(navigator.getByRole("button", { name: "Collapse Chores" }));
    fireEvent.click(navigator.getByRole("button", { name: "Expand Chores" }));
    await act(async () => { pending.forEach((finish) => finish()); });
    fireEvent.click(screen.getByRole("button", { name: /Tasks/ }));
    await waitFor(() => {
      for (const node of [feature, ...feature.children]) {
        expect(navigator.getByRole("link", { name: node.title }).parentElement?.querySelector(".state-glyph"))
          .toHaveAttribute("data-status", "done");
      }
    });
    expect(navigator.getByText("First Audit Feature")).toBeInTheDocument();
    for (const task of feature.children) {
      const row = screen.getAllByRole("link", { name: task.title })
        .find((link) => link.closest(".instr-taskrow"));
      expect(within(row!.closest(".instr-taskrow") as HTMLElement).getByText("done"))
        .toBeInTheDocument();
    }
  });

  it("keeps failed resources usable, then replaces them with successful empty responses", async () => {
    const routes = baseRoutes({
      "/api/ls?noun=bug": listResult("bug", [bugRow]),
      "/api/ls?noun=chore": listResult("chore", [choreRow]),
      "/api/ls?noun=question": listResult("question", [{ id: "Q-a", status: "open" }]),
    });
    const errors: Record<string, string> = {};
    const fetchMock = stubFetch(routes, errors);
    renderExplore();
    const nav = await screen.findByRole("navigation", { name: "Definition navigator" });
    const navigator = within(nav);
    await navigator.findByText("Foundation");
    fireEvent.click(navigator.getByRole("button", { name: "Open all" }));
    const taskLink = navigator.getByRole("link", { name: "Boot the app" });
    const bugLink = navigator.getByRole("link", { name: shelfRow(bugRow.title) });

    errors["/api/tree"] = "tree unavailable";
    errors["/api/ls?noun=bug"] = "bugs unavailable";
    errors["/api/ls?noun=question"] = "questions unavailable";
    routes["/api/ls?noun=chore"] = listResult("chore", [{ ...choreRow, status: "done" }]);
    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    const status = navigator.getByRole("status", { name: "Navigator refresh" });
    await waitFor(() => {
      expect(status).toHaveTextContent("Tree: refresh failed");
      expect(status).toHaveTextContent("Bugs: refresh failed");
      expect(status).toHaveTextContent("Questions: refresh failed");
    });
    expect(navigator.getByRole("link", { name: "Boot the app" })).toBe(taskLink);
    expect(navigator.getByRole("link", { name: shelfRow(bugRow.title) })).toBe(bugLink);
    expect(screen.getByRole("link", { name: /Questions/ })).toHaveTextContent("1 open");
    expect(navigator.getByRole("link", { name: shelfRow(choreRow.title) }).querySelector(".state-glyph"))
      .toHaveAttribute("data-status", "done");
    fireEvent.click(navigator.getByRole("button", { name: "Collapse Bugs" }));
    fireEvent.click(navigator.getByRole("button", { name: "Expand Bugs" }));
    expect(navigator.getByRole("link", { name: shelfRow(bugRow.title) })).toHaveAttribute("href", "/node/bug/B-a");

    const request = Promise.withResolvers<Response>();
    const immediate = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation((input: unknown) =>
      String(input) === "/api/tree" ? request.promise : immediate(input));
    for (const path of Object.keys(errors)) delete errors[path];
    routes["/api/ls?noun=bug"] = emptyList("bug");
    routes["/api/ls?noun=question"] = emptyList("question");
    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    expect(status).toHaveTextContent("Tree: refresh failed");
    expect(status).toHaveTextContent("retrying");
    expect(navigator.getByRole("link", { name: "Boot the app" })).toBe(taskLink);
    await act(async () => {
      request.resolve(new Response(JSON.stringify({ nodes: [] })));
    });
    await navigator.findByText("No epics yet.");
    expect(navigator.queryByRole("link", { name: "Boot the app" })).not.toBeInTheDocument();
    expect(navigator.queryByRole("link", { name: shelfRow(bugRow.title) })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Questions/ })).not.toHaveTextContent("open");
    expect(navigator.getByRole("link", { name: shelfRow(choreRow.title) })).toBeVisible();
    expect(status).not.toHaveTextContent("failed");
    expect(status).not.toHaveTextContent("refreshing");
  });

  it("keeps the split mounted while manual refresh waits for project summary and after failure", async () => {
    const fetchMock = stubFetch(baseRoutes());
    const { container } = renderExplore();
    const nav = await screen.findByRole("navigation", { name: "Definition navigator" });
    const navigator = within(nav);
    await navigator.findByText("Foundation");
    fireEvent.click(navigator.getByRole("button", { name: "Open all" }));
    const taskLink = navigator.getByRole("link", { name: "Boot the app" });
    const search = navigator.getByRole("textbox", { name: "Search" });
    fireEvent.change(search, { target: { value: "unfinished search" } });
    const divider = screen.getByRole("separator", { name: "Resize navigator" });
    fireEvent.keyDown(divider, { key: "ArrowRight" });
    const width = divider.getAttribute("aria-valuenow");
    const detail = container.querySelector(".explore-detail");
    expect(detail).toBeInTheDocument();
    taskLink.focus();
    const summary = Promise.withResolvers<Response>();
    const immediate = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation((input: unknown) =>
      String(input) === "/api/status" ? summary.promise : immediate(input));

    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    const status = navigator.getByRole("status", { name: "Navigator refresh" });
    await waitFor(() => expect(status).toHaveTextContent("Project summary: refreshing"));
    expect(screen.getByRole("navigation", { name: "Definition navigator" })).toBe(nav);
    expect(navigator.getByRole("link", { name: "Boot the app" })).toBe(taskLink);
    expect(taskLink).toHaveFocus();
    expect(navigator.getByRole("textbox", { name: "Search" })).toHaveValue("unfinished search");
    expect(screen.getByRole("separator", { name: "Resize navigator" })).toBe(divider);
    expect(divider).toHaveAttribute("aria-valuenow", width);
    expect(container.querySelector(".explore-detail")).toBe(detail);
    await act(async () => summary.reject(new Error("summary unavailable")));
    expect(status).toHaveTextContent("Project summary: refresh failed");
    expect(navigator.getByRole("link", { name: "Boot the app" })).toBe(taskLink);
    expect(taskLink).toHaveFocus();

    fetchMock.mockImplementation(immediate);
    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    await waitFor(() => expect(status).not.toHaveTextContent("failed"));
    expect(screen.getByRole("navigation", { name: "Definition navigator" })).toBe(nav);
    expect(navigator.getByRole("link", { name: "Boot the app" })).toBe(taskLink);
  });
});

describe("Navigator: manual expansion survives selection navigation", () => {
  // The Explore layout route renders the navigator at one stable element
  // position and stays mounted across /explore and every /node/:noun/* child,
  // so React preserves the navigator's component state across a selection.
  // This test pins that: if a future route change forces a remount (a
  // per-route key, a different element per path, the node route moved out
  // from under the layout), the manually expanded epic collapses and this
  // fails loudly. Proven able to fail during development by temporarily
  // keying the parameterized route's element.
  it("keeps a manually expanded, non-selected epic open across the first selection", async () => {
    const page: NodePage = {
      node: {
        noun: "feature",
        id: "F-pueftm",
        key: "foundation/app-skeleton-ci-boot",
        title: "App Skeleton & Green CI Boot",
        status: "ready",
      },
      ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/feature/F-pueftm": page }));

    renderExplore();

    await screen.findByText("Foundation");
    // Expand an epic the selection will NOT land in...
    fireEvent.click(screen.getByRole("button", { name: /Expand Audit Engine/i }));
    await screen.findByText("First Audit Feature");
    // ...then select a feature in a different epic.
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    fireEvent.click(await screen.findByText("App Skeleton & Green CI Boot"));
    await screen.findByRole("heading", { level: 1, name: "App Skeleton & Green CI Boot" });

    // The manual expansion survived the navigation.
    expect(screen.getByText("First Audit Feature")).toBeInTheDocument();
  });
});

describe("Navigator: selecting a feature expands its own tasks (not just an ancestor's)", () => {
  // The epic auto-opens when a selection lands inside it (autoOpen
  // materializes the ancestor ids into the lifted expansion map, task 1); a
  // feature clicked straight from the tree used to show its node page but
  // leave its own tasks collapsed - the pre-lift FeatureRow's derived `open`
  // expression dropped `selectedHere`, unlike EpicRow's. Pins the fix: a
  // feature carries the same idiom its own ancestor already does.
  it("reveals a selected feature's task rows on a direct link, no further click needed", async () => {
    const page: NodePage = {
      node: {
        noun: "feature",
        id: "F-pueftm",
        key: "foundation/app-skeleton-ci-boot",
        title: "App Skeleton & Green CI Boot",
        status: "ready",
      },
      ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/feature/F-pueftm": page }));

    renderExplore("/node/feature/F-pueftm");

    await screen.findByRole("heading", { level: 1, name: "App Skeleton & Green CI Boot" });
    // No click on the feature's own chevron happened - the deep link alone
    // should have opened it, the same way a deep-linked epic opens itself.
    expect(await screen.findByText("Boot the app")).toBeInTheDocument();
    expect(screen.getByText("Serve the health check")).toBeInTheDocument();
  });

  it("keeps a manually collapsed feature collapsed across a selection navigation onto itself", async () => {
    const page: NodePage = {
      node: {
        noun: "feature",
        id: "F-pueftm",
        key: "foundation/app-skeleton-ci-boot",
        title: "App Skeleton & Green CI Boot",
        status: "ready",
      },
      ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/feature/F-pueftm": page }));

    renderExplore();

    await screen.findByText("Foundation");
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    await screen.findByText("App Skeleton & Green CI Boot");
    // Open it by hand, then collapse it by hand - the expansion map now
    // holds this id explicitly false, which is what autoOpen's hand-closed
    // rule checks before it would otherwise open the branch on the reselect
    // below.
    fireEvent.click(screen.getByRole("button", { name: /Expand App Skeleton & Green CI Boot/i }));
    await screen.findByText("Boot the app");
    fireEvent.click(screen.getByRole("button", { name: /Collapse App Skeleton & Green CI Boot/i }));
    expect(screen.queryByText("Boot the app")).not.toBeInTheDocument();

    // Selecting the very feature the reader just collapsed must not
    // override that manual choice.
    fireEvent.click(screen.getByText("App Skeleton & Green CI Boot"));
    await screen.findByRole("heading", { level: 1, name: "App Skeleton & Green CI Boot" });

    expect(screen.queryByText("Boot the app")).not.toBeInTheDocument();
  });
});

describe("Navigator: sticky expansion (an unrelated selection does not close what it did not open)", () => {
  // Task 1's own reason to exist: the old model derived a row's openness from
  // the CURRENT selection at render time, so a branch that opened only
  // because a selection landed inside it collapsed the instant the selection
  // moved elsewhere - nothing pinned that before this test. The inversion
  // (docs/spec/ui-surface.md) makes expansion an explicit
  // record of what has been opened, so navigation alone never closes a
  // branch; only a hand toggle (or close-all, task 2) does.
  it("keeps a deep-linked epic's branch open after selecting a node under a different epic", async () => {
    const taskPage: NodePage = {
      node: {
        noun: "task",
        id: "T-boot",
        key: "foundation/app-skeleton-ci-boot/boot-the-app",
        title: "Boot the app",
        status: "in_progress",
      },
      ref: "foundation/app-skeleton-ci-boot/boot-the-app (T-boot)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    const auditPage: NodePage = {
      node: { noun: "epic", id: "E-audit1", key: "audit-engine", title: "Audit Engine", status: "draft" },
      ref: "audit-engine (E-audit1)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(
      baseRoutes({
        "/api/node/task/T-boot": taskPage,
        "/api/node/epic/E-audit1": auditPage,
      }),
    );

    renderExplore("/node/task/T-boot");

    // The deep link auto-opened Foundation (and its feature) with no click.
    expect(await screen.findByText("App Skeleton & Green CI Boot")).toBeInTheDocument();

    // Select a node under a wholly different epic - a plain title click, no
    // expansion required first (an epic's own row is always visible).
    fireEvent.click(await screen.findByText("Audit Engine"));
    await screen.findByRole("heading", { level: 1, name: "Audit Engine" });

    // Foundation's branch, opened only by the earlier selection landing
    // inside it, is still open: navigation never closes what it did not
    // close by hand.
    expect(screen.getByText("App Skeleton & Green CI Boot")).toBeInTheDocument();

    // The deeper tier the deep link itself pointed at survives the same
    // navigation, not just the feature tier one level up: the task row
    // ("Boot the app") stays visible too, since the feature it lives under
    // is still held open.
    expect(screen.getByText("Boot the app")).toBeInTheDocument();
  });
});

// Task 2 (docs/spec/ui-surface.md): "Open all" and
// "Close all" put the whole tree under one click either way, consuming
// Task 1's openAll/closeAll directly. The exact accessible names below
// ("Open all", "Close all") are deliberately distinct vocabulary from a
// chevron's own load-bearing "Expand"/"Collapse" aria-label, so none of the
// ~15 existing /Expand|Collapse/ name-pattern queries above can ever match
// these two new buttons.
describe("Navigator: open-all and close-all controls", () => {
  it("opens every epic's features and every feature's tasks in one click, including a branch closed by hand", async () => {
    stubFetch(baseRoutes());

    renderExplore();

    await screen.findByText("Foundation");
    // Hand-close a branch first: expand Foundation and its App Skeleton
    // feature down to the task tier, then collapse the feature by hand -
    // its entry now holds an explicit false, the same hand-closed state
    // the sticky-expansion tests above pin. Open-all must override it.
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    await screen.findByText("App Skeleton & Green CI Boot");
    fireEvent.click(screen.getByRole("button", { name: /Expand App Skeleton & Green CI Boot/i }));
    await screen.findByText("Boot the app");
    fireEvent.click(screen.getByRole("button", { name: /Collapse App Skeleton & Green CI Boot/i }));
    expect(screen.queryByText("Boot the app")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open all" }));

    // Every epic's own features, across every epic in the tree...
    expect(await screen.findByText("First Ops Feature")).toBeInTheDocument();
    expect(screen.getByText("First Audit Feature")).toBeInTheDocument();
    expect(screen.getByText("Acme Program Settings")).toBeInTheDocument();
    // ...and the hand-closed feature's own tasks, reopened by the explicit
    // open-all command the same way a fresh click on its own chevron would.
    expect(screen.getByText("Boot the app")).toBeInTheDocument();
    expect(screen.getByText("Serve the health check")).toBeInTheDocument();
  });

  it("collapses everything on close-all without reopening the currently selected branch, then auto-opens a later selection normally", async () => {
    const page: NodePage = {
      node: {
        noun: "feature",
        id: "F-pueftm",
        key: "foundation/app-skeleton-ci-boot",
        title: "App Skeleton & Green CI Boot",
        status: "ready",
      },
      ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    const auditPage: NodePage = {
      node: { noun: "epic", id: "E-audit1", key: "audit-engine", title: "Audit Engine", status: "draft" },
      ref: "audit-engine (E-audit1)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(
      baseRoutes({
        "/api/node/feature/F-pueftm": page,
        "/api/node/epic/E-audit1": auditPage,
      }),
    );

    renderExplore("/node/feature/F-pueftm");

    // The deep link auto-opened Foundation and the selected feature's own
    // tasks, no click needed.
    await screen.findByRole("heading", { level: 1, name: "App Skeleton & Green CI Boot" });
    expect(await screen.findByText("Boot the app")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Close all" }));

    // Everything collapses, including the still-selected branch: close-all
    // writes the empty map directly and never touches the selection, so the
    // auto-open effect (which only fires on a selection CHANGE) does not
    // run and immediately reopen what was just closed.
    expect(screen.queryByText("Boot the app")).not.toBeInTheDocument();
    expect(screen.getByText("Foundation")).toBeInTheDocument();

    // Selecting a node under a different epic afterwards auto-opens its
    // ancestry exactly as a fresh deep link would - close-all only cleared
    // the map, it did not disable the auto-open effect going forward.
    fireEvent.click(await screen.findByText("Audit Engine"));
    await screen.findByRole("heading", { level: 1, name: "Audit Engine" });
    expect(await screen.findByText("First Audit Feature")).toBeInTheDocument();
  });
});

// Task 3 (docs/spec/ui-surface.md): the resizable
// split. jsdom carries no layout engine, so getBoundingClientRect on any
// element returns all zeros by default and no assertion here can observe a
// real pixel - the brief's own "assert the wiring, not pixels" line. The
// clamp math itself is splitSize.test.ts's job; what these tests exercise
// is that Explore.tsx actually wires a mousedown/mousemove/mouseup/keydown/
// dblclick sequence through to the `--explore-nav-w` custom property on
// .explore-split, as far as a DOM with no layout engine allows a wiring
// test to reach.
describe("Explore: the resizable navigator/reading-pane split", () => {
  it("renders the divider with the separator role, sitting between the navigator and the outlet", async () => {
    stubFetch(baseRoutes());

    const { container } = renderExplore();
    await screen.findByText("Foundation");

    const nav = container.querySelector(".explore-nav");
    const detail = container.querySelector(".explore-detail");
    const divider = screen.getByRole("separator", { name: "Resize navigator" });

    expect(nav).not.toBeNull();
    expect(detail).not.toBeNull();
    expect(divider.getAttribute("aria-orientation")).toBe("vertical");
    expect(divider.tabIndex).toBe(0);
    // WAI-ARIA window-splitter practice on a keyboard-operable separator:
    // the bounds are fixed, and valuenow starts at the same 300px default
    // the split itself renders at.
    expect(divider.getAttribute("aria-valuemin")).toBe("200");
    expect(divider.getAttribute("aria-valuemax")).toBe("560");
    expect(divider.getAttribute("aria-valuenow")).toBe("300");
    // Between the nav and the outlet, in DOM order - not just present
    // somewhere in the split.
    expect(Boolean(nav!.compareDocumentPosition(divider) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(
      true,
    );
    expect(
      Boolean(divider.compareDocumentPosition(detail!) & Node.DOCUMENT_POSITION_FOLLOWING),
    ).toBe(true);
  });

  it("starts the split at the default 300px, expressed as the --explore-nav-w custom property on .explore-split", async () => {
    stubFetch(baseRoutes());

    const { container } = renderExplore();
    await screen.findByText("Foundation");

    const split = container.querySelector(".explore-split") as HTMLElement;
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("300px");
  });

  it("nudges the width by 16px per ArrowLeft/ArrowRight, through the clamp, and prevents the key's default so the page does not scroll", async () => {
    stubFetch(baseRoutes());

    const { container } = renderExplore();
    await screen.findByText("Foundation");

    const split = container.querySelector(".explore-split") as HTMLElement;
    const divider = screen.getByRole("separator", { name: "Resize navigator" });

    const leftNotPrevented = fireEvent.keyDown(divider, { key: "ArrowLeft" });
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("284px");
    // aria-valuenow tracks the same width the custom property does - a
    // keyboard nudge is one of the three ways a width can change (drag,
    // keyboard, dblclick reset), and this is the one this test can pin
    // without a layout engine.
    expect(divider.getAttribute("aria-valuenow")).toBe("284");
    // testing-library's fireEvent returns false when the handler called
    // preventDefault on a cancelable event - the wiring for "the page does
    // not scroll" on an arrow key.
    expect(leftNotPrevented).toBe(false);

    fireEvent.keyDown(divider, { key: "ArrowRight" });
    fireEvent.keyDown(divider, { key: "ArrowRight" });
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("316px");
    expect(divider.getAttribute("aria-valuenow")).toBe("316");
  });

  it("clamps a run of ArrowLeft presses at the 200px minimum rather than going negative", async () => {
    stubFetch(baseRoutes());

    const { container } = renderExplore();
    await screen.findByText("Foundation");

    const split = container.querySelector(".explore-split") as HTMLElement;
    const divider = screen.getByRole("separator", { name: "Resize navigator" });

    // 300 down by 16 seven times is 188, past the 200 floor.
    for (let i = 0; i < 7; i++) {
      fireEvent.keyDown(divider, { key: "ArrowLeft" });
    }
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("200px");
  });

  it("resets to the default 300px on a double-click, after the width has moved away from it", async () => {
    stubFetch(baseRoutes());

    const { container } = renderExplore();
    await screen.findByText("Foundation");

    const split = container.querySelector(".explore-split") as HTMLElement;
    const divider = screen.getByRole("separator", { name: "Resize navigator" });

    fireEvent.keyDown(divider, { key: "ArrowRight" });
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("316px");

    fireEvent.doubleClick(divider);
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("300px");
    expect(divider.getAttribute("aria-valuenow")).toBe("300");
  });

  it("drags to a new width computed from the pointer's x offset inside the split's own bounding rect, and stops tracking after mouseup", async () => {
    stubFetch(baseRoutes());

    const { container } = renderExplore();
    await screen.findByText("Foundation");

    const split = container.querySelector(".explore-split") as HTMLElement;
    const divider = screen.getByRole("separator", { name: "Resize navigator" });

    // jsdom has no layout engine, so getBoundingClientRect is stubbed to a
    // fixed rect the same way any jsdom geometry test has to - the point
    // under test is that the handler subtracts the split's own left edge
    // from the pointer's clientX, not the page's raw coordinate.
    vi.spyOn(split, "getBoundingClientRect").mockReturnValue({
      left: 40,
      top: 0,
      right: 840,
      bottom: 600,
      width: 800,
      height: 600,
      x: 40,
      y: 0,
      toJSON: () => ({}),
    });

    fireEvent.mouseDown(divider);
    expect(split.className).toContain("dragging");

    // buttons: 1 - the left button held down - is what a real drag's
    // mousemove carries; the wedge-guard test below is the one that leaves
    // this unset to simulate the button having let go outside the window.
    fireEvent.mouseMove(window, { clientX: 290, buttons: 1 });
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("250px");
    expect(divider.getAttribute("aria-valuenow")).toBe("250");

    fireEvent.mouseUp(window);
    expect(split.className).not.toContain("dragging");

    // The window listeners came off on mouseup: a further move changes
    // nothing.
    fireEvent.mouseMove(window, { clientX: 500, buttons: 1 });
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("250px");
  });

  it("disarms a wedged drag when the mouse button released outside the browser window (buttons reads 0 on the next move)", async () => {
    stubFetch(baseRoutes());

    const { container } = renderExplore();
    await screen.findByText("Foundation");

    const split = container.querySelector(".explore-split") as HTMLElement;
    const divider = screen.getByRole("separator", { name: "Resize navigator" });

    vi.spyOn(split, "getBoundingClientRect").mockReturnValue({
      left: 40,
      top: 0,
      right: 840,
      bottom: 600,
      width: 800,
      height: 600,
      x: 40,
      y: 0,
      toJSON: () => ({}),
    });

    fireEvent.mouseDown(divider);
    expect(split.className).toContain("dragging");

    // No mouseup ever arrives in this scenario - the button let go outside
    // the browser window, so the next move this window ever sees carries
    // buttons: 0 (fireEvent's own default, standing in for "nothing held").
    // The guard reads that as the drag having ended and disarms it itself,
    // rather than leaving the nav resizing button-up until the next click.
    fireEvent.mouseMove(window, { clientX: 290 });
    expect(split.className).not.toContain("dragging");
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("300px");

    // The guard's own setDragging(false) already tore the window listeners
    // down (the drag effect's cleanup), so a further move - even one that
    // claims the button is held again - changes nothing.
    fireEvent.mouseMove(window, { clientX: 500, buttons: 1 });
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("300px");
  });

  it("removes the drag's window listeners on unmount even mid-drag, so a drag that outlives the component does not leak", async () => {
    stubFetch(baseRoutes());

    const { unmount } = renderExplore();
    await screen.findByText("Foundation");

    const divider = screen.getByRole("separator", { name: "Resize navigator" });
    const removeSpy = vi.spyOn(window, "removeEventListener");

    fireEvent.mouseDown(divider);
    unmount();

    const removedTypes = removeSpy.mock.calls.map((c) => c[0]);
    expect(removedTypes).toContain("mousemove");
    expect(removedTypes).toContain("mouseup");

    // No leaked handler fires after unmount: dispatching on window must not
    // throw (a React "state update on an unmounted component" path would).
    expect(() => {
      fireEvent.mouseMove(window, { clientX: 999 });
      fireEvent.mouseUp(window);
    }).not.toThrow();
  });
});

describe("Navigator: a failed tree fetch surfaces its error honestly", () => {
  it("never renders an empty navigator in place of a real error", async () => {
    stubFetch({
      "/api/ls?noun=question": emptyList("question"),
    });

    renderExplore();

    expect(await screen.findByText(/GET \/api\/tree failed \(404 Not Found\)/)).toBeInTheDocument();
    expect(screen.queryByText("No epics yet.")).not.toBeInTheDocument();
  });
});

// NavSearch (Navigator.tsx, plan.md Phase 4 item 3): the unified bar's one
// persistent input, re-dressing rather than replacing the existing
// mode=search transition SearchMode's own pill already drives. It carried
// zero coverage of its own before a later fix - the one Phase 4 surface with
// none - despite being a router navigation, not a pure render, and so the one
// thing in this file most in need of a real MemoryRouter round-trip rather than
// a snapshot.
describe("Navigator: unified search bar (NavSearch)", () => {
  it("links Tweak search hits by their TW identity", async () => {
    const result: SearchResult = {
      query: "spacing", count: 1, truncated: false, kinds: [{ kind: "tweak", count: 1 }], documents: [],
      nodes: [{ noun: "tweak", id: "TW-abc123", key: "tweaks/button", title: "Adjust button", status: "ready", ref: "tweaks/button (TW-abc123)", match: "text", count: 1, hits: [{ path: ".tiller/tweaks/button/tweak.md", line: 1, snippet: "Adjust spacing" }] }],
    };
    stubFetch(baseRoutes({ "/api/search?q=spacing": result }));
    renderExplore("/explore?mode=search&q=spacing");
    expect(await screen.findByRole("link", { name: "Adjust button" })).toHaveAttribute("href", "/node/tweak/TW-abc123");
  });
  it("submits a keyword and navigates to /explore?mode=search&q=<trimmed>", async () => {
    const result: SearchResult = { query: "boot", count: 0, truncated: false, kinds: [], nodes: [], documents: [] };
    const fetchMock = stubFetch(baseRoutes({ "/api/search?q=boot": result }));

    renderExplore();

    await screen.findByText("Foundation");
    const input = screen.getByRole("textbox", { name: "Search" });
    // Leading/trailing whitespace around the real keyword: the trim happens
    // in NavSearch's own submit handler, not in the browser's input value,
    // so a pass here proves the component trims rather than the fixture
    // arriving pre-trimmed.
    fireEvent.change(input, { target: { value: "  boot  " } });
    fireEvent.submit(input.closest("form") as HTMLFormElement);

    // SearchMode only renders once it is actually mounted at ?mode=search,
    // and only fetches once q is non-empty - so either one landing is
    // itself proof the navigation happened, not just that the form fired.
    // A zero-hit result renders SearchMode's own EmptyNote rather than a
    // "0 matches" count line (Results' own hits.length === 0 branch).
    await screen.findByText("No matches.");
    const urls = fetchMock.mock.calls.map((c) => String(c[0]));
    expect(urls).toContain("/api/search?q=boot");
  });

  it("is a no-op on an empty or whitespace-only submit", async () => {
    const fetchMock = stubFetch(baseRoutes());

    renderExplore();

    await screen.findByText("Foundation");
    const input = screen.getByRole("textbox", { name: "Search" });
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.submit(input.closest("form") as HTMLFormElement);

    // Still on browse mode: the tree is what's showing, never a search
    // result, its own loading line, or a request for one.
    expect(screen.getByText("Foundation")).toBeInTheDocument();
    expect(screen.queryByText(/match/)).not.toBeInTheDocument();
    const urls = fetchMock.mock.calls.map((c) => String(c[0]));
    expect(urls.some((u) => u.startsWith("/api/search"))).toBe(false);
  });
});

// The question ListRow shape carries a bounded title for scan surfaces and the
// complete statement for the question page. It also carries answer until
// answered, due as the queue-compatible hint, and blocks_refs as resolved
// RelatedNode rows. Q-h2n7m5 carries two blocks on purpose: a resolvable one
// (Eligibility rules) and a dangling one (empty noun/status, ref repeating
// the id - api.ts's RelatedNode contract). This is the focused coverage for
// QuestionsPane rendering that dangling edge through RefLink's guard.
function questionRows(): ListRow[] {
  return [
    {
      id: "Q-h2n7m5",
      status: "open",
      title: QUESTION_TITLE,
      statement: QUESTION_STATEMENT,
      due: "Friday",
      blocks_refs: [
        {
          noun: "feature",
          id: "F-elig",
          key: "intake/eligibility-rules",
          ref: "intake/eligibility-rules (F-elig)",
          status: "active",
          title: "Eligibility rules",
        },
        // The dangling posture, verbatim: noun and status arrive as empty
        // strings, ref repeats the id, and there is no key or title.
        {
          noun: "",
          id: "F-gone",
          ref: "F-gone",
          status: "",
        },
      ],
      ref: "Q-h2n7m5",
    },
    {
      id: "Q-m8b3v7",
      status: "answered",
      title: "Applicant sign-in method",
      statement: "Do applicants authenticate with passwords or magic links?",
      answer: "Magic links only. No password surface anywhere in the applicant portal.",
      ref: "Q-m8b3v7",
    },
  ];
}

describe("Explore: questions ledger navigation", () => {
  it("shows the open count and routes to /questions", async () => {
    stubFetch(
      baseRoutes({ "/api/ls?noun=question": { noun: "question", count: 2, nodes: questionRows() } }),
    );

    renderExplore();

    await screen.findByText("Foundation");
    const rail = within(screen.getByRole("navigation", { name: "Explore mode" }));
    const questionsLink = rail.getByRole("link", { name: /Questions/ });
    expect(questionsLink).toHaveTextContent("1 open");
    expect(questionsLink).not.toHaveAttribute("aria-current");
    expect(within(screen.getByRole("navigation", { name: "Definition navigator" }))
      .queryByRole("link", { name: /Questions|Amendments/ })).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("Questions"));

    expect(await screen.findByRole("heading", { level: 1, name: "Questions" })).toBeInTheDocument();
    expect(screen.getByText("Foundation")).toBeInTheDocument();
    expect(rail.getByRole("link", { name: /Questions/ })).toHaveAttribute("aria-current", "page");
  });

  it("omits the accent badge when nothing is open (Global Constraint: accent marks only what needs the human)", async () => {
    const answeredOnly = questionRows().filter((r) => r.status !== "open");
    stubFetch(
      baseRoutes({
        "/api/ls?noun=question": { noun: "question", count: answeredOnly.length, nodes: answeredOnly },
      }),
    );

    renderExplore();

    await screen.findByText("Foundation");
    expect(await screen.findByRole("link", { name: "Questions" })).toBeVisible();
    expect(screen.queryByText(/open/)).not.toBeInTheDocument();
  });
});

describe("Questions ledger: /questions renders inside the Explore layout", () => {
  it("renders the ledger directly at /questions - hard-refresh survival, navigator standing beside it", async () => {
    stubFetch(
      baseRoutes({ "/api/ls?noun=question": { noun: "question", count: 2, nodes: questionRows() } }),
    );

    // A fresh MemoryRouter mount at /questions, with no prior in-app
    // navigation, is the router-level stand-in for a hard refresh: the
    // route has to answer from a cold start, the same acceptance line the
    // /recap redirect tests pin for their own address.
    renderExplore("/questions");

    expect(await screen.findByRole("heading", { level: 1, name: "Questions" })).toBeInTheDocument();
    expect(screen.getByText("Foundation")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Questions/ })).toHaveAttribute("aria-current", "page");
  });

  it("leads every question card with its title, keeps the id, and omits the full statement", async () => {
    stubFetch(
      baseRoutes({ "/api/ls?noun=question": { noun: "question", count: 2, nodes: questionRows() } }),
    );

    const { container } = renderExplore("/questions");

    await screen.findByRole("heading", { level: 1, name: "Questions" });
    const card = container.querySelector(".qcard") as HTMLElement;
    const title = within(card).getByRole("link", { name: QUESTION_TITLE });
    expect(title).toHaveAttribute("href", "/node/question/Q-h2n7m5");
    expect(card.querySelector(".q-top")?.firstElementChild).toBe(title);
    expect(within(card).getByText("Q-h2n7m5")).toBeInTheDocument();
    expect(within(card).queryByText((_, element) => element?.textContent === QUESTION_STATEMENT)).not.toBeInTheDocument();
    expect(within(card).getByText("Due Friday")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Eligibility rules" })).toHaveAttribute(
      "href",
      "/node/feature/F-elig",
    );
    // The dangling second blocks_refs entry degrades to RefLink's plain-span
    // fallback: its bare id shows, but never as a link to nowhere.
    expect(screen.getByText("F-gone")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "F-gone" })).not.toBeInTheDocument();
    expect(
      screen.getByText('tiller answer question Q-h2n7m5 --answer "..."'),
    ).toBeInTheDocument();
  });

  it("narrows the ledger to one status through the status rail and ?status=", async () => {
    stubFetch(
      baseRoutes({ "/api/ls?noun=question": { noun: "question", count: 2, nodes: questionRows() } }),
    );

    const { container } = renderExplore("/questions");

    const rail = within(await screen.findByRole("navigation", { name: "Question status" }));
    expect(rail.getByRole("button", { name: "All · 2" })).toHaveAttribute("aria-current", "true");
    expect(container.querySelectorAll(".qcard")).toHaveLength(2);

    fireEvent.click(rail.getByRole("button", { name: "Open · 1" }));
    await waitFor(() => expect(container.querySelectorAll(".qcard")).toHaveLength(1));
    expect(container.querySelector(".qcard.open-q")).not.toBeNull();

    fireEvent.click(rail.getByRole("button", { name: "Answered · 1" }));
    await waitFor(() => expect(container.querySelector(".qcard.open-q")).toBeNull());
    expect(container.querySelectorAll(".qcard")).toHaveLength(1);
  });

  it("opens on the status a ?status= link names, and reads an unknown one as All", async () => {
    stubFetch(
      baseRoutes({ "/api/ls?noun=question": { noun: "question", count: 2, nodes: questionRows() } }),
    );

    const { container, unmount } = renderExplore("/questions?status=answered");
    const rail = within(await screen.findByRole("navigation", { name: "Question status" }));
    expect(rail.getByRole("button", { name: "Answered · 1" })).toHaveAttribute("aria-current", "true");
    expect(container.querySelectorAll(".qcard")).toHaveLength(1);
    unmount();

    renderExplore("/questions?status=bogus");
    const rail2 = within(await screen.findByRole("navigation", { name: "Question status" }));
    expect(rail2.getByRole("button", { name: "All · 2" })).toHaveAttribute("aria-current", "true");
  });

  it("keeps an answered question's answer visible and carries no copy command for it", async () => {
    stubFetch(
      baseRoutes({ "/api/ls?noun=question": { noun: "question", count: 2, nodes: questionRows() } }),
    );

    renderExplore("/questions");

    await screen.findByRole("heading", { level: 1, name: "Questions" });
    expect(
      screen.getByText(
        "Magic links only. No password surface anywhere in the applicant portal.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('tiller answer question Q-m8b3v7 --answer "..."'),
    ).not.toBeInTheDocument();
  });
});

describe("Filter subtab: question rows", () => {
  it("uses the bounded title, never the statement fallback, and falls back to ref only without a handle", async () => {
    const missingHandle = {
      id: "Q-missing",
      status: "open",
      statement: "This statement must never become a scan-row label.",
      ref: "Q-missing",
    };
    stubFetch(
      baseRoutes({
        "/api/ls?noun=questions": {
          noun: "question",
          count: 2,
          nodes: [questionRows()[0], missingHandle],
        },
      }),
    );

    const { container } = renderExplore("/explore?mode=filter&noun=questions");

    const title = await screen.findByRole("link", { name: QUESTION_TITLE });
    const questionRow = title.closest(".explore-result-row") as HTMLElement;
    const questionId = within(questionRow).getByText("Q-h2n7m5");
    expect(title.compareDocumentPosition(questionId) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(questionId.className).toContain("mono");
    expect(screen.queryByText((_, element) => element?.textContent === QUESTION_STATEMENT)).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Q-missing" })).toBeInTheDocument();
    expect(screen.queryByText(missingHandle.statement)).not.toBeInTheDocument();
    expect(container.querySelectorAll(".explore-result-row")).toHaveLength(2);
  });

  it("keeps a keyed noun's key in the address slot without adding its id", async () => {
    stubFetch(
      baseRoutes({
        "/api/ls?noun=features": {
          noun: "feature",
          count: 1,
          nodes: [
            {
              id: "F-filter",
              key: "audit/export",
              title: "Audit export",
              status: "draft",
              ref: "audit/export (F-filter)",
            },
          ],
        },
      }),
    );

    const { container } = renderExplore("/explore?mode=filter&noun=features");

    const title = await screen.findByRole("link", { name: "Audit export" });
    const row = title.closest(".explore-result-row") as HTMLElement;
    expect(within(row).getByText("audit/export").className).toContain("mono");
    expect(row.textContent).not.toContain("F-filter");
    expect(container.querySelectorAll(".explore-result-row")).toHaveLength(1);
  });
});

const facetAxes: Record<string, Facet["axis"][]> = {
  epic: ["status", "contributor"], feature: ["status", "contributor", "epic"],
  task: ["status", "owner", "epic", "feature"], question: ["status", "owner"],
  amendment: ["status"], bug: ["status", "owner"], chore: ["status", "owner"],
};

function facetOption(value: string, extra: Partial<FacetOption> = {}): FacetOption {
  return { value, label: value, count: 1, ...extra };
}

function facetList(noun: string, options: Partial<Record<Facet["axis"], FacetOption[]>> = {},
  nodes: ListRow[] = [], unresolved: ListResult["unresolved"] = {}): ListResult {
  return { noun, nodes, count: nodes.length, axes: facetAxes, unresolved,
    facets: facetAxes[noun].map((axis) => ({ axis, options: options[axis] ?? [] })) };
}

function requestedPaths(mock: { mock: { calls: [unknown][] } }): string[] {
  return mock.mock.calls.map(([input]) => String(input));
}

// Deferred requests below exercise real URL-driven fetch cancellation, not
// mock component state, so response ordering stays deterministic.
describe("Explore filter facets", () => {
  it.each([["bugs", "bug"], ["chores", "chore"]])("renders successful %s alias responses", async (urlNoun, noun) => {
    stubFetch(baseRoutes({
      [`/api/ls?noun=${urlNoun}`]: facetList(noun, { status: [facetOption("ready")] },
        [{ id: "W-alias", ref: "W-alias", title: "Alias work", status: "ready" }]),
    }));
    renderExplore(`/explore?mode=filter&noun=${urlNoun}`);
    expect(await screen.findByRole("link", { name: "Alias work" })).toHaveAttribute("href", `/node/${noun}/W-alias`);
    expect(screen.getByRole("combobox", { name: "Noun" })).toHaveValue(noun);
    expect(screen.getByRole("combobox", { name: "status" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "owner" })).toBeInTheDocument();
  });

  it("updates rows and other-axis counts on status pick, then restores the pick with Back", async () => {
    const task: ListRow = { id: "T-filter", ref: "T-filter", title: "Filtered task", status: "in_progress" };
    const mock = stubFetch(baseRoutes({
      "/api/ls?noun=tasks": facetList("task", {
        status: [facetOption("unfinished", { count: 2 })], owner: [facetOption("jc", { label: "JC Grubbs", count: 2 })],
      }, [task, { id: "T-done", ref: "T-done", title: "Completed task", status: "done" }]),
      "/api/ls?noun=tasks&status=unfinished": facetList("task", {
        status: [facetOption("unfinished", { selected: true })], owner: [facetOption("jc", { label: "JC Grubbs", count: 1 })],
      }, [task]),
    }));
    render(<MemoryRouter initialEntries={["/explore?mode=filter&noun=tasks"]}><BrowserBack /><App /></MemoryRouter>);
    fireEvent.focus(await screen.findByRole("combobox", { name: "status" }));
    fireEvent.keyDown(screen.getByRole("combobox", { name: "status" }), { key: "Enter" });
    await waitFor(() => expect(requestedPaths(mock)).toContain("/api/ls?noun=tasks&status=unfinished"));
    await waitFor(() => expect(screen.getByRole("combobox", { name: "status" })).toHaveValue("unfinished"));
    expect(screen.queryByRole("link", { name: "Completed task" })).not.toBeInTheDocument();
    fireEvent.focus(screen.getByRole("combobox", { name: "owner" }));
    expect(screen.getByRole("option", { name: /JC Grubbs/ })).toHaveTextContent("1");
    fireEvent.blur(screen.getByRole("combobox", { name: "owner" }));
    fireEvent.click(screen.getByRole("button", { name: "Browser Back" }));
    expect(await screen.findByRole("link", { name: "Completed task" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "status" })).toHaveValue("");
  });

  it("renders only served axes and applies contributor IDs on pick without free text or apply", async () => {
    const feature: ListRow = { id: "F-mixed", ref: "F-mixed", title: "Shared feature", status: "active",
      contributors: [{ id: "sam", name: "Sam" }, { id: "retired", name: "retired" }] };
    const mock = stubFetch(baseRoutes({
      "/api/ls?noun=features": facetList("feature", { contributor: [facetOption("sam", { label: "Sam" })] }),
      "/api/ls?noun=features&contributor=sam": facetList("feature",
        { contributor: [facetOption("sam", { label: "Sam", selected: true })] }, [feature]),
    }));
    renderExplore("/explore?mode=filter&noun=features");
    const input = await screen.findByRole("combobox", { name: "contributor" });
    expect(screen.queryByRole("combobox", { name: "owner" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "feature" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "apply" })).not.toBeInTheDocument();
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: "not-a-person" } });
    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.blur(input);
    expect(requestedPaths(mock).filter((path) => path.startsWith("/api/ls?noun=features"))).toEqual(["/api/ls?noun=features"]);
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: "SAM" } });
    fireEvent.mouseDown(screen.getByRole("option", { name: /Sam/ }));
    const row = (await screen.findByRole("link", { name: "Shared feature" })).closest("li") as HTMLElement;
    expect(requestedPaths(mock)).toContain("/api/ls?noun=features&contributor=sam");
    expect(screen.getByRole("combobox", { name: "contributor" })).toHaveValue("Sam");
    expect(row).toHaveTextContent("Contributors:");
    expect(within(row).getByText("retired")).toBeInTheDocument();
    expect(row).not.toHaveTextContent("Owner:");
  });

  it("replaces a stale-axis history entry instead of trapping Back on the invalid URL", async () => {
    const mock = stubFetch(baseRoutes({
      "/api/ls?noun=epics&owner=jc": facetList("epic"),
      "/api/ls?noun=epics": facetList("epic"),
    }));
    render(<MemoryRouter initialEntries={["/explore?mode=search", "/explore?mode=filter&noun=epics&owner=jc"]} initialIndex={1}>
      <BrowserBack /><App />
    </MemoryRouter>);
    await waitFor(() => expect(requestedPaths(mock)).toContain("/api/ls?noun=epics"));
    fireEvent.click(screen.getByRole("button", { name: "Browser Back" }));
    await waitFor(() => expect(screen.queryByRole("combobox", { name: "Noun" })).not.toBeInTheDocument());
  });

  it("displays a legacy parent key by title and clears an unresolved parent note", async () => {
    const mock = stubFetch(baseRoutes({
      "/api/ls?noun=features&epic=payments": facetList("feature",
        { epic: [facetOption("E-1", { label: "Payments", key: "payments", selected: true })] }),
      "/api/ls?noun=features&epic=nope": facetList("feature",
        { epic: [facetOption("nope", { selected: true, count: 0 })] }, [], { epic: "nope" }),
      "/api/ls?noun=features": facetList("feature"),
    }));
    const first = renderExplore("/explore?mode=filter&noun=features&epic=payments");
    expect(await screen.findByRole("combobox", { name: "epic" })).toHaveValue("Payments");
    first.unmount();
    renderExplore("/explore?mode=filter&noun=features&epic=nope");
    expect(await screen.findByText('no epic "nope"')).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "epic" })).toHaveAccessibleDescription('no epic "nope"');
    expect(screen.getByText("No matches.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear epic" }));
    await waitFor(() => expect(requestedPaths(mock)).toContain("/api/ls?noun=features"));
    await waitFor(() => expect(screen.queryByText('no epic "nope"')).not.toBeInTheDocument());
  });

  it("carries owner to bugs while dropping task-only parent axes", async () => {
    const mock = stubFetch(baseRoutes({
      "/api/ls?noun=tasks&owner=jc&feature=F-1": facetList("task", {
        owner: [facetOption("jc", { selected: true, label: "JC Grubbs" })],
        feature: [facetOption("F-1", { selected: true, parent: "E-1" })],
      }),
      "/api/ls?noun=bug&owner=jc": facetList("bug", { owner: [facetOption("jc", { selected: true, label: "JC Grubbs", count: 0 })] }),
    }));
    renderExplore("/explore?mode=filter&noun=tasks&owner=jc&feature=F-1");
    await screen.findByRole("combobox", { name: "feature" });
    fireEvent.change(screen.getByRole("combobox", { name: "Noun" }), { target: { value: "bug" } });
    await waitFor(() => expect(requestedPaths(mock)).toContain("/api/ls?noun=bug&owner=jc"));
    expect(await screen.findByRole("combobox", { name: "owner" })).toHaveValue("JC Grubbs");
    expect(screen.queryByRole("combobox", { name: "feature" })).not.toBeInTheDocument();
  });

  it("carries an illegal status across nouns, keeping a selected zero-count option and clear", async () => {
    const mock = stubFetch(baseRoutes({
      "/api/ls?noun=features&status=specced": facetList("feature", { status: [facetOption("specced", { selected: true })] }),
      "/api/ls?noun=tasks&status=specced": facetList("task",
        { status: [facetOption("unfinished"), facetOption("specced", { selected: true, count: 0 })] }),
    }));
    renderExplore("/explore?mode=filter&noun=features&status=specced");
    await screen.findByRole("combobox", { name: "status" });
    fireEvent.change(screen.getByRole("combobox", { name: "Noun" }), { target: { value: "tasks" } });
    await waitFor(() => expect(requestedPaths(mock)).toContain("/api/ls?noun=tasks&status=specced"));
    expect(await screen.findByRole("combobox", { name: "status" })).toHaveValue("specced");
    expect(screen.getByRole("button", { name: "Clear status" })).toBeInTheDocument();
    fireEvent.focus(screen.getByRole("combobox", { name: "status" }));
    expect(screen.getByRole("option", { name: /specced/ })).toHaveTextContent("0");
  });

  it.each([["E-1", true], ["E-2", false]])("picks epic %s and retains its feature only when beneath it", async (epic, keep) => {
    const target = `/api/ls?noun=tasks${keep ? "&feature=payments%2Fcheckout" : ""}&epic=${epic}`;
    const mock = stubFetch(baseRoutes({
      "/api/ls?noun=tasks&feature=payments%2Fcheckout": facetList("task", {
        epic: [facetOption("E-1", { label: "Payments" }), facetOption("E-2", { label: "Audit" })],
        feature: [facetOption("F-1", { label: "Checkout", key: "payments/checkout", selected: true, parent: "E-1" })],
      }),
      [target]: facetList("task"),
    }));
    renderExplore("/explore?mode=filter&noun=tasks&feature=payments%2Fcheckout");
    const input = await screen.findByRole("combobox", { name: "epic" });
    fireEvent.focus(input);
    fireEvent.mouseDown(screen.getByRole("option", { name: keep ? /Payments/ : /Audit/ }));
    await waitFor(() => expect(requestedPaths(mock)).toContain(target));
  });

  it("picks a feature without filling epic and clears all axes at once", async () => {
    const mock = stubFetch(baseRoutes({
      "/api/ls?noun=tasks&status=unfinished&owner=jc": facetList("task", {
        status: [facetOption("unfinished", { selected: true })], owner: [facetOption("jc", { selected: true })],
        feature: [facetOption("F-1", { label: "Checkout", parent: "E-1" })],
      }),
      "/api/ls?noun=tasks&status=unfinished&owner=jc&feature=F-1": facetList("task", {
        feature: [facetOption("F-1", { selected: true, parent: "E-1" })],
      }),
      "/api/ls?noun=tasks": facetList("task"),
    }));
    renderExplore("/explore?mode=filter&noun=tasks&status=unfinished&owner=jc");
    fireEvent.focus(await screen.findByRole("combobox", { name: "feature" }));
    fireEvent.mouseDown(screen.getByRole("option", { name: /Checkout/ }));
    await waitFor(() => expect(requestedPaths(mock)).toContain("/api/ls?noun=tasks&status=unfinished&owner=jc&feature=F-1"));
    fireEvent.click(screen.getByRole("button", { name: "clear all" }));
    await waitFor(() => expect(requestedPaths(mock)).toContain("/api/ls?noun=tasks"));
    expect(screen.queryByRole("button", { name: "clear all" })).not.toBeInTheDocument();
  });

  it("keeps same-noun fields mounted through pending picks and ignores superseded query responses", async () => {
    const initial = facetList("task", { owner: [facetOption("jc", { label: "JC Grubbs" }), facetOption("bob", { label: "Bob Chen", selected: true })] });
    const routes = baseRoutes({ "/api/ls?noun=tasks&owner=bob": initial });
    let bobRequests = 0;
    const pending = new Map<string, (value: unknown) => void>();
    const mock = vi.fn((input: unknown) => {
      const path = String(input);
      if (path === "/api/ls?noun=tasks&owner=jc" ||
        (path === "/api/ls?noun=tasks&owner=bob" && bobRequests++ > 0)) {
        const request = Promise.withResolvers<unknown>();
        pending.set(path, request.resolve);
        return request.promise;
      }
      if (!(path in routes)) return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: async () => "" });
      return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[path]) });
    });
    vi.stubGlobal("fetch", mock);
    renderExplore("/explore?mode=filter&noun=tasks&owner=bob");
    const owner = await screen.findByRole("combobox", { name: "owner" });
    fireEvent.focus(owner);
    fireEvent.mouseDown(screen.getByRole("option", { name: /JC Grubbs/ }));
    await waitFor(() => expect(pending.has("/api/ls?noun=tasks&owner=jc")).toBe(true));
    expect(screen.getByRole("combobox", { name: "owner" })).toBe(owner);
    fireEvent.focus(owner);
    fireEvent.mouseDown(screen.getByRole("option", { name: /Bob Chen/ }));
    await waitFor(() => expect(pending.has("/api/ls?noun=tasks&owner=bob")).toBe(true));
    await act(async () => pending.get("/api/ls?noun=tasks&owner=bob")?.({ ok: true, json: async () =>
      facetList("task", { owner: [facetOption("bob", { label: "Bob Chen", selected: true })] }) }));
    expect(owner).toHaveValue("Bob Chen");
    await act(async () => pending.get("/api/ls?noun=tasks&owner=jc")?.({ ok: true, json: async () =>
      facetList("task", { owner: [facetOption("jc", { label: "JC Grubbs", selected: true })] }) }));
    expect(owner).toHaveValue("Bob Chen");
  });

  it("never uses retained old-noun facets to clean a new noun while its response is pending", async () => {
    const routes = baseRoutes({ "/api/ls?noun=features&contributor=sam": facetList("feature",
      { contributor: [facetOption("sam", { selected: true })] }) });
    const request = Promise.withResolvers<unknown>();
    const mock = vi.fn((input: unknown) => {
      const path = String(input);
      if (path === "/api/ls?noun=tasks") return request.promise;
      if (!(path in routes)) return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: async () => "" });
      return Promise.resolve({ ok: true, json: async () => routes[path] });
    });
    vi.stubGlobal("fetch", mock);
    renderExplore("/explore?mode=filter&noun=features&contributor=sam");
    await screen.findByRole("combobox", { name: "contributor" });
    fireEvent.change(screen.getByRole("combobox", { name: "Noun" }), { target: { value: "tasks" } });
    await waitFor(() => expect(mock.mock.calls.map(([path]) => path)).toContain("/api/ls?noun=tasks"));
    expect(screen.queryByRole("combobox", { name: "contributor" })).not.toBeInTheDocument();
    await act(async () => request.resolve({ ok: true, json: async () => facetList("task", { owner: [facetOption("jc")] }) }));
    expect(await screen.findByRole("combobox", { name: "owner" })).toBeInTheDocument();
    expect(mock.mock.calls.map(([path]) => path)).not.toContain("/api/ls?noun=tasks&contributor=sam");
  });
});


function BrowserBack() {
  const navigate = useNavigate();
  return <button type="button" onClick={() => void navigate(-1)}>Browser Back</button>;
}

describe("Amendments: permanent navigation and identity-safe filter results", () => {
  const amendmentRows: ListRow[] = [
    { id: "A-proposed", key: "reshape", ref: "reshape (A-proposed)", title: "New reshape", status: "proposed" },
    { id: "A-approved", key: "reshape", ref: "reshape (A-approved)", title: "Earlier reshape", status: "approved" },
    { id: "A-withdrawn", key: "reshape", ref: "reshape (A-withdrawn)", title: "Withdrawn reshape", status: "withdrawn" },
  ];

  it("opens the ledger beside the navigator, opens immutable amendment detail and preserves the list on browser Back", async () => {
    const page: NodePage = {
      node: { noun: "amendment", ...amendmentRows[1] },
      ref: amendmentRows[1].ref,
      prose: { path: ".tiller/amendments/A-approved-reshape.md", content: "Earlier amendment narrative." },
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      amendment: { operations: [] },
    };
    stubFetch(baseRoutes({
      "/api/ls?noun=amendments": listResult("amendment", amendmentRows),
      "/api/node/amendment/A-approved": page,
    }));
    render(
      <MemoryRouter initialEntries={["/explore?owner=someone&epic=unrelated&feature=elsewhere&status=active&q=old"]}>
        <BrowserBack />
        <App />
      </MemoryRouter>,
    );
    const entry = await screen.findByRole("link", { name: "Amendments" });
    expect(entry).toHaveAttribute("href", "/amendments");
    fireEvent.click(entry);
    expect(await screen.findByRole("heading", { level: 1, name: "Amendments" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Earlier reshape" })).toHaveAttribute("href", "/node/amendment/A-approved");
    expect(screen.getByRole("link", { name: "New reshape" })).toHaveAttribute("href", "/node/amendment/A-proposed");
    expect(screen.getByRole("link", { name: "Withdrawn reshape" })).toHaveAttribute("href", "/node/amendment/A-withdrawn");
    expect(screen.getByRole("navigation", { name: "Definition navigator" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Amendments" })).toHaveAttribute("aria-current", "page");
    fireEvent.click(screen.getByRole("link", { name: "Earlier reshape" }));
    expect(await screen.findByText("Earlier amendment narrative.")).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Node detail tab" })).not.toBeInTheDocument();
    expect(screen.queryByText(/tiller approve amendment/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Browser Back" }));
    expect(await screen.findByRole("link", { name: "Earlier reshape" })).toHaveAttribute("href", "/node/amendment/A-approved");
    fireEvent.click(screen.getByRole("button", { name: "Browser Back" }));
    expect(await screen.findByRole("link", { name: "Amendments" })).toHaveAttribute("href", "/amendments");
  });

  it("narrows the ledger by status through ?status= and keeps every status countable", async () => {
    stubFetch(baseRoutes({ "/api/ls?noun=amendments": listResult("amendment", amendmentRows) }));
    renderExplore("/amendments?status=withdrawn");

    const rail = within(await screen.findByRole("navigation", { name: "Amendment status" }));
    expect(rail.getByRole("button", { name: "Withdrawn · 1" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("link", { name: "Withdrawn reshape" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "New reshape" })).not.toBeInTheDocument();

    fireEvent.click(rail.getByRole("button", { name: "All · 3" }));
    expect(await screen.findByRole("link", { name: "New reshape" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Earlier reshape" })).toBeInTheDocument();
    expect(rail.getByRole("button", { name: "Proposed · 1" })).toBeInTheDocument();
    expect(rail.getByRole("button", { name: "Approved · 1" })).toBeInTheDocument();
  });

  it.each([false, true])("keeps the permanent entry usable with an empty project and failed fetches=%s", async (failed) => {
    stubFetch(baseRoutes({
      "/api/tree": { project: "acme", nodes: [] } satisfies TreeModel,
      "/api/ls?noun=amendments": emptyList("amendment"),
    }), failed ? {
      "/api/status": "Summary unavailable",
      "/api/tree": "Tree unavailable",
      "/api/ls?noun=question": "Questions unavailable",
      "/api/ls?noun=amendments": "Amendment list unavailable",
    } : {});
    renderExplore();
    fireEvent.click(await screen.findByRole("link", { name: "Amendments" }));
    expect(await screen.findByText(failed ? "Amendment list unavailable" : "No amendments yet.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: /^browse$/i }));
    expect(await screen.findByRole("link", { name: "Amendments" })).toHaveAttribute("href", "/amendments");
  });

  it("still lists amendments as a Filter noun", async () => {
    stubFetch(baseRoutes({ "/api/ls?noun=amendments": listResult("amendment", amendmentRows) }));
    renderExplore("/explore?mode=filter&noun=amendments");
    expect(await screen.findByRole("link", { name: "Earlier reshape" })).toHaveAttribute("href", "/node/amendment/A-approved");
    expect(screen.getByRole("combobox", { name: "Noun" })).toHaveValue("amendments");
    expect(screen.getByRole("link", { name: "Amendments" })).toHaveAttribute("aria-current", "page");
  });
});

describe("Relationship rows: question labels", () => {
  it("shows a question id, keeps keyed nouns on keys, and leaves dangling refs honest", () => {
    const question = {
      noun: "question",
      id: "Q-related",
      ref: "Q-related",
      status: "open",
      title: QUESTION_TITLE,
      statement: QUESTION_STATEMENT,
    };
    const feature = {
      noun: "feature",
      id: "F-related",
      key: "audit/export",
      ref: "audit/export (F-related)",
      status: "draft",
      title: "Audit export",
    };
    const dangling = {
      noun: "",
      id: "F-gone",
      ref: "F-gone",
      status: "",
    };

    const { container } = render(
      <MemoryRouter>
        <RelatedList items={[question, feature, dangling]} />
      </MemoryRouter>,
    );

    const rows = container.querySelectorAll(".related-list > li");
    expect(within(rows[0] as HTMLElement).getByRole("link", { name: QUESTION_TITLE })).toHaveAttribute(
      "href",
      "/node/question/Q-related",
    );
    expect(within(rows[0] as HTMLElement).getByText("Q-related").className).toContain("mono");
    expect(within(rows[1] as HTMLElement).getByText("audit/export").className).toContain("mono");
    expect(rows[1].textContent).not.toContain("F-related");
    expect(within(rows[2] as HTMLElement).getByText("F-gone")).toBeInTheDocument();
    expect(within(rows[2] as HTMLElement).queryByRole("link")).not.toBeInTheDocument();
    expect(screen.queryByText((_, element) => element?.textContent === QUESTION_STATEMENT)).not.toBeInTheDocument();
  });
});

describe("Reading pane: load failures", () => {
  it("frames a 404 as no-such-node (a stale deep link), not a raw fetch string", async () => {
    // baseRoutes maps no /api/node route, so the selection 404s - exactly
    // what a deep link to a since-deleted node produces.
    stubFetch(baseRoutes());

    renderExplore("/node/feature/F-gone");

    expect(await screen.findByText("No such node")).toBeInTheDocument();
    expect(screen.getByText("feature/F-gone")).toBeInTheDocument();
  });

  it("keeps the honest raw error for a non-404 failure", async () => {
    const routes = baseRoutes();
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown) => {
        const url = String(input);
        if (url === "/api/node/feature/F-500") {
          return Promise.resolve({
            ok: false,
            status: 500,
            statusText: "Internal Server Error",
            text: () => Promise.resolve("boom"),
          });
        }
        if (url in routes) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
        }
        return Promise.resolve({
          ok: false,
          status: 404,
          statusText: "Not Found",
          text: () => Promise.resolve(""),
        });
      }),
    );

    renderExplore("/node/feature/F-500");

    expect(
      await screen.findByText(
        "GET /api/node/feature/F-500 failed (500 Internal Server Error): boom",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("No such node")).not.toBeInTheDocument();
  });
});

describe("The hierarchy persists on every node page (JC, 2026-08-05)", () => {
  it("keeps the navigator beside a node page and highlights a key-addressed task", async () => {
    const page: NodePage = {
      node: {
        noun: "task",
        id: "T-health",
        key: "foundation/app-skeleton-ci-boot/serve-the-health-check",
        title: "Serve the health check",
        status: "proposed",
      },
      ref: "foundation/app-skeleton-ci-boot/serve-the-health-check (T-health)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(
      baseRoutes({
        "/api/node/task/foundation/app-skeleton-ci-boot/serve-the-health-check": page,
      }),
    );

    // The address is the canonical KEY, not the id: the navigator's rows must
    // still find the task in the tree payload (TreeNode carries both) and mark
    // it selected, or a human pasting a key off the terminal gets a sidebar
    // opened to nothing.
    const { container } = renderExplore(
      "/node/task/foundation/app-skeleton-ci-boot/serve-the-health-check",
    );

    await screen.findByText("App Skeleton & Green CI Boot");
    const selected = container.querySelector(".explore-task.is-selected");
    expect(selected).not.toBeNull();
    expect(selected?.textContent).toContain("Serve the health check");
  });

  it("opens the other epic when a cross-epic dependency is followed", async () => {
    const opsFeature: NodePage = {
      node: {
        noun: "feature",
        id: "F-ops1",
        key: "ops-security/first",
        title: "First Ops Feature",
        status: "draft",
      },
      ref: "ops-security/first (F-ops1)",
      prose: null,
      relationships: {
        epic: { noun: "epic", id: "E-ops1", key: "ops-security", ref: "ops-security (E-ops1)", status: "ready" },
        depends_on: [
          {
            noun: "feature",
            id: "F-aud1",
            key: "audit-engine/first",
            ref: "audit-engine/first (F-aud1)",
            status: "draft",
            title: "First Audit Feature",
          },
        ],
      },
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    const auditFeature: NodePage = {
      node: {
        noun: "feature",
        id: "F-aud1",
        key: "audit-engine/first",
        title: "First Audit Feature",
        status: "draft",
      },
      ref: "audit-engine/first (F-aud1)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(
      baseRoutes({
        "/api/node/feature/F-ops1": opsFeature,
        "/api/node/feature/F-aud1": auditFeature,
      }),
    );

    const { container } = renderExplore("/node/feature/F-ops1");

    await screen.findByRole("heading", { level: 1, name: "First Ops Feature" });
    // Tab labels are Title Case.
    fireEvent.click(screen.getByRole("button", { name: /Dependencies/ }));
    // The dependency row's link leads with the target's title.
    fireEvent.click(await screen.findByRole("link", { name: "First Audit Feature" }));

    // The navigator opened the OTHER epic because it now holds the selection,
    // and the row itself carries the selected mark.
    await screen.findByRole("heading", { level: 1, name: "First Audit Feature" });
    const selected = container.querySelector(".explore-feat.is-selected");
    expect(selected).not.toBeNull();
    expect(selected?.textContent).toContain("First Audit Feature");
  });

  it("renders a question page with its title in the shell and the complete statement in the body", async () => {
    // A question has no position in the epic/feature tree, so its page keeps
    // the navigator for context and simply marks nothing selected.
    const page: NodePage = {
      node: {
        noun: "question",
        id: "Q-1",
        status: "open",
        title: QUESTION_TITLE,
        statement: QUESTION_STATEMENT,
      },
      ref: "Q-1",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/question/Q-1": page }));

    const { container } = renderExplore("/node/question/Q-1");

    expect(await screen.findByRole("heading", { level: 1, name: QUESTION_TITLE })).toBeInTheDocument();
    const statement = container.querySelector(".question-statement") as HTMLElement;
    const paragraphs = statement.querySelectorAll(":scope > p");
    expect(container.querySelectorAll(".question-statement")).toHaveLength(1);
    expect(paragraphs).toHaveLength(2);
    expect(paragraphs[0].textContent).toBe(
      "Should the audit select one record per project or one record per project phase?",
    );
    expect(paragraphs[1].textContent).toBe(
      "The answer changes how multi-year projects are counted in review queues and exported reports.",
    );
    expect(screen.getByText("Foundation")).toBeInTheDocument();
    expect(container.querySelector(".is-selected")).toBeNull();
  });

  it("redirects the old /explore/:noun/:id selection URL to the node page", async () => {
    const page: NodePage = {
      node: {
        noun: "feature",
        id: "F-pueftm",
        key: "foundation/app-skeleton-ci-boot",
        title: "App Skeleton & Green CI Boot",
        status: "ready",
      },
      ref: "foundation/app-skeleton-ci-boot (F-pueftm)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    stubFetch(baseRoutes({ "/api/node/feature/F-pueftm": page }));

    renderExplore("/explore/feature/F-pueftm");

    // A pre-change bookmark still lands on the node, now under the shell.
    expect(
      await screen.findByRole("heading", { level: 1, name: "App Skeleton & Green CI Boot" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Foundation")).toBeInTheDocument();
  });
});

// Task 4 (docs/spec/ui-surface.md): both the expansion
// map and the split width survive a reload. A MemoryRouter remount (unmount,
// then a fresh render) is this suite's stand-in for a real reload, the same
// idiom build.test.tsx's own persistence tests use for the epic selection
// and the panel state. The afterEach above now clears localStorage between
// tests (added as part of this task, since Explore had persisted nothing
// before it), so every test here starts from a clean slate unless it seeds
// storage itself.
describe("Explore: expansion and split width persist across a remount (task 4)", () => {
  it("restores an opened branch and a dragged width, visible on the tree's very first render", async () => {
    stubFetch(baseRoutes());

    const first = renderExplore();
    await screen.findByText("Foundation");
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    await screen.findByText("App Skeleton & Green CI Boot");

    // Drag the width to 250px, the same mechanics the resizable-split suite
    // above already pins.
    const split = first.container.querySelector(".explore-split") as HTMLElement;
    const divider = screen.getByRole("separator", { name: "Resize navigator" });
    vi.spyOn(split, "getBoundingClientRect").mockReturnValue({
      left: 40,
      top: 0,
      right: 840,
      bottom: 600,
      width: 800,
      height: 600,
      x: 40,
      y: 0,
      toJSON: () => ({}),
    });
    fireEvent.mouseDown(divider);
    fireEvent.mouseMove(window, { clientX: 290, buttons: 1 });
    fireEvent.mouseUp(window);
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("250px");

    // The persist effects are plain useEffects; give them a tick to land
    // before treating this as "what a reload would find in storage" - a real
    // reload always happens well after the browser has painted and React has
    // flushed, never mid-render.
    await waitFor(() => {
      expect(window.localStorage.getItem("tiller-explore-split")).toBe("250");
    });
    await waitFor(() => {
      expect(window.localStorage.getItem("tiller-explore-open:acme")).not.toBeNull();
    });

    first.unmount();
    stubFetch(baseRoutes());
    renderExplore();

    // The FIRST render already shows the restored open set - no click
    // needed to reveal Foundation's features this time.
    expect(await screen.findByText("App Skeleton & Green CI Boot")).toBeInTheDocument();

    const restoredSplit = document.querySelector(".explore-split") as HTMLElement;
    expect(restoredSplit.style.getPropertyValue("--explore-nav-w")).toBe("250px");
  });

  it("does not restore a hand-closed branch's opened sibling across a remount, either (a full open/close mix round-trips)", async () => {
    stubFetch(baseRoutes());

    const first = renderExplore();
    await screen.findByText("Foundation");
    // Open Foundation, open its App Skeleton feature down to the task tier,
    // then hand-close the feature - the map now holds Foundation true and
    // App Skeleton false side by side.
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    await screen.findByText("App Skeleton & Green CI Boot");
    fireEvent.click(screen.getByRole("button", { name: /Expand App Skeleton & Green CI Boot/i }));
    await screen.findByText("Boot the app");
    fireEvent.click(screen.getByRole("button", { name: /Collapse App Skeleton & Green CI Boot/i }));
    expect(screen.queryByText("Boot the app")).not.toBeInTheDocument();

    await waitFor(() => {
      expect(window.localStorage.getItem("tiller-explore-open:acme")).not.toBeNull();
    });

    first.unmount();
    stubFetch(baseRoutes());
    renderExplore();

    // Foundation is open (its features show)...
    expect(await screen.findByText("App Skeleton & Green CI Boot")).toBeInTheDocument();
    // ...but the hand-closed feature stayed closed, exactly as remembered.
    expect(screen.queryByText("Boot the app")).not.toBeInTheDocument();
  });

  it("a second project's stored expansion does not bleed into this one", async () => {
    // Seeded for a project this fixture set never names; baseRoutes' own
    // project.key is "acme".
    window.localStorage.setItem(
      "tiller-explore-open:other-project",
      JSON.stringify([["E-u2w9wv", true]]),
    );
    stubFetch(baseRoutes());

    renderExplore();

    // The default, collapsed-by-hand render applies, exactly as if nothing
    // had ever been stored for THIS project.
    await screen.findByText("Foundation");
    expect(screen.queryByText("App Skeleton & Green CI Boot")).not.toBeInTheDocument();
  });

  it("first-ever visit (nothing stored for this project) renders today's defaults: everything collapsed, 300px split", async () => {
    stubFetch(baseRoutes());

    const { container } = renderExplore();

    await screen.findByText("Foundation");
    expect(screen.queryByText("App Skeleton & Green CI Boot")).not.toBeInTheDocument();
    const split = container.querySelector(".explore-split") as HTMLElement;
    expect(split.style.getPropertyValue("--explore-nav-w")).toBe("300px");

    // A first-ever visit must leave storage
    // genuinely untouched, not quietly write the untouched default map back
    // out as "[]" the moment the project key resolves. Settle well past
    // when that would have happened (the persist effect is a plain
    // useEffect, already flushed by the findByText above) and check the key
    // is still absent outright, not merely empty.
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(window.localStorage.getItem("tiller-explore-open:acme")).toBeNull();
  });

  it("close-all's empty map persists as the remembered state and stays fully collapsed on remount, distinct from nothing having been stored", async () => {
    stubFetch(baseRoutes());

    const first = renderExplore();
    await screen.findByText("Foundation");
    fireEvent.click(screen.getByRole("button", { name: /Expand Foundation/i }));
    await screen.findByText("App Skeleton & Green CI Boot");

    fireEvent.click(screen.getByRole("button", { name: "Close all" }));
    expect(screen.queryByText("App Skeleton & Green CI Boot")).not.toBeInTheDocument();

    // The persisted value is the present-but-empty array, not an absent key
    // (expansionStorage.ts's own round-trip contract: a real, empty map
    // reads back distinct from nothing stored).
    await waitFor(() => {
      expect(window.localStorage.getItem("tiller-explore-open:acme")).toBe("[]");
    });

    first.unmount();
    stubFetch(baseRoutes());
    renderExplore();

    // Stays fully collapsed on the fresh mount - the remembered close-all,
    // not a restore that silently did nothing.
    await screen.findByText("Foundation");
    expect(screen.queryByText("App Skeleton & Green CI Boot")).not.toBeInTheDocument();
  });
});

// Review of task 4: a Critical bug the reviewer confirmed with a
// throwaway probe - a deep link's auto-opened ancestry rendered, then
// vanished ~30ms later the instant /api/status resolved after /api/tree,
// because the restore effect replaced the map outright instead of composing
// with what auto-open had already materialized. stubFetchWithDelay is what
// makes this reproducible at all: baseRoutes' plain stubFetch resolves every
// route in one microtask drain, so /api/tree and /api/status always land in
// the same commit and the bug never had a window to show itself there.
describe("Explore: restore composes with auto-open regardless of fetch arrival order (fix round)", () => {
  it("keeps a deep link's auto-opened ancestry when /api/tree resolves before /api/status, and remembers it afterward", async () => {
    const taskPage: NodePage = {
      node: {
        noun: "task",
        id: "T-health",
        key: "foundation/app-skeleton-ci-boot/serve-the-health-check",
        title: "Serve the health check",
        status: "proposed",
      },
      ref: "foundation/app-skeleton-ci-boot/serve-the-health-check (T-health)",
      prose: null,
      relationships: {},
      backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [],
      eval: null,
      
    };
    // Stored for THIS project already, but for a wholly different branch -
    // it says nothing about Foundation or its App Skeleton feature, the
    // ancestry this deep link needs auto-open to materialize on its own.
    window.localStorage.setItem("tiller-explore-open:acme", JSON.stringify([["E-audit1", true]]));
    stubFetchWithDelay(baseRoutes({ "/api/node/task/T-health": taskPage }), ["/api/status"]);

    renderExplore("/node/task/T-health");

    // /api/tree (not delayed) lands first, but the split does not paint at
    // all until summarySettled (Explore.tsx), and the auto-open effect holds
    // on that same flag - so by the time this line can find
    // anything, /api/status has already resolved and auto-open has already
    // had its one chance to run. An earlier version of this comment claimed
    // auto-open ran "well before /api/status resolved"; that was true only
    // before the arrival-order race was closed. What this test actually
    // pins is the composed OUTCOME, not an ordering: the deep link's own
    // ancestry materializes regardless of which fetch happens to land first.
    expect(await screen.findByText("App Skeleton & Green CI Boot")).toBeInTheDocument();

    // /api/status resolves ~20ms later. Give it, and the render it drives,
    // comfortably longer than that to land, then assert the ancestry is
    // STILL there - the exact vanish the reviewer's probe caught would fail
    // this line, not the findByText above (which only proves it appeared
    // once, not that it stayed).
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(screen.getByText("App Skeleton & Green CI Boot")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 1, name: "Serve the health check" })).toBeInTheDocument();

    // The composed map is what gets remembered too, not the stale stored
    // value alone: the next persist carries both the pre-existing Audit
    // Engine entry AND the newly materialized Foundation ancestry.
    //
    // These three ids are each defined by only ONE side (Audit Engine by
    // stored alone, Foundation and the feature by auto-open alone), so this
    // block alone cannot catch an inverted merge direction (stored-wins
    // instead of live-wins) - the same map comes out either way. The merge's
    // own precedence direction is pinned directly, at a grain where that
    // limitation does not apply, by mergeRestored's own unit tests
    // (expansion.test.ts): a conflicting id resolves to live's
    // value in both directions, independent of whatever any integration
    // scenario can or cannot construct. The determinism-pin tests below
    // cover the other half of this rule - the hand-closed case, decided by
    // autoOpen's own skip-if-held-false check, not by this merge.
    await waitFor(() => {
      const raw = window.localStorage.getItem("tiller-explore-open:acme");
      expect(raw).not.toBeNull();
      const stored = new Map(JSON.parse(raw ?? "[]") as [string, boolean][]);
      expect(stored.get("E-audit1")).toBe(true);
      expect(stored.get("E-u2w9wv")).toBe(true); // Foundation
      expect(stored.get("F-pueftm")).toBe(true); // App Skeleton feature
    });
  });

  it("renders no navigator to click until /api/status settles, closing the early-click race the same fix relies on", async () => {
    stubFetchWithDelay(baseRoutes(), ["/api/status"]);

    renderExplore();

    // The tree itself is not delayed, but nothing from it is on screen yet -
    // the whole split, chevrons included, waits behind the summary gate.
    expect(screen.queryByText("Foundation")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Expand/i })).not.toBeInTheDocument();

    // Once /api/status settles, the real split (and Foundation's own row)
    // appears - there was never a window where a chevron existed to click
    // ahead of the restore effect that guards it.
    expect(await screen.findByText("Foundation")).toBeInTheDocument();
  });
});

// A later gap surfaced on top of the five confirmed
// earlier fixes - the hand-closed rule ("a branch closed by hand stays
// closed even when selected", the plan's and the own settled
// words) was arrival-order dependent. Tree-first: auto-open ran before any
// stored map had been merged in, found no entry for the hand-closed id, and
// opened it - the restore's own compose then let that "true" survive over
// the remembered "false" (prev, being the more recent write, outranked
// stored), and the next persist rewrote storage without the close.
// Status-first: the stored "false" was already merged in by the time
// auto-open ran, so its own skip-if-held-false rule correctly left it shut.
// Same reader, same stored state, opposite outcomes by fetch timing alone.
//
// The fix: the auto-open effect now holds until summarySettled (Explore.tsx,
// same gate the split's own first paint already waits on), which guarantees
// the restore effect above it has always had its one chance to merge a
// stored map in before auto-open ever calls autoOpen() at all - closing the
// window where fetch arrival order could decide the outcome. These two
// tests run the identical hand-closed-deep-link arc under BOTH orderings
// and assert the SAME result either way: the branch stays closed, and
// storage still says so afterward.
describe("Explore: the hand-closed rule holds deterministically regardless of fetch arrival order (round 2 fix)", () => {
  const taskPage: NodePage = {
    node: {
      noun: "task",
      id: "T-health",
      key: "foundation/app-skeleton-ci-boot/serve-the-health-check",
      title: "Serve the health check",
      status: "proposed",
    },
    ref: "foundation/app-skeleton-ci-boot/serve-the-health-check (T-health)",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    
  };

  // Shared arc: the App Skeleton feature (F-pueftm) is stored hand-closed;
  // the deep link addresses T-health, one of ITS OWN tasks. Foundation (the
  // epic, one tier up) carries no stored entry at all, so auto-open still
  // opens it normally - "App Skeleton & Green CI Boot" (the feature's own
  // title, gated by the EPIC's openness, not the feature's) stays visible
  // either way, which is what lets this test tell "the hand-closed feature
  // itself stayed shut" apart from "the whole branch never opened."
  async function assertHandCloseHolds(delayedPaths: string[]) {
    window.localStorage.setItem("tiller-explore-open:acme", JSON.stringify([["F-pueftm", false]]));
    stubFetchWithDelay(baseRoutes({ "/api/node/task/T-health": taskPage }), delayedPaths);

    const { container } = renderExplore("/node/task/T-health");

    // The reading pane renders the selected task regardless of the
    // navigator's own tree state (a separate fetch, /api/node/task/T-health) -
    // this is NOT proof the navigator opened anything, only that the page
    // itself loaded.
    expect(
      await screen.findByRole("heading", { level: 1, name: "Serve the health check" }),
    ).toBeInTheDocument();

    // Foundation opened (its own feature title shows)...
    expect(await screen.findByText("App Skeleton & Green CI Boot")).toBeInTheDocument();
    // ...but the hand-closed feature's own children never rendered: were
    // autoOpen to have won this id even once, T-health's own navigator row
    // would exist (FeatureRow only omits `.explore-tasks` while `open` is
    // false, Navigator.tsx:408-414).
    expect(container.querySelector(".explore-task")).toBeNull();

    // Give any in-flight delayed fetch and its own effects comfortably
    // longer than the mock's delay to settle, then re-check: the exact
    // "renders, then flips open" shape a race would produce would pass the
    // assertion above by pure timing luck if checked too early.
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(container.querySelector(".explore-task")).toBeNull();

    // And the remembered close survives the round-trip: the next persist
    // still carries F-pueftm as false, not silently upgraded to true.
    await waitFor(() => {
      const raw = window.localStorage.getItem("tiller-explore-open:acme");
      expect(raw).not.toBeNull();
      const stored = new Map(JSON.parse(raw ?? "[]") as [string, boolean][]);
      expect(stored.get("F-pueftm")).toBe(false);
    });
  }

  it("stays closed when /api/tree resolves before /api/status (the ordering that used to lose)", async () => {
    await assertHandCloseHolds(["/api/status"]);
  });

  it("stays closed when /api/status resolves before /api/tree (the ordering that already worked)", async () => {
    await assertHandCloseHolds(["/api/tree"]);
  });
});
