import { cleanup, render, screen, waitFor, within, act } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";

import App from "../../App";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

it.each([
  ["Home", "/", "/api/status"],
  ["Explore", "/explore", "/api/tree"],
  ["Build", "/build", "/api/build"],
  ["Fix", "/fix", "/api/work-items"],
])("%s announces pending reads and leaves loading when they fail", async (title, path, apiPath) => {
  const request = Promise.withResolvers<void>();
  vi.stubGlobal("fetch", vi.fn(async () => {
    await request.promise;
    return new Response(JSON.stringify({ error: "read unavailable" }), {
      status: 503,
      statusText: "Service Unavailable",
    });
  }));
  render(<MemoryRouter initialEntries={[path]}><App /></MemoryRouter>);

  const main = within(screen.getByRole("main"));
  expect(main.getByRole("heading", { level: 1, name: title })).toBeVisible();
  const loading = main.getByRole("status");
  expect(loading).toBeVisible();
  expect(main.queryByRole("region", { name: "Ready" })).not.toBeInTheDocument();

  await act(async () => request.resolve());
  await waitFor(() => expect(loading).not.toBeInTheDocument());
  expect(main.getByText(`GET ${apiPath} failed (503 Service Unavailable): read unavailable`)).toBeVisible();
});
