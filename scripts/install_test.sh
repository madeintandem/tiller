#!/usr/bin/env bash
# install.sh against local release fixtures: a fake curl on PATH serves a
# checksum-valid archive from disk, so nothing touches the network, the real
# release assets, or the installed tiller. Destination is a temp directory.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/tiller-install.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  *) arch=arm64 ;;
esac
version=v9.9.9
archive="tiller_9.9.9_${os}_${arch}.tar.gz"

sha() { if command -v sha256sum >/dev/null; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }

# fixture <name> <binary content> <mode>: builds $TMP/rel/<name>/{archive,checksums.txt}
fixture() {
  local d="$TMP/rel/$1" stage="$TMP/stage-$1"
  mkdir -p "$d" "$stage"
  printf '%s' "$2" >"$stage/tiller"
  chmod "$3" "$stage/tiller"
  tar -czf "$d/$archive" -C "$stage" tiller
  printf '%s  %s\n' "$(sha "$d/$archive")" "$archive" >"$d/checksums.txt"
}

fixture good $'#!/bin/sh\necho "tiller version 9.9.9"\n' 0755
fixture broken $'#!/bin/sh\nexit 3\n' 0755
fixture noexec $'not a program\n' 0755

# Fake curl: copies the file named by the URL's basename from $FIXTURE_DIR.
mkdir -p "$TMP/bin"
cat >"$TMP/bin/curl" <<'CURL'
#!/bin/sh
out=""; url=""
while [ $# -gt 0 ]; do
  case "$1" in -o) out="$2"; shift 2 ;; -H) shift 2 ;; -*) shift ;; *) url="$1"; shift ;; esac
done
cp "$FIXTURE_DIR/$(basename "$url")" "$out"
CURL
chmod +x "$TMP/bin/curl"

failures=0
# run <fixture> <destdir>: sets status, out
run() {
  status=0
  out=$(env -u GITHUB_TOKEN PATH="$TMP/bin:$PATH" FIXTURE_DIR="$TMP/rel/$1" \
    TILLER_VERSION=$version TILLER_INSTALL_DIR="$2" sh "$ROOT/install.sh" 2>&1) || status=$?
}
check() { # check <label> <condition-status>
  if [[ $2 -ne 0 ]]; then
    echo "FAIL: $1" >&2; printf '%s\n' "$out" | sed 's/^/  /' >&2; failures=$((failures + 1))
  fi
}

# Normal install into a fresh directory.
dest="$TMP/d1"
run good "$dest"
check "valid install exits 0" $status
check "valid install places an executable file" $([[ -f $dest/tiller && -x $dest/tiller ]]; echo $?)

# Replacement of an existing ordinary executable.
dest="$TMP/d2"; mkdir -p "$dest"; printf '#!/bin/sh\necho old\n' >"$dest/tiller"; chmod 0755 "$dest/tiller"
run good "$dest"
check "replacement exits 0" $status
check "replacement installs the new binary" $("$dest/tiller" | grep -q 9.9.9; echo $?)

# A directory at the command destination is refused with contents preserved.
dest="$TMP/d3"; mkdir -p "$dest/tiller"; echo keep >"$dest/tiller/keep"
run good "$dest"
check "directory destination exits nonzero" $([[ $status -ne 0 ]]; echo $?)
check "directory destination is not nested into" $([[ ! -e $dest/tiller/tiller ]]; echo $?)
check "directory destination stays a directory with its contents" $([[ -d $dest/tiller && -f $dest/tiller/keep ]]; echo $?)
check "directory destination does not claim success" $(! grep -q '^tiller-install: installed' <<<"$out"; echo $?)

# A checksum-valid candidate that cannot execute is refused.
for f in broken noexec; do
  dest="$TMP/d4-$f"; mkdir -p "$dest"; printf '#!/bin/sh\necho old\n' >"$dest/tiller"; chmod 0755 "$dest/tiller"
  run $f "$dest"
  check "$f candidate exits nonzero" $([[ $status -ne 0 ]]; echo $?)
  check "$f candidate does not claim success" $(! grep -q '^tiller-install: installed' <<<"$out"; echo $?)
  check "$f candidate leaves the existing executable in place" $("$dest/tiller" | grep -q old; echo $?)
done

[[ $failures -eq 0 ]] || { echo "$failures installer check(s) failed" >&2; exit 1; }
echo "install.sh: ok"
