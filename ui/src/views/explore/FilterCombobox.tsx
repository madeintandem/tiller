// Only a picked option becomes a filter. Typing narrows locally and closing
// reverts to the selected label (ui-surface.md, Explore).
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import type { FacetOption } from "../../api";

interface FilterComboboxProps {
  name: string;
  options: FacetOption[];
  note?: string;
  title?: string;
  onPick: (value: string) => void;
  onClear: () => void;
}

function displayLabel(option: FacetOption) {
  return option.you === true ? `You · ${option.label}` : option.label;
}

export default function FilterCombobox({ name, options, note, title, onPick, onClear }: FilterComboboxProps) {
  const listId = useId();
  const noteId = useId();
  const listRef = useRef<HTMLUListElement>(null);
  const selected = options.find((option) => option.selected === true);
  const [query, setQuery] = useState<string | null>(null);
  // Track identity, not index: incoming counts may reorder or remove options.
  const [highlight, setHighlight] = useState<string | null>(null);
  const open = query !== null;
  const needle = (query ?? "").trim().toLowerCase();
  const shown = needle === "" ? options : options.filter((option) =>
    option.label.toLowerCase().includes(needle) || option.value.toLowerCase().includes(needle) ||
    (option.key ?? "").toLowerCase().includes(needle));
  const found = shown.findIndex((option) => option.value === highlight);
  const active = shown.length === 0 ? -1 : Math.max(found, 0);
  const activeId = open && active >= 0 ? `${listId}-${active}` : undefined;

  useEffect(() => {
    if (activeId !== undefined) {
      const option = listRef.current?.querySelector('[data-active="true"]');
      option?.scrollIntoView?.({ block: "nearest" });
    }
  }, [activeId, highlight]);

  function close() {
    setQuery(null);
    setHighlight(null);
  }

  function pick(option: FacetOption) {
    close();
    if (option.selected !== true) onPick(option.value);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const index = !open ? (event.key === "ArrowUp" ? shown.length - 1 : 0) :
        Math.max(0, Math.min(active + (event.key === "ArrowDown" ? 1 : -1), shown.length - 1));
      setQuery(query ?? "");
      setHighlight(shown[index]?.value ?? null);
    } else if (event.key === "Enter" && open) {
      event.preventDefault();
      const option = shown[active];
      if (option !== undefined) pick(option);
      else close();
    } else if (event.key === "Escape" && open) {
      event.preventDefault();
      event.stopPropagation();
      close();
    }
  }

  const selectedLabel = selected === undefined ? "" : displayLabel(selected);
  return (
    <div className="filter-combobox">
      <div className="filter-input">
        <input className="control" type="text" role="combobox" aria-label={name}
          aria-expanded={open} aria-controls={open ? listId : undefined} aria-autocomplete="list"
          aria-activedescendant={activeId} aria-describedby={note ? noteId : undefined}
          title={title} placeholder={open && selectedLabel !== "" ? selectedLabel : name}
          value={query ?? selectedLabel}
          onFocus={() => { setQuery(""); setHighlight(null); }}
          onClick={() => { if (!open) { setQuery(""); setHighlight(null); } }}
          onChange={(event) => { setQuery(event.currentTarget.value); setHighlight(null); }}
          onKeyDown={onKeyDown} onBlur={close} />
        {selected !== undefined && <button type="button" className="filter-clear" aria-label={`Clear ${name}`}
          onClick={() => { close(); onClear(); }}>×</button>}
        {open && <div className="filter-options">
          <ul id={listId} role="listbox" aria-label={name} ref={listRef}>
            {shown.map((option, index) => <li key={option.value} id={`${listId}-${index}`} role="option"
              aria-selected={option.selected === true} data-active={index === active}
              className={`filter-option${index === active ? " active" : ""}${option.cancelled === true ? " cancelled" : ""}`}
              onMouseDown={(event) => { event.preventDefault(); pick(option); }}>
              <span className="filter-option-label">{displayLabel(option)}</span>{" "}
              {option.key && <span className="mono muted filter-option-key">{option.key}</span>}{" "}
              <span className="filter-option-count">{option.count}</span>
            </li>)}
          </ul>
          {shown.length === 0 && <p className="filter-options-empty muted" role="status">No matches</p>}
        </div>}
      </div>
      {note && <p id={noteId} className="filter-note">{note}</p>}
    </div>
  );
}
