# Distribution and release (as-is)

Pointer page. The contract lives in the sources named below; this page maps them and records what
the code does where no source covers it. Checked against the tree at `7c4ca1a`, stamp 0.24.0.

## What this surface is

This surface turns the source tree into a published binary, gets it onto a developer's machine,
and keeps it current. It holds the tag-triggered release (GoReleaser and its workflow), the POSIX
installer, the generated Homebrew cask, the self-update mechanism in `internal/selfupdate`, CI, the
third-party notices generator and license provenance, the check scripts, the Makefile, and the
GitHub plumbing (Dependabot, templates, CODEOWNERS). Its users are the maintainer cutting a
release, contributors running `make` and CI, and developers installing or upgrading. It stops at
the mechanism: the `upgrade` command's flags, phases, and result envelope belong to the CLI page,
migration, harness wiring, identity, and onboarding to other pages, and the site's build and
deploy to the docs-site page, although that workflow file sits under `.github` here.

The contract is in `docs/spec/distribution.md`, "What Tiller is built as" with "Third-party
licenses", "Install", and "Update" with "Installation ownership" and "Retention and drain
recovery"; in `docs/releasing.md`, the opening paragraphs, "Cutting one", "Dry run", and
"Repository settings"; in `CONTRIBUTING.md`, "Set up", "Pull requests", and "Sign your commits
(DCO)"; and in `SECURITY.md`, "Supported versions" and the installation paragraph of "Scope notes".

## Data it owns

It owns no project state. The version stamp is a constant in `internal/cli/root.go` (CLI page);
this surface's scripts read it and a release tag must equal it. A release is four archives named
`tiller_<version>_<os>_<arch>.tar.gz`, a SHA-256 `checksums.txt`, a GitHub Release, and
`Casks/tiller.rb` in the tap repository. That archive name is one contract held in three places:
`.goreleaser.yaml`, `install.sh`, and `AssetName` in `internal/selfupdate/swap.go`. See
`docs/releasing.md`, opening paragraphs.

On a machine, self-update keeps each outgoing build at
`~/.local/share/tiller/previous/<version>-<sha256>/tiller`, in private directories, never pruned
and never overwritten. See `docs/spec/distribution.md`, "Retention and drain recovery" and the
retained-executables row of "What Tiller places on a machine". In memory, `ExecutableIdentity` and
`HomebrewInstall` bind a path to the bytes and file identity observed at verification, and every
later use rechecks them; no source documents these types.

Committed: the generated `THIRD_PARTY_NOTICES.txt`, and the hand-kept license provenance under
`third_party/`, where `npm-notices.json` and `npm/` cover packages that ship no license file and
`pagefind-wasm-1.5.2/manifest.json` pins the Pagefind source archive's name, release, and
checksum. See `docs/spec/distribution.md`, "Third-party licenses". The privacy check carries its
denylist as SHA-256 digests inside `scripts/check-client-privacy.py`, and the gitleaks allowlist
lives in the root `.gitleaks.toml`; only their own comments document either design.

## Behavior to preserve

A tag that disagrees with the stamp never builds (`scripts/check-version-stamp.sh`, a GoReleaser
before hook), and the docs that repeat the stamp must name the current one and drop the previous
one (`scripts/check-doc-versions.sh`). See `docs/releasing.md`, opening paragraphs and "Cutting
one". Two rules live only in that script's header: a previous stamp that was tagged may still
appear in its `vX.Y.Z` form, and a shallow clone with no earlier stamp exits 2.

The release workflow runs `make test`, then GoReleaser pinned at v2.19.0: static builds for macOS
and Linux on amd64 and arm64, archives carrying `LICENSE`, `NOTICE`, `TRADEMARKS`, `README.md`,
and `THIRD_PARTY_NOTICES.txt`, grouped notes that leave out the stamp commit, and the cask. See
`docs/releasing.md`, "Cutting one", "Before the first release on a new machine or a new secret",
and "Dry run". The cask's post-install step removes the quarantine attribute because the binaries
are unsigned; only comments in `.goreleaser.yaml` say so.

The installer verifies the archive against `checksums.txt` and refuses on a missing entry or a
mismatch, runs the candidate's `--version` before it replaces anything, refuses a directory at the
destination, and installs to `/usr/local/bin` when writable, otherwise `~/.local/bin`. See
`docs/spec/distribution.md`, "Install", for the one-line contract. `TILLER_VERSION`,
`TILLER_INSTALL_DIR`, `GITHUB_TOKEN`, and the refusals are documented only in the header of
`install.sh` and exercised by `scripts/install_test.sh`.

Self-update follows `docs/spec/distribution.md`, "Update", "Installation ownership", and
"Retention and drain recovery", and `SECURITY.md`, "Scope notes". The code matches on what was
checked: checksum verification before use, extraction of only a regular archive member named
`tiller`, replacement by rename with a same-directory copy when the rename crosses devices,
identity rechecks before replacement, delegation to the owning prefix's own `brew` with the two
environment guards, and refusal of a Homebrew-looking path whose metadata does not verify.
Undocumented in any source: a 30 second HTTP timeout that covers the whole download, a 10 second
version probe, 30 second brew queries with auto-update off, a 10 minute brew upgrade, a 1 MiB cap
on subprocess output, and the test seams `TILLER_UPGRADE_API_BASE`,
`TILLER_UPGRADE_DOWNLOAD_BASE`, and `TILLER_UPGRADE_TARGET`.

CI has four jobs: the Go job (privacy, doc-version, and script tests, then build, lint, and test);
console tests with the `ui/dist` and notices freshness gate; gitleaks with a self-test of its
allowlist; and DCO sign-off on pull requests. See
`docs/releasing.md`, "Repository settings" and "Dependency updates", and `CONTRIBUTING.md`, "Set
up", "The embedded console", and "Sign your commits (DCO)". Notices generation and its ordering
rule are in `docs/spec/distribution.md`, "Third-party licenses", and `docs/releasing.md`, "Cutting
one". The Makefile's reasons for `-count=1`, the git maintenance settings, and the macOS git path
are in its own comments and `CONTRIBUTING.md`, "Set up".

No source documents the workspace scripts. `scripts/reset-bare.sh` installs a built binary and
resets a scratch run repository, `scripts/compaction-acceptance.sh` clones a project and proves
compaction changes no read, and both use `scripts/path-guards.sh` to refuse a path that is,
contains, or sits inside a protected directory, comparing resolved paths. Their `_test.sh`
companions and `scripts/install_test.sh` run in CI.

## Edges

`internal/cli` (`upgrade.go`, `upgrade_child.go`, `maintenance.go`, `maintenance_result.go`) is the
only caller of `internal/selfupdate`, which uses `internal/rootpath` for symlink-safe opens and
builds only for Unix. Outside the repository it talks to the GitHub Releases API and download
host, optionally with `GITHUB_TOKEN`, to Homebrew, and through the `HOMEBREW_TAP_GITHUB_TOKEN`
secret to `madeintandem/homebrew-tap`. Branch protection and security settings are GitHub
configuration, described in `docs/releasing.md`, "Repository settings", and not checkable here.

`scripts/third-party-notices.mjs` is imported by the console build and the site build. It reads
`go.mod`, `ui/package-lock.json`, `ui/public/fonts`, and both `node_modules` trees, and writes the
root notices, `ui/public`, `site/public`, and the site's font licenses.
`site/scripts/fetch-pagefind-source.mjs` reads the Pagefind manifest under `third_party`. The Go
CI job also runs `site/screenshots/acme_test.sh`, and `.github/workflows/docs.yml` builds and
deploys the site. CI reads the root `.gitleaks.toml` and `.gitleaksignore` and takes the Go
toolchain from `go.mod`.

## Described paths

- `internal/selfupdate`
- `install.sh`
- `.goreleaser.yaml`
- `.github`
- `scripts`
- `Makefile`
- `tools.go`
- `third_party`
- `LICENSE`
- `NOTICE`
- `TRADEMARKS`
- `THIRD_PARTY_NOTICES.txt`
- `.gitleaks.toml`
- `.gitleaksignore`
- `go.mod`
- `go.sum`

## Discrepancies and gaps

- `CONTRIBUTING.md`, "Set up", calls lint a gofmt gate on tracked Go files. The `lint` target in
  `Makefile` runs `gofmt -l .`, which covers every Go file in the working tree.
- `docs/releasing.md`, "Dependency updates", says one grouped PR per ecosystem. For Go modules and
  both npm directories `.github/dependabot.yml` groups majors apart, so a month can bring two.
- `docs/releasing.md` says notes are grouped by prefix and `CONTRIBUTING.md`, "Pull requests", lets
  a feature go without one. The Features group in `.goreleaser.yaml` matches only subjects that
  begin with "feat", so an unprefixed feature lands under Other.
- Comments in `.goreleaser.yaml` (header) and `.github/workflows/release.yml` still say Homebrew
  formula and `Formula/tiller.rb`. The configuration generates a cask at `Casks/tiller.rb`, as
  `docs/releasing.md` says.
- `tools.go` says no non-test source imports `gopkg.in/yaml.v3` yet. Many packages now do, so the
  pin is vestigial. No source mentions the file.
- Planned only, not built: the launcher, engine cache, `upgrade --project`, and update nudge
  (`docs/spec/distribution.md`, "Versioning", "Upgrading from an older binary", "What's next"), and
  a builder script for the Pagefind source archive (`docs/releasing.md`, "Cutting one").
- Not inspected: the `upgrade` orchestration in `internal/cli`, the selfupdate tests, and the body
  of the notices tests.
