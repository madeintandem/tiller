// Fix's Attached to filter: a searchable multi-select over the nodes that
// carry attached work. It replaces a rail of links that grew with every
// epic, feature and task. Picked nodes sit as chips above a fixed-height
// field; results scroll inside the popover listbox. Unattached is not a
// search result: the panel offers it as its own fixed toggle.
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";

import type { RelatedNode } from "../../api";
import { has } from "../../lib/has";
import { kindLabel } from "../../lib/kind";
import { UNATTACHED } from "./board";

interface AttachmentFilterProps {
  options: RelatedNode[];
  selected: string[];
  onToggle: (id: string) => void;
}

const titleOf = (node: RelatedNode) => (has(node.title) ? node.title : node.ref);
const secondaryOf = (node: RelatedNode) => (has(node.key) ? node.key : node.ref);

export default function AttachmentFilter({ options, selected, onToggle }: AttachmentFilterProps) {
  const listId = useId();
  const listRef = useRef<HTMLUListElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  // Track identity, not index: narrowing the query reorders the results.
  const [highlight, setHighlight] = useState<string | null>(null);

  const needle = query.trim().toLowerCase();
  const shown = needle === "" ? options : options.filter((node) =>
    [node.title, node.key, node.id].some((field) => (field ?? "").toLowerCase().includes(needle)));
  const found = shown.findIndex((node) => node.id === highlight);
  const active = shown.length === 0 ? -1 : Math.max(found, 0);
  const activeId = open && active >= 0 ? `${listId}-${active}` : undefined;
  const chips = selected.filter((id) => id !== UNATTACHED).map((id) =>
    options.find((node) => node.id === id) ?? { noun: "", id, ref: id, status: "" });

  useEffect(() => {
    if (activeId !== undefined) {
      listRef.current?.querySelector('[data-active="true"]')?.scrollIntoView?.({ block: "nearest" });
    }
  }, [activeId]);

  function close() {
    setOpen(false);
    setQuery("");
    setHighlight(null);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const index = !open ? (event.key === "ArrowUp" ? shown.length - 1 : 0) :
        Math.max(0, Math.min(active + (event.key === "ArrowDown" ? 1 : -1), shown.length - 1));
      setOpen(true);
      setHighlight(shown[index]?.id ?? null);
    } else if (event.key === "Enter" && open) {
      event.preventDefault();
      const node = shown[active];
      if (node !== undefined) {
        setHighlight(node.id);
        onToggle(node.id);
      }
    } else if (event.key === "Escape" && open) {
      event.preventDefault();
      event.stopPropagation();
      close();
    } else if (event.key === "Backspace" && query === "" && chips.length > 0) {
      event.preventDefault();
      onToggle(chips[chips.length - 1].id);
    }
  }

  return (
    <div className="attachment-filter">
      {chips.length > 0 && (
        <ul className="attachment-chips" aria-label="Selected attachments">
          {chips.map((node) => (
            <li key={node.id} className="attachment-chip">
              <span className="attachment-chip-label" title={node.ref}>{titleOf(node)}</span>
              <button type="button" className="attachment-chip-remove" aria-label={`Remove ${titleOf(node)}`}
                onClick={() => { onToggle(node.id); inputRef.current?.focus(); }}>×</button>
            </li>
          ))}
        </ul>
      )}
      <div className="attachment-input">
        <input className="control" type="text" role="combobox" aria-label="Attached to" ref={inputRef}
          aria-expanded={open} aria-controls={open ? listId : undefined} aria-autocomplete="list"
          aria-activedescendant={activeId} placeholder="Search nodes"
          value={query}
          onFocus={() => setOpen(true)}
          onClick={() => setOpen(true)}
          onChange={(event) => { setQuery(event.currentTarget.value); setHighlight(null); setOpen(true); }}
          onKeyDown={onKeyDown} onBlur={close} />
        {open && (
          <div className="attachment-options">
            <ul id={listId} role="listbox" aria-label="Attached to" aria-multiselectable="true" ref={listRef}>
              {shown.map((node, index) => (
                <li key={`${node.noun}-${node.id}`} id={`${listId}-${index}`} role="option"
                  aria-selected={selected.includes(node.id)} data-active={index === active}
                  className={`attachment-option${index === active ? " active" : ""}`}
                  onMouseEnter={() => setHighlight(node.id)}
                  onMouseDown={(event) => { event.preventDefault(); setHighlight(node.id); onToggle(node.id); }}>
                  <span className="attachment-option-kind" data-kind={node.noun}>{kindLabel(node.noun)}</span>
                  <span className="attachment-option-title">{titleOf(node)}</span>
                  <span className="attachment-option-key" title={secondaryOf(node)}>{secondaryOf(node)}</span>
                </li>
              ))}
            </ul>
            {shown.length === 0 && <p className="attachment-options-empty muted" role="status">No matches</p>}
          </div>
        )}
      </div>
    </div>
  );
}
