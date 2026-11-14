# Docs site (as-is)

Pointer page. The contract lives in the sources named below; this page maps them and records what
the code does where no source covers it. Checked against the tree at `7c4ca1a`.

## What this surface is

`site/` is the public documentation site, built with Astro and Starlight and served from GitHub
Pages at `https://madeintandem.github.io/tiller/`, so every URL sits under `/tiller`. It has four
parts: pages generated from the repository's own Markdown, a handful of pages written here, a
standalone homepage, and the tooling that shoots the console screenshots and the link preview. Its
readers are people evaluating or using Tiller; its maintainers are whoever changes the specs, the
console's look, or the release. It owns no product behavior. It restates the product, so it has to
follow the binary and the specs.

The contract is `site/AGENTS.md` (the whole file), `site/README.md`, `CONTRIBUTING.md`, "The docs
site", and `site/screenshots/README.md` for the screenshot tooling. `docs/spec/distribution.md`,
"Documentation", states what the human-readable docs are meant to provide, and its "Third-party
licenses" section holds the search-source obligation.

## Data it owns

It owns no Tiller project state. What is tracked and written by hand: the pages `quickstart.md`,
`source-quickstart.md`, `concepts.md`, `skills.md`, and `console.md` under `src/content/docs/`, the
homepage `src/pages/index.astro`, the styles, the Starlight configuration and sidebar in
`astro.config.mjs`, and the search component override. Tracked but produced by tools: the console
screenshots and `social-preview.png` in `public/`, `public/THIRD_PARTY_NOTICES.txt`, and the font
license texts in `src/fonts/` and `public/fonts/`, which the notices generator copies from the
console's fonts.

Generated on every dev and build and ignored by the root `.gitignore`: `src/content/docs/spec/`,
`contributing.md`, `releasing.md`, and `cli.md`. The sync deletes and rebuilds `spec/` each run, so
an edit to a generated copy is lost. Also ignored: `dist/`, `.astro/`, both `node_modules/`
directories, and the fetched Pagefind source archive in `public/`. See `site/AGENTS.md`, first
bullet, and the header of `site/scripts/sync-docs.mjs`.

The screenshot fixture is a throwaway project outside the repository (default
`/tmp/tiller-shots/acme`) that carries a sentinel file marking it as replaceable. See
`site/screenshots/README.md`, "Reshoot".

## Behavior to preserve

Generated versus authored. `site/scripts/sync-docs.mjs` copies every file in `docs/spec/`, plus
`CONTRIBUTING.md`, `docs/releasing.md`, and `docs/tiller-commands.md`, into the content collection.
It takes the first `# ` line as the title, adds an edit link back to the source on GitHub, orders
the five specs, turns links between specs into site links, and turns every other relative link
into a GitHub URL on `main`. `predev` and `prebuild` run it. See `site/AGENTS.md`, first two
bullets, and `CONTRIBUTING.md`, "The docs site". The link rewriting and the spec order are
documented only in the script.

Stamped pages. `quickstart.md`, `source-quickstart.md`, and `skills.md` must name the current
version stamp and drop the previous one; `scripts/check-doc-versions.sh` enforces it in CI. See
`docs/releasing.md`, "Cutting one".

Homepage and look. The homepage is a standalone page whose scripted terminal walk-through must
stay true to what the binary prints, and the palette and fonts are the console's, kept in step by
hand in both themes. See `site/AGENTS.md`, the homepage and look bullets. Nothing checks either
mechanically.

Search source. Pagefind's search WebAssembly carries GPL-covered code, so every build must place
the matching corresponding-source archive in `public/`: `prebuild` fetches it with `gh` from the
release named in the manifest and refuses bytes whose SHA-256 differs, a version with no pinned
manifest, or a missing archive. After the build, a hook checks Pagefind's emitted scripts against
the reviewed inventory and fails on any change. The Project sidebar links the notices and the
archive. See `docs/spec/distribution.md`, "Third-party licenses" (last two paragraphs),
`docs/releasing.md`, "Cutting one" (the two Pagefind paragraphs), and `site/AGENTS.md`, the
Commands paragraph. `npm run dev` needs none of it.

Screenshots. Console images come from an Acme stand-in project built only through the `tiller`
CLI and shot in WebKit; state a shot needs is added to the fixture script, never posed by editing
`.tiller/` by hand. The fixture builder replaces only a directory that carries its own sentinel as
a regular file, which `screenshots/acme_test.sh` proves in CI. See `site/AGENTS.md`, last
paragraph, and `site/screenshots/README.md`, "Reshoot" and "The link preview".

Deploy. `.github/workflows/docs.yml` builds on pushes to `main` and on pull requests that touch
`site/`, `docs/`, `CONTRIBUTING.md`, the workflow, the notices generator, `third_party/`, or the
console fonts, and on manual dispatch. The build runs the fetch test, builds, fails when the
committed website notices or font licenses are stale, and uploads `site/dist`. Only non-PR runs
deploy to the `github-pages` environment. `site/AGENTS.md`, first paragraph, names the workflow;
the triggers and the freshness gate are described only in the workflow file.

No source documents `src/components/Search.astro`. It wraps the browser's `Worker` so the base
path in Pagefind's init message becomes an absolute same-origin URL, which Pagefind 1.5 needs
because it resolves that path inside a blob worker.

## Edges

Inputs from the rest of the repository: `docs/spec/`, `CONTRIBUTING.md`, `docs/releasing.md`,
`docs/tiller-commands.md`, the two root logo SVGs, `scripts/third-party-notices.mjs` (imported by
`astro.config.mjs` as a Vite plugin and a build hook), `third_party/npm-notices.json`, the Pagefind
manifest under `third_party/`, the console's font licenses in `ui/public/fonts/`, and the
console's tokens in `ui/src/styles/tokens.css` as the design source for `src/styles/tiller.css`.

External: GitHub Pages, the GitHub CLI and the `third-party/pagefind-1.5.2-source` release asset,
Playwright with WebKit and `cwebp` for screenshots, and a `tiller` binary on `PATH` or in `TILLER`
for the fixture. Dependabot covers `site/` but not `site/screenshots/`, which has its own
`package.json`.

Consumers: the Go CI job runs `screenshots/acme_test.sh`, and the release procedure rebuilds
the site and its notices (`docs/releasing.md`, "Cutting one").

## Described paths

- `site`

## Discrepancies and gaps

- `site/AGENTS.md`, second bullet, lists the authored pages as `quickstart.md`, `concepts.md`,
  `skills.md`, and `console.md`. `site/src/content/docs/source-quickstart.md` is also tracked,
  authored here, in the sidebar in `site/astro.config.mjs`, and on the version-stamp list.
- `site/AGENTS.md` says the generated pages are gitignored. They are, by the root `.gitignore`,
  not by `site/.gitignore`.
- The docs workflow's path filter does not include the root logo SVGs that `astro.config.mjs`
  reads, so a logo-only change does not rebuild or deploy the site. No source says either way.
- `docs/spec/distribution.md`, "Documentation", names five visuals and says further tutorials are
  not built. The five diagrams exist as inline SVG in `concepts.md`; no tutorial pages exist.
- Not inspected: the body of `src/pages/index.astro` beyond its image references, the content of
  the authored pages, `screenshots/homepage.sh`, and the shoot scripts' selectors.
