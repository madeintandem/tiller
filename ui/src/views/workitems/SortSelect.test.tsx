import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import SortSelect, { type SortOption } from "./SortSelect";

type Sort = "shelf" | "severity" | "added";
const OPTIONS: SortOption<Sort>[] = [
  { value: "shelf", label: "Shelf order" },
  { value: "severity", label: "Severity" },
  { value: "added", label: "Date added" },
];

function Harness({ onChange }: { onChange: (value: Sort) => void }) {
  const [value, setValue] = useState<Sort>("shelf");
  return (
    <SortSelect label="Sort" options={OPTIONS} value={value}
      onChange={(next) => { setValue(next); onChange(next); }} />
  );
}

function setup() {
  const onChange = vi.fn();
  render(<Harness onChange={onChange} />);
  const trigger = screen.getByRole("combobox", { name: "Sort" });
  trigger.focus();
  return { onChange, trigger };
}

const activeLabel = (trigger: HTMLElement) =>
  document.getElementById(trigger.getAttribute("aria-activedescendant") ?? "")?.textContent;

describe("SortSelect", () => {
  afterEach(cleanup);

  it("opens on ArrowDown at the current value, moves, and selects with Enter", () => {
    const { onChange, trigger } = setup();
    expect(screen.queryByRole("listbox")).toBeNull();
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(activeLabel(trigger)).toBe("Shelf order");
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    expect(activeLabel(trigger)).toBe("Date added");
    fireEvent.keyDown(trigger, { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith("added");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(trigger).toHaveTextContent("Date added");
  });

  it("closes on Escape without changing the value", () => {
    const { onChange, trigger } = setup();
    fireEvent.keyDown(trigger, { key: " " });
    fireEvent.keyDown(trigger, { key: "End" });
    fireEvent.keyDown(trigger, { key: "Escape" });
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(onChange).not.toHaveBeenCalled();
    expect(trigger).toHaveTextContent("Shelf order");
  });

  it("does not report choosing the value already selected", () => {
    const { onChange, trigger } = setup();
    fireEvent.click(trigger);
    fireEvent.mouseDown(screen.getByRole("option", { name: "Shelf order" }));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("closes when focus leaves", () => {
    const { trigger } = setup();
    fireEvent.click(trigger);
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    fireEvent.blur(trigger);
    expect(screen.queryByRole("listbox")).toBeNull();
  });
});
