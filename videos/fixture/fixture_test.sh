#!/usr/bin/env bash
# The brownfield Acme fixture records real stage snapshots and marks: every
# line is JSON, every stage holds the Tiller steps its video shows, every
# command but a declared expected failure exits 0, and the final mark is a
# repo where the feature is done.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/tiller-video-fixture.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

go build -o "$TMP/bin/tiller" "$ROOT/cmd/tiller"
# A relative --out resolves against the caller's directory, not the fixture's.
(cd "$TMP" && TILLER="$TMP/bin/tiller" bash "$ROOT/videos/fixture/run.sh" --out out) >"$TMP/run.log" 2>&1 ||
  { tail -40 "$TMP/run.log" >&2; fail "run.sh exited non-zero"; }
[[ -d "$TMP/out/snapshots" && -d "$TMP/out/marks" ]] ||
  fail "run.sh --out out did not write under the caller's directory"

for stage in s1 s2 s3; do
  snap="$TMP/out/snapshots/$stage.jsonl"
  [[ -s "$snap" ]] || fail "$stage.jsonl missing or empty"
  n=0
  while IFS= read -r line; do
    n=$((n + 1))
    jq -e . >/dev/null 2>&1 <<<"$line" || fail "$stage.jsonl line $n is not JSON"
  done <"$snap"
done

# has_cmd <stage> <substring>: some cmd step's command line contains substring.
has_cmd() {
  jq -e --arg s "$2" 'select(.kind == "cmd" and (.cmd | contains($s)))' \
    "$TMP/out/snapshots/$1.jsonl" >/dev/null || fail "$1.jsonl has no step running '$2'"
}
has_cmd s1 "tiller init"
has_cmd s1 "--kind brownfield"
has_cmd s1 "tiller add contributor"
has_cmd s1 "tiller join"
has_cmd s1 "tiller readout derive-baseline"
has_cmd s1 "tiller approve design"
has_cmd s2 "tiller approve epic"
has_cmd s2 "tiller approve feature"
has_cmd s3 "tiller check task"
has_cmd s3 "tiller submit task"
has_cmd s3 "tiller sync"
has_cmd s3 "tiller check feature"

bad=$(cat "$TMP"/out/snapshots/s*.jsonl |
  jq -r 'select(.kind == "cmd") | select((.id | endswith("-expect-fail")) != (.exit != 0)) | "\(.id) exit \(.exit)"')
[[ -z "$bad" ]] || fail "steps with an unexpected exit status: $bad"

ids=$(cat "$TMP"/out/snapshots/s*.jsonl | jq -r .id | sort | uniq -d)
[[ -z "$ids" ]] || fail "duplicate step ids: $ids"

for mark in s1-end s2-end s3-review s3-end tour; do
  [[ -s "$TMP/out/marks/$mark.tar.gz" ]] || fail "mark $mark missing"
done

mkdir "$TMP/s3-end"
tar -xzf "$TMP/out/marks/s3-end.tar.gz" -C "$TMP/s3-end"
status=$(cd "$TMP/s3-end" && "$TMP/bin/tiller" status feature orders/status-filter)
grep -q "state: done" <<<"$status" || fail "feature is not done at s3-end: $status"

echo "PASS: fixture snapshots and marks"
