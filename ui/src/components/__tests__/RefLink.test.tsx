// RefLink has no dedicated coverage anywhere else in the tree (it is
// exercised only indirectly, through whatever page happens to render one);
// task 6 adds the amendment noun to /api/node, and the binding calls out
// "RefLink target" as its own required scenario, so this file pins RefLink
// itself rather than leaving the amendment case to be inferred from another
// component's test.

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import RefLink from "../RefLink";

afterEach(() => {
  cleanup();
});

describe("RefLink: the amendment noun", () => {
  it("addresses an amendment's own node page by id, the same generic rule every noun gets", () => {
    render(
      <MemoryRouter>
        <RefLink node={{ noun: "amendment", id: "A-000001", ref: "iphone-only (A-000001)" }} />
      </MemoryRouter>,
    );
    const link = screen.getByRole("link", { name: "iphone-only (A-000001)" });
    expect(link).toHaveAttribute("href", "/node/amendment/A-000001");
  });
});
