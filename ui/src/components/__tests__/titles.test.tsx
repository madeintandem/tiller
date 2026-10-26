// Browser tab titles (every tab used to say "Tiller"). Each view
// sets its own title through useDocumentTitle; the shape is "<page> · Tiller"
// with the distinguishing word first, since tabs truncate from the right.
// Rendered through App with the same stubFetch-keyed-by-URL convention
// routing.test.tsx uses, so the title comes from the real route and fetch.

import { cleanup, fireEvent, render, renderHook, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { NodePage, ProjectSummary } from "../../api";
import App from "../../App";
import { titleOf, useDocumentTitle } from "../../hooks";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  document.title = "Tiller";
});

function summary(): ProjectSummary {
  return {
    project: { name: "Acme Platform", key: "acme", kind: "greenfield" },
    contributors: [{ id: "jc", name: "JC Grubbs" }],
    counts: { epics: 0, features: 0, tasks: 0, questions: 0 },
    whose_move: { human: 0, agent: 0 },
  };
}

function featurePage(): NodePage {
  return {
    node: { noun: "feature", id: "F-9", title: "Wire the billing webhook", status: "ready" },
    ref: "F-9",
    prose: null,
    relationships: {},
    backlinks: { groups: [], count: 0, diagnostics: [], complete: true },
    history: [],
    eval: null,
    
  };
}

function stubFetch(routes: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: unknown) => {
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
    }),
  );
}

function renderAt(route: string) {
  return render(
    <MemoryRouter initialEntries={[route]}>
      <App />
    </MemoryRouter>,
  );
}

const exploreRoutes = {
  "/api/status": summary(),
  "/api/tree": { project: "acme", nodes: [] },
  "/api/ls?noun=question": { noun: "question", count: 0, nodes: [] },
};

describe("useDocumentTitle", () => {
  it("sets document.title, ignores null, and follows the latest value", () => {
    const { rerender } = renderHook(({ t }: { t: string | null }) => useDocumentTitle(t), {
      initialProps: { t: "Build · Tiller" as string | null },
    });
    expect(document.title).toBe("Build · Tiller");
    rerender({ t: null });
    expect(document.title).toBe("Build · Tiller");
    rerender({ t: "Drift · Tiller" });
    expect(document.title).toBe("Drift · Tiller");
  });

  it("titleOf puts the page first and the product last", () => {
    expect(titleOf("Build")).toBe("Build · Tiller");
  });
});

describe("Page titles", () => {
  it("Home carries the project name", async () => {
    stubFetch({ "/api/status": summary() });
    renderAt("/");
    await waitFor(() => expect(document.title).toBe("Acme Platform · Tiller"));
  });

  it("Home falls back to Tiller while the summary is unavailable", async () => {
    stubFetch({});
    renderAt("/");
    await screen.findAllByText(/failed/);
    expect(document.title).toBe("Tiller");
  });

  it("a static page carries its page name", async () => {
    stubFetch({ "/api/status": summary() });
    renderAt("/build");
    await waitFor(() => expect(document.title).toBe("Build · Tiller"));
  });

  it("the not-found page says so", async () => {
    stubFetch({ "/api/status": summary() });
    renderAt("/nowhere");
    await waitFor(() => expect(document.title).toBe("Not found · Tiller"));
  });

  it("a node page carries the header's handle", async () => {
    stubFetch({ ...exploreRoutes, "/api/node/feature/F-9": featurePage() });
    renderAt("/node/feature/F-9");
    await waitFor(() => expect(document.title).toBe("Wire the billing webhook · Tiller"));
  });

  it("a missing node says so", async () => {
    stubFetch(exploreRoutes);
    renderAt("/node/feature/F-gone");
    await waitFor(() => expect(document.title).toBe("No such node · Tiller"));
  });

  it("a file page carries the file's basename and line", async () => {
    stubFetch({
      ...exploreRoutes,
      "/api/prose?path=.tiller%2Fdesign%2Fas-is%2Fapp.md&line=12": {
        path: ".tiller/design/as-is/app.md",
        content: "# App\n\nline\n",
        line: 12,
        line_text: "line",
      },
    });
    renderAt("/file/.tiller/design/as-is/app.md#L12");
    await waitFor(() => expect(document.title).toBe("app.md:12 · Tiller"));
  });
});

// Filter, Search, and Map replace the reading pane, so the node or file title
// that pane set must not linger over a mode that no longer shows it.
describe("Explore mode titles", () => {
  const fileRoute = "/api/prose?path=.tiller%2Fdesign%2Fas-is%2Fapp.md&line=12";
  const routes = {
    ...exploreRoutes,
    "/api/node/feature/F-9": featurePage(),
    [fileRoute]: {
      path: ".tiller/design/as-is/app.md",
      content: "# App\n\nline\n",
      line: 12,
      line_text: "line",
    },
  };

  function Jump() {
    const navigate = useNavigate();
    return (
      <>
        {["filter", "search", "map"].map((mode) => (
          <button key={mode} onClick={() => navigate(`/explore?mode=${mode}`)}>
            jump {mode}
          </button>
        ))}
        <button onClick={() => navigate(-1)}>jump back</button>
      </>
    );
  }

  function renderWithJump(route: string) {
    return render(
      <MemoryRouter initialEntries={[route]}>
        <Jump />
        <App />
      </MemoryRouter>,
    );
  }

  for (const mode of ["filter", "search", "map"]) {
    it(`a direct ${mode} load reads Explore`, async () => {
      document.title = "Wire the billing webhook · Tiller";
      stubFetch(routes);
      renderAt(`/explore?mode=${mode}`);
      await waitFor(() => expect(document.title).toBe("Explore · Tiller"));
    });

    it(`node to ${mode} reads Explore, and back restores the node title`, async () => {
      stubFetch(routes);
      renderWithJump("/node/feature/F-9");
      await waitFor(() => expect(document.title).toBe("Wire the billing webhook · Tiller"));

      fireEvent.click(screen.getByRole("button", { name: `jump ${mode}` }));
      await waitFor(() => expect(document.title).toBe("Explore · Tiller"));

      fireEvent.click(screen.getByRole("button", { name: "jump back" }));
      await waitFor(() => expect(document.title).toBe("Wire the billing webhook · Tiller"));
    });

    it(`file to ${mode} reads Explore, and back restores the file title`, async () => {
      stubFetch(routes);
      renderWithJump("/file/.tiller/design/as-is/app.md#L12");
      await waitFor(() => expect(document.title).toBe("app.md:12 · Tiller"));

      fireEvent.click(screen.getByRole("button", { name: `jump ${mode}` }));
      await waitFor(() => expect(document.title).toBe("Explore · Tiller"));

      fireEvent.click(screen.getByRole("button", { name: "jump back" }));
      await waitFor(() => expect(document.title).toBe("app.md:12 · Tiller"));
    });
  }

  it("Browse with no selection reads Explore", async () => {
    document.title = "Wire the billing webhook · Tiller";
    stubFetch(routes);
    renderAt("/explore");
    await waitFor(() => expect(document.title).toBe("Explore · Tiller"));
  });
});

