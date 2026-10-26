// Vitest/RTL coverage for the Build board (console v2 phase 3),
// the epic-selection view replacing the Phase 1 stub.
// Fetch stubbing follows home.test.tsx / drift.test.tsx's stubFetch-keyed-
// by-URL convention (no MSW in this harness).
//
// The scenarios below are the brief's own required list: the four `?epics=`
// URL cases (absent -> default, present-empty -> honest empty state,
// present-non-empty -> exact selection, unknown keys ignored gracefully);
// column counts from a stubbed payload; the four renderable card conditions
// (ask, blocked, held, drift), including a card carrying blocked AND held
// at once, since the board requires them to render as two distinct flags
// rather than folding into one; the expand chevron's own click target
// stopping propagation so the card underneath does not also navigate;
// panel-collapse persistence; and the honest empty state.
//
// blocked was a real gap this suite once had to work around: Task 12's
// first BuildFeature shipped `held` only, with no `blocked` field at all
// (task-13-report.md's own concern). Commit 9b4c778 (cross-task finding)
// added it; the fixtures and assertions below cover the field now that it
// exists on the wire.

import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigationType } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BuildBoard, BuildEpic, BuildFeature, DriftReportView, ProjectSummary, Queue, RelatedNode } from "../../api";
import Build from "../../views/Build";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

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

function summary(overrides: Partial<ProjectSummary> = {}): ProjectSummary {
  return {
    project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
    contributors: [{ id: "jc", name: "JC Grubbs" }],
    counts: { epics: 3, features: 6, tasks: 4, questions: 0 },
    whose_move: { human: 0, agent: 0 },
    active_epics: ["intake", "form-engine"],
    ...overrides,
  };
}

function task(id: string, status: string, title: string): RelatedNode {
  return { noun: "task", id, key: `intake/eligibility-rules/${id}`, ref: `${id} (${id})`, status, title };
}

function feature(overrides: Partial<BuildFeature> & Pick<BuildFeature, "id" | "status" | "title">): BuildFeature {
  return {
    ref: `${overrides.title} (${overrides.id})`,
    open_work_items: 0,
    task_done: 0,
    task_total: 0,
    tasks: [],
    contributors: [],
    column: overrides.status,
    ...overrides,
  };
}

function epic(overrides: Partial<BuildEpic> & Pick<BuildEpic, "id" | "key" | "title" | "features">): BuildEpic {
  return { noun: "epic", ref: `${overrides.key} (${overrides.id})`, status: "active", contributors: [], ...overrides };
}

function board(): BuildBoard {
  return {
    epics: [
      epic({
        id: "E-1",
        key: "intake",
        title: "Intake & Eligibility",
        // feature_counts is non-nil iff features is non-empty (buildEpicOf's
        // own contract, build_test.go): a fixture carrying features with no
        // feature_counts is a shape the server never actually sends (fix
        // wave, test-fixture realism finding). E-5 below already matched
        // this; E-1..E-4 did not.
        feature_counts: { draft: 1, active: 1, in_review: 1 },
        features: [
          feature({ id: "F-1", status: "draft", title: "Household income worksheet" }),
          feature({
            id: "F-2",
            status: "active",
            title: "Eligibility rules",
            contributors: [{ id: "pat", name: "Pat" }],
            task_done: 1,
            task_total: 4,
            tasks: [
              task("T-1", "done", "Rule table schema"),
              task("T-2", "ready", "Threshold evaluation"),
              task("T-3", "ready", "Phase-count logic"),
              task("T-4", "ready", "Determination record"),
            ],
          }),
          feature({
            id: "F-3",
            status: "in_review",
            title: "Applicant portal auth",
            contributors: [{ id: "pat", name: "Pat" }],
            task_done: 5,
            task_total: 5,
            tasks: [task("T-5", "in_review", "Portal shell wiring")],
          }),
        ],
      }),
      epic({
        id: "E-2",
        key: "form-engine",
        title: "Form Engine",
        feature_counts: { draft: 1, specced: 1 },
        features: [
          feature({ id: "F-4", status: "draft", title: "Versioned schemas" }),
          feature({
            id: "F-5",
            status: "specced",
            title: "Conditional logic",
            task_done: 0,
            task_total: 6,
            tasks: [task("T-6", "draft", "Rule grammar & parser")],
          }),
        ],
      }),
      epic({
        id: "E-3",
        key: "payments",
        title: "Payments",
        feature_counts: { active: 1 },
        features: [
          feature({
            id: "F-6",
            status: "active",
            title: "ACH batching",
            held: { noun: "epic", id: "E-9", ref: "cancelled-scope (E-9)", state: "cancelled" },
          }),
        ],
      }),
      // A dedicated epic for the two blocked-register fixtures, kept out of
      // active_epics and every other test's explicit selection, so adding it
      // never perturbs an existing count assertion elsewhere in this file.
      epic({
        id: "E-4",
        key: "ops",
        title: "Ops",
        feature_counts: { active: 2 },
        features: [
          feature({ id: "F-7", status: "active", title: "Billing sync", blocked: true }),
          feature({
            id: "F-8",
            status: "active",
            title: "Legacy migration",
            blocked: true,
            held: { noun: "epic", id: "E-10", ref: "archived-scope (E-10)", state: "archived" },
          }),
        ],
      }),
      // A dedicated epic for the cancelled-feature counting fixture, same
      // isolation reasoning as "ops" above: kept out of active_epics and every
      // other test's explicit selection. feature_counts mirrors what
      // buildEpicOf actually produces server-side - every feature increments
      // its own literal status key, cancelled included, so the fixture carries
      // "cancelled": 1 the same way a real payload would rather than only the
      // six column keys.
      epic({
        id: "E-5",
        key: "reporting",
        title: "Reporting",
        feature_counts: { draft: 2, cancelled: 1 },
        features: [
          feature({ id: "F-9", status: "draft", title: "Status rollup" }),
          feature({ id: "F-10", status: "draft", title: "Export pipeline" }),
          feature({ id: "F-11", status: "cancelled", title: "Retired dashboard" }),
        ],
      }),
    ],
  };
}

function emptyQueue(): Queue {
  return { entries: [] };
}

function emptyDrift(): DriftReportView {
  return { compared: 0 };
}

function baseRoutes(extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    "/api/build": board(),
    "/api/status": summary(),
    "/api/queue": emptyQueue(),
    "/api/drift": emptyDrift(),
    ...extra,
  };
}

function LocationProbe() {
  const loc = useLocation();
  return <span data-testid="loc">{loc.pathname + loc.search}</span>;
}

// The URL alone cannot distinguish replace from push - both leave the same
// final location. useNavigationType() reports the LAST
// navigation's own kind ("POP" | "PUSH" | "REPLACE"), which is the one
// signal that actually tells them apart.
function NavigationTypeProbe() {
  const type = useNavigationType();
  return <span data-testid="navtype">{type}</span>;
}

function renderBuild(entryOrFeatureOverrides: string | Partial<BuildFeature> = "/build") {
  const entry = typeof entryOrFeatureOverrides === "string" ? entryOrFeatureOverrides : "/build";
  if (typeof entryOrFeatureOverrides !== "string") {
    const payload: BuildBoard = {
      epics: [
        epic({
          id: "E-000001",
          key: "form-engine",
          title: "Form Engine",
          feature_counts: { draft: 1 },
          features: [
            feature({
              id: "F-000001",
              status: "draft",
              title: "Schema engine",
              ...entryOrFeatureOverrides,
            }),
          ],
        }),
      ],
    };
    stubFetch(baseRoutes({ "/api/build": payload }));
  }

  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Build />
      <LocationProbe />
      <NavigationTypeProbe />
    </MemoryRouter>,
  );
}

function loc(): string {
  return screen.getByTestId("loc").textContent ?? "";
}

function navType(): string {
  return screen.getByTestId("navtype").textContent ?? "";
}

it("shows contributors on the feature card and leaves task detail to the expand", async () => {
  renderBuild({
    status: "active",
    contributors: [{ id: "past", name: "Past contributor" }, { id: "sam", name: "Sam" }],
    tasks: [task("T-done", "done", "Finished work"), task("T-live", "check", "Current work")],
    task_done: 1, task_total: 2,
  });
  const featureCard = (await screen.findByText("Schema engine")).closest(".fcard") as HTMLElement;
  expect(within(featureCard).getByText("Past contributor")).toBeInTheDocument();
  expect(within(featureCard).queryByRole("list")).not.toBeInTheDocument();
  expect(within(featureCard).queryByText("Current work")).not.toBeInTheDocument();
});

// ---------- URL semantics: the four cases, exact ----------

describe("Build selection: the four URL cases", () => {
  it("absent ?epics= selects the default (summary.active_epics)", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");

    expect(await screen.findByText("Household income worksheet")).toBeInTheDocument();
    expect(screen.getByText("Versioned schemas")).toBeInTheDocument();
    // Payments (E-3) is not in active_epics, so its card stays off the board.
    expect(screen.queryByText("ACH batching")).not.toBeInTheDocument();
  });

  it("?epics=payments selects exactly that epic, never falling back to the default", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=payments");

    expect(await screen.findByText("ACH batching")).toBeInTheDocument();
    expect(screen.queryByText("Household income worksheet")).not.toBeInTheDocument();
    expect(screen.queryByText("Versioned schemas")).not.toBeInTheDocument();
  });

  it("?epics= (present, empty) is the honest empty state - not the default, not a blank board", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=");

    // The exact board-bar sentence, not the (longer) EmptyNote sentence
    // beneath it - both contain the same "No epics selected" substring, so
    // an exact match keeps this assertion from being ambiguous about which
    // one it is pinning.
    await screen.findByText("No epics selected.");
    expect(screen.getByText(/No epics selected\. Choose one or more/)).toBeInTheDocument();
    // Not the default epics' cards:
    expect(screen.queryByText("Household income worksheet")).not.toBeInTheDocument();
    // Not a blank six-column board either - no column heads render at all
    // while nothing is selected, only the honest empty-state note.
    expect(screen.queryByText("Draft")).not.toBeInTheDocument();
  });

  it("unknown keys are ignored gracefully, never a crash, and never inflate the count", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=bogus,intake");

    expect(await screen.findByText("Eligibility rules")).toBeInTheDocument();
    expect(screen.getByText(/Showing 3 features across 1 epic/)).toBeInTheDocument();
  });

  it("a selection change replaces the URL rather than pushing a history entry", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=intake");
    await screen.findByText("Household income worksheet");
    // The initial mount's own navigation is POP (MemoryRouter's starting
    // entry), the baseline this test's REPLACE assertion below has to beat -
    // a push would ALSO leave navType at something other than the initial
    // POP, so this alone would not distinguish the two either; the real
    // proof is the exact "REPLACE" value asserted after the click.
    expect(navType()).toBe("POP");

    fireEvent.click(screen.getByRole("button", { name: /form engine/i }));

    await waitFor(() => {
      expect(loc()).toContain("epics=intake%2Cform-engine");
    });
    // The URL alone is satisfied identically by a push;
    // this is the assertion that actually pins replace-not-push.
    expect(navType()).toBe("REPLACE");
  });
});

// ---------- Epic selection persistence (localStorage bootstrap) ----------
// The URL stays the single source of truth downstream (the four-URL-cases
// suite above is unchanged by any of this); localStorage only seeds it back
// on the one case the URL itself cannot answer - a fresh load with no
// `?epics=` at all. Namespaced per project key (summary().project.key is
// "acme" throughout this file's fixtures), the same house-style flat-key
// idiom panelStorage.ts uses, just with a project suffix.

describe("Build epic selection persistence", () => {
  it("restores a stored selection when no ?epics= is present in the URL", async () => {
    window.localStorage.setItem("tiller-build-epics:acme", "payments");
    stubFetch(baseRoutes());
    renderBuild("/build");

    expect(await screen.findByText("ACH batching")).toBeInTheDocument();
    // Not the served default (active_epics: intake, form-engine).
    expect(screen.queryByText("Household income worksheet")).not.toBeInTheDocument();

    // Bootstrap only: the restored selection lands on the URL itself, not
    // just in memory, so it stays the one source everything downstream reads.
    await waitFor(() => {
      expect(loc()).toContain("epics=payments");
    });
  });

  it("an explicit ?epics= wins over a conflicting stored selection and overwrites it", async () => {
    window.localStorage.setItem("tiller-build-epics:acme", "payments");
    stubFetch(baseRoutes());
    renderBuild("/build?epics=intake");

    await screen.findByText("Household income worksheet");
    expect(screen.queryByText("ACH batching")).not.toBeInTheDocument();

    // The pasted URL's own selection becomes the new remembered state, not
    // merely ignored-in-place: a later fresh load with no param must
    // reproduce THIS selection, not the stale "payments" it overwrote.
    await waitFor(() => {
      expect(window.localStorage.getItem("tiller-build-epics:acme")).toBe("intake");
    });
  });

  it("toggling an epic persists the new selection to storage", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=intake");
    await screen.findByText("Household income worksheet");

    fireEvent.click(screen.getByRole("button", { name: /form engine/i }));

    await waitFor(() => {
      expect(window.localStorage.getItem("tiller-build-epics:acme")).toBe("intake,form-engine");
    });
  });

  it("deselecting every epic persists as empty and stays empty across a remount", async () => {
    stubFetch(baseRoutes());
    const first = renderBuild("/build?epics=");
    await screen.findByText("No epics selected.");

    await waitFor(() => {
      // The present-but-empty form, not omitted - the same distinction
      // selection.ts's own epicsParamValue draws for the URL.
      expect(window.localStorage.getItem("tiller-build-epics:acme")).toBe("");
    });

    first.unmount();
    stubFetch(baseRoutes());
    renderBuild("/build"); // a genuinely fresh load: no ?epics= at all

    await screen.findByText("No epics selected.");
    // Stays deselected - a stored empty entry is not the same as no entry,
    // so this must NOT revive active_epics.
    expect(screen.queryByText("Household income worksheet")).not.toBeInTheDocument();
  });

  it("a stored selection under a different project key does not bleed into this one", async () => {
    window.localStorage.setItem("tiller-build-epics:other-project", "payments");
    stubFetch(baseRoutes()); // summary().project.key is "acme" here
    renderBuild("/build");

    // No entry under "acme"; falls through to the active_epics default,
    // exactly as if localStorage were empty.
    await screen.findByText("Household income worksheet");
    expect(screen.queryByText("ACH batching")).not.toBeInTheDocument();
  });
});

// ---------- Column counts ----------

describe("Build columns", () => {
  it("counts each column from the served payload, filtered to the selection", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=intake,form-engine");
    await screen.findByText("Household income worksheet");

    function countFor(label: string): string {
      const head = screen.getByText(label).closest(".col-head");
      if (head === null) throw new Error(`no column head for ${label}`);
      return within(head as HTMLElement).getByText(/^\d+$/).textContent ?? "";
    }

    // draft: F-1, F-4 (2); specced: F-5 (1); active: F-2 (1);
    // in_review: F-3 (1); done: none (0).
    expect(countFor("Draft")).toBe("2");
    expect(countFor("Specced")).toBe("1");
    expect(countFor("Ready")).toBe("0");
    expect(countFor("Active")).toBe("1");
    expect(countFor("In review")).toBe("1");
    expect(countFor("Done")).toBe("0");
  });

  it("keeps every column head in the static heads row, outside the scrolling body", async () => {
    stubFetch(baseRoutes());
    const { container } = renderBuild("/build?epics=intake,form-engine");
    await screen.findByText("Household income worksheet");

    const heads = container.querySelector(".board-scroll > .board-heads");
    const body = container.querySelector(".board-scroll > .board-body");
    expect(heads?.querySelectorAll(".col-head")).toHaveLength(6);
    expect(body?.querySelectorAll(".board-column")).toHaveLength(6);
    expect(body?.querySelector(".col-head")).toBeNull();
    expect(within(body as HTMLElement).getByText("Household income worksheet")).toBeInTheDocument();
  });
});

// ---------- A cancelled feature: counted but never rendered ----------

describe("Build counting agrees with what actually renders", () => {
  it("the board bar, the panel badge, and the mix bar all agree with the rendered cards, excluding a cancelled feature", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=reporting");
    await screen.findByText("Status rollup");

    // The board bar: 2, not 3 (F-11 is cancelled and renders on no column).
    expect(screen.getByText(/Showing 2 features across 1 epic/)).toBeInTheDocument();

    // The panel's er-n badge, scoped to the Reporting row specifically.
    const row = screen.getByText("Reporting", { selector: ".er-name" }).closest(".epic-row");
    expect(row).not.toBeNull();
    expect(within(row as HTMLElement).getByText("2")).toBeInTheDocument();

    // The mix bar's own accessible name: the visible segment PLUS the
    // excluded remainder named explicitly, so the caption never implies six
    // columns are the whole epic when a feature landed in neither. This is
    // this task's own answer to "decide whether the mix caption should name
    // the excluded remainder" - it does, as a trailing "+N status" clause.
    expect(screen.getByRole("img", { name: "2 draft · +1 cancelled" })).toBeInTheDocument();

    // The rendered cards themselves: exactly the two draft features, the
    // cancelled one nowhere on the page - not merely uncounted, genuinely
    // never rendered, which is what "agrees with the rendered cards" means.
    expect(screen.getByText("Status rollup")).toBeInTheDocument();
    expect(screen.getByText("Export pipeline")).toBeInTheDocument();
    expect(screen.queryByText("Retired dashboard")).not.toBeInTheDocument();
  });
});

// ---------- The epic row's accessible name ----------

describe("Build epic row accessibility", () => {
  it("StateBar sits outside the toggle button, so the button's own name is its label and count only", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=reporting");
    await screen.findByText("Status rollup");

    const row = screen.getByText("Reporting", { selector: ".er-name" }).closest(".epic-row");
    expect(row).not.toBeNull();
    const toggle = within(row as HTMLElement).getByRole("button");
    // The mix bar's own caption ("2 draft · +1 cancelled", pinned above) is
    // NOT part of this name: before this fix, StateBar nested inside the
    // button folded its aria-label into the button's accessible name (a
    // childless epic used to announce "...no features yet" on top of its
    // own label), which this exact match would have failed to find.
    expect(toggle).toHaveAccessibleName("Reporting 2");

    // The bar itself is still reachable, just as an independent img-role
    // sibling rather than button content (pinned by the previous test's own
    // getByRole("img", ...) query succeeding outside of any button scope).
    expect(within(row as HTMLElement).queryByRole("img")).not.toBeNull();
  });

  it("a childless epic's bar carries no caption (StateBar's own aria-hidden contract, Navigator.tsx's precedent)", async () => {
    stubFetch(
      baseRoutes({
        "/api/build": {
          epics: [epic({ id: "E-6", key: "empty-epic", title: "Nothing yet", features: [] })],
        },
      }),
    );
    renderBuild("/build?epics=empty-epic");
    // The empty selection would read as "no epics selected" if the epic
    // itself failed to match; finding the row's own name first proves the
    // epic matched and rendered with zero features, not that selection
    // failed silently.
    await screen.findByText("Nothing yet");

    const row = screen.getByText("Nothing yet").closest(".epic-row");
    expect(row).not.toBeNull();
    expect(within(row as HTMLElement).getByRole("button")).toHaveAccessibleName("Nothing yet 0");
    // No img role at all: StateBar renders aria-hidden (no role) rather than
    // role="img" when its caption prop is undefined.
    expect(within(row as HTMLElement).queryByRole("img")).toBeNull();
  });
});

// ---------- Card conditions ----------

describe("Build card conditions", () => {
  it("links a nonzero open shelf count to the feature roll-up with all terminal work", async () => {
    renderBuild({ open_work_items: 2 });

    const link = await screen.findByRole("link", { name: "2 open fixes attached" });
    expect(link).toHaveAttribute("href", "/fix?attached=F-000001&terminal=all");
    expect(link.parentElement).toHaveClass("f-meta");
  });

  it("omits the shelf glyph when the open count is zero", async () => {
    renderBuild({ open_work_items: 0 });

    await screen.findByText("Schema engine");
    expect(screen.queryByRole("link", { name: /open fixes attached/ })).not.toBeInTheDocument();
  });

  it("renders the ask flag from a queue entry joined by feature id", async () => {
    stubFetch(
      baseRoutes({
        "/api/queue": {
          entries: [
            {
              kind: "attest_criterion",
              noun: "feature",
              id: "F-3",
              ref: "intake/applicant-portal-auth (F-3)",
              command: "tiller attest feature intake/applicant-portal-auth --criterion C3 --status pass",
              prose: "C3",
            },
          ],
        },
      }),
    );
    renderBuild("/build");
    await screen.findByText("Applicant portal auth");

    const card = screen.getByText("Applicant portal auth").closest(".fcard");
    expect(card).not.toBeNull();
    expect(within(card as HTMLElement).getByText("attest C3")).toHaveClass("flag", "ask");
  });

  it("renders the held condition in the blocked register", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=payments");
    await screen.findByText("ACH batching");

    const card = screen.getByText("ACH batching").closest(".fcard");
    expect(card).not.toBeNull();
    const flag = within(card as HTMLElement).getByText("held");
    expect(flag).toHaveClass("flag", "blocked");
  });

  it("renders the manual blocked flag alone (BuildFeature.blocked, commit 9b4c778)", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=ops");
    await screen.findByText("Billing sync");

    const card = screen.getByText("Billing sync").closest(".fcard");
    expect(card).not.toBeNull();
    const flags = within(card as HTMLElement).getAllByText(/^(blocked|held)$/);
    expect(flags).toHaveLength(1);
    expect(flags[0]).toHaveTextContent("blocked");
    expect(flags[0]).toHaveClass("flag", "blocked");
  });

  it("renders blocked AND held together as two distinct flags, never folded into one (issue #39)", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build?epics=ops");
    await screen.findByText("Legacy migration");

    const card = screen.getByText("Legacy migration").closest(".fcard");
    expect(card).not.toBeNull();
    const blockedFlag = within(card as HTMLElement).getByText("blocked");
    const heldFlag = within(card as HTMLElement).getByText("held");
    expect(blockedFlag).toHaveClass("flag", "blocked");
    expect(heldFlag).toHaveClass("flag", "blocked");
    // Two separate elements, not one span carrying both words.
    expect(blockedFlag).not.toBe(heldFlag);
  });

  it("renders ask and blocked together as two flags from different registers, distinct elements", async () => {
    stubFetch(
      baseRoutes({
        "/api/queue": {
          entries: [
            {
              kind: "approve_feature",
              noun: "feature",
              id: "F-7",
              ref: "ops/billing-sync (F-7)",
              command: "tiller approve feature ops/billing-sync",
            },
          ],
        },
      }),
    );
    renderBuild("/build?epics=ops");
    await screen.findByText("Billing sync");

    const card = screen.getByText("Billing sync").closest(".fcard");
    expect(card).not.toBeNull();
    const askFlag = within(card as HTMLElement).getByText("approve");
    const blockedFlag = within(card as HTMLElement).getByText("blocked");
    expect(askFlag).toHaveClass("flag", "ask");
    expect(blockedFlag).toHaveClass("flag", "blocked");
    // Two distinct elements, one per register, never one span carrying both.
    expect(askFlag).not.toBe(blockedFlag);
  });

  // Completes the cross-register pair matrix cheaply on one fixture:
  // blocked+held is pinned above, ask+blocked is pinned above, this is
  // drift+ask - the two overlay-joined registers (queue and drift) sharing a
  // card, still rendered as two distinct elements rather than one folding
  // into the other.
  it("renders drift and ask together as two flags from different registers, distinct elements", async () => {
    stubFetch(
      baseRoutes({
        "/api/queue": {
          entries: [
            {
              kind: "attest_criterion",
              noun: "feature",
              id: "F-1",
              ref: "intake/household-income-worksheet (F-1)",
              command: "tiller attest feature intake/household-income-worksheet --criterion C1 --status pass",
              prose: "C1",
            },
          ],
        },
        "/api/drift": {
          compared: 1,
          candidates: {
            stale_spec: [{ marker: "stale_spec", noun: "feature", id: "F-1" }],
          },
        },
      }),
    );
    renderBuild("/build");
    await screen.findByText("Household income worksheet");

    const card = screen.getByText("Household income worksheet").closest(".fcard");
    expect(card).not.toBeNull();
    const askFlag = within(card as HTMLElement).getByText("attest C1");
    const driftFlag = within(card as HTMLElement).getByText("drift");
    expect(askFlag).toHaveClass("flag", "ask");
    expect(driftFlag).toHaveClass("flag", "drift");
    // Two distinct elements, one per register, never one span carrying both.
    expect(askFlag).not.toBe(driftFlag);
  });

  it("renders the neutral drift flag for an id /api/drift names as a candidate", async () => {
    stubFetch(
      baseRoutes({
        "/api/drift": {
          compared: 1,
          candidates: {
            stale_spec: [{ marker: "stale_spec", noun: "feature", id: "F-1" }],
          },
        },
      }),
    );
    renderBuild("/build");
    await screen.findByText("Household income worksheet");

    const card = screen.getByText("Household income worksheet").closest(".fcard");
    expect(card).not.toBeNull();
    expect(within(card as HTMLElement).getByText("drift")).toHaveClass("flag", "drift");
  });
});

// ---------- Overlay-failure notice ----------
// A failed /api/queue or /api/drift read already degrades every card to
// carrying no ask/drift flag (best-effort, above); on its own that reads
// identically to a clean board with nothing to flag. The board bar names
// the failure instead of going quiet about it.

describe("Build overlay-failure notice", () => {
  it("names a failed queue read without blocking the board's own cards", async () => {
    const routes = baseRoutes();
    delete routes["/api/queue"];
    stubFetch(routes);
    renderBuild("/build");

    await screen.findByText("Eligibility rules");
    expect(screen.getByText("asks unavailable")).toBeInTheDocument();
    expect(screen.queryByText("drift unavailable")).not.toBeInTheDocument();
    // Best-effort still stands: the card itself renders, just with no ask
    // flag (queue.error !== null never reaches askLabelsFrom's null guard
    // any differently than a queue that is simply still loading).
    expect(screen.getByText("Eligibility rules").closest(".fcard")).not.toBeNull();
  });

  it("names a failed drift read the same way", async () => {
    const routes = baseRoutes();
    delete routes["/api/drift"];
    stubFetch(routes);
    renderBuild("/build");

    await screen.findByText("Eligibility rules");
    expect(screen.getByText("drift unavailable")).toBeInTheDocument();
    expect(screen.queryByText("asks unavailable")).not.toBeInTheDocument();
  });

  it("stays silent about both overlays once they load cleanly", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");

    await screen.findByText("Eligibility rules");
    expect(screen.queryByText("asks unavailable")).not.toBeInTheDocument();
    expect(screen.queryByText("drift unavailable")).not.toBeInTheDocument();
  });
});

// ---------- Expand: card navigates, chevron does not ----------

describe("Build expanded task rows: readable status", () => {
  it("names each task's raw status as text, in the link and on dangling rows", async () => {
    renderBuild({
      status: "active",
      task_done: 0,
      task_total: 4,
      tasks: [
        task("T-a", "in_progress", "Wire the parser"),
        task("T-b", "check", "Grade the parser"),
        task("T-c", "in_review", "Review the parser"),
        { noun: "", id: "", ref: "T-gone", status: "check", title: "Vanished task" },
      ],
    });
    await screen.findByText("Schema engine");
    const card = screen.getByText("Schema engine").closest(".fcard") as HTMLElement;
    fireEvent.click(within(card).getByRole("button", { name: /expand tasks/i }));

    for (const [title, status, id] of [
      ["Wire the parser", "in_progress", "T-a"],
      ["Grade the parser", "check", "T-b"],
      ["Review the parser", "in_review", "T-c"],
    ]) {
      const link = within(card).getByRole("link", { name: new RegExp(title) });
      expect(link).toHaveAttribute("href", `/node/task/${id}`);
      expect(link).toHaveAccessibleName(expect.stringContaining(status));
    }
    const dangling = within(card).getByText("Vanished task").closest(".trow") as HTMLElement;
    expect(dangling).toHaveTextContent("check");
  });
});

describe("Build card expand vs navigate", () => {
  it("clicking the card body navigates to the feature's node page", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");
    await screen.findByText("Eligibility rules");

    fireEvent.click(screen.getByText("Eligibility rules"));

    await waitFor(() => {
      expect(loc()).toBe("/node/feature/F-2");
    });
  });

  it("clicking the expand chevron opens the task list and does not navigate", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");
    await screen.findByText("Eligibility rules");

    const card = screen.getByText("Eligibility rules").closest(".fcard");
    expect(card).not.toBeNull();
    const chevron = within(card as HTMLElement).getByRole("button", { name: /expand tasks/i });
    fireEvent.click(chevron);

    expect(within(card as HTMLElement).getByText("Threshold evaluation")).toBeInTheDocument();
    expect(loc()).toBe("/build");
  });

  it("a task row inside the expanded list navigates to the task's own page, not the feature's", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");
    await screen.findByText("Eligibility rules");

    const card = screen.getByText("Eligibility rules").closest(".fcard");
    fireEvent.click(within(card as HTMLElement).getByRole("button", { name: /expand tasks/i }));
    fireEvent.click(screen.getByText("Threshold evaluation"));

    await waitFor(() => {
      expect(loc()).toBe("/node/task/T-2");
    });
  });

  it("a feature with no tasks renders no expand chevron at all", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");
    await screen.findByText("Household income worksheet");

    const card = screen.getByText("Household income worksheet").closest(".fcard");
    expect(card).not.toBeNull();
    expect(within(card as HTMLElement).queryByRole("button")).not.toBeInTheDocument();
    expect(within(card as HTMLElement).getByText("no tasks yet")).toBeInTheDocument();
  });

  // A keydown bubbles from whatever is actually
  // focused, so the card's own onKeyDown used to see Enter no matter which
  // descendant fired it and navigate the card away underneath a focused
  // chevron - or race a focused task row's own Link navigation. The target
  // guard fixes it at the source: only a keydown whose target IS the card
  // (never a bubbled one from a child) reaches open().
  it("Enter on the focused expand chevron does not navigate the card away", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");
    await screen.findByText("Eligibility rules");

    const card = screen.getByText("Eligibility rules").closest(".fcard");
    expect(card).not.toBeNull();
    const chevron = within(card as HTMLElement).getByRole("button", { name: /expand tasks/i });

    // jsdom does not synthesize the browser's own default action (a focused
    // <button> firing a click when Enter is pressed on it), so the keydown
    // and the click it would produce are fired as the two separate events a
    // real browser's own activation sequence produces. The keydown alone is
    // what this fix is pinning: before it, the card's handler read Enter off
    // this bubbled event and navigated regardless of where it started.
    fireEvent.keyDown(chevron, { key: "Enter" });
    expect(loc()).toBe("/build");

    fireEvent.click(chevron);
    expect(within(card as HTMLElement).getByText("Threshold evaluation")).toBeInTheDocument();
    expect(loc()).toBe("/build");
  });

  it("Enter on a focused task row navigates the task, not the card underneath it", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");
    await screen.findByText("Eligibility rules");

    const card = screen.getByText("Eligibility rules").closest(".fcard");
    expect(card).not.toBeNull();
    fireEvent.click(within(card as HTMLElement).getByRole("button", { name: /expand tasks/i }));
    const taskRow = screen.getByText("Threshold evaluation");

    // The bubbled keydown must not fire the card's own open() out from under
    // the row's real Link navigation (a race the old, ungated handler lost
    // silently since both would have called navigate()).
    fireEvent.keyDown(taskRow, { key: "Enter" });
    expect(loc()).toBe("/build");
  });

  it("Enter fired directly on the card itself still navigates (target === currentTarget)", async () => {
    stubFetch(baseRoutes());
    renderBuild("/build");
    await screen.findByText("Eligibility rules");

    const card = screen.getByText("Eligibility rules").closest(".fcard");
    expect(card).not.toBeNull();

    fireEvent.keyDown(card as HTMLElement, { key: "Enter" });

    await waitFor(() => {
      expect(loc()).toBe("/node/feature/F-2");
    });
  });
});

// ---------- Panel collapse persistence ----------

describe("Build epic panel collapse", () => {
  it("persists across a remount via localStorage", async () => {
    stubFetch(baseRoutes());
    const first = renderBuild("/build");
    await screen.findByText("Household income worksheet");

    fireEvent.click(screen.getByRole("button", { name: "⟨ Epics" }));
    expect(screen.getByRole("button", { name: "⟩ Epics" })).toBeInTheDocument();
    expect(window.localStorage.getItem("tiller-build-panel")).toBe("closed");

    first.unmount();
    stubFetch(baseRoutes());
    renderBuild("/build");
    await screen.findByText("Household income worksheet");

    // A fresh mount reads the persisted collapsed state rather than
    // flashing open first.
    expect(screen.getByRole("button", { name: "⟩ Epics" })).toBeInTheDocument();
  });
});
