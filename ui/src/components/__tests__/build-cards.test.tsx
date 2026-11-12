// Vitest/RTL coverage for the Build card's progress and readable task rows:
// the task-mix StateBar and its caption, the quiet empty Contributors chip,
// the glyph-led expanded task row, and the epic eyebrow shown once per run
// of same-epic cards. Split from build.test.tsx, which sits near the file
// size ceiling; the fixtures below are a small local copy of its own.

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { BuildBoard, BuildEpic, BuildFeature, RelatedNode } from "../../api";
import Build from "../../views/Build";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

function stubFetch(routes: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: unknown) => {
      const url = String(input);
      if (url in routes) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(routes[url]) });
      }
      return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
    }),
  );
}

function task(id: string, status: string, title: string): RelatedNode {
  return { noun: "task", id, key: `intake/rules/${id}`, ref: `${id} (${id})`, status, title };
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

function epic(id: string, key: string, title: string, features: BuildFeature[], contributors = [] as BuildEpic["contributors"]): BuildEpic {
  return {
    noun: "epic",
    id,
    key,
    title,
    ref: `${key} (${id})`,
    status: "active",
    contributors,
    feature_counts: { draft: features.length },
    features,
  };
}

function renderBoard(epics: BuildEpic[], entry = "/build") {
  const payload: BuildBoard = { epics };
  stubFetch({
    "/api/build": payload,
    "/api/status": {
      project: { name: "Acme", key: "acme", kind: "greenfield" },
      contributors: [],
      counts: { epics: epics.length, features: 0, tasks: 0, questions: 0 },
      whose_move: { human: 0, agent: 0 },
      active_epics: epics.map((e) => e.key),
    },
    "/api/queue": { entries: [] },
    "/api/drift": { compared: 0 },
  });
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <Build />
    </MemoryRouter>,
  );
}

function sevenInReview(): BuildFeature {
  return feature({
    id: "F-1",
    status: "in_review",
    title: "Schema engine",
    task_done: 0,
    task_total: 7,
    tasks: Array.from({ length: 7 }, (_, i) => task(`T-${String(i)}`, "in_review", `Task ${String(i)}`)),
  });
}

describe("Build card task progress", () => {
  it("shows a state bar whose caption states the mix in words, and no bare count", async () => {
    renderBoard([epic("E-1", "form", "Form Engine", [sevenInReview()])]);
    const card = (await screen.findByText("Schema engine")).closest(".fcard") as HTMLElement;
    expect(within(card).getByRole("img", { name: "0 of 7 done, 7 in review" })).toBeInTheDocument();
    expect(card).not.toHaveTextContent("0 / 7");
  });

  it("counts live tasks only: a cancelled task is not part of the denominator", async () => {
    renderBoard([
      epic("E-1", "form", "Form Engine", [
        feature({
          id: "F-1",
          status: "active",
          title: "Schema engine",
          tasks: [task("T-1", "done", "One"), task("T-2", "cancelled", "Two"), task("T-3", "in_progress", "Three")],
        }),
      ]),
    ]);
    const card = (await screen.findByText("Schema engine")).closest(".fcard") as HTMLElement;
    expect(within(card).getByRole("img", { name: "1 of 2 done, 1 in progress" })).toBeInTheDocument();
  });

  it("keeps the plain words for a feature with no tasks, with no bar", async () => {
    renderBoard([epic("E-1", "form", "Form Engine", [feature({ id: "F-1", status: "draft", title: "Schema engine" })])]);
    const card = (await screen.findByText("Schema engine")).closest(".fcard") as HTMLElement;
    expect(within(card).getByText("no tasks yet")).toBeInTheDocument();
    expect(card.querySelector(".state-bar")).toBeNull();
  });
});

describe("Build quiet empty contributors", () => {
  it("renders no contributors chip on a card with no contributors, and one when there are some", async () => {
    renderBoard([
      epic("E-1", "form", "Form Engine", [
        feature({ id: "F-1", status: "draft", title: "Bare feature" }),
        feature({ id: "F-2", status: "draft", title: "Staffed feature", contributors: [{ id: "pat", name: "Pat" }] }),
      ]),
    ]);
    const bare = (await screen.findByText("Bare feature")).closest(".fcard") as HTMLElement;
    const staffed = screen.getByText("Staffed feature").closest(".fcard") as HTMLElement;
    expect(bare.querySelector(".contributors-chip")).toBeNull();
    expect(within(staffed).getByText("Pat")).toBeInTheDocument();
    expect(staffed.querySelector(".contributors-chip")).not.toBeNull();
  });

  it("renders no Contributors line on an epic row with none", async () => {
    renderBoard([
      epic("E-1", "form", "Form Engine", [feature({ id: "F-1", status: "draft", title: "A" })]),
      epic("E-2", "pay", "Payments", [feature({ id: "F-2", status: "draft", title: "B" })], [{ id: "sam", name: "Sam" }]),
    ]);
    await screen.findByText("B");
    const rows = Array.from(document.querySelectorAll<HTMLElement>(".epic-row"));
    const bare = rows.find((r) => r.textContent.includes("Form Engine"))!;
    const staffed = rows.find((r) => r.textContent.includes("Payments"))!;
    expect(bare).not.toHaveTextContent("Contributors:");
    expect(staffed).toHaveTextContent("Contributors:");
  });
});

describe("Build expanded task rows lead with a glyph", () => {
  it("puts a named status glyph before the title and no trailing chip", async () => {
    renderBoard([epic("E-1", "form", "Form Engine", [sevenInReview()])]);
    const card = (await screen.findByText("Schema engine")).closest(".fcard") as HTMLElement;
    fireEvent.click(within(card).getByRole("button", { name: /expand tasks/i }));
    const row = within(card).getByText("Task 0").closest(".trow") as HTMLElement;
    const glyph = within(row).getByRole("img", { name: "In review" });
    expect(glyph).toHaveClass("state-glyph");
    expect(row.querySelector(".chip")).toBeNull();
    expect(glyph.compareDocumentPosition(within(row).getByText("Task 0")) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

describe("Build epic eyebrow", () => {
  const three = (prefix: string) =>
    ["a", "b", "c"].map((s) => feature({ id: `F-${prefix}${s}`, status: "draft", title: `${prefix} feature ${s}` }));

  it("shows the epic once above a run of same-epic cards when several epics are selected", async () => {
    renderBoard(
      [epic("E-1", "form", "Form Engine", three("form")), epic("E-2", "pay", "Payments", three("pay"))],
      "/build?epics=form,pay",
    );
    await screen.findByText("form feature a");
    const eyebrows = Array.from(document.querySelectorAll(".fcard .f-epic")).map((e) => e.textContent);
    expect(eyebrows).toEqual(["Form Engine", "Payments"]);
  });

  it("shows no eyebrow when one epic is selected", async () => {
    renderBoard([epic("E-1", "form", "Form Engine", three("form"))], "/build?epics=form");
    await screen.findByText("form feature a");
    expect(document.querySelectorAll(".fcard .f-epic")).toHaveLength(0);
  });
});

describe("Build amendment hold flag", () => {
  it("labels a held draft in words and leaves an unheld draft alone", async () => {
    renderBoard([
      epic("E-1", "form", "Form Engine", [
        feature({
          id: "F-held",
          status: "draft",
          title: "Held feature",
          amendment_hold: { id: "A-9", key: "iphone-only", ref: "iphone-only (A-9)", title: "iPhone only", status: "draft" },
        }),
        feature({ id: "F-plain", status: "draft", title: "Plain feature" }),
      ]),
    ]);
    const held = (await screen.findByText("Held feature")).closest(".fcard") as HTMLElement;
    const flag = within(held).getByText("In amendment");
    expect(flag).toHaveAttribute("title", "Held by amendment iPhone only");
    expect(flag).not.toHaveClass("blocked");
    const plain = screen.getByText("Plain feature").closest(".fcard") as HTMLElement;
    expect(within(plain).queryByText("In amendment")).toBeNull();
  });
});
