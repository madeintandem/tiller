#!/usr/bin/env bash
# compaction-acceptance.sh must never delete or write into anything it did not
# create. It refuses a work directory that is, contains, or sits inside the
# source repository (also through a symlink) and leaves everything as it was;
# on a good run it works in a directory of its own and leaves the source and
# whatever else the work directory already held untouched.
#
# A fake tiller stands in for the binary (its `serve` is a few lines of
# python3 on a loopback port), and every source, work directory, and temp
# directory sits under one mktemp -d. No real project, binary, or network.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
SCRIPT="$ROOT/scripts/compaction-acceptance.sh"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/tiller-compaction-acceptance.XXXXXX")
TMP=$(cd "$TMP" && pwd -P)
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/bin" "$TMP/tmp"
cat >"$TMP/bin/serve.py" <<'PY'
import http.server, json, sys

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({"route": self.path}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass

http.server.HTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
PY
# Fake tiller: `upgrade` moves the two old events into one segment, the reads
# answer fixed JSON, and `serve --port N` answers every route.
cat >"$TMP/bin/tiller" <<'TILLER'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  upgrade)
    printf '{"id":"e1"}\n{"id":"e2"}\n' >.tiller/events/2026-01.jsonl
    rm .tiller/events/e1.yml .tiller/events/e2.yml
    echo "upgraded"
    ;;
  status | next) echo '{"ok":true}' ;;
  ls) echo '{"nodes":[{"id":"T-aaaaaa"},{"id":"F-bbbbbb"}]}' ;;
  readout) printf 'committed at `abc123`\nthe readout\n' ;;
  serve) exec python3 "$(dirname "$0")/serve.py" "$3" ;;
  *) echo "fake tiller: unexpected command: $*" >&2; exit 2 ;;
esac
TILLER
chmod +x "$TMP/bin/tiller"

# The script commits inside its clone; give git an identity and no user config.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com

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

# source_repo <dir>: a committed project with three loose events.
source_repo() {
  mkdir -p "$1/.tiller/events"
  git -C "$1" init -q -b main
  printf 'project:\n  integration_branch: main\n' >"$1/.tiller/tiller.yml"
  local e
  for e in e1 e2 e3; do printf 'id: %s\n' "$e" >"$1/.tiller/events/$e.yml"; done
  git -C "$1" add -A
  git -C "$1" commit -qm "fixture"
}

# acceptance <source> [work-dir]: runs the script; sets status and $TMP/out.
acceptance() {
  status=0
  TMPDIR="$TMP/tmp" "$SCRIPT" "$TMP/bin/tiller" "$@" >"$TMP/out" 2>&1 || status=$?
}

failures=0
flunk() {
  echo "FAIL: $1" >&2
  sed 's/^/  /' "$TMP/out" >&2
  failures=$((failures + 1))
}

# refuses <label> <case-root> <source> <work-dir>: the script must exit
# non-zero with a "refusing:" line and leave the whole case tree byte-for-byte
# as it was.
refuses() {
  local label=$1 root=$2 before after
  before=$(manifest "$root")
  acceptance "$3" "$4"
  after=$(manifest "$root")
  if [ "$status" -eq 0 ] || ! grep -q '^refusing:' "$TMP/out"; then
    flunk "$label: expected a refusal, exit $status"
  elif [ "$before" != "$after" ]; then
    flunk "$label: the refusal changed the fixture"
    diff <(printf '%s\n' "$before") <(printf '%s\n' "$after") | sed 's/^/  /' >&2 || true
  fi
}

# The reported scenario: the source is the fixed child the script used to delete.
c="$TMP/collision"
source_repo "$c/demo/clone"
refuses "the source is <work>/clone" "$c" "$c/demo/clone" "$c/demo"

c="$TMP/contains"
source_repo "$c/area/repo"
refuses "the work directory contains the source" "$c" "$c/area/repo" "$c/area"

c="$TMP/same"
source_repo "$c/repo"
refuses "the work directory is the source" "$c" "$c/repo" "$c/repo"

c="$TMP/inside"
source_repo "$c/repo"
mkdir "$c/repo/clone"
printf 'part of the source\n' >"$c/repo/clone/keep.txt"
refuses "the work directory is inside the source" "$c" "$c/repo" "$c/repo/.tiller"
refuses "the work directory is a new path inside the source" "$c" "$c/repo" "$c/repo/scratch/work"

c="$TMP/alias"
source_repo "$c/demo/clone"
ln -s "$c/demo" "$c/work link"
ln -s "$c/demo/clone" "$c/source link"
refuses "the work directory is a symlink to a directory containing the source" "$c" "$c/demo/clone" "$c/work link"
refuses "the source is given through a symlink" "$c" "$c/source link" "$c/demo"
refuses "a new work path under a symlink into the source" "$c" "$c/demo/clone" "$c/source link/scratch"

# On a filesystem that ignores letter case (the macOS default), another
# spelling of a path is the same directory.
c="$TMP/letter-case"
source_repo "$c/demo/clone"
if [ -d "$c/DEMO" ]; then
  refuses "the work directory contains the source in another letter case" "$c" "$c/demo/clone" "$c/DEMO"
  refuses "the work directory is the source in another letter case" "$c" "$c/demo/clone" "$c/demo/CLONE"
else
  echo "skipped: letter-case aliases (this filesystem is case-sensitive)"
fi

# A valid run in a work directory that already holds unrelated directories with
# the names the script uses. Paths carry spaces, and the work directory shares
# a name prefix with the source beside it.
c="$TMP/valid"
src="$c/my repo"
work="$c/my repo work"
source_repo "$src"
for d in clone A B C; do
  mkdir -p "$work/$d"
  printf 'unrelated %s\n' "$d" >"$work/$d/keep.txt"
done
src_before=$(manifest "$src")
work_before=$(manifest "$work")
acceptance "$src" "$work"
created=$(find "$work" -mindepth 1 -maxdepth 1 -name 'compaction-acceptance.*' | wc -l | tr -d ' ')
if [ "$status" -ne 0 ] || ! grep -q '^PASS' "$TMP/out"; then
  flunk "a valid work directory: expected PASS, exit $status"
elif [ "$(manifest "$src")" != "$src_before" ]; then
  flunk "a valid run changed the source"
elif [ "$created" != 1 ]; then
  flunk "a valid run left $created directories of its own in the work directory, want 1"
else
  # Without the run's own directory the work directory is exactly as it was.
  rm -rf "$work"/compaction-acceptance.*
  [ "$(manifest "$work")" = "$work_before" ] || flunk "a valid run changed what the work directory already held"
fi

# The default: no work directory given, so the run goes under TMPDIR.
c="$TMP/default"
source_repo "$c/repo"
src_before=$(manifest "$c/repo")
acceptance "$c/repo"
if [ "$status" -ne 0 ] || ! grep -q '^PASS' "$TMP/out"; then
  flunk "the default work directory: expected PASS, exit $status"
elif [ "$(manifest "$c/repo")" != "$src_before" ]; then
  flunk "a default run changed the source"
fi

if [ "$failures" -ne 0 ]; then
  echo "$failures compaction-acceptance.sh case(s) failed" >&2
  exit 1
fi
echo "compaction-acceptance.sh: every case passed"
