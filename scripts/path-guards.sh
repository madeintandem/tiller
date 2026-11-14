#!/usr/bin/env bash
# Path checks shared by the scripts that create or delete a workspace
# (reset-bare.sh, compaction-acceptance.sh). Source this file; it defines
# functions and runs nothing.

# canonical_path prints the absolute, symlink-free form of a path. The deepest
# directory that exists is resolved with pwd -P and the components that do not
# exist yet are appended as written. It fails on an empty path, on a "." or
# ".." component below the last existing directory, and on a directory it
# cannot enter. A caller treats failure as a refusal.
#
# The result keeps the letter case it was given, so two canonical paths can
# differ as strings and still name one directory on a filesystem that ignores
# case. Compare them with path_within, never with = alone.
canonical_path() {
  local path=$1 missing="" leaf resolved
  [ -n "$path" ] || return 1
  case $path in /*) ;; *) path="$PWD/$path" ;; esac
  while [ ! -d "$path" ]; do
    while [ "${path%/}" != "$path" ]; do path=${path%/}; done
    leaf=${path##*/}
    case $leaf in . | ..) return 1 ;; esac
    missing="$leaf${missing:+/$missing}"
    path=${path%/*}
    [ -n "$path" ] || path=/
  done
  resolved=$(cd "$path" && pwd -P) || return 1
  if [ -z "$missing" ]; then
    printf '%s\n' "$resolved"
  else
    printf '%s/%s\n' "${resolved%/}" "$missing"
  fi
}

# path_within succeeds when <path> is <dir> or sits beneath it. Both come from
# canonical_path. It walks up from <path> one whole component at a time, so
# /a/run-2 is not within /a/run, and at each step asks whether this is <dir>:
# the same string, or (-ef) the same directory on disk, which is what catches
# another letter case of an existing directory.
path_within() {
  local path=$1 dir=$2
  while :; do
    if [ "$path" = "$dir" ] || [ "$path" -ef "$dir" ]; then return 0; fi
    [ "$path" != / ] || return 1
    path=${path%/*}
    [ -n "$path" ] || path=/
  done
}
