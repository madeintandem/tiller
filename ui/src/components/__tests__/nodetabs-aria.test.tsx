// NodeTabs as real ARIA tabs (Task 37): the rail is a tablist, each entry a
// tab with aria-selected, one tabpanel labelled by the selected tab, roving
// tabindex, and arrow/Home/End keys that move selection and focus together.

import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import NodeTabs from "../NodeTabs";

afterEach(() => {
  cleanup();
});

const TABS = [
  { key: "spec", label: "Spec", render: () => <p>Spec panel</p> },
  { key: "tasks", label: "Tasks", count: 4, render: () => <p>Tasks panel</p> },
  { key: "history", label: "History", render: () => <p>History panel</p> },
];

// A stateful host stands in for the URL: selecting writes the key back.
function Host({ initial = "spec", onSelect }: { initial?: string; onSelect?: (key: string) => void }) {
  const [active, setActive] = useState(initial);
  return (
    <NodeTabs
      tabs={TABS}
      active={active}
      onSelect={(key) => {
        onSelect?.(key);
        setActive(key);
      }}
    />
  );
}

describe("NodeTabs roles", () => {
  it("is a tablist with its label, not a nav, and keeps the mode-nav class", () => {
    render(<Host />);
    const list = screen.getByRole("tablist", { name: "Node detail tab" });
    expect(list.tagName).not.toBe("NAV");
    expect(list).toHaveClass("mode-nav");
    expect(screen.queryByRole("navigation")).toBeNull();
  });

  it("marks tabs with aria-selected and never aria-current", () => {
    render(<Host initial="tasks" />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(3);
    expect(screen.getByRole("tab", { name: "Tasks · 4" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Spec" })).toHaveAttribute("aria-selected", "false");
    for (const tab of tabs) expect(tab).not.toHaveAttribute("aria-current");
  });

  it("puts only the selected tab in the Tab order", () => {
    render(<Host initial="tasks" />);
    expect(screen.getByRole("tab", { name: "Tasks · 4" })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("tab", { name: "Spec" })).toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("tab", { name: "History" })).toHaveAttribute("tabindex", "-1");
  });

  it("renders one focusable tabpanel, labelled by the selected tab, that every tab controls", () => {
    render(<Host initial="tasks" />);
    const panel = screen.getByRole("tabpanel");
    expect(panel).toHaveTextContent("Tasks panel");
    expect(panel).toHaveAttribute("tabindex", "0");
    expect(panel.id).not.toBe("");
    const selected = screen.getByRole("tab", { name: "Tasks · 4" });
    expect(panel).toHaveAttribute("aria-labelledby", selected.id);
    for (const tab of screen.getAllByRole("tab")) {
      expect(tab).toHaveAttribute("aria-controls", panel.id);
    }
    expect(screen.getByRole("tabpanel", { name: "Tasks · 4" })).toBe(panel);
  });

  it("an unknown active key reads as the first tab, selected and labelling the panel", () => {
    render(<Host initial="nope" />);
    expect(screen.getByRole("tab", { name: "Spec" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Spec panel");
  });

  it("a pinned rail is still a tablist with a tabpanel", () => {
    render(<NodeTabs tabs={TABS} active="spec" onSelect={() => {}} pinned={<p>compact</p>} />);
    expect(screen.getByRole("tablist")).toBeInTheDocument();
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Spec panel");
  });
});

describe("NodeTabs warning tone is never color alone", () => {
  const WARN = [
    { key: "overview", label: "Overview", render: () => <p>Overview</p> },
    { key: "drift", label: "Drift", count: 3, countTone: "warning" as const, render: () => <p>Drift panel</p> },
    { key: "recovered", label: "Recovered reviews", count: 2, render: () => <p>Recovered</p> },
  ];

  it("a warning-toned tab carries a shape mark and the words in its accessible name", () => {
    render(<NodeTabs tabs={WARN} active="overview" onSelect={() => {}} />);
    const tab = screen.getByRole("tab", { name: "Drift · 3, needs attention" });
    const mark = tab.querySelector(".tab-count-mark");
    expect(mark).not.toBeNull();
    expect(mark?.tagName.toLowerCase()).toBe("svg");
    expect(mark).toHaveAttribute("aria-hidden", "true");
    expect(tab.querySelector(".visually-hidden")).toHaveTextContent(", needs attention");
  });

  it("a tab without the tone has neither the mark nor the words", () => {
    render(<NodeTabs tabs={WARN} active="overview" onSelect={() => {}} />);
    const tab = screen.getByRole("tab", { name: "Recovered reviews · 2" });
    expect(tab.querySelector(".tab-count-mark")).toBeNull();
    expect(tab.querySelector(".visually-hidden")).toBeNull();
    expect(screen.getByRole("tab", { name: "Overview" }).querySelector(".tab-count-mark")).toBeNull();
  });
});

describe("NodeTabs keys", () => {
  function key(name: string, k: string) {
    fireEvent.keyDown(screen.getByRole("tab", { name }), { key: k });
  }

  it("ArrowRight selects and focuses the next tab", () => {
    const onSelect = vi.fn();
    render(<Host onSelect={onSelect} />);
    screen.getByRole("tab", { name: "Spec" }).focus();
    key("Spec", "ArrowRight");
    expect(onSelect).toHaveBeenLastCalledWith("tasks");
    const tasks = screen.getByRole("tab", { name: "Tasks · 4" });
    expect(tasks).toHaveAttribute("aria-selected", "true");
    expect(document.activeElement).toBe(tasks);
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Tasks panel");
  });

  it("ArrowRight wraps from the last tab to the first", () => {
    render(<Host initial="history" />);
    screen.getByRole("tab", { name: "History" }).focus();
    key("History", "ArrowRight");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Spec" }));
    expect(screen.getByRole("tab", { name: "Spec" })).toHaveAttribute("aria-selected", "true");
  });

  it("ArrowLeft moves back and wraps from the first tab to the last", () => {
    render(<Host />);
    screen.getByRole("tab", { name: "Spec" }).focus();
    key("Spec", "ArrowLeft");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "History" }));
    key("History", "ArrowLeft");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Tasks · 4" }));
  });

  it("Home and End jump to the ends", () => {
    render(<Host initial="tasks" />);
    screen.getByRole("tab", { name: "Tasks · 4" }).focus();
    key("Tasks · 4", "End");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "History" }));
    expect(screen.getByRole("tab", { name: "History" })).toHaveAttribute("aria-selected", "true");
    key("History", "Home");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Spec" }));
    expect(screen.getByRole("tab", { name: "Spec" })).toHaveAttribute("aria-selected", "true");
  });

  it("ignores other keys and modified arrows", () => {
    const onSelect = vi.fn();
    render(<Host onSelect={onSelect} />);
    key("Spec", "a");
    key("Spec", "ArrowDown");
    fireEvent.keyDown(screen.getByRole("tab", { name: "Spec" }), { key: "ArrowRight", altKey: true });
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("a click still selects", () => {
    const onSelect = vi.fn();
    render(<Host onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("tab", { name: "History" }));
    expect(onSelect).toHaveBeenCalledWith("history");
  });
});
