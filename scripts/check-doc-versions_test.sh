#!/usr/bin/env bash
# check-doc-versions.sh must refuse a doc that names the stamp only in part:
# a stale previous stamp left beside the new one, or a longer version that
# merely contains the stamp as a substring.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/tiller-doc-versions.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

repo="$TMP/repo"
mkdir -p "$repo/scripts" "$repo/internal/cli"
cp "$ROOT/scripts/check-doc-versions.sh" "$repo/scripts/"
git -C "$repo" init -q
git -C "$repo" config user.email test@example.com
git -C "$repo" config user.name test

stamp() {
  printf 'package cli\n\nconst version = "%s"\n' "$1" >"$repo/internal/cli/root.go"
}
commit() {
  git -C "$repo" add -A
  git -C "$repo" commit -qm "$1"
}

stamp 0.4.1
printf 'Install 0.4.1 today.\n' >"$repo/guide.md"
commit "stamp 0.4.1"
stamp 0.4.2
printf 'Install 0.4.2 today.\n' >"$repo/guide.md"
commit "stamp 0.4.2"

failures=0
expect() {
  local want=$1 label=$2 status=0
  "$repo/scripts/check-doc-versions.sh" guide.md >"$TMP/out" 2>&1 || status=$?
  if [[ $want == pass && $status -ne 0 ]] || [[ $want == fail && $status -eq 0 ]]; then
    echo "FAIL: $label (expected $want, exit $status)" >&2
    sed 's/^/  /' "$TMP/out" >&2
    failures=$((failures + 1))
  fi
}

expect pass "a doc naming only the current stamp"

printf 'Install 0.4.2 today.\nVERSION=0.4.1\n' >"$repo/guide.md"
expect fail "a doc that still names the previous stamp"

printf 'Install 0.4.20 today.\n' >"$repo/guide.md"
expect fail "a doc naming a longer version that contains the stamp"

printf 'Install 10.4.2 today.\n' >"$repo/guide.md"
expect fail "a doc naming a version that ends with the stamp"

printf 'Install v0.4.2.\nOlder: 0.4.10 and v0.3.1.\n' >"$repo/guide.md"
expect pass "a doc naming the stamp beside unrelated versions"

# An uncommitted stamp bump treats the committed stamp as the previous one.
stamp 0.4.3
printf 'Install 0.4.3 today.\nVERSION=0.4.2\n' >"$repo/guide.md"
expect fail "an uncommitted bump that leaves the committed stamp behind"
printf 'Install 0.4.3 today.\n' >"$repo/guide.md"
expect pass "an uncommitted bump with every mention moved"

# A previous stamp that was published names a real release, not a stale stamp,
# but only in its tagged vX.Y.Z form.
printf 'Install 0.4.3 today. The latest published release is v0.4.2.\n' >"$repo/guide.md"
expect fail "a v-prefixed previous stamp with no release tag"
git -C "$repo" tag v0.4.2
expect pass "a v-prefixed previous stamp that names its published release"
printf 'Install 0.4.3 today. Shipped in 0.4.2.\n' >"$repo/guide.md"
expect fail "a bare previous stamp even when it was published"

if [[ $failures -ne 0 ]]; then
  exit 1
fi
echo "check-doc-versions tests passed"
