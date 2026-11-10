// Shared harness for the reread tests (reread-*.test.tsx): a reread is a
// background revalidation, so every surface keeps what it shows while the
// new answer is out, then updates in place. Each test mounts the whole App
// at a real address, bumps the fetch generation with every following read
// held open, checks the page, then releases the reads with changed data.

import { act, render } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { expect, vi } from "vitest";

import type { ListResult, ProjectSummary } from "../../api";
import { bumpFetchGeneration } from "../../api";
import App from "../../App";

export type Routes = Record<string, unknown>;

export function summary(key = "acme"): ProjectSummary {
  return {
    project: { name: "Acme Platform", key, kind: "greenfield" },
    contributors: [{ id: "jc", name: "JC Grubbs" }],
    counts: { epics: 0, features: 0, tasks: 0, questions: 0 },
    whose_move: { human: 0, agent: 0 },
  };
}

export function emptyList(noun: string): ListResult {
  return { noun, count: 0, nodes: [] };
}

// Every read Home makes, all empty, plus the shell's own.
export function homeRoutes(): Routes {
  return {
    "/api/status": summary(),
    "/api/version": { version: "0.0.0-test" },
    "/api/queue": { entries: [] },
    "/api/ls?noun=epic": emptyList("epic"),
    "/api/ls?noun=feature": emptyList("feature"),
    "/api/ls?noun=task": emptyList("task"),
    "/api/ls?noun=question": emptyList("question"),
    "/api/ls?noun=bug": emptyList("bug"),
    "/api/ls?noun=chore": emptyList("chore"),
    "/api/ls?noun=tweak": emptyList("tweak"),
    "/api/recap/peek": { since: "2026-07-24T00:00:00Z", reader: "jc", total: 0, asks: 0 },
    "/api/recap?window=24h": { since: "", window: "24h", reader: "jc", total: 0, groups: [] },
    "/api/board": { contributors: [] },
    "/api/next": { reader: "jc", caught_up: true, candidates: [] },
    "/api/drift": { compared: 0, uncovered: [] },
    "/api/execution": { observations: [], diagnostics: [] },
  };
}

function answer(table: Routes, url: string): Promise<unknown> {
  if (url in table) {
    return Promise.resolve({ ok: true, json: () => Promise.resolve(structuredClone(table[url])) });
  }
  return Promise.resolve({ ok: false, status: 404, statusText: "Not Found", text: () => Promise.resolve("") });
}

export interface Call {
  url: string;
  marked: boolean;
}

// A fetch stub keyed by exact URL. reread() bumps the fetch generation and
// holds every read that follows; release() answers them from the table with
// the changes applied, and later reads answer at once again. fail() answers
// the held reads with a server error instead; hold() holds without a bump.
export function stubReread(table: Routes) {
  let current = table;
  let held: { url: string; settle: (value: unknown) => void }[] | null = null;
  const calls: Call[] = [];
  vi.stubGlobal("fetch", vi.fn((input: unknown, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, marked: new Headers(init?.headers).get("X-Tiller-Visit") === "1" });
    if (held !== null) {
      const queue = held;
      return new Promise((resolve) => queue.push({ url, settle: resolve }));
    }
    return answer(current, url);
  }));
  return {
    calls,
    reread() {
      held = [];
      act(() => bumpFetchGeneration());
    },
    // Holds every read from here on without a bump, for a navigation.
    hold() {
      held = [];
    },
    heldUrls(): string[] {
      return (held ?? []).map((h) => h.url);
    },
    async release(changes: Routes = {}) {
      current = { ...current, ...changes };
      const pending = held ?? [];
      held = null;
      await act(async () => {
        for (const h of pending) h.settle(await answer(current, h.url));
      });
    },
    async fail() {
      const pending = held ?? [];
      held = null;
      await act(async () => {
        for (const h of pending) {
          h.settle({ ok: false, status: 500, statusText: "Internal Server Error", text: () => Promise.resolve("disk on fire") });
        }
      });
    },
  };
}

// Loading words a reread must never put on screen over content that was
// already there.
export const LOADING_TEXT = /Loading|Reading branch history|Searching\.\.\.|Sweeping the working tree/;

// Records every loading text that appears anywhere in the document from the
// moment it is called, including one that comes and goes between two
// assertions.
export function watchLoading() {
  const seen: string[] = [];
  const check = () => {
    const match = document.body.textContent?.match(LOADING_TEXT);
    if (match) seen.push(match[0]);
  };
  const observer = new MutationObserver(check);
  observer.observe(document.body, { childList: true, subtree: true, characterData: true });
  return {
    seen,
    stop() {
      observer.takeRecords();
      check();
      observer.disconnect();
    },
  };
}

function Where() {
  const location = useLocation();
  return <p data-testid="where">{`${location.pathname}${location.search}${location.hash}`}</p>;
}

export function renderApp(entry: string) {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <App />
      <Where />
    </MemoryRouter>,
  );
}

// The whole check most tests make: the page held what it showed and put no
// loading text up while the reread was out.
export function expectNoLoading(watch: ReturnType<typeof watchLoading>) {
  watch.stop();
  expect(watch.seen).toEqual([]);
}
