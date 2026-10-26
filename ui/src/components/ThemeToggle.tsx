// The topbar's Auto/Light/Dark segmented control (Phase 0 task 3). Visual
// shape from the mock's own .theme-toggle (the retired console mock, styles at
// :237-244); all attribute/localStorage semantics live in lib/theme.ts, which
// main.tsx's pre-paint boot call also uses, so the control and the no-flash
// apply can never disagree on what "auto" or a garbage stored value mean.

import { useState } from "react";

import { readStoredTheme, setTheme, type ThemePick } from "../lib/theme";

const OPTIONS: ReadonlyArray<{ pick: ThemePick; label: string }> = [
  { pick: "auto", label: "Auto" },
  { pick: "light", label: "Light" },
  { pick: "dark", label: "Dark" },
];

export default function ThemeToggle() {
  // Initialized from storage, not from the DOM attribute main.tsx already
  // set: readStoredTheme applies the same garbage-to-"auto" fold, so a
  // hand-edited value renders as Auto here too, matching what bootTheme put
  // on the root a moment earlier.
  //
  // This re-reads localStorage a moment after bootTheme's own pre-paint read
  // already set the root attribute (task-3-report.md's own trade-off,
  // ratified here rather than reworked - phase-5 review, Task 3 minor):
  // deliberate duplication, not an oversight. It aids testability (this
  // component owns its own initial state instead of reading the DOM
  // attribute back out of the root), and it cannot drift from what
  // bootTheme applied, since both calls go through the same
  // normalizeTheme fold on the same stored value.
  const [pick, setPick] = useState<ThemePick>(() => readStoredTheme());

  return (
    <div className="segmented-control" role="group" aria-label="Theme">
      {OPTIONS.map((option) => (
        <button
          key={option.pick}
          type="button"
          className={option.pick === pick ? "active" : undefined}
          aria-pressed={option.pick === pick}
          onClick={() => {
            setTheme(option.pick);
            setPick(option.pick);
          }}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}
