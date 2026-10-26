# Releasing tiller

A release is a tag. Push `vX.Y.Z` and `.github/workflows/release.yml` runs the
suite, then GoReleaser, which produces four binaries (macOS and Linux, amd64 and
arm64), `checksums.txt`, a GitHub Release with notes grouped by commit prefix,
and an updated `Casks/tiller.rb` in
[madeintandem/homebrew-tap](https://github.com/madeintandem/homebrew-tap).

The binary reports the version stamped in `internal/cli/root.go`, not an
injected one, so the tag and the stamp have to agree. GoReleaser refuses a tag
that doesn't match (`scripts/check-version-stamp.sh`).

## Cutting one

Publication requires an explicit maintainer decision. A local snapshot and
green tests do not authorize a tag push, a release, or a visibility change.
These instructions match v0.21.2. Before publishing a newer release, update
the release Quickstart to the reviewed artifact's exact command contract and
complete it using the installed artifact, not a source binary.

Install patched Node tooling first (CI uses Node 22.22.2 or newer in major 22),
then rebuild notices and both asset surfaces:

```sh
(cd ui && npm ci)
(cd site && npm ci)
node site/scripts/fetch-pagefind-source.mjs
node scripts/third-party-notices.mjs
(cd ui && npm test && npm run build)
(cd site && npm run build)
node --test scripts/third-party-notices.test.mjs site/scripts/fetch-pagefind-source.test.mjs
python3 scripts/check-client-privacy.py --self-test
python3 scripts/check-client-privacy.py
```

Commit the rebuilt console and generated notices with the dependency changes.
The console build replaces the initial conservative notice inventory with its
actual bundled dependencies; do not run standalone notice generation after it.
Archives must contain Tiller's LICENSE, README.md, and THIRD_PARTY_NOTICES.txt,
and the embedded console must serve full notices and font licenses.

The website's Pagefind search WASM includes GPL-covered code. Its versioned
corresponding-source archive, upstream licenses, and same-site notices/source
links are required distribution assets, not optional documentation. Review
that source packet whenever Pagefind changes; an unknown version must not
silently reuse old notices. Independent Tiller code and the console are Apache 2.0.

The archive itself lives outside git as a release asset that is never marked latest. Its tag points at an empty orphan commit, so neither GoReleaser nor `tiller upgrade` ever sees it as a Tiller release. The archive was assembled by hand. Its own `README.txt` and `source-provenance.json` record every input and hash, and they are the recipe for the next version until a builder script replaces them. For a new Pagefind version, build the archive, publish it with `gh release create third-party/pagefind-<version>-source <archive> --verify-tag --latest=false` against a tag on a fresh orphan commit, and pin `correspondingSource` in the new manifest.

Set the release once, then use that value for the stamp, the tag, and the
verification:

```sh
VERSION=0.21.2

# 1. Stamp the running version in source.
$EDITOR internal/cli/root.go
# Update every doc the check lists so it repeats the new stamp. The check
# also fails while any listed doc still names the previous stamp.
scripts/check-doc-versions.sh

# Upgrade fixtures derive the running version from the source stamp.
# Historical asset-name fixtures keep the release they exercise.
go test -count=1 ./internal/cli -run \
  'TestUpgrade|TestMaintenanceChild|TestServe' -v
scripts/check-version-stamp.sh "v$VERSION"
go build -o /tmp/tiller-release-check ./cmd/tiller
/tmp/tiller-release-check --version
make test
make lint
git add internal/cli/root.go internal/cli/root_test.go README.md \
  docs/releasing.md docs/spec/distribution.md docs/spec/skill-set.md \
  docs/tiller-commands.md site/src/content/docs/quickstart.md \
  site/src/content/docs/source-quickstart.md site/src/content/docs/skills.md
git commit -m "Release: stamp v$VERSION"

# 2. Create the annotated tag and push the stamp commit plus tag.
git tag -a "v$VERSION" -m "v$VERSION"
git push origin main "v$VERSION"

# 3. Watch GoReleaser and inspect what it published.
gh run watch --exit-status
gh release view "v$VERSION"
```

The stamp commit is left out of the release notes. Everything else since the
last tag is grouped under Features, Fixes, Build and tooling, Docs, Tests, and
Other by its prefix.

## Dependency updates

Dependabot (`.github/dependabot.yml`) opens one grouped PR per ecosystem each
month: Go modules, the console under `ui/`, the docs site under `site/`, and
the actions the workflows pin. Its commits carry a `Signed-off-by` trailer,
so the DCO job passes on them like any other. It waits ten days after a
release before proposing it. That is project policy: a new package version
gets ten days of scrutiny before it can land, and contributors who set npm's
`min-release-age` to the same ten days can install every version it names. Review and
merge them like any other PR, with two exceptions.

A `ui/` PR fails CI's "ui/dist and generated notices match ui/src" gate until
the embedded console and notices are rebuilt on the branch. CI builds on Node 22, so build with the same major
to reproduce its bytes:

```sh
gh pr checkout <number>
(cd ui && npm ci && npm run build)
git add ui/dist ui/public THIRD_PARTY_NOTICES.txt site/src/fonts site/public/fonts
git commit -s -m "Build: regenerate ui/dist for the dependency bump"
git push
```

The Go toolchain is not Dependabot's to move. Before a release, or whenever
the Go security mailing list announces a fix, check by hand:

```sh
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
(cd ui && npm audit)
(cd site && npm audit)
```

A govulncheck finding needs the named dependency or toolchain patched; bumping
the `go` line alone does not fix a vulnerable module. CI and release workflows
read the toolchain from `go.mod`. For npm findings, update supported direct
dependencies and transitive locks, rerun both full audits, and rebuild assets
and notices. Do not interpret a clean production-only console audit as a clean
contribution toolchain.

## Before the first release on a new machine or a new secret

- `HOMEBREW_TAP_GITHUB_TOKEN` must exist as an Actions secret on this repo: a
  fine-grained token with Contents read/write on `madeintandem/homebrew-tap`
  only. Without it the release fails at the Homebrew step, after the GitHub
  Release is already published; re-run the job once the secret exists.
- The tap repo must exist and be public.

## Dry run

To see exactly what a release would produce without publishing anything:

```sh
VERSION=0.21.2
scripts/check-version-stamp.sh "v$VERSION"
SNAPSHOT_DIR=$(mktemp -d)
cp .goreleaser.yaml "$SNAPSHOT_DIR/config.yaml"
printf '\ndist: "%s/dist"\nsnapshot:\n  version_template: "%s-prelaunch"\n' \
  "$SNAPSHOT_DIR" "$VERSION" >> "$SNAPSHOT_DIR/config.yaml"
HOMEBREW_TAP_GITHUB_TOKEN=dummy go run github.com/goreleaser/goreleaser/v2@v2.18.2 \
  release --snapshot --skip=publish,announce,validate,before \
  --config "$SNAPSHOT_DIR/config.yaml"
printf 'Inspect archives, checksums, and rendered cask in %s/dist\n' "$SNAPSHOT_DIR"
```

The release workflow pins the same GoReleaser version, so the dry run rehearses
the binary that publishes. Dependabot does not bump it; move this recipe and
`.github/workflows/release.yml` together.

This leaves existing distribution files untouched. Snapshot tags otherwise come
from the last published tag, so the recipe validates the source stamp first and
skips the pre-build hook phase. Real releases must retain the tag/stamp guard.
Snapshot success does not establish clean-revision or published-release acceptance.

## Repository settings

Branch protection on main requires the build, lint, test, console freshness
(`ui/dist` and generated notices match `ui/src`), gitleaks, and DCO checks.
It requires no approving review, and admin bypass stays on, so those checks
do not promise independent review. Keep conduct in CONTRIBUTING.md; GitHub
Projects, the wiki, and Discussions stay off.

Keep these security settings enabled: private vulnerability reporting (the
path SECURITY.md describes), secret scanning with push protection, and
Dependabot alerts and security updates.
