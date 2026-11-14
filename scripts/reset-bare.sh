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
# Safety: before anything is built, installed, or deleted, it refuses a run path
# that is, or contains, the tiller source, the design-input source, or $HOME
# (which covers the filesystem root), and a run path inside the tiller source or
# the design-input source. Paths are compared in their resolved form, so a
# symlink to any of those is refused the same way. No sentinel and no
# TILLER_FORCE=1 lifts these refusals.
#
# Separately, it only deletes a run repo it can call its own: one carrying the
# .tiller-test-run sentinel it writes, or one holding nothing but harness
# artifacts. TILLER_FORCE=1 skips that ownership check and nothing else.
#
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

# shellcheck source=scripts/path-guards.sh
. "$SCRIPT_DIR/path-guards.sh"

# guard_run_repo refuses a run-repo path whose deletion would take a protected
# directory, or part of one, with it. It runs before the ownership check and
# neither the sentinel nor TILLER_FORCE=1 reaches it.
guard_run_repo() {
  local run="$1" c_run name c_protected
  c_run="$(canonical_path "$run")" || {
    echo "refusing: cannot resolve the run repo path ($run)" >&2; exit 1
  }
  if [ "$c_run" = "/" ]; then
    echo "refusing: run repo path is the filesystem root" >&2; exit 1
  fi
  for name in TILLER_SRC DESIGN_SRC HOME; do
    c_protected="$(canonical_path "${!name}")" || {
      echo "refusing: cannot resolve the protected directory $name (${!name})" >&2; exit 1
    }
    if path_within "$c_protected" "$c_run"; then
      echo "refusing: run repo path ($c_run) is or contains a protected directory ($name: $c_protected)" >&2; exit 1
    fi
    # The default run repo lives under $HOME, so only the two source trees
    # refuse a run path inside them.
    if [ "$name" != HOME ] && path_within "$c_run" "$c_protected"; then
      echo "refusing: run repo path ($c_run) is inside a protected directory ($name: $c_protected)" >&2; exit 1
    fi
  done
}

guard_run_repo "$RUN"

# Refuse to delete a directory that holds content this script did not create.
# Reset is allowed when the run repo carries our sentinel, OR it contains only
# disposable harness artifacts (a stale .git/.tiller/.bin and nothing precious,
# which covers a lost sentinel), OR TILLER_FORCE=1 is set. TILLER_FORCE=1 skips
# only this ownership check: guard_run_repo above has already refused any path
# that would reach $HOME, /, the tiller source, or the design inputs.
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
      echo "  remove it manually, or re-run with TILLER_FORCE=1 to reset it anyway" >&2
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
