#!/usr/bin/env sh
# check-version-stamp.sh refuses a release whose tag does not match the version
# stamped in the source. `tiller --version` reports the const in
# internal/cli/root.go, set by the `Release: stamp vX.Y.Z` commit; GoReleaser
# runs this before building so a tag can never ship a binary that names a
# different version. Usage: check-version-stamp.sh v0.9.4
set -eu
tag="${1:-${GORELEASER_CURRENT_TAG:-}}"
[ -n "$tag" ] || { echo "check-version-stamp: no tag given" >&2; exit 2; }
want="${tag#v}"
have="$(sed -n 's/^const version = "\(.*\)"$/\1/p' "$(dirname "$0")/../internal/cli/root.go")"
if [ "$have" != "$want" ]; then
  echo "check-version-stamp: tag $tag but internal/cli/root.go stamps version \"$have\"; run the Release: stamp commit first" >&2
  exit 1
fi
echo "check-version-stamp: $tag matches the stamped version"
