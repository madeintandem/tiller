#!/usr/bin/env bash
# The fixture builder may replace only a directory it previously created.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/tiller-acme-guard.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

protected="$TMP/protected"
mkdir -p "$protected"
printf 'keep me\n' >"$protected/marker.txt"

set +e
TILLER=/bin/false "$ROOT/site/screenshots/acme.sh" "$protected" >"$TMP/output" 2>&1
status=$?
set -e

if [[ $status -eq 0 ]]; then
  echo "acme fixture unexpectedly accepted a protected directory" >&2
  exit 1
fi
if [[ ! -f "$protected/marker.txt" ]]; then
  echo "acme fixture deleted a directory it did not create" >&2
  exit 1
fi
if ! grep -q "not a tiller-owned screenshot fixture" "$TMP/output"; then
  echo "acme fixture refusal did not explain its ownership guard" >&2
  exit 1
fi

sentinel_value="tiller-owned Acme screenshot fixture"
sentinel_name=".tiller-screenshot-fixture"

symlinked="$TMP/symlinked"
mkdir -p "$symlinked"
printf 'keep me\n' >"$symlinked/marker.txt"
printf '%s\n' "$sentinel_value" >"$TMP/sentinel-target"
ln -s "$TMP/sentinel-target" "$symlinked/$sentinel_name"
set +e
TILLER=/bin/false "$ROOT/site/screenshots/acme.sh" "$symlinked" >"$TMP/symlink-output" 2>&1
status=$?
set -e
if [[ $status -eq 0 ]] || [[ ! -f "$symlinked/marker.txt" ]]; then
  echo "acme fixture accepted a symlink sentinel" >&2
  exit 1
fi

inexact="$TMP/inexact"
mkdir -p "$inexact"
printf 'keep me\n' >"$inexact/marker.txt"
printf '%s\n\n' "$sentinel_value" >"$inexact/$sentinel_name"
set +e
TILLER=/bin/false "$ROOT/site/screenshots/acme.sh" "$inexact" >"$TMP/inexact-output" 2>&1
status=$?
set -e
if [[ $status -eq 0 ]] || [[ ! -f "$inexact/marker.txt" ]]; then
  echo "acme fixture accepted an inexact sentinel" >&2
  exit 1
fi

# Source commits are required fixture state; their failures must propagate.
REAL_TILLER="$TMP/tiller-real"
go build -o "$REAL_TILLER" "$ROOT/cmd/tiller"
REAL_GIT=$(command -v git)
mkdir -p "$TMP/git-fail-bin"
cat >"$TMP/git-fail-bin/git" <<'EOF'
#!/usr/bin/env bash
if [[ "${1-}" == "commit" ]] && [[ "$*" == *"shelves: two bugs, two chores"* ]]; then
  exit 24
fi
exec "$REAL_GIT" "$@"
EOF
chmod +x "$TMP/git-fail-bin/git"

set +e
PATH="$TMP/git-fail-bin:$PATH" REAL_GIT="$REAL_GIT" TILLER="$REAL_TILLER" \
  "$ROOT/site/screenshots/acme.sh" "$TMP/commit-failure" >"$TMP/commit-output" 2>&1
status=$?
set -e
if [[ $status -ne 24 ]]; then
  echo "acme fixture did not propagate source commit failure (exit $status)" >&2
  exit 1
fi

