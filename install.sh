#!/bin/sh
# Installs tiller from GitHub Releases: picks the binary for this OS and arch,
# verifies its sha256 against the release's checksums.txt, and places it on
# the PATH. POSIX sh; the whole body is a function invoked on the last line so
# a truncated `curl | sh` runs nothing.
#
#   curl -fsSL https://raw.githubusercontent.com/madeintandem/tiller/main/install.sh | sh
#
# Environment:
#   TILLER_VERSION      a tag to install, e.g. v0.9.5 (default: latest release)
#   TILLER_INSTALL_DIR  where to put the binary (default: /usr/local/bin if
#                       writable, else ~/.local/bin)
#   GITHUB_TOKEN        sent to the GitHub API if set (rate limits, private repos)

set -eu

REPO="madeintandem/tiller"

main() {
  need curl
  need tar
  need uname

  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin|linux) ;;
    *) die "unsupported OS: $os. Binaries are built for macOS and Linux; see https://github.com/$REPO/releases" ;;
  esac
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) die "unsupported architecture: $arch. Binaries are built for amd64 and arm64; see https://github.com/$REPO/releases" ;;
  esac

  version="${TILLER_VERSION:-}"
  if [ -z "$version" ]; then
    version="$(latest_tag)" || die "could not resolve the latest release from the GitHub API; set TILLER_VERSION=vX.Y.Z"
  fi
  case "$version" in v*) ;; *) version="v$version" ;; esac
  bare="${version#v}"

  archive="tiller_${bare}_${os}_${arch}.tar.gz"

  tmp="$(mktemp -d 2>/dev/null || mktemp -d -t tiller)"
  trap 'rm -rf "$tmp"' EXIT INT TERM

  say "downloading tiller $version for $os/$arch"
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    # With a token, go through the API so a private repository's assets
    # resolve too (the browser download URL answers 404 for those).
    fetch_asset "$version" "$archive" "$tmp/$archive" || die "download failed: release $version has no asset $archive, or the token cannot read $REPO"
    fetch_asset "$version" "checksums.txt" "$tmp/checksums.txt" || die "download failed: release $version has no checksums.txt"
  else
    base="https://github.com/$REPO/releases/download/$version"
    fetch "$base/$archive" "$tmp/$archive" || die "download failed: $base/$archive (does release $version exist with an asset for $os/$arch?)"
    fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "download failed: $base/checksums.txt"
  fi

  expected="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
  [ -n "$expected" ] || die "checksums.txt has no entry for $archive; refusing to install"
  actual="$(sha256 "$tmp/$archive")"
  [ "$actual" = "$expected" ] || die "checksum mismatch for $archive: expected $expected, got $actual. Refusing to install."
  say "checksum verified"

  tar -xzf "$tmp/$archive" -C "$tmp" tiller
  chmod 0755 "$tmp/tiller"

  # Prove the candidate runs before it replaces anything.
  "$tmp/tiller" --version >/dev/null 2>&1 || die "the downloaded tiller binary failed its version check; refusing to install. Nothing was changed."

  dir="$(install_dir)"
  mkdir -p "$dir" 2>/dev/null || die "cannot create $dir; set TILLER_INSTALL_DIR to a directory you own"
  dest="$dir/tiller"
  [ ! -d "$dest" ] || die "$dest is a directory, not a tiller binary; remove or rename it, or set TILLER_INSTALL_DIR elsewhere. Nothing was changed."
  if ! mv -f "$tmp/tiller" "$dest" 2>/dev/null; then
    die "cannot write to $dir. Re-run with TILLER_INSTALL_DIR=\$HOME/.local/bin, or move the binary yourself: sudo mv $tmp/tiller $dest"
  fi

  installed_version="$("$dest" --version 2>&1)" || die "installed $dest but it failed its version check: $installed_version"
  say "installed $dest ($installed_version)"
  case ":$PATH:" in
    *":$dir:"*) ;;
    *) say "note: $dir is not on your PATH. Add it, for example: export PATH=\"$dir:\$PATH\"" ;;
  esac
}

latest_tag() {
  url="https://api.github.com/repos/$REPO/releases/latest"
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" "$url"
  else
    curl -fsSL "$url"
  fi | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1 | grep .
}

fetch() {
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -o "$2" "$1"
  else
    curl -fsSL -o "$2" "$1"
  fi
}

# fetch_asset downloads a named release asset through the GitHub API: look the
# asset id up on the release, then request it as an octet stream. Needs
# GITHUB_TOKEN. Works for public and private repositories alike.
fetch_asset() {
  rel_json="$(curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/$REPO/releases/tags/$1")" || return 1
  # Whitespace goes (asset names carry none), then one asset per line.
  asset_url="$(printf '%s' "$rel_json" | tr -d '\n\r\t ' | sed 's/},{/}\
{/g' | grep "\"name\":\"$2\"" | sed -n 's/.*"url":"\(https:[^"]*\/releases\/assets\/[0-9][0-9]*\)".*/\1/p' | head -n 1)"
  [ -n "$asset_url" ] || return 1
  curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/octet-stream" -o "$3" "$asset_url"
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$1" | awk '{print $NF}'
  else
    die "need sha256sum, shasum, or openssl to verify the download"
  fi
}

install_dir() {
  if [ -n "${TILLER_INSTALL_DIR:-}" ]; then
    printf '%s\n' "$TILLER_INSTALL_DIR"
  elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    printf '%s\n' /usr/local/bin
  else
    printf '%s\n' "$HOME/.local/bin"
  fi
}

need() { command -v "$1" >/dev/null 2>&1 || die "need $1 on the PATH"; }
say()  { printf 'tiller-install: %s\n' "$*"; }
die()  { printf 'tiller-install: %s\n' "$*" >&2; exit 1; }

main "$@"
