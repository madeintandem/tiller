// Fix's Sort control: one choice of three, so a select-only combobox rather
// than a rail of links. It is drawn in the console's own register (the
// trigger reads like the rail's active link, the popover like the Attached
// to results) instead of the browser's stock select. Keyboard follows the
// WAI-ARIA select-only combobox pattern: focus stays on the trigger and the
// highlighted option is announced through aria-activedescendant.
import { useId, useRef, useState, type KeyboardEvent } from "react";

export interface SortOption<T extends string> {
  value: T;
  label: string;
}

interface SortSelectProps<T extends string> {
  label: string;
  options: SortOption<T>[];
  value: T;
  onChange: (value: T) => void;
}

export default function SortSelect<T extends string>({ label, options, value, onChange }: SortSelectProps<T>) {
  const listId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const selectedIndex = Math.max(0, options.findIndex((option) => option.value === value));
  const [highlight, setHighlight] = useState(selectedIndex);
  const current = options[selectedIndex];

  function show(index: number) {
    setHighlight(index);
    setOpen(true);
  }

  function choose(index: number) {
    setOpen(false);
    const option = options[index];
    if (option !== undefined && option.value !== value) onChange(option.value);
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const last = options.length - 1;
    if (!open) {
      if (["ArrowDown", "ArrowUp", "Enter", " "].includes(event.key)) {
        event.preventDefault();
        show(selectedIndex);
      } else if (event.key === "Home" || event.key === "End") {
        event.preventDefault();
        show(event.key === "Home" ? 0 : last);
      }
      return;
    }
    switch (event.key) {
      case "ArrowDown":
      case "ArrowUp":
        event.preventDefault();
        if (event.altKey) choose(highlight);
        else setHighlight(Math.max(0, Math.min(highlight + (event.key === "ArrowDown" ? 1 : -1), last)));
        break;
      case "Home":
      case "End":
        event.preventDefault();
        setHighlight(event.key === "Home" ? 0 : last);
        break;
      case "Enter":
      case " ":
        event.preventDefault();
        choose(highlight);
        break;
      case "Tab":
        choose(highlight);
        break;
      case "Escape":
        event.preventDefault();
        event.stopPropagation();
        setOpen(false);
        break;
    }
  }

  return (
    <div className="sort-select">
      <button
        type="button"
        ref={triggerRef}
        className="sort-select-trigger"
        role="combobox"
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={open ? `${listId}-${highlight}` : undefined}
        onClick={() => {
          // Safari does not focus a button on click; without focus the
          // arrow keys go nowhere and an outside click never blurs it closed.
          triggerRef.current?.focus();
          if (open) setOpen(false);
          else show(selectedIndex);
        }}
        onKeyDown={onKeyDown}
        onBlur={() => setOpen(false)}
      >
        <span className="sort-select-value">{current?.label}</span>
        <span className="sort-select-chevron" aria-hidden="true" />
      </button>
      {open && (
        <ul id={listId} role="listbox" aria-label={label} className="sort-select-options">
          {options.map((option, index) => (
            <li
              key={option.value}
              id={`${listId}-${index}`}
              role="option"
              aria-selected={index === selectedIndex}
              className={`sort-select-option${index === highlight ? " active" : ""}`}
              onMouseEnter={() => setHighlight(index)}
              onMouseDown={(event) => {
                // Keep focus on the trigger so its blur doesn't close first.
                event.preventDefault();
                choose(index);
                triggerRef.current?.focus();
              }}
            >
              {option.label}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
