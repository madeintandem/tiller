import { type FormEvent, type KeyboardEvent, useRef, useState } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { useSlashFocus } from "../lib/useSlashFocus";

// The one search field, in the topbar of every page. Submitting goes to
// Explore's search page, which owns the results; on that page the field shows
// the query in the URL, so a direct load, a back step, and a shared link all
// fill it. An empty submit goes to the same page without a query, which
// answers with its own prompt.
export default function TopbarSearch() {
  const navigate = useNavigate();
  const location = useLocation();
  const [params] = useSearchParams();
  const inputRef = useRef<HTMLInputElement>(null);
  useSlashFocus(inputRef);

  const urlQuery = location.pathname === "/explore" && params.get("mode") === "search" ? (params.get("q") ?? "") : "";
  // The field is editable, so it holds its own text; it takes the URL's query
  // again whenever the URL's query changes.
  const [value, setValue] = useState(urlQuery);
  const [seen, setSeen] = useState(urlQuery);
  if (seen !== urlQuery) {
    setSeen(urlQuery);
    setValue(urlQuery);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const next = new URLSearchParams({ mode: "search" });
    if (value.trim() !== "") {
      next.set("q", value.trim());
    }
    navigate(`/explore?${next.toString()}`);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Escape") {
      event.currentTarget.blur();
    }
  }

  return (
    <form className="topbar-search" role="search" onSubmit={submit}>
      <input
        ref={inputRef}
        className="topbar-search-input"
        type="text"
        value={value}
        onChange={(event) => setValue(event.target.value)}
        onKeyDown={onKeyDown}
        placeholder="Search"
        aria-label="Search"
        aria-keyshortcuts="/"
        autoComplete="off"
        spellCheck={false}
      />
      <kbd className="topbar-search-hint" aria-hidden="true">/</kbd>
    </form>
  );
}
