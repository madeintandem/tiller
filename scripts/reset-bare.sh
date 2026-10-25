#!/usr/bin/env bash
set -euo pipefail

# reset-bare.sh sets up the FROM-SCRATCH onboarding test. It installs the tiller
# binary onto the PATH (simulating an install like `brew install tiller`) and
# resets the run repo to a BARE, empty git repo with NO .tiller/ at all, so the
# whole Tiller onboarding (tiller init, bringing in design inputs, decompose) can
# be walked through by hand as a brand-new user would. Design inputs live
# elsewhere (DESIGN_SRC) and are brought in during the walkthrough, not by this
# script.
#
# Safety: it refuses a run path that resolves to the
# tiller source, the design-input source, $HOME, or the filesystem root, and never
# deletes a directory it did not create (a .tiller-test-run sentinel proves it).
# It builds the binary before any destructive step, so a build failure leaves the
# existing repo untouched.
#
# Configuration (all overridable by env):
#   TILLER_TEST_RUN  the bare run repo        (default: $HOME/dev/tiller-test-run)
#   TILLER_INSTALL   where to install tiller  (default: $HOME/.local/bin/tiller)
#   DESIGN_SRC       design inputs (read-only) (default: $HOME/dev/design-inputs)

RUN="${TILLER_TEST_RUN:-$HOME/dev/tiller-test-run}"
INSTALL="${TILLER_INSTALL:-$HOME/.local/bin/tiller}"
DESIGN_SRC="${DESIGN_SRC:-$HOME/dev/design-inputs}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
TILLER_SRC="$(cd "$SCRIPT_DIR/.." && pwd -P)"
SENTINEL=".tiller-test-run"

# canon resolves a path to an absolute, symlink-free form, canonicalizing the
# existing parent and re-appending the leaf when the leaf does not exist yet.
canon() {
  local p="$1" dir base
  if [ -d "$p" ]; then ( cd "$p" && pwd -P ); return; fi
  dir="$(dirname "$p")"; base="$(basename "$p")"
  if [ -d "$dir" ]; then printf '%s/%s\n' "$(cd "$dir" && pwd -P)" "$base"; else printf '%s\n' "$p"; fi
}

# guard_run_repo refuses a run-repo path that would put a destructive operation
# onto a protected directory.
guard_run_repo() {
  local run="$1" c_run forbidden
  c_run="$(canon "$run")"
  if [ -z "$c_run" ] || [ "$c_run" = "/" ]; then
    echo "refusing: run repo path is empty or the filesystem root ($c_run)" >&2; exit 1
  fi
  for forbidden in "$(canon "$TILLER_SRC")" "$(canon "$DESIGN_SRC")" "$(canon "$HOME")"; do
    if [ "$c_run" = "$forbidden" ]; then
      echo "refusing: run repo path ($c_run) resolves to a protected directory ($forbidden)" >&2; exit 1
    fi
  done
}

guard_run_repo "$RUN"

# Refuse to delete a directory that holds content this script did not create.
# Reset is allowed when the run repo carries our sentinel, OR it contains only
# disposable harness artifacts (a stale .git/.tiller/.bin and nothing precious,
# which covers a lost sentinel), OR TILLER_FORCE=1 is set. The path guards above
# always apply regardless, so $HOME, /, and the source repos are never at risk.
if [ -e "$RUN" ]; then
  [ -d "$RUN" ] || { echo "refusing: $RUN exists and is not a directory" >&2; exit 1; }
  if [ -f "$RUN/$SENTINEL" ]; then
    :
  elif [ "${TILLER_FORCE:-}" = "1" ]; then
    echo "warning: $RUN has no $SENTINEL sentinel; resetting anyway (TILLER_FORCE=1)" >&2
  else
    unexpected="$(ls -A "$RUN" 2>/dev/null | grep -vxE '\.git|\.tiller|\.bin|\.gitignore|\.tiller-test-run' || true)"
    if [ -n "$unexpected" ]; then
      echo "refusing: $RUN has no $SENTINEL sentinel and holds content this script did not create:" >&2
      echo "$unexpected" | sed 's/^/    /' >&2
      echo "  remove it manually, or re-run with TILLER_FORCE=1 (the path guards still protect \$HOME, /, and the source repos)" >&2
      exit 1
    fi
  fi
fi

# Build and install the binary BEFORE any destructive step. Stage it, then move
# it into place only after a good build, so a failure leaves everything untouched.
STAGE_DIR="$(mktemp -d)"
trap 'rm -rf "$STAGE_DIR"' EXIT
if ! command -v go >/dev/null 2>&1; then
  echo "refusing: 'go' is not on PATH, cannot build the tiller binary; nothing changed" >&2; exit 1
fi
( cd "$TILLER_SRC" && go build -o "$STAGE_DIR/tiller" ./cmd/tiller )
mkdir -p "$(dirname "$INSTALL")"
mv "$STAGE_DIR/tiller" "$INSTALL"
echo "installed tiller -> $INSTALL"

# Reset the run repo to a BARE, empty git repo. No tiller init, no .tiller/, no
# design seed: this is a brand-new project the way a founder would start it.
rm -rf "$RUN"
mkdir -p "$RUN"
touch "$RUN/$SENTINEL"
git -C "$RUN" init -b main -q
git -C "$RUN" config user.email "tiller-test@example.com"
git -C "$RUN" config user.name "Tiller Test Run"
git -C "$RUN" config commit.gpgsign false
# Keep the safety sentinel out of the user's working view.
printf '%s\n' "$SENTINEL" > "$RUN/.git/info/exclude"

PATH_NOTE="on PATH"
case ":$PATH:" in *":$(dirname "$INSTALL"):"*) ;; *) PATH_NOTE="NOT on PATH; add $(dirname "$INSTALL") to PATH";; esac

echo
echo "ready: BARE run repo at $RUN (no .tiller/, a founder's blank slate)"
echo "  tiller:  $INSTALL  ($PATH_NOTE)"
echo "  design inputs available (read-only) at: $DESIGN_SRC/docs"
echo "  next:    cd \"$RUN\"  and start from a bare shell"
