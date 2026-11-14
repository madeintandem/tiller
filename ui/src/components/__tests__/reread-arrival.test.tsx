// Arriving at Build or Fix again while the board is already showing: the
// topbar link for the open page, or the browser's back and forward between
// two addresses of the same board. The page stays mounted, so nothing
// remounts to read; the arrival itself has to reread the board. The page's
// own URL changes (an epic toggle, a filter, the saved-selection restore) are
// not arrivals and read nothing.

import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BuildBoard, BuildFeature, WorkItemsBoard } from "../../api";
import App from "../../App";
import { expectNoLoading, settleReads, stubReread, summary, watchLoading, type Routes } from "./rereadHarness";

afterEach(async () => {
  await settleReads();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
  Reflect.deleteProperty(document, "visibilityState");
});

function Probe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <p data-testid="where">{`${location.pathname}${location.search}${location.hash}`}</p>
      <button onClick={() => navigate(-1)}>history back</button>
      <button onClick={() => navigate(1)}>history forward</button>
    </>
  );
}

function renderConsole(entry: string) {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <App />
      <Probe />
    </MemoryRouter>,
  );
}

const where = () => screen.getByTestId("where").textContent;

function topbarLink(name: string): HTMLElement {
  const nav = document.querySelector(".site-nav");
  if (nav === null) throw new Error("no primary navigation");
  return within(nav as HTMLElement).getByRole("link", { name });
}

type Stub = ReturnType<typeof stubReread>;
const reads = (stub: Stub, url: string) => stub.calls.filter((call) => call.url === url).length;

// Lets any read a click might have started reach the stub before a count.
const settle = () => act(async () => { await Promise.resolve(); });

function buildBoard(status: string): BuildBoard {
  const feature = (id: string, title: string, featureStatus: string): BuildFeature => ({
    id, title, status: featureStatus, column: featureStatus, ref: `${title} (${id})`,
    open_work_items: 0, task_done: 0, task_total: 0, tasks: [], contributors: [],
  });
  return {
    epics: [
      {
        noun: "epic", id: "E-1", key: "form", title: "Form Engine", ref: "form (E-1)", status: "active",
        contributors: [], feature_counts: { [status]: 1 }, features: [feature("F-1", "Schema engine", status)],
      },
      {
        noun: "epic", id: "E-2", key: "intake", title: "Intake", ref: "intake (E-2)", status: "active",
        contributors: [], feature_counts: { draft: 1 }, features: [feature("F-2", "Eligibility rules", "draft")],
      },
    ],
  };
}

function buildRoutes(status: string): Routes {
  return {
    "/api/status": { ...summary(), active_epics: ["form", "intake"] },
    "/api/version": { version: "0.0.0-test" },
    "/api/build": buildBoard(status),
    "/api/queue": { entries: [] },
    "/api/drift": { compared: 0 },
  };
}

const BUILD_READS = ["/api/build", "/api/drift", "/api/queue", "/api/status"];

function buildColumnCount(label: string): string {
  const head = Array.from(document.querySelectorAll(".col-head")).find((el) => within(el as HTMLElement).queryByText(label));
  if (head === undefined) throw new Error(`no column head for ${label}`);
  return within(head as HTMLElement).getByText(/^\d+$/).textContent ?? "";
}

// Build at an explicit one-epic selection, stored and on screen.
async function openBuild(stub: Stub) {
  renderConsole("/build?epics=form");
  expect(await screen.findByText("Schema engine")).toBeInTheDocument();
  await waitFor(() => expect(window.localStorage.getItem("tiller-build-epics:acme")).not.toBeNull());
  await waitFor(() => expect(reads(stub, "/api/drift")).toBe(1));
}

describe("arriving at Build while it is showing", () => {
  it("rereads the board once on the topbar link, restores the selection, and shows the changed status", async () => {
    const stub = stubReread(buildRoutes("active"));
    await openBuild(stub);
    expect(buildColumnCount("Active")).toBe("1");
    const before = Object.fromEntries(BUILD_READS.map((url) => [url, reads(stub, url)]));

    const watch = watchLoading();
    stub.hold();
    fireEvent.click(topbarLink("Build"));
    await waitFor(() => expect(where()).toBe("/build?epics=form"));
    await settle();

    // One new read of each of the page's own resources, and no more: the
    // arrival and the restore's URL rewrite are one arrival.
    for (const url of BUILD_READS) expect(reads(stub, url) - before[url], url).toBe(1);
    // The board on screen stays until the answer lands.
    expect(screen.getByText("Schema engine")).toBeInTheDocument();
    expect(buildColumnCount("Active")).toBe("1");

    await stub.release({ "/api/build": buildBoard("in_review") });
    await waitFor(() => expect(buildColumnCount("In review")).toBe("1"));
    expect(buildColumnCount("Active")).toBe("0");
    expect(screen.queryByText("Eligibility rules")).not.toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("rereads on the topbar link when the address does not change at all", async () => {
    const stub = stubReread(buildRoutes("active"));
    renderConsole("/build");
    expect(await screen.findByText("Schema engine")).toBeInTheDocument();
    await waitFor(() => expect(reads(stub, "/api/drift")).toBe(1));
    const before = reads(stub, "/api/build");

    fireEvent.click(topbarLink("Build"));
    await settle();

    expect(where()).toBe("/build");
    expect(reads(stub, "/api/build") - before).toBe(1);
  });

  it("reads each resource once on a first arrival that restores a stored selection", async () => {
    window.localStorage.setItem("tiller-build-epics:acme", "form");
    const stub = stubReread(buildRoutes("active"));
    renderConsole("/build");
    expect(await screen.findByText("Schema engine")).toBeInTheDocument();
    await waitFor(() => expect(where()).toBe("/build?epics=form"));
    await waitFor(() => expect(reads(stub, "/api/drift")).toBe(1));
    await settle();

    for (const url of ["/api/build", "/api/queue", "/api/drift"]) expect(reads(stub, url), url).toBe(1);
  });

  it("reads nothing when an epic is toggled", async () => {
    const stub = stubReread(buildRoutes("active"));
    await openBuild(stub);
    const before = stub.calls.length;

    fireEvent.click(screen.getByRole("button", { name: /intake/i }));
    await waitFor(() => expect(where()).toBe("/build?epics=form%2Cintake"));
    expect(await screen.findByText("Eligibility rules")).toBeInTheDocument();
    await settle();

    expect(stub.calls.length).toBe(before);
  });

  it("rereads once on browser back and once on forward", async () => {
    const stub = stubReread(buildRoutes("active"));
    await openBuild(stub);
    fireEvent.click(topbarLink("Build"));
    await waitFor(() => expect(where()).toBe("/build?epics=form"));
    await settle();
    const before = reads(stub, "/api/build");

    fireEvent.click(screen.getByRole("button", { name: "history back" }));
    await settle();
    expect(where()).toBe("/build?epics=form");
    expect(reads(stub, "/api/build") - before).toBe(1);

    fireEvent.click(screen.getByRole("button", { name: "history forward" }));
    await settle();
    expect(where()).toBe("/build?epics=form");
    expect(reads(stub, "/api/build") - before).toBe(2);
  });

  it("keeps the board and says it is out of date when the arrival's reread fails", async () => {
    const stub = stubReread(buildRoutes("active"));
    await openBuild(stub);

    stub.hold();
    fireEvent.click(topbarLink("Build"));
    await waitFor(() => expect(stub.heldUrls()).toContain("/api/build"));
    await stub.fail();

    expect(screen.getByText("Schema engine")).toBeInTheDocument();
    expect(screen.getByText(/board out of date/)).toBeInTheDocument();
    expect(where()).toBe("/build?epics=form");
  });

  it("still rereads once per manual refresh and once per return to the window after an arrival", async () => {
    const stub = stubReread(buildRoutes("active"));
    await openBuild(stub);
    fireEvent.click(topbarLink("Build"));
    await waitFor(() => expect(where()).toBe("/build?epics=form"));
    await settle();
    const before = reads(stub, "/api/build");

    fireEvent.click(screen.getByRole("button", { name: "Refresh now" }));
    await settle();
    expect(reads(stub, "/api/build") - before).toBe(1);

    // The focus reread waits for the page to be 30 seconds old.
    await waitFor(() => expect(screen.getByRole("button", { name: "Refresh now" })).toBeEnabled());
    const later = Date.now() + 31_000;
    vi.spyOn(Date, "now").mockReturnValue(later);
    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
      window.dispatchEvent(new Event("focus"));
    });
    await settle();
    expect(reads(stub, "/api/build") - before).toBe(2);
  });
});

function fixBoard(status: string): WorkItemsBoard {
  return {
    open: [
      { noun: "chore", id: "CH-1", key: "chores/mike", title: "Mike", status, ref: "chores/mike (CH-1)" },
      { noun: "bug", id: "B-1", key: "bugs/zulu", title: "Zulu", status: "ready", ref: "bugs/zulu (B-1)" },
    ],
    closed: [],
    attachments: [],
  };
}

function fixRoutes(status: string): Routes {
  return {
    "/api/status": summary(),
    "/api/version": { version: "0.0.0-test" },
    "/api/work-items": fixBoard(status),
  };
}

const fixColumn = (label: string) => screen.getByRole("region", { name: label });

// Fix at a chores-only filter, stored and on screen.
async function openFix(stub: Stub) {
  renderConsole("/fix?noun=chore");
  expect(await screen.findByText("Mike")).toBeInTheDocument();
  await waitFor(() => expect(window.localStorage.getItem("tiller-fix-filters:acme")).toBe("noun=chore"));
  await waitFor(() => expect(reads(stub, "/api/work-items")).toBe(1));
}

describe("arriving at Fix while it is showing", () => {
  it("rereads the board once on the topbar link, restores the filters, and shows the changed status", async () => {
    const stub = stubReread(fixRoutes("ready"));
    await openFix(stub);
    expect(within(fixColumn("Ready")).getByText("Mike")).toBeInTheDocument();
    const before = reads(stub, "/api/work-items");

    const watch = watchLoading();
    stub.hold();
    fireEvent.click(topbarLink("Fix"));
    await waitFor(() => expect(where()).toBe("/fix?noun=chore"));
    await settle();

    expect(reads(stub, "/api/work-items") - before).toBe(1);
    expect(within(fixColumn("Ready")).getByText("Mike")).toBeInTheDocument();

    await stub.release({ "/api/work-items": fixBoard("in_review") });
    expect(await within(fixColumn("In review")).findByText("Mike")).toBeInTheDocument();
    expect(screen.queryByText("Zulu")).not.toBeInTheDocument();
    expectNoLoading(watch);
  });

  it("rereads on the topbar link when the address does not change at all", async () => {
    const stub = stubReread(fixRoutes("ready"));
    renderConsole("/fix");
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    await settle();
    const before = reads(stub, "/api/work-items");

    fireEvent.click(topbarLink("Fix"));
    await settle();

    expect(where()).toBe("/fix");
    expect(reads(stub, "/api/work-items") - before).toBe(1);
  });

  it("reads the board once on a first arrival that restores stored filters", async () => {
    window.localStorage.setItem("tiller-fix-filters:acme", "noun=chore");
    const stub = stubReread(fixRoutes("ready"));
    renderConsole("/fix");
    expect(await screen.findByText("Mike")).toBeInTheDocument();
    await waitFor(() => expect(where()).toBe("/fix?noun=chore"));
    await settle();

    expect(reads(stub, "/api/work-items")).toBe(1);
  });

  it("reads nothing when a filter, the sort, or Clear filters changes the URL", async () => {
    const stub = stubReread(fixRoutes("ready"));
    await openFix(stub);
    const before = stub.calls.length;

    const nouns = screen.getByRole("navigation", { name: "Work item type" });
    fireEvent.click(within(nouns).getByRole("link", { name: "Bugs" }));
    await waitFor(() => expect(where()).toBe("/fix?noun=bug%2Cchore"));
    // The filter that is already the address: a link to where the page is.
    const severity = screen.getByRole("navigation", { name: "Severity" });
    fireEvent.click(within(severity).getByRole("link", { name: "All" }));
    await settle();
    fireEvent.click(screen.getByRole("link", { name: "Clear filters" }));
    await waitFor(() => expect(where()).toBe("/fix"));
    expect(await screen.findByText("Zulu")).toBeInTheDocument();
    await settle();

    expect(stub.calls.length).toBe(before);
  });

  it("rereads once on browser back and once on forward", async () => {
    const stub = stubReread(fixRoutes("ready"));
    await openFix(stub);
    const nouns = screen.getByRole("navigation", { name: "Work item type" });
    fireEvent.click(within(nouns).getByRole("link", { name: "Bugs" }));
    await waitFor(() => expect(where()).toBe("/fix?noun=bug%2Cchore"));
    await settle();
    const before = reads(stub, "/api/work-items");

    fireEvent.click(screen.getByRole("button", { name: "history back" }));
    await waitFor(() => expect(where()).toBe("/fix?noun=chore"));
    await settle();
    expect(reads(stub, "/api/work-items") - before).toBe(1);

    fireEvent.click(screen.getByRole("button", { name: "history forward" }));
    await waitFor(() => expect(where()).toBe("/fix?noun=bug%2Cchore"));
    await settle();
    expect(reads(stub, "/api/work-items") - before).toBe(2);
  });

  it("keeps the board and says it is out of date when the arrival's reread fails", async () => {
    const stub = stubReread(fixRoutes("ready"));
    await openFix(stub);

    stub.hold();
    fireEvent.click(topbarLink("Fix"));
    await waitFor(() => expect(stub.heldUrls()).toContain("/api/work-items"));
    await stub.fail();

    expect(screen.getByText("Mike")).toBeInTheDocument();
    expect(screen.getByText(/board out of date/)).toBeInTheDocument();
    expect(where()).toBe("/fix?noun=chore");
  });
});
