#!/usr/bin/env bash
# reset-bare.sh must never delete the tiller source, the design inputs, or
# $HOME, whether the run path names one of them, contains one, sits inside the
# source or the design inputs, or reaches any of those through a symlink, and
# neither TILLER_FORCE=1 nor a sentinel may change that.
#
# Every case runs a copy of the script inside its own fixture tree under one
# temp directory, with HOME, the source, the design inputs, the run repo, and
# the install path all inside that tree, and a fake go on PATH so nothing
# compiles. Nothing here touches the real checkout, the real $HOME, or the
# installed tiller.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/tiller-reset-bare.XXXXXX")
TMP=$(cd "$TMP" && pwd -P)
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/bin" "$TMP/tmp"
# Fake go: `go build -o <path> ./cmd/tiller` writes a stub binary.
cat >"$TMP/bin/go" <<'GO'
#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  case "$1" in -o) out="$2"; shift 2 ;; *) shift ;; esac
done
[ -n "$out" ] || exit 2
printf '#!/bin/sh\necho "tiller fake"\n' >"$out"
chmod +x "$out"
GO
chmod +x "$TMP/bin/go"

if command -v sha256sum >/dev/null; then SUM=(sha256sum); else SUM=(shasum -a 256); fi

# manifest <dir>: every directory, symlink target, and file digest beneath it.
manifest() {
  [ -d "$1" ] || { echo "MISSING $1"; return 0; }
  (
    cd "$1"
    find . -type d | sed 's/$/\//'
    find . -type l -exec sh -c 'printf "%s -> %s\n" "$1" "$(readlink "$1")"' _ {} \;
    find . -type f -exec "${SUM[@]}" {} +
  ) | LC_ALL=C sort
}

# fixture <name>: lays a fresh tree and sets c, home, src, design. The names
# carry spaces on purpose, and the default-shaped run repo (tiller-test-run)
# shares a name prefix with the source beside it.
fixture() {
  c="$TMP/$1"
  home="$c/home dir"
  src="$home/dev/tiller"
  design="$c/client inputs/design-inputs"
  mkdir -p "$src/scripts" "$src/cmd/tiller" "$src/docs" "$design/docs"
  cp "$ROOT/scripts/reset-bare.sh" "$ROOT/scripts/path-guards.sh" "$src/scripts/"
  printf 'package main\n' >"$src/cmd/tiller/main.go"
  printf 'the spec\n' >"$src/docs/spec.md"
  printf 'the brief\n' >"$design/docs/brief.md"
  printf 'a file in home\n' >"$home/keep.txt"
}

# reset <run-path> [force]: runs the fixture's script; sets status and $TMP/out.
reset() {
  local run=$1 force=${2:-}
  case $run in
    "$TMP"/*) ;;
    *) echo "test bug: run path is outside the fixture: $run" >&2; exit 2 ;;
  esac
  status=0
  env HOME="$home" PATH="$TMP/bin:$PATH" TMPDIR="$TMP/tmp" \
    GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
    TILLER_TEST_RUN="$run" TILLER_INSTALL="$c/install/tiller" DESIGN_SRC="$design" TILLER_FORCE="$force" \
    "$src/scripts/reset-bare.sh" >"$TMP/out" 2>&1 || status=$?
}

failures=0
flunk() {
  echo "FAIL: $1" >&2
  sed 's/^/  /' "$TMP/out" >&2
  failures=$((failures + 1))
}

# refuses <label> <run-path> [force]: the script must exit non-zero with a
# "refusing:" line and leave the whole fixture byte-for-byte as it was.
refuses() {
  local label=$1 before after
  before=$(manifest "$c")
  reset "$2" "${3:-}"
  after=$(manifest "$c")
  if [ "$status" -eq 0 ] || ! grep -q '^refusing:' "$TMP/out"; then
    flunk "$label: expected a refusal, exit $status"
  elif [ "$before" != "$after" ]; then
    flunk "$label: the refusal changed the fixture"
    diff <(printf '%s\n' "$before") <(printf '%s\n' "$after") | sed 's/^/  /' >&2 || true
  fi
}

# inputs: the protected inputs a successful reset must leave alone.
inputs() {
  manifest "$src"
  manifest "$design"
  cat "$home/keep.txt" 2>&1 || true
}

# resets <label> <run-path> [force]: the script must succeed, leave a bare
# sentineled repo and an installed binary, and leave every protected input alone.
resets() {
  local label=$1 run=$2 before after
  before=$(inputs)
  reset "$run" "${3:-}"
  after=$(inputs)
  if [ "$status" -ne 0 ]; then
    flunk "$label: expected a reset, exit $status"
  elif [ ! -f "$run/.tiller-test-run" ] || [ ! -d "$run/.git" ] || [ ! -x "$c/install/tiller" ]; then
    flunk "$label: no sentineled bare repo at $run, or no installed binary"
  elif [ "$before" != "$after" ]; then
    flunk "$label: the reset changed a protected input"
  fi
}

# A valid disposable workspace: created, then reset again because it carries
# the sentinel.
fixture "fresh run"
run="$home/dev/tiller-test-run"
resets "a run repo that does not exist yet" "$run"
printf 'left over\n' >"$run/leftover.txt"
resets "a run repo carrying the sentinel" "$run"
[ ! -e "$run/leftover.txt" ] || flunk "a sentineled run repo was not emptied"

# Ownership: content the script did not create is refused, and force is what
# bypasses that check.
fixture "unowned run"
run="$home/dev/scratch"
mkdir -p "$run"
printf 'not ours\n' >"$run/precious.txt"
refuses "an unsentineled run repo holding unknown content" "$run"
resets "the same run repo under TILLER_FORCE=1" "$run" 1
[ ! -e "$run/precious.txt" ] || flunk "TILLER_FORCE=1 did not reset an unowned run repo"

# Exact protected paths, forced and sentineled.
for target in src design home; do
  fixture "exact $target"
  touch "${!target}/.tiller-test-run"
  refuses "the run path is the $target directory (sentinel, force)" "${!target}" 1
done

# Protected ancestors: the run path contains a protected directory.
fixture "ancestor of source forced"
refuses "the run path contains the source (force)" "$home/dev" 1

fixture "ancestor of source sentineled"
touch "$home/dev/.tiller-test-run"
refuses "the run path contains the source (sentinel)" "$home/dev"

fixture "ancestor of design"
refuses "the run path contains the design inputs (force)" "$c/client inputs" 1

fixture "ancestor of home"
refuses "the run path contains HOME (force)" "$c" 1

# Inside the source or the design inputs: deleting it would delete part of them.
fixture "inside source"
refuses "the run path is a directory of the source (force)" "$src/docs" 1
refuses "the run path is a new directory inside the source" "$src/new-run"

fixture "inside design"
refuses "the run path is a directory of the design inputs (force)" "$design/docs" 1

# Symlink aliases of each of the above.
fixture "alias of source"
ln -s "$src" "$c/alias"
refuses "the run path is a symlink to the source (force)" "$c/alias" 1

fixture "alias of an ancestor"
ln -s "$c/client inputs" "$c/alias"
refuses "the run path is a symlink to a directory containing the design inputs (force)" "$c/alias" 1

fixture "alias into source"
ln -s "$src/docs" "$c/alias"
refuses "the run path is a symlink into the source (force)" "$c/alias" 1

fixture "aliased parent"
ln -s "$home" "$c/home link"
refuses "the run path reaches the source's parent through a symlinked parent (force)" "$c/home link/dev" 1

# A path that will not resolve cannot be checked, so it is refused: the missing
# "new" would otherwise hide that this names the source.
fixture "unresolvable"
refuses "the run path has .. below a directory that does not exist" "$home/dev/new/../tiller" 1

# On a filesystem that ignores letter case (the macOS default), another
# spelling of a protected path is the same directory.
fixture "letter case"
if [ -d "$home/DEV" ]; then
  refuses "the run path is the source in another letter case (force)" "$home/dev/TILLER" 1
  refuses "the run path contains the source in another letter case (force)" "$home/DEV" 1
  refuses "the run path is inside the source in another letter case (force)" "$home/dev/TILLER/docs" 1
else
  echo "skipped: letter-case aliases (this filesystem is case-sensitive)"
fi

if [ "$failures" -ne 0 ]; then
  echo "$failures reset-bare.sh case(s) failed" >&2
  exit 1
fi
echo "reset-bare.sh: every case passed"
