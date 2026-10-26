# Security policy

## Reporting a vulnerability

Please don't open a public issue for a security problem.

Report it privately through GitHub instead. Open the repository's **Security**
tab and choose **Report a vulnerability**; that creates a private advisory only
the maintainers can see. If GitHub isn't an option for you, email
hello@madeintandem.com.

Include what you can. The tiller version (`tiller --version`), your OS, the
harness you drive it from if it matters, and steps or a minimal `.tiller/` tree
that reproduces the problem all help.

## What to expect

- An acknowledgement within three business days.
- A maintainer confirms the problem, works out severity with you, and aims to
  ship a fix in a patch release as soon as one is ready. We'll tell you when it
  ships and credit you in the release notes unless you'd rather we didn't.
- We ask for a reasonable window to fix the problem before you publish details.

## Supported versions

Tiller is pre-1.0, so only the latest release gets security fixes. Upgrade with
your package manager or by re-running the installer.

## Scope notes

Tiller is a local developer tool for a trusted checkout. Review the repository,
its `.tiller/tiller.yml`, and configured gate commands before running it. Gates
execute with your user's permissions; Tiller is not a sandbox for hostile
commands or repository content.

Repository-owned state, prose, and cache operations must stay within the
selected checkout and refuse symlink escapes. Explicitly configured external
reference code roots and imported design inputs are intentional exceptions for
reading input. The selected checkout root and its ancestors are trusted.
Descriptor-relative confinement does not prevent hard-link aliases, mount
changes, or someone moving an already-open directory outside the checkout.

Installation and maintenance intentionally operate outside the repository:
the installer places the executable on PATH, and self-upgrade downloads
checksum-verified GitHub release assets and retains verified previous binaries
under `~/.local/share/tiller/previous/`. Homebrew-owned installs delegate to
Homebrew, which may refresh metadata and install package dependencies. Tiller
does not store release credentials. An explicitly supplied `GITHUB_TOKEN` is
sent with release requests, which only raises GitHub's API rate limit.

`tiller serve` defaults to the loopback address `127.0.0.1`. Its request boundary
checks Host, a present Origin, and supplied browser fetch metadata before serving
project data, assets, or updating visit memory. `--host` explicitly enables
supported IP binds on a trusted network or private VPN. This exposes the
planning ledger, the `.tiller/` Markdown corpus, Markdown documents the ledger
cites, and gate evidence files the ledger records, without authentication or
TLS. Never expose it directly to the public internet. The file reader serves
only the `.tiller/` corpus and ledger-cited Markdown, and the evidence reader
serves only refs some node records; any other checkout path, including
gitignored secrets, dot directories outside `.tiller/`, and dot directories
below it such as `.tiller/.cache/`, answers not found on every bind. Both
readers refuse a symlink at any path segment, even one whose target stays in
the checkout, so a served name never yields another file's bytes. Each read
is capped at 8 MiB, including evidence read from a task's execution worktree
or branch, and the server times out slow or idle connections. `0.0.0.0` covers all IPv4 interfaces and `::` all IPv6
interfaces, including other connected networks. Use a specific locally assigned
IP when only one network is trusted.

Accepted Host names and IPs follow the bind policy, not the client's identity.
Wildcard interface addresses are snapshotted at startup; restart after address
changes. Anyone who can reach the socket can construct an admitted Host. There
is no hostname discovery, forwarded-header trust, or trusted reverse-proxy mode.
The [serve contract](docs/spec/cli-surface.md#tiller-serve) lists the per-bind
sets and unsupported literals, including link-local, zoned, and mapped addresses.

Network mode also rejects a present foreign or malformed Referer before reads
and sends same-origin framing protections on every response, including errors.
Plain HTTP LAN browsers may omit Origin, Referer, and fetch metadata. Such
headerless requests remain allowed for direct navigation and CLI access; they
cannot identify the initiating site. Because any page or peer can send one, the
console's only disk write, the recap's last-visit mark in
`.tiller/.cache/console.json`, advances only for a request carrying the
console's `X-Tiller-Visit: 1` header. A cross-site page cannot attach that
header without a CORS preflight, and the console answers no preflight and
grants no CORS access. Unmarked requests read the recap without moving the
mark. Framing policy blocks rendering, not the
request or its reads. Browser same-origin policy limits cross-origin script
reads, but this is not authentication or a guarantee against every foreign
navigation. It does not restore all localhost protections on plain HTTP.
Future write endpoints require a separate security design; this read-only
network policy does not make them safe. Older published binaries may lack
these protections.

The console renders authored Markdown. Remote Markdown images can cause the
browser to contact their hosts, disclosing normal browser request metadata.
Git/host operations, release downloads, browser opening, configured gates, and
those image requests mean file and network activity is not repository-only or
Git-only.

Report unintended repository-state reads or writes outside these boundaries,
foreign browser access to project data, and unsafe release verification.
