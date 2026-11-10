// Every visible status word goes through statusLabel: the browser walk of
// batches A to C found wire words (in_progress, in_review, specced, done) still
// on screen in the sites below. Each test asserts the plain words and the
// absence of the wire token.

import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ExecutionRecord, ListResult, NodePage, Streams } from "../../api";
import type { FetchState } from "../../hooks";
import { BUILD_COLUMNS } from "../../views/build/columns";
import FilterMode from "../../views/explore/FilterMode";
import HeaderBand from "../../views/home/HeaderBand";
import { InFlightColumn } from "../../views/home/Pulse";
import NodeDetail from "../NodeDetail";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function settled<T>(data: T): FetchState<T> {
  return { data, error: null, loading: false };
}

const WIRE = /\b(in_progress|in_review|specced)\b/;

function featurePage(): NodePage {
  return {
    node: { noun: "feature", id: "F-1", key: "billing/refunds", title: "Refund pipeline", status: "ready", column: "active" },
    ref: "billing/refunds (F-1)",
    prose: null,
    relationships: {
      tasks: [
        { noun: "task", id: "T-1", ref: "T-1", status: "ready", title: "Wire the job" },
        { noun: "task", id: "T-2", ref: "T-2", status: "in_progress", title: "Post the ledger" },
        { noun: "task", id: "T-3", ref: "T-3", status: "in_review", title: "Refuse duplicates" },
      ],
    },
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
  };
}

describe("status words on the node page", () => {
  it("names the served checkout's status in words in the execution note", () => {
    const observation: ExecutionRecord = {
      id: "T-one", key: "epic/feature/task", branch: "task-one", source: "local_branch",
      revision: "abcdef123456", observed_at: "2026-10-06T12:00:00Z", uncommitted: false,
      status: "in_review", owner: "developer", integration_status: "in_review",
    };
    const page: NodePage = {
      node: { noun: "task", id: "T-one", status: "in_review" },
      ref: "T-one", prose: null, relationships: {}, backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
      history: [], eval: null, execution: observation,
    };
    const { container } = render(<MemoryRouter><NodeDetail page={page} /></MemoryRouter>);
    const note = container.querySelector(".execution-boundary");
    expect(note?.textContent).toContain("On the checkout this console serves, this task is in review.");
    expect(note?.textContent).not.toMatch(WIRE);
  });

  it("states the feature header's task mix in words", () => {
    const { container } = render(<MemoryRouter><NodeDetail page={featurePage()} /></MemoryRouter>);
    const caption = container.querySelector(".node-mix-caption");
    expect(caption?.textContent).toBe("3 tasks · 1 ready · 1 in progress · 1 in review");
  });

  it("states the Tasks tab's caption in words", () => {
    const { container } = render(
      <MemoryRouter initialEntries={["/?tab=tasks"]}><NodeDetail page={featurePage()} /></MemoryRouter>,
    );
    const caption = container.querySelector(".instr-fh-caption");
    expect(caption?.textContent).toBe("1 ready · 1 in progress · 1 in review");
  });
});

describe("status words on Explore's filter", () => {
  it("labels the status axis's options through statusLabel and sends the stored value", async () => {
    const result: ListResult = {
      noun: "task",
      count: 0,
      nodes: [],
      facets: [
        {
          axis: "status",
          options: [
            { value: "in_progress", label: "in_progress", count: 1 },
            { value: "in_review", label: "in_review", count: 1 },
            { value: "specced", label: "specced", count: 2 },
          ],
        },
        { axis: "owner", options: [{ value: "jc", label: "JC Grubbs", count: 1 }] },
      ],
    };
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(result) })));
    render(
      <MemoryRouter initialEntries={["/explore?mode=filter&noun=tasks"]}>
        <FilterMode />
      </MemoryRouter>,
    );
    const status = await screen.findByRole("combobox", { name: "status" });
    fireEvent.focus(status);
    const list = screen.getByRole("listbox", { name: "status" });
    const text = list.textContent ?? "";
    expect(text).toContain("In progress");
    expect(text).toContain("In review");
    expect(text).toContain("Specified");
    expect(text).not.toMatch(WIRE);

    fireEvent.mouseDown(within(list).getByRole("option", { name: /In review/ }));
    await waitFor(() => {
      const calls = vi.mocked(fetch).mock.calls.map((c) => String(c[0]));
      expect(calls).toContain("/api/ls?noun=tasks&status=in_review");
    });
  });

  it("keeps every other axis's served label", async () => {
    const result: ListResult = {
      noun: "task",
      count: 0,
      nodes: [],
      facets: [{ axis: "owner", options: [{ value: "jc_g", label: "JC Grubbs", count: 1 }] }],
    };
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(result) })));
    render(
      <MemoryRouter initialEntries={["/explore?mode=filter&noun=tasks"]}>
        <FilterMode />
      </MemoryRouter>,
    );
    const owner = await screen.findByRole("combobox", { name: "owner" });
    fireEvent.focus(owner);
    expect(within(screen.getByRole("listbox", { name: "owner" })).getByRole("option", { name: /JC Grubbs/ })).toBeInTheDocument();
  });
});

describe("status words on Home", () => {
  it("reads In review beside In progress in In flight", () => {
    const streams: Streams = {
      contributors: [
        {
          contributor: { id: "jc", name: "JC" },
          active: [{ noun: "task", id: "T-2", ref: "T-2", title: "Post the ledger", status: "in_progress", display: "in_progress" }],
          gates_in_review: [{ noun: "task", id: "T-3", ref: "T-3", title: "Refuse duplicates" }],
          blocked: [],
          open_questions: [],
        },
      ],
    };
    const { container } = render(<MemoryRouter><InFlightColumn state={settled(streams)} /></MemoryRouter>);
    const text = container.textContent ?? "";
    expect(text).toContain("· In progress");
    expect(text).toContain("· In review");
    expect(text).not.toContain("in review");
  });

  it("labels the funnel's columns through statusLabel, Specified included", () => {
    const empty = (noun: string): FetchState<ListResult> => settled({ noun, count: 0, nodes: [] });
    const features: FetchState<ListResult> = settled({
      noun: "feature",
      count: 2,
      nodes: [
        { id: "F-1", status: "specced", ref: "F-1" },
        { id: "F-2", status: "in_review", ref: "F-2" },
      ],
    });
    const { container } = render(
      <MemoryRouter>
        <HeaderBand
          epics={empty("epic")}
          features={features}
          tasks={empty("task")}
          questions={empty("question")}
          bugs={empty("bug")}
          chores={empty("chore")}
          tweaks={empty("tweak")}
          drift={{ data: null, error: null, loading: true }}
        />
      </MemoryRouter>,
    );
    const labels = Array.from(container.querySelectorAll(".fseg .lbl")).map((n) => n.textContent);
    expect(labels).toEqual(["Draft", "Specified", "Ready", "Active", "In review", "Done"]);
  });
});

describe("status words on Build", () => {
  it("names each column through statusLabel", () => {
    expect(BUILD_COLUMNS.map((c) => c.label)).toEqual(["Draft", "Specified", "Ready", "Active", "In review", "Done"]);
  });
});
