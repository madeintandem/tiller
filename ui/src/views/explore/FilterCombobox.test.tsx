import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FacetOption } from "../../api";
import FilterCombobox from "./FilterCombobox";

afterEach(cleanup);

const people: FacetOption[] = [
  { value: "jc", label: "JC Grubbs", count: 4, you: true },
  { value: "bob", label: "Bob Chen", count: 2, selected: true },
  { value: "dana", label: "Dana Lee", count: 1 },
];

function setup(options = people, note?: string) {
  const onPick = vi.fn();
  const onClear = vi.fn();
  const view = render(<FilterCombobox name="owner" options={options} note={note} onPick={onPick} onClear={onClear} />);
  return { ...view, input: screen.getByRole("combobox", { name: "owner" }), onPick, onClear };
}

function activeOption(input: HTMLElement) {
  const id = input.getAttribute("aria-activedescendant");
  return id === null ? null : document.getElementById(id);
}

describe("FilterCombobox", () => {
  it("shows the selected label, opens all choices with counts and You, and connects the listbox", () => {
    const { input } = setup();
    expect(input).toHaveValue("Bob Chen");
    expect(input).toHaveAttribute("aria-expanded", "false");
    fireEvent.focus(input);
    expect(input).toHaveValue("");
    expect(input).toHaveAttribute("placeholder", "Bob Chen");
    const list = screen.getByRole("listbox", { name: "owner" });
    expect(input).toHaveAttribute("aria-controls", list.id);
    expect(within(list).getAllByRole("option")).toHaveLength(3);
    expect(screen.getByRole("option", { name: /You · JC Grubbs/ })).toHaveTextContent("4");
    expect(screen.getByRole("option", { name: /Bob Chen/ })).toHaveAttribute("aria-selected", "true");
  });

  it("narrows case-insensitively by label, id and parent key", () => {
    const { input } = setup([{ value: "E-1", label: "Payments", key: "payments/checkout", count: 3 }]);
    for (const query of ["PAY", "E-1", "checkout"]) {
      fireEvent.focus(input);
      fireEvent.change(input, { target: { value: query } });
      expect(screen.getAllByRole("option")).toHaveLength(1);
      expect(screen.getByRole("option", { name: /Payments/ })).toHaveTextContent("payments/checkout");
    }
  });

  it("moves the keyboard highlight and picks on Enter", () => {
    const { input, onPick } = setup();
    fireEvent.focus(input);
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(activeOption(input)).toHaveTextContent("Bob Chen");
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowUp" });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onPick).toHaveBeenCalledExactlyOnceWith("dana");
    expect(input).toHaveValue("Bob Chen");
    expect(input).toHaveAttribute("aria-expanded", "false");
  });

  it("opens with either arrow after closing while input remains focused", () => {
    const { input } = setup();
    fireEvent.focus(input);
    fireEvent.keyDown(input, { key: "Escape" });
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(input).toHaveAttribute("aria-expanded", "true");
    expect(activeOption(input)).toHaveTextContent("Dana Lee");
  });

  it("narrows to a pick and does not re-pick an already selected choice", () => {
    const { input, onPick } = setup();
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: "da" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onPick).toHaveBeenCalledExactlyOnceWith("dana");
    onPick.mockClear();
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: "bob" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onPick).not.toHaveBeenCalled();
    expect(input).toHaveAttribute("aria-expanded", "false");
  });

  it.each(["Enter", "Escape", "blur"])("reverts unmatched typing on %s without a pick", (exit) => {
    const { input, onPick } = setup();
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: "zzz" } });
    expect(screen.getByText("No matches")).toBeInTheDocument();
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(input).not.toHaveAttribute("aria-activedescendant");
    if (exit === "blur") fireEvent.blur(input);
    else fireEvent.keyDown(input, { key: exit });
    expect(onPick).not.toHaveBeenCalled();
    expect(input).toHaveValue("Bob Chen");
  });

  it("recovers the highlight when matching options return after an empty result", () => {
    const { input, onPick } = setup();
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: "zzz" } });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.change(input, { target: { value: "da" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onPick).toHaveBeenCalledExactlyOnceWith("dana");
  });

  it("keeps a valid highlight when fetched options shrink or reorder", () => {
    const { input, onPick, onClear, rerender } = setup();
    fireEvent.focus(input);
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    rerender(<FilterCombobox name="owner" options={[people[2], people[0]]} onPick={onPick} onClear={onClear} />);
    expect(activeOption(input)).toHaveTextContent("Dana Lee");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onPick).toHaveBeenCalledExactlyOnceWith("dana");
  });

  it("handles a vanished highlighted option and an empty incoming list without an invalid active descendant", () => {
    const { input, onPick, onClear, rerender } = setup();
    fireEvent.focus(input);
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    rerender(<FilterCombobox name="owner" options={[]} onPick={onPick} onClear={onClear} />);
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(input).not.toHaveAttribute("aria-activedescendant");
    rerender(<FilterCombobox name="owner" options={[people[0]]} onPick={onPick} onClear={onClear} />);
    expect(activeOption(input)).toHaveTextContent("JC Grubbs");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onPick).toHaveBeenCalledExactlyOnceWith("jc");
  });

  it("renders cancelled parents dimmed with their title, key and zero count", () => {
    const { input } = setup([{
      value: "E-2", label: "Audit Engine", key: "audit", count: 0, selected: true, cancelled: true,
    }]);
    expect(input).toHaveValue("Audit Engine");
    fireEvent.focus(input);
    const option = screen.getByRole("option", { name: /Audit Engine/ });
    expect(option).toHaveClass("cancelled");
    expect(option).toHaveTextContent("audit");
    expect(option).toHaveTextContent("0");
  });

  it("picks by mouse before blur and exposes an accessible clear", () => {
    const { input, onPick, onClear } = setup();
    fireEvent.focus(input);
    fireEvent.mouseDown(screen.getByRole("option", { name: /Dana Lee/ }));
    expect(onPick).toHaveBeenCalledExactlyOnceWith("dana");
    fireEvent.click(screen.getByRole("button", { name: "Clear owner" }));
    expect(onClear).toHaveBeenCalledOnce();
  });

  it("shows no clear for an unset axis and associates unresolved notes with the input", () => {
    const { input } = setup([], 'no owner "ghost"');
    expect(screen.queryByRole("button", { name: "Clear owner" })).not.toBeInTheDocument();
    expect(input).toHaveAccessibleDescription('no owner "ghost"');
  });

  it("reopens by click after a mouse pick keeps focus on the input", () => {
    const { input, onPick } = setup();
    fireEvent.focus(input);
    fireEvent.mouseDown(screen.getByRole("option", { name: /Dana Lee/ }));
    expect(onPick).toHaveBeenCalledWith("dana");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    fireEvent.click(input);
    expect(screen.getByRole("listbox", { name: "owner" })).toBeInTheDocument();
  });

  it("reopens by click after Escape closes the list", () => {
    const { input } = setup();
    fireEvent.focus(input);
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    fireEvent.click(input);
    expect(screen.getByRole("listbox", { name: "owner" })).toBeInTheDocument();
  });
});
