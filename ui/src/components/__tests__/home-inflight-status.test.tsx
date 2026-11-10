// In flight shows each row's status in plain words, never the wire token.

import { cleanup, render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import type { Streams } from "../../api";
import { InFlightColumn } from "../../views/home/Pulse";

afterEach(() => {
  cleanup();
});

describe("In flight status words", () => {
  it("reads an in_progress row as In progress and never shows the token", () => {
    const streams: Streams = { contributors: [
      { contributor: { id: "sam", name: "Sam" }, blocked: [], open_questions: [],
        active: [{ noun: "bug", id: "B-1", key: "crash-loop", ref: "crash-loop (B-1)", title: "Crash loop", status: "in_progress", display: "in_progress" }],
        gates_in_review: [{ noun: "task", id: "T-2", ref: "T-2", status: "in_review" } as never] },
    ] };
    const { container } = render(
      <MemoryRouter><InFlightColumn state={{ data: streams, error: null, loading: false }} /></MemoryRouter>,
    );
    const text = container.textContent ?? "";
    expect(text).toContain("In progress");
    expect(text).not.toContain("in_progress");
    expect(text).not.toContain("in_review");
    expect(text).toContain("in review");
  });
});
