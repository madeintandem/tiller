#!/usr/bin/env bash
# Prove that compacting event history changes no read.
#
# Usage: scripts/compaction-acceptance.sh <tiller-binary> <source-repo> [work-dir]
#
# Clones the source (never touching it), upgrades the clone to the current
# schema, then captures the same reads three ways and diffs them:
#   A  compacted: loose files older than the delay are gone, segments exist
#   B  loose: the upgraded tree rebuilt with no segments at all
#   C  both: segments restored beside the loose files
# Any difference is a failure. The work directory defaults to a mktemp -d
# and must never sit inside either repository.
set -euo pipefail

if [ $# -lt 2 ]; then
  echo "usage: $0 <tiller-binary> <source-repo> [work-dir]" >&2
  exit 2
fi
bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
src=$(cd "$2" && pwd)
work=${3:-$(mktemp -d)}
mkdir -p "$work"
work=$(cd "$work" && pwd)
contributor=${CONTRIBUTOR:-jc}
clone="$work/clone"
serve_pid=""

cleanup() {
  if [ -n "$serve_pid" ]; then kill "$serve_pid" 2>/dev/null || true; fi
}
trap cleanup EXIT

# Every JSON capture is sorted so key order is not a difference. The only
# clock-derived fields are stamped with the time of the read: as_of at the top
# of the report payloads and through at the top of the recap. Each is removed
# from its own capture only; nothing else is touched.
normalize() { jq -S '.'; }
normalize_route() {
  case $1 in
    report | report/usage) jq -S 'del(.as_of)' ;;
    recap) jq -S 'del(.through)' ;;
    *) normalize ;;
  esac
}
# The readout's first line names the commit it read, and each set is its own
# commit, so the hash is the one harness-induced difference there.
normalize_readout() { sed '1s/committed at `[0-9a-f]*`/committed at `HASH`/'; }

rm -rf "$clone"
git clone --quiet --no-hardlinks "$src" "$clone"
cd "$clone"
for r in $(git remote); do git remote remove "$r"; done
if [ -n "$(git remote)" ]; then echo "clone still has a remote" >&2; exit 1; fi
integration=$(sed -n 's/^ *integration_branch: *"\{0,1\}\([^" #]*\).*/\1/p' .tiller/tiller.yml | head -1)
git checkout --quiet "$integration"

count_loose() { find .tiller/events -name '*.yml' | wc -l | tr -d ' '; }
count_segments() { find .tiller/events -name '*.jsonl' | wc -l | tr -d ' '; }
fail() { echo "FAIL: $*" >&2; exit 1; }
loose_before=$(count_loose)
echo "event files before: $loose_before loose, $(count_segments) segments"

start=$(date +%s)
"$bin" upgrade --project-only --contributor "$contributor" | tee "$work/upgrade.out"
end=$(date +%s)
echo "upgrade wall time: $((end - start))s"
git add -A
git commit --quiet -m "upgrade to the current schema" --no-gpg-sign
compacted=$(git rev-parse HEAD)
echo "event files after: $(count_loose) loose, $(count_segments) segments"
# Each set must hold the form it claims, or the diffs below pass vacuously.
[ "$(count_segments)" -gt 0 ] || fail "set A holds no segment; the upgrade compacted nothing"

free_port() {
  python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'
}

capture() {
  local out=$1
  mkdir -p "$out"
  "$bin" status --json | normalize >"$out/status.json"
  "$bin" ls tasks --json | normalize >"$out/ls-tasks.json"
  "$bin" ls features --json | normalize >"$out/ls-features.json"
  "$bin" next --json | normalize >"$out/next.json"
  "$bin" readout execute | normalize_readout >"$out/readout-execute.txt"

  local port
  port=$(free_port)
  "$bin" serve --port "$port" --open=false >"$work/serve-$(basename "$out").log" 2>&1 &
  serve_pid=$!
  local tries=0
  until curl --fail --silent "http://127.0.0.1:$port/api/version" >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [ "$tries" -gt 100 ]; then echo "serve did not answer" >&2; exit 1; fi
    sleep 0.2
  done
  local route
  for route in recap report report/usage events board build tree work-items queue; do
    curl --fail --silent --show-error "http://127.0.0.1:$port/api/$route" \
      | normalize_route "$route" >"$out/api-${route//\//-}.json"
  done
  local id
  "$bin" ls tasks --json | jq -r '.. | objects | .id? // empty | select(startswith("T-"))' \
    | sort -u | awk 'NR % 20 == 1' >"$out/node-ids.txt"
  [ -s "$out/node-ids.txt" ] || fail "no task ids to capture node pages for in $out"
  while read -r id; do
    curl --fail --silent --show-error "http://127.0.0.1:$port/api/node/task/$id" \
      | normalize >"$out/node-$id.json"
  done <"$out/node-ids.txt"
  echo "captured $(wc -l <"$out/node-ids.txt" | tr -d ' ') node pages in $out"
  kill "$serve_pid" 2>/dev/null || true
  wait "$serve_pid" 2>/dev/null || true
  serve_pid=""
  local empty
  empty=$(find "$out" -type f -size 0)
  [ -z "$empty" ] || fail "empty capture files: $empty"
}

rm -rf "$work/A" "$work/B" "$work/C"
capture "$work/A"

# B: the loose form of the upgraded tree, with no segment anywhere.
git checkout --quiet HEAD~1 -- .tiller/events
find .tiller/events -name '*.jsonl' -delete
git add -A
git commit --quiet -m "loose form" --no-gpg-sign
echo "set B tree: $(count_loose) loose, $(count_segments) segments"
[ "$(count_segments)" -eq 0 ] || fail "set B still holds segments"
loose_b=$(count_loose)
capture "$work/B"

# C: segments restored beside the loose files, every old event in both forms.
git checkout --quiet "$compacted" -- '.tiller/events/*.jsonl'
git add -A
git commit --quiet -m "both forms" --no-gpg-sign
echo "set C tree: $(count_loose) loose, $(count_segments) segments"
[ "$(count_segments)" -gt 0 ] || fail "set C holds no segment"
[ "$(count_loose)" -eq "$loose_b" ] || fail "set C lost loose files: $(count_loose), want $loose_b"
capture "$work/C"

status=0
diff -r "$work/A" "$work/B" >"$work/A-vs-B.diff" || status=1
diff -r "$work/A" "$work/C" >"$work/A-vs-C.diff" || status=1
echo "A vs B: $(wc -l <"$work/A-vs-B.diff" | tr -d ' ') diff lines"
echo "A vs C: $(wc -l <"$work/A-vs-C.diff" | tr -d ' ') diff lines"
if [ "$status" -ne 0 ]; then
  echo "FAIL: compaction changed a read; see $work/A-vs-B.diff and $work/A-vs-C.diff" >&2
  exit 1
fi
echo "PASS: no read answers differently because history was compacted"
