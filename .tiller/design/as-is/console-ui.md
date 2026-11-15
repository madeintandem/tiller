# Console UI: the React application embedded in `tiller serve`

## What this surface is

This is the browser half of the read-only local console: a React 19 single-page application, built
by Vite, that the binary serves from its embedded `ui/dist`. Its one reader is the human in the
loop. It renders the server's read API and shows copyable CLI commands; it sends no write, with one
exception that is still a GET, the `Mark caught up` press on History.

`ui/src/App.tsx` owns the shell (topbar, primary navigation, search field, as-of line, theme
control) and the route table. The pages are Home (`/`), Build (`/build`), Fix (`/fix`), Report
(`/report`), and History (`/history`), plus Explore as a pathless layout route whose outlet swaps
among the no-selection pane (`/explore`), the questions ledger (`/questions`), the amendments
ledger (`/amendments`), node detail (`/node/:noun/*`), and file detail (`/file/*`). Drift is a tab
on Home, not a page. Four retired addresses forward with a history replace: `/explore/:noun/:id`,
`/recap`, `/bugs-chores`, and `/drift`. An unclaimed path renders "No such page".

Authoritative: `docs/spec/ui-surface.md`, "What the console is", "Who it serves, and identity",
"The bounding principles", "The views" and each of its subsections (Home, Build, Ownership,
contribution, and shared review, Fix, History, Explore, File detail, Drift report, Node detail,
Report, Agent use), and "What the console refuses". Those sections hold the per-page contract; this
page does not restate it.

## Data it owns

The UI owns no project data and no server state. Everything it shows is fetched per mount and held
in React state; there is no persistent response cache (`ui-surface.md`, "Liveness").

What it does own is per-viewer convenience state in the browser's `localStorage`, every read and
write wrapped so that blocked or corrupt storage falls back to the default. Keyed per project key:
Build's epic selection (`tiller-build-epics:<key>`), Fix's filters and sort
(`tiller-fix-filters:<key>`), Explore's expanded branches (`tiller-explore-open:<key>`), Explore's recently
opened nodes (`tiller-explore-recent:<key>`, eight at most), and the Map's open set and viewport
(`tiller.map.<key>`). Flat, per browser: the theme pick (`tiller-theme`), the board scrolling mode
(`tiller-board-scroll`), the Build and Fix sidebar collapse (`tiller-build-panel`,
`tiller-fix-panel`), the Explore divider width (`tiller-explore-split`), and Hide cancelled
(`tiller-explore-hide-cancelled`). The contracts for what is saved, what a URL overrides, and the
visit-only values (`?epics=all`, `?filters=none`) are in `ui-surface.md`, "Build" ("Selection is
remembered per project"), "Fix" ("Filters are remembered per project"), "Explore", and "The views"
(theme). The storage key names themselves appear in no source; the code is the record.

Shareable page state rides the URL instead (tabs, windows, filters, sort, selection, query). The
last-visit horizon is the server's, not the UI's.

## Behavior to preserve

**Data fetching.** Every read goes through `fetchJSON` in `ui/src/api.ts`, and almost every view
reads through `useFetch(path, { refreshKey, retainData })` in `ui/src/hooks.ts`. A path change drops
the old data at once; a reread of the same path keeps what is on screen until the new answer lands
and keeps it beside the error if the reread fails; a superseded or unmounted read aborts its
request. A reread is one module counter (`bumpFetchGeneration`) that every mounted `useFetch`
subscribes to. Two things bump it: the topbar refresh control (`ui/src/components/AsOf.tsx`) and
the window coming back when the newest successful read is at least 30 seconds old
(`ui/src/lib/useRefetchOnFocus.ts`), skipped while any read is still out. Build and Fix also reread
on an arrival at the board already showing (`ui/src/lib/useArrivalKey.ts`), and the shell's status read and Explore's navigator
reread when the pathname changes. Nothing polls. The only request that carries `X-Tiller-Visit: 1`
is `markCaughtUp`, deliberately outside `useFetch`. See `ui-surface.md`, "Liveness" (all of
"Current behavior" through "Read cost"), "History" (the horizon paragraphs), and "Task execution
sources" (the two-step node history read).

**Rendering rules.** One Markdown renderer (`ui/src/components/Prose.tsx`, on `react-markdown` and
`remark-gfm`) applies the link policy, escapes raw HTML, and renders every image as its alt text
only. Status color always travels with a word or shape; one fold maps stored statuses onto the
seven chart states (`ui/src/lib/states.ts`). Tab rails are one component with real tab semantics
(`ui/src/components/NodeTabs.tsx`). Charts are hand-written SVG. See `ui-surface.md`, "The views"
("Tab rails are real tabs", "Shared rendering conventions", "Markdown reference safety", and the
palette and type paragraphs).

**Copyable commands.** Most commands arrive in the payload (queue entries, ranked moves, drift
remedies, the questions ledger). The UI composes the rest itself, which no source states: the node
page's fallback attest, answer, and approve commands (`ui/src/lib/attentionFallback.ts`), a decision
band's batch approve (`ui/src/views/home/bands.ts`), a question page's cancel and unlink
(`ui/src/views/node/QuestionBody.tsx`), the stub-spec hint and `tiller approve amendment` (`ui/src/views/node/ProseSection.tsx`),
`tiller approve design` on a file page (`ui/src/views/FileDetail.tsx`), and the Map's `tiller query`
line (`ui/src/views/explore/map/relationshipPresentation.ts`). A CLI grammar change must be mirrored in those files. It never composes a drift command.

**Visual system.** `ui/src/styles/tokens.css` defines role-named surfaces and inks, one accent, the
seven `--st-<state>` pairs, two radii, and the spacing scale, themed through `light-dark()` and the
`data-theme` attribute that `ui/src/lib/theme.ts` and an inline pre-paint script in `ui/index.html`
set. `ui/src/styles.css` is the base layer and loads before the view sheets;
`ui/src/styles/fonts.css` declares the self-hosted families. See `ui-surface.md`, "The views" (the
logo, palette, and font paragraphs). The token names and file layout appear in no source.

**The embedded build.** `ui/embed.go` embeds `ui/dist` (`//go:embed all:dist`), and `ui/dist` is
committed so `go build` needs no Node. A change under `ui/src` ships with a rebuilt `ui/dist` in the
same change (`make ui`), and CI fails when the committed build and generated notices do not
reproduce. The build also writes `THIRD_PARTY_NOTICES.txt` to the repository root, `ui/public`, and
`ui/dist`, and the three copies must match. Fonts are ten self-hosted `woff2` files with their three
full OFL texts under `ui/public/fonts`, copied into `ui/dist/fonts`; the app loads nothing from a
CDN. See `CONTRIBUTING.md`, "The embedded console"; `docs/releasing.md`, "Cutting one" (the Node
and notices steps) and "Dependency updates" (the `ui/` PR rule); `docs/spec/distribution.md`, "What
Tiller is built as" and "Third-party licenses".

**Tests.** Vitest with jsdom and Testing Library (`npm test` in `ui`), configured in
`ui/vite.config.ts` with `ui/src/test/setup.ts`; 162 test files sit beside the code they cover or in `__tests__/` folders next to it, and
real CSS loads only for the sheets the config lists, so tests can pin layout rules by source.
`ui/embed_test.go` is a Go test over the embedded notices and font licenses. The commands
appear in `CONTRIBUTING.md`, "Set up", and `docs/releasing.md`; no source documents the Vitest
configuration.

## Edges

The server contract is the JSON of `/api/*`, typed field for field in `ui/src/api.ts` against the
Go structs in `internal/views`, `internal/gen`, `internal/search`, and `internal/serve`; the Go json
tags are the contract and nothing generates the types. `ui/src/lib/vocabulary.json` is pinned from
the Go side by tests in `internal/views`, `internal/core/drift`, and `internal/core/lifecycle`, so
a new status, event type, drift marker, or drift reason fails a Go test until the UI lists it.
The embedded assets reach the server as `ui.Dist` through `internal/cli/serve_cmd.go`, and
`internal/serve` sets their cache headers.

The build reaches outside `ui`: `ui/src/App.tsx` imports `tiller-logo.svg` and
`tiller-logo-inverted.svg` from the repository root, `ui/vite.config.ts` imports
`scripts/third-party-notices.mjs`, which reads `third_party/npm-notices.json`, and the console
build also refreshes the docs site's font license copies under `site/`. In development,
`npm run dev` proxies `/api` to a running `tiller serve` on `127.0.0.1:4400`. Outbound, the app
links to GitHub for commits, branches, and PRs and to authored `https://` links; it makes no other
network request.

## Described paths

- `ui`

## Discrepancies and gaps

- **Markdown images.** `SECURITY.md`, "Scope notes", says "Remote Markdown images can cause the
  browser to contact their hosts". The code renders every Markdown image as alt text with no image
  element or request (`ui/src/components/Prose.tsx`, the `img` entry of `COMPONENTS`, pinned by
  `ui/src/components/__tests__/prose.test.tsx`), and `ui-surface.md`, "The views" ("Markdown
  reference safety"), agrees with the code. `SECURITY.md` is the stale side.
- **Unmounted rail component.** `ui/src/components/RailSection.tsx` and its `tiller.home.rail.`
  storage prefix exist, but no view mounts it; only a test imports it. This matches `ui-surface.md`,
  "Home" ("Future rail design"), which says Home has no rail tenant.
- **Planned, not built.** Write buttons with revision checks, the uncommitted-changes indicator,
  automatic updates, Report export and an executive summary, Map status, owner, and type filters and
  image export, and a local approved-spec blob reader are designs only: `ui-surface.md`, "Acting at
  the gates", "Liveness" ("Unbuilt two-tier design"), "Explore" ("Unbuilt map scope"), "Node detail",
  and "What's next". The code has none of them.
