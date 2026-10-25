import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { RelatedNode } from "../../api";
import AttachmentFilter from "./AttachmentFilter";

afterEach(cleanup);

const nodes: RelatedNode[] = [
  { noun: "epic", id: "E-1", key: "payments", ref: "payments (E-1)", status: "active", title: "Payments" },
  { noun: "feature", id: "F-1", key: "payments/checkout", ref: "payments/checkout (F-1)", status: "active", title: "Checkout" },
  { noun: "task", id: "T-1", ref: "T-1", status: "ready", title: "Retry webhook" },
];

function Harness({ initial = [], onToggle }: { initial?: string[]; onToggle?: (id: string) => void }) {
  const [selected, setSelected] = useState(initial);
  return (
    <AttachmentFilter options={nodes} selected={selected} onToggle={(id) => {
      onToggle?.(id);
      setSelected((current) => current.includes(id) ? current.filter((entry) => entry !== id) : [...current, id]);
    }} />
  );
}

function setup(initial?: string[]) {
  const onToggle = vi.fn();
  render(<Harness initial={initial} onToggle={onToggle} />);
  return { input: screen.getByRole("combobox", { name: "Attached to" }), onToggle };
}

function activeOption(input: HTMLElement) {
  const id = input.getAttribute("aria-activedescendant");
  return id === null ? null : document.getElementById(id);
}

describe("AttachmentFilter", () => {
  it("opens a multiselectable listbox whose results name their type and key", () => {
    const { input } = setup();
    expect(input).toHaveAttribute("aria-expanded", "false");
    fireEvent.focus(input);
    const list = screen.getByRole("listbox", { name: "Attached to" });
    expect(input).toHaveAttribute("aria-controls", list.id);
    expect(list).toHaveAttribute("aria-multiselectable", "true");
    const options = within(list).getAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual([
      "EpicPaymentspayments",
      "FeatureCheckoutpayments/checkout",
      "TaskRetry webhookT-1",
    ]);
    expect(within(options[1]).getByText("payments/checkout")).toHaveAttribute("title", "payments/checkout");
    expect(input).toHaveAttribute("placeholder", "Search nodes");
  });

  it("narrows case-insensitively by title, key and id", () => {
    const { input } = setup();
    fireEvent.focus(input);
    for (const [query, title] of [["CHECK", "Checkout"], ["payments/", "Checkout"], ["t-1", "Retry webhook"]]) {
      fireEvent.change(input, { target: { value: query } });
      expect(screen.getAllByRole("option")).toHaveLength(1);
      expect(screen.getByRole("option")).toHaveTextContent(title);
    }
    fireEvent.change(input, { target: { value: "zzz" } });
    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByRole("status")).toHaveTextContent("No matches");
  });

  it("moves with arrows, toggles on Enter while staying open, and closes on Escape", () => {
    const { input, onToggle } = setup();
    fireEvent.focus(input);
    expect(activeOption(input)).toHaveTextContent("Payments");
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(activeOption(input)).toHaveTextContent("Checkout");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onToggle).toHaveBeenLastCalledWith("F-1");
    expect(input).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("option", { name: /Checkout/ })).toHaveAttribute("aria-selected", "true");
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onToggle).toHaveBeenLastCalledWith("T-1");
    fireEvent.keyDown(input, { key: "ArrowUp" });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onToggle).toHaveBeenLastCalledWith("F-1");
    expect(screen.getByRole("option", { name: /Checkout/ })).toHaveAttribute("aria-selected", "false");
    fireEvent.keyDown(input, { key: "Escape" });
    expect(input).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(activeOption(input)).toHaveTextContent("Retry webhook");
  });

  it("shows picks as removable chips and Backspace in an empty input removes the last one", () => {
    const { input, onToggle } = setup(["none", "E-1", "T-1"]);
    const chips = screen.getByRole("list", { name: "Selected attachments" });
    expect(within(chips).getAllByRole("listitem").map((chip) => chip.textContent)).toEqual(["Payments×", "Retry webhook×"]);
    fireEvent.change(input, { target: { value: "x" } });
    fireEvent.keyDown(input, { key: "Backspace" });
    expect(onToggle).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value: "" } });
    fireEvent.keyDown(input, { key: "Backspace" });
    expect(onToggle).toHaveBeenLastCalledWith("T-1");
    fireEvent.click(screen.getByRole("button", { name: "Remove Payments" }));
    expect(onToggle).toHaveBeenLastCalledWith("E-1");
    expect(screen.queryByRole("list", { name: "Selected attachments" })).not.toBeInTheDocument();
    fireEvent.keyDown(input, { key: "Backspace" });
    expect(onToggle).toHaveBeenCalledTimes(2);
  });

  it("returns focus to the input after a chip's remove button is clicked", () => {
    const { input } = setup(["E-1"]);
    fireEvent.click(screen.getByRole("button", { name: "Remove Payments" }));
    expect(input).toHaveFocus();
  });

  it("moves the highlight with the mouse", () => {
    const { input } = setup();
    fireEvent.focus(input);
    const option = screen.getByRole("option", { name: /Checkout/ });
    fireEvent.mouseEnter(option);
    expect(activeOption(input)).toBe(option);
  });

  it("toggles a result on click without closing the list", () => {
    const { input, onToggle } = setup();
    fireEvent.focus(input);
    fireEvent.mouseDown(screen.getByRole("option", { name: /Payments/ }));
    expect(onToggle).toHaveBeenCalledExactlyOnceWith("E-1");
    expect(screen.getByRole("option", { name: /Payments/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("list", { name: "Selected attachments" })).toHaveTextContent("Payments");
  });
});
