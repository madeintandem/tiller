// Vitest coverage for console v2's design-token layer (Phase 0 task 2):
// stateColor and the five shared primitives every page rebuild consumes.
// jsdom never resolves a CSS custom property (var(--st-draft) reads back
// exactly as written, never as a color), so this suite asserts the var()
// reference stateColor/statusColor/statusTextColor emit rather than a
// resolved value - the actual hex, the light/dark pair, and the AA contrast
// trade between a state's fill and its text twin all live in tokens.css and
// the mock alone now (lib/states.ts's own header comment says why).

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { NodeStatus } from "../../api";
import { stateColor, statusColor, statusTextColor } from "../../lib/states";
import Chip from "../Chip";
import CopyCmd, { CopyRef } from "../CopyCmd";
import RailSection from "../RailSection";
import SectionHead from "../SectionHead";
import StateBar from "../StateBar";
import StateDot from "../StateDot";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  // RailSection persists its collapse per section id, so a leaked key would
  // decide the next test's opening state.
  window.localStorage.clear();
});

// The seven chart states stateColor covers.
const SEVEN_STATES: NodeStatus[] = [
  "draft",
  "specced",
  "ready",
  "active",
  "in_review",
  "done",
  "blocked",
];

describe("stateColor", () => {
  it.each(SEVEN_STATES)("maps %s to its own custom property", (state) => {
    expect(stateColor(state)).toBe(`var(--st-${state})`);
  });
});

describe("statusTextColor", () => {
  it.each(SEVEN_STATES)("maps %s to its text twin's custom property", (state) => {
    expect(statusTextColor(state)).toBe(`var(--st-${state}-t)`);
  });

  // Total over any wire string, folding through the same table statusColor
  // uses, so a chip's word and its dot can never name two different states.
  it("folds a wire-only status the same way statusColor does", () => {
    expect(statusTextColor("open")).toBe(statusTextColor("in_review"));
    expect(statusTextColor("proposed")).toBe(statusTextColor("specced"));
    expect(statusTextColor("answered")).toBe(statusTextColor("done"));
    expect(statusTextColor("never-heard-of-it")).toBe(statusTextColor("draft"));
  });
});

describe("statusColor", () => {
  it.each(SEVEN_STATES)("keeps the native %s custom property unchanged", (state) => {
    expect(statusColor(state)).toBe(`var(--st-${state})`);
  });

  // The wire vocabulary beyond the seven chart states folds onto them (plan
  // amendment 74dfe3a): proposed shares specced's awaiting-your-signature
  // register; a question's open/answered are the mock's own in_review and
  // done precedents; everything terminal or unknown rests at the draft
  // slate, with the adjacent word carrying the meaning, never color alone.
  it.each([
    ["proposed", "specced"],
    ["open", "in_review"],
    ["answered", "done"],
    ["cancelled", "draft"],
    ["archived", "draft"],
    ["garbage-future-state", "draft"],
    // An Object.prototype member name must not leak a prototype-chain hit
    // past the fold; it is just another unknown status.
    ["toString", "draft"],
  ])("maps wire status %s to %s's custom property", (status, folded) => {
    expect(statusColor(status)).toBe(`var(--st-${folded})`);
  });
});

describe("StateDot", () => {
  it.each(SEVEN_STATES)("paints %s with statusColor's var()", (state) => {
    const { container } = render(<StateDot status={state} />);
    const dot = container.querySelector(".state-dot");
    expect(dot).not.toBeNull();
    expect((dot as HTMLElement).style.background).toBe(`var(--st-${state})`);
  });

  it("paints a non-chart wire status through the total mapping, never a blank dot", () => {
    const { container } = render(<StateDot status="open" />);
    expect((container.querySelector(".state-dot") as HTMLElement).style.background).toBe(
      "var(--st-in_review)",
    );
  });

  it("defaults to an 8px dot", () => {
    const { container } = render(<StateDot status="ready" />);
    expect((container.querySelector(".state-dot") as HTMLElement).style.width).toBe("8px");
  });

  it("accepts a smaller explicit size", () => {
    const { container } = render(<StateDot status="ready" size={6} />);
    expect((container.querySelector(".state-dot") as HTMLElement).style.width).toBe("6px");
  });

  it("is decorative: aria-hidden, since the adjacent word or caption carries the meaning", () => {
    const { container } = render(<StateDot status="blocked" />);
    expect(container.querySelector(".state-dot")?.getAttribute("aria-hidden")).toBe("true");
  });
});

describe("StateBar", () => {
  it("renders one segment per non-zero state, in mix order", () => {
    const { container } = render(
      <StateBar
        mix={[
          { state: "draft", n: 63 },
          { state: "specced", n: 6 },
          { state: "ready", n: 1 },
        ]}
        caption="63 draft · 6 specced · 1 ready"
      />,
    );
    const segments = container.querySelectorAll(".state-bar-segment");
    expect(segments).toHaveLength(3);
    expect(Array.from(segments).map((s) => s.getAttribute("data-state"))).toEqual([
      "draft",
      "specced",
      "ready",
    ]);
  });

  it("skips zero-count entries and renders an empty track when the total is zero", () => {
    const { container } = render(
      <StateBar
        mix={[
          { state: "draft", n: 0 },
          { state: "ready", n: 0 },
        ]}
      />,
    );
    expect(container.querySelectorAll(".state-bar-segment")).toHaveLength(0);
    // The track itself still renders (the recessed groove), just with no
    // segments.
    expect(container.querySelector(".state-bar")).not.toBeNull();
  });

  it("exposes the adjacent caption prop as its accessible text fallback", () => {
    render(<StateBar mix={[{ state: "done", n: 9 }]} caption="9 epics · all ready" />);
    expect(screen.getByRole("img", { name: "9 epics · all ready" })).toBeInTheDocument();
  });

  it("is aria-hidden with no caption, never bare color with no accessible text", () => {
    const { container } = render(<StateBar mix={[{ state: "done", n: 9 }]} />);
    expect(container.querySelector(".state-bar")?.getAttribute("aria-hidden")).toBe("true");
  });
});

describe("Chip", () => {
  it.each(SEVEN_STATES)("always renders the %s word", (state) => {
    render(<Chip status={state}>{state}</Chip>);
    expect(screen.getByText(state)).toBeInTheDocument();
  });

  it("falls back to the raw status word when children is omitted", () => {
    render(<Chip status="in_review" />);
    expect(screen.getByText("in_review")).toBeInTheDocument();
  });

  it("pairs the word with a dot colored by statusColor", () => {
    const { container } = render(<Chip status="active">active</Chip>);
    const dot = container.querySelector(".state-dot") as HTMLElement;
    expect(dot).not.toBeNull();
    expect(dot.style.background).toBe(statusColor("active"));
  });

  it("renders a wire-only status with its word and folded color", () => {
    const { container } = render(<Chip status="open" />);
    expect(screen.getByText("open")).toBeInTheDocument();
    const dot = container.querySelector(".state-dot") as HTMLElement;
    expect(dot.style.background).toBe("var(--st-in_review)");
  });

  it("tints at the mock's --tint, riding the same status color", () => {
    const { container } = render(<Chip status="ready">ready</Chip>);
    const chip = container.querySelector(".chip") as HTMLElement;
    expect(chip.style.background).toContain("var(--tint)");
    expect(chip.style.background).toContain(statusColor("ready"));
  });

  // Console v2's deliberate behavior change to Chip (the chart-table pass
  // mixed toward transparent - see the old mock's .chip.ready/.chip.active,
  // 12% color-mix toward transparent - so a chip's wash rode whatever sat
  // behind it): the fill now mixes toward --panel, an opaque wash that reads
  // the same on every surface the chip lands on, not just the panel it was
  // designed against.
  it("mixes its tint toward --panel, not transparent", () => {
    const { container } = render(<Chip status="ready">ready</Chip>);
    const chip = container.querySelector(".chip") as HTMLElement;
    expect(chip.style.background).toContain("var(--panel)");
    expect(chip.style.background).not.toContain("transparent");
  });

  // The word does NOT ride the fill's own var(): the mock pairs each state's
  // fill with a distinct text custom property (mock:87-124) and Chip composes
  // both rather than deriving one from the other.
  it("writes the word in the text twin's custom property, not the fill's", () => {
    const { container } = render(<Chip status="ready">ready</Chip>);
    const chip = container.querySelector(".chip") as HTMLElement;
    expect(chip.style.color).toBe("var(--st-ready-t)");
    expect(chip.style.color).not.toBe(statusColor("ready"));
  });
});

describe("CopyCmd", () => {
  let writeText: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
  });

  it("writes the exact command to the clipboard and shows the shared Copied toast", async () => {
    const cmd = "tiller approve feature foundation/identity-roles-program-model";
    render(<CopyCmd cmd={cmd} />);

    fireEvent.click(screen.getByRole("button", { name: cmd }));

    expect(writeText).toHaveBeenCalledWith(cmd);
    expect(await screen.findByText("Copied")).toBeInTheDocument();
  });

  it("routes a second command through the same shared toast root", async () => {
    render(<CopyCmd cmd="tiller show feature foo" />);
    render(<CopyCmd cmd="tiller approve feature foo" />);

    fireEvent.click(screen.getByRole("button", { name: "tiller show feature foo" }));
    expect(await screen.findByText("Copied")).toBeInTheDocument();

    // Exactly one toast node exists across both mounted instances.
    expect(document.querySelectorAll(".copy-toast")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "tiller approve feature foo" }));
    expect(await screen.findByText("Copied")).toBeInTheDocument();
    expect(document.querySelectorAll(".copy-toast")).toHaveLength(1);
  });

  // A console served over the network on plain http is not a secure context,
  // so the browser withholds navigator.clipboard entirely. The copy must still
  // land, through the selection-based fallback.
  it("falls back to a selection copy when navigator.clipboard is absent", async () => {
    Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true });
    let copied = "";
    const execCommand = vi.fn((command: string) => {
      copied = command === "copy" ? (document.activeElement as HTMLTextAreaElement).value : "";
      return true;
    });
    Object.defineProperty(document, "execCommand", { value: execCommand, configurable: true });
    render(<CopyCmd cmd="tiller show feature foo" />);

    fireEvent.click(screen.getByRole("button", { name: "tiller show feature foo" }));

    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(copied).toBe("tiller show feature foo");
    expect(await screen.findByText("Copied")).toBeInTheDocument();
    expect(document.querySelector("textarea")).toBeNull();
  });

  it("falls back to a selection copy when the clipboard refuses the write", async () => {
    writeText.mockRejectedValue(new DOMException("denied", "NotAllowedError"));
    const execCommand = vi.fn(() => true);
    Object.defineProperty(document, "execCommand", { value: execCommand, configurable: true });
    render(<CopyCmd cmd="tiller show feature foo" />);

    fireEvent.click(screen.getByRole("button", { name: "tiller show feature foo" }));

    await waitFor(() => expect(execCommand).toHaveBeenCalledWith("copy"));
    expect(await screen.findByText("Copied")).toBeInTheDocument();
  });

  it("carries no accent by default", () => {
    const { container } = render(<CopyCmd cmd="tiller show feature foo" />);
    expect(container.querySelector(".copy-cmd-accent")).toBeNull();
  });

  it("opts into the accent only via an explicit prop", () => {
    const { container } = render(<CopyCmd cmd="tiller approve feature foo" accent />);
    expect(container.querySelector(".copy-cmd-accent")).not.toBeNull();
  });
});

describe("CopyRef", () => {
  let writeText: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
  });

  it("copies the node's id, not the displayed ref, and shows the shared Copied toast", async () => {
    // A keyed node's ref reads "key (id)"; the paste-able handle is the id.
    render(<CopyRef display="billing/refunds (F-1)" id="F-1" />);

    fireEvent.click(screen.getByRole("button", { name: "billing/refunds (F-1)" }));

    expect(writeText).toHaveBeenCalledWith("F-1");
    expect(await screen.findByText("Copied")).toBeInTheDocument();
  });

  it("routes through the same single toast root CopyCmd owns", async () => {
    render(<CopyCmd cmd="tiller show question Q-1" />);
    render(<CopyRef display="Q-1" id="Q-1" />);

    fireEvent.click(screen.getByRole("button", { name: "Q-1" }));
    expect(await screen.findByText("Copied")).toBeInTheDocument();
    expect(document.querySelectorAll(".copy-toast")).toHaveLength(1);
  });

  it("never wears the command chip's chrome or its accent icon", () => {
    const { container } = render(<CopyRef display="Q-1" id="Q-1" />);
    expect(container.querySelector(".copy-cmd")).toBeNull();
    expect(container.querySelector(".copy-cmd-icon")).toBeNull();
    expect(container.querySelector(".copy-ref")).not.toBeNull();
  });
});

describe("SectionHead", () => {
  it("always renders the label", () => {
    render(<SectionHead label="Needs you" />);
    expect(screen.getByText("Needs you")).toBeInTheDocument();
  });

  it("renders no badge element when none is given", () => {
    const { container } = render(<SectionHead label="Needs you" />);
    expect(container.querySelector(".count-badge")).toBeNull();
  });

  it("renders the accent count badge when given one", () => {
    const { container } = render(<SectionHead label="Needs you" badge="2 decisions" />);
    expect(screen.getByText("2 decisions")).toBeInTheDocument();
    expect(container.querySelector(".count-badge")).not.toBeNull();
  });

  it("renders no trailing slot when none is given", () => {
    const { container } = render(<SectionHead label="Needs you" badge="2 decisions" />);
    expect(container.querySelector(".eyebrow-row-extra")).toBeNull();
  });

  // The trailing slot is last in the row and pushed to the far edge, so the
  // badge stays beside its label (Home puts its recap signal here).
  it("renders a header extra last in the row when given one", () => {
    const { container } = render(
      <SectionHead label="Needs you" badge="2 decisions" headerExtra={<a href="/recap">Recap</a>} />,
    );
    const row = container.querySelector(".eyebrow-row");
    expect(row).not.toBeNull();
    expect(row!.lastElementChild?.className).toBe("eyebrow-row-extra");
    expect(screen.getByRole("link", { name: "Recap" })).toBeInTheDocument();
  });
});

// RailSection's collapse persistence, the same behavior home.test.tsx pinned
// while Home still mounted a rail. ui-surface.md keeps the collapsible chrome
// specified for the first future tenant even though the rail has none today,
// so the coverage follows the component here rather than dying with its last
// caller: a future tenant inherits a chrome whose persistence is still
// tested.
describe("RailSection", () => {
  it("persists a collapsed section across a remount via localStorage", () => {
    const first = render(
      <RailSection id="probe" title="Probe">
        <p>body content</p>
      </RailSection>,
    );
    expect(first.container.querySelector('[data-rail-id="probe"] .rail-section-body')).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Probe/ }));
    expect(first.container.querySelector('[data-rail-id="probe"] .rail-section-body')).toBeNull();
    expect(window.localStorage.getItem("tiller.home.rail.probe")).toBe("0");

    first.unmount();

    const second = render(
      <RailSection id="probe" title="Probe">
        <p>body content</p>
      </RailSection>,
    );
    // Reads the persisted collapse on a fresh mount, without another click.
    expect(second.container.querySelector('[data-rail-id="probe"] .rail-section-body')).toBeNull();
  });

  // Only the body collapses: the eyebrow, the chevron, and any header extra
  // stay reachable while a section is closed.
  it("keeps the header and its extra reachable while closed", () => {
    render(
      <RailSection id="probe" title="Probe" headerExtra={<Chip status="active">active</Chip>}>
        <p>body content</p>
      </RailSection>,
    );

    fireEvent.click(screen.getByRole("button", { name: /Probe/ }));

    expect(screen.queryByText("body content")).not.toBeInTheDocument();
    expect(screen.getByText("Probe")).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
  });
});
