#!/usr/bin/env bash
# Build the brownfield Acme Orders fixture the how-to videos are recorded
# from: a fresh repo holding the existing app, taken through Tiller by three
# stage scripts that record every command's real output (snapshots) and save
# the whole checkout at named moments (marks).
#
# usage: videos/fixture/run.sh [--out DIR] [--check]
#   default   write videos/snapshots/s{1,2,3}.jsonl and videos/fixture/marks/*.tar.gz
#   --out     write DIR/snapshots/*.jsonl and DIR/marks/*.tar.gz instead
#   --check   write nothing; compare the fresh steps (ids, actors, commands,
#             notes, with node IDs masked) against the committed snapshots
# env: TILLER=<path to a tiller binary>   default: tiller on PATH
set -euo pipefail
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
VIDEOS=$(dirname "$HERE")

OUT="" CHECK=0
while [[ $# -gt 0 ]]; do
  case $1 in
    --out) OUT=${2:?--out needs a directory}; shift 2 ;;
    --check) CHECK=1; shift ;;
    *) echo "usage: run.sh [--out DIR] [--check]" >&2; exit 2 ;;
  esac
done

TILLER=$(command -v "${TILLER:-tiller}") || { echo "no tiller binary found" >&2; exit 2; }
WORK=$(mktemp -d "${TMPDIR:-/tmp}/acme-orders-fixture.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

# The fixture never reads the machine's git configuration or identity.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null GIT_CONFIG_NOSYSTEM=1
export NO_COLOR=1 TZ=UTC
export TILLER CONTENT="$HERE/content" APP="$HERE/app" MARKS="$WORK/out/marks"
mkdir -p "$WORK/out/snapshots" "$MARKS" "$WORK/acme-orders"
FIXTURE_VARS=()

source "$HERE/lib.sh"
cd "$WORK/acme-orders"
for stage in 1 2 3; do
  SNAP="$WORK/out/snapshots/s$stage.jsonl"
  : >"$SNAP"
  source "$HERE/stage$stage.sh"
done

# shape <file>: the parts of each step a script depends on, node IDs masked.
shape() {
  jq -c '{id, actor, kind, cmd, text} | with_entries(select(.value != null))
    | walk(if type == "string" then gsub("\\b[A-Z]{1,2}-[a-z0-9]{6}\\b"; "<ID>") else . end)' "$1"
}

if [[ $CHECK -eq 1 ]]; then
  status=0
  for stage in s1 s2 s3; do
    if ! diff <(shape "$VIDEOS/snapshots/$stage.jsonl") <(shape "$WORK/out/snapshots/$stage.jsonl"); then
      echo "$stage: steps differ from the committed snapshot" >&2
      status=1
    fi
  done
  [[ $status -eq 0 ]] && echo "fixture steps match the committed snapshots"
  exit $status
fi

if [[ -n "$OUT" ]]; then
  SNAPDIR="$OUT/snapshots" MARKDIR="$OUT/marks"
else
  SNAPDIR="$VIDEOS/snapshots" MARKDIR="$HERE/marks"
fi
mkdir -p "$SNAPDIR" "$MARKDIR"
rm -f "$MARKDIR"/*.tar.gz
cp "$WORK"/out/snapshots/*.jsonl "$SNAPDIR/"
cp "$WORK"/out/marks/*.tar.gz "$MARKDIR/"
echo "wrote $(ls "$SNAPDIR"/s?.jsonl | wc -l | tr -d ' ') snapshots to $SNAPDIR and $(ls "$MARKDIR"/*.tar.gz | wc -l | tr -d ' ') marks to $MARKDIR"
