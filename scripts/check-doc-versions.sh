#!/usr/bin/env sh
# check-doc-versions.sh fails when a reader-facing doc that repeats the source
# version stamp does not name the current one, or still names the previous
# stamp somewhere. The stamp lives in internal/cli/root.go; these files must
# move in the same Release: stamp commit. Versions match whole, so 0.15.60 does
# not stand in for 0.15.6. The previous stamp comes from git history, so the
# check needs a checkout with enough history to reach the last stamp change.
#
# Usage: check-doc-versions.sh [file ...]  (repo-relative; defaults to the list below)
set -eu
root="$(cd "$(dirname "$0")/.." && pwd)"
stamp_file=internal/cli/root.go
stamp_of() { sed -n 's/^const version = "\(.*\)"$/\1/p'; }

have="$(stamp_of <"$root/$stamp_file")"
[ -n "$have" ] || { echo "check-doc-versions: no version stamp in $stamp_file" >&2; exit 2; }

# The previous stamp is the newest committed stamp that differs from the
# current one, so an uncommitted bump compares against the committed stamp.
previous=""
for commit in $(git -C "$root" log --format=%H -G '^const version = ' -- "$stamp_file" 2>/dev/null); do
  was="$(git -C "$root" show "$commit:$stamp_file" | stamp_of)"
  if [ -n "$was" ] && [ "$was" != "$have" ]; then
    previous="$was"
    break
  fi
done
if [ -z "$previous" ] && [ "$(git -C "$root" rev-parse --is-shallow-repository 2>/dev/null)" = true ]; then
  echo "check-doc-versions: shallow clone has no earlier stamp; fetch more history to check for stale mentions" >&2
  exit 2
fi

# names_version FILE VERSION succeeds when FILE names VERSION as a whole
# version: no digit or dot directly before it, and no further digit after it.
names_version() {
  pattern="$(printf '%s' "$2" | sed 's/\./\\./g')"
  grep -qE "(^|[^0-9.])$pattern(\.?[^0-9.]|\.?\$)" "$root/$1"
}

# names_bare_version is names_version without the vX.Y.Z form.
names_bare_version() {
  pattern="$(printf '%s' "$2" | sed 's/\./\\./g')"
  grep -qE "(^|[^0-9.v])$pattern(\.?[^0-9.]|\.?\$)" "$root/$1"
}

# A previous stamp that was tagged is the published release, so its vX.Y.Z
# form may name that release; a bare mention is still a stale stamp.
stale_previous=names_version
if [ -n "$previous" ] && git -C "$root" rev-parse -q --verify "refs/tags/v$previous" >/dev/null 2>&1; then
  stale_previous=names_bare_version
fi

if [ "$#" -eq 0 ]; then
  set -- README.md docs/plans/STATUS.md docs/releasing.md docs/spec/distribution.md \
    docs/spec/skill-set.md docs/tiller-commands.md site/astro.config.mjs \
    site/src/content/docs/quickstart.md site/src/content/docs/source-quickstart.md \
    site/src/content/docs/skills.md
fi

status=0
for file in "$@"; do
  if [ ! -f "$root/$file" ]; then
    echo "check-doc-versions: $file does not exist" >&2
    status=1
    continue
  fi
  if ! names_version "$file" "$have"; then
    echo "check-doc-versions: $file does not name the stamped version $have" >&2
    status=1
  fi
  if [ -n "$previous" ] && "$stale_previous" "$file" "$previous"; then
    echo "check-doc-versions: $file still names the previous stamp $previous" >&2
    status=1
  fi
done
[ "$status" -ne 0 ] || echo "check-doc-versions: every listed doc names $have and none names ${previous:-an earlier stamp}"
exit "$status"
