# Console server: the read API and projections behind `tiller serve`

## What this surface is

This is the Go half of the read-only local console. `internal/serve` builds one `http.Handler`
(`serve.Handler(root, assets, version, authority)`): a request boundary wrapped around a mux that
answers `/api/*` with JSON read projections and every other path with the embedded React app,
falling back to `index.html` so client routes deep-link. Every route is GET and HEAD only. Its one
caller is `internal/cli/serve_cmd.go`, which owns the flags, listener, timeouts, and browser opener.

The other four packages are the read models the handlers project through, and none is console-only.
`internal/views` holds the pure projections (loaded tree in, JSON view structs out) that the CLI and
the console both render. `internal/search` is the keyword scan behind `tiller search` and
`/api/search`. `internal/relationships` builds the request-local relationship index, catalog, and
query traversal. `internal/corpus` is the confined reader for the `.tiller/` Markdown corpus and the
one admission policy (`corpus.Servable`) for which Markdown coordinates may be served at all.

Authoritative: `docs/spec/ui-surface.md`, "What the console is", "Who it serves, and identity",
"The bounding principles", "What the console refuses", and "How it maps to the CLI";
`docs/spec/cli-surface.md`, "tiller serve"; `SECURITY.md`, "Scope notes" (the three `tiller serve`
paragraphs).

No source carries the whole route inventory, so it is stated here from `internal/serve/serve.go`.
"Observed" means the handler projects over the tree overlaid with task execution observations;
"served" means the served checkout alone. Sections are in `ui-surface.md` unless marked `cli`.

| Route | Served from | Tree | Section |
|---|---|---|---|
| `/api/version` | the binary's version string | none | "The views" (topbar) |
| `/api/status` | `views.Summary` | observed | not named by route; "Home" |
| `/api/board` | `views.StreamsOfInput` | observed | "Home" (In flight); cli "tiller serve" |
| `/api/queue` | `views.QueueOfInput` | served | "Home" (action queue) |
| `/api/next` | `views.NextOf`, cut to the shortlist | ranked over served, observations as input | "Home" (Up next); cli "Ranked next" |
| `/api/execution` | execution snapshot report plus titles | observed | "Task execution sources" |
| `/api/tree` | `gen.BrowseTree` | observed | not named by route; "Explore" (Browse) |
| `/api/ls` | `views` list rows, facets, axes | observed | "Explore" (Filter); "How it maps to the CLI" |
| `/api/map` | `views.MapOf` over a relationship index | observed | "Explore" (Map) |
| `/api/query` | `relationships.Query` | observed | cli "Relationship queries" |
| `/api/search` | `search.Search` | observed | "Explore"; cli "Searching: search" |
| `/api/node/<noun>/<address>` | `views.NodePageOf` plus workflow, eval, drift, basis | observed | "Node detail"; "Task execution sources" |
| `/api/build`, `/api/work-items` | `views.BuildBoardOf`, `views.WorkItemsBoardOf` | observed | "Build"; "Fix" |
| `/api/recap`, `/api/recap/peek` | `views.RecapOf` | served | "History"; "Home" (Just happened) |
| `/api/report`, `/api/report/usage` | `views.ReportOf`, `views.UsageReportOf` | served; epic progress observed | "Report"; "Agent use" |
| `/api/drift` | `drift.Run` then `views.DriftReportOf` | served | "Drift report" |
| `/api/prose` | confined Markdown reader | served | "File detail"; cli "tiller serve" |
| `/api/evidence` | recorded-evidence reader, plain text | observed | cli "tiller serve"; "Task execution sources" |
| `/api/events` | raw event feed, newest first, `limit` default 100, cap 1000 | served | no source documents it |

## Data it owns

The server persists one file, `.tiller/.cache/console.json`, holding a single `last_visit` instant.
It goes through the store's checkout-anchored, symlink-refusing file operations, is written
atomically, and is written only by a recap request carrying `X-Tiller-Visit: 1`. An absent, unsafe,
or corrupt file reads as no horizon. See `ui-surface.md`, "History" (its closing paragraphs on the
horizon, the cache file, and the marked request), and `cli-surface.md`, "tiller serve".

Everything else is in memory and disposable: the `Authority` (a startup snapshot of the bind and its
admitted Host names), the shared-load group (loads in flight only), and a relationship index built
and discarded per request. `internal/relationships` keeps one process-wide mention cache keyed by
SHA-256 of freshly read document bytes (2048 entries and 8 MiB at most, least recently used
evicted), holding work ids and line numbers only. `internal/corpus` has no cache: every walk reads
and validates current bytes. The event and usage process caches the handler switches on belong to
`internal/core/events` and `internal/usage`, and the contract parse cache to `internal/readinput`.
`internal/views` and `internal/search` own no state; their JSON field sets are the contract. See
`ui-surface.md`, "Liveness" ("Read cost"), and `cli-surface.md`, "Completeness, failures, and read
lifetime".

## Behavior to preserve

The request boundary runs before any project file, visit memory, or asset is read: Host must be in
the bind's admitted set, a present Origin must match, `Sec-Fetch-Site` must be `none` or
`same-origin`, and network binds also check Referer and send framing headers on every response. A
missing `Authority` refuses everything. See `ui-surface.md`, "Who it serves, and identity";
`cli-surface.md`, "tiller serve"; `SECURITY.md`, "Scope notes".

Every read loads the working tree fresh or joins an identical load in flight, projects, then checks
stability. A moved task source costs only its own observation; a moved served checkout is a 500,
after one reload for a read that joined another's load. See `ui-surface.md`, "Liveness" and "Task
execution sources".

The evidence reader serves only a ref some node records verbatim: lexical misuse is 400 before the
record check, an unrecorded ref is 404 before disk is touched, a symlink at any segment is 400, over
8 MiB is 413, and the body is `text/plain` with `nosniff`. Its execution-source form (`task`,
`revision`, `digest`) answers 409 when the source or the bytes changed. The prose reader serves the
`.tiller/` Markdown corpus plus Markdown a ledger record cites, never a hidden path, and adds
`baseline` and `drift_detail` for a registered as-is doc. See `cli-surface.md`, "tiller serve";
`ui-surface.md`, "File detail" and "Task execution sources".

Console-only additions never change a CLI shape: the board `column`, filter `facets`, `axes`, and
`unresolved`, lenient parent resolution on `/api/ls`, an open question's `commands`, and GitHub
`pr_url`, `commit_url`, and `branch_url`. See `ui-surface.md`, "How it maps to the CLI" and "Task
execution sources". Per-page rules live in the view sections the table names; search rules in
`cli-surface.md`, "Searching: search"; relationship rules in "Relationship queries", "Relationship
catalog", and "Completeness, failures, and read lifetime".

No source documents these, so the code is the record (`internal/serve/serve.go`,
`internal/serve/boundary.go`): a refused request answers 403, another method answers 405 with
`Allow: GET, HEAD`, an API error body is `{"error": "<message>"}`, API answers are `no-store`, and
the embedded app is cached by directory (`assets/` immutable for a year, `fonts/` for a day,
everything else `no-cache`).

## Edges

`internal/views` is shared with the CLI. `internal/cli` renders `status`, `show`, `ls`, `queue`,
`next`, `guide`, `readout` and its correction block, `drift`, and the gate-act readouts that the
approval, attestation, question, amendment, and `sync` commands print, so a change to a shared view
changes CLI output and its goldens. Only the console calls `BuildBoardOf`, `WorkItemsBoardOf`,
`MapOf`, `NodePageOf`, `RecapOf`, `ReportOf`, `UsageReportOf`, `StreamsOfInput`, and `Facets`.
`internal/search` backs `tiller search`; `internal/relationships` is also imported by
`internal/cli`, `internal/gen`, `internal/core/mutate`, and `internal/readinput`; `internal/corpus`
by `internal/core/mutate`, `internal/readinput`, and `internal/search`.

The handlers depend on `internal/store` (tree load, stability check, cache file),
`internal/readinput` (workflow input, relationship index assembly), `internal/execution` (task
source observations), `internal/gen` (`BrowseTree`, `BoardStatus`), `internal/core/drift`,
`internal/core/events`, `internal/usage`, `internal/vcs` (read-only), and `internal/rootpath`
(confined opens). Assets arrive as an `fs.FS` from `ui.Dist`. The outward contract is the JSON that
`ui/src/api.ts` types; a test in `internal/views` pins the status vocabulary against
`ui/src/lib/vocabulary.json`. The server calls no host.

## Described paths

- `internal/serve`
- `internal/views`
- `internal/search`
- `internal/relationships`
- `internal/corpus`

## Discrepancies and gaps

- **Prose extension case.** `cli-surface.md`, "tiller serve", says every direct read rejects "an
  extension other than lowercase `.md`". `validateProseRef` in `internal/serve/prose.go` compares
  the extension with `strings.EqualFold`, so a path ending `.MD` under `.tiller/` passes validation
  and scope and reaches the disk read. No test pins either answer.
- **Unnamed routes.** `/api/status`, `/api/tree`, `/api/queue`, `/api/recap/peek`, and `/api/events`
  are served but no source names them by route, and `/api/events` has no caller in `ui/src` outside
  the typed client.
- **Planned, not built.** Write endpoints, a state-commit action, a watcher or polling, two-tier
  freshness, and Report export are designs only (`ui-surface.md`, "Acting at the gates", "Liveness",
  and "What's next"). The code has none of them.
