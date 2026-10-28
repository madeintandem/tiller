#!/usr/bin/env bash
# Carry an Acme fixture that acme.sh just built further along for the
# homepage's console shots: two more Foundation features (one merged, one half
# done), a written-out task to read in Explore, two features specced and
# waiting on their plan approval, and a task added after its plan gate. It
# leaves the checkout on the second owner's task branch, where acme.sh left it,
# with these commits merged in.
#
# usage: site/screenshots/homepage.sh [dir]    default /tmp/tiller-shots/acme
# env:   TILLER=<path to a tiller binary>       default: tiller on PATH
#
# Run it after acme.sh and after shooting the documentation images, since it
# changes what Home, the map, and Fix show.
set -euo pipefail
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)

DIR=${1:-/tmp/tiller-shots/acme}
TILLER=${TILLER:-tiller}
SENTINEL_VALUE="tiller-owned Acme screenshot fixture"
OWNER_BRANCH=tiller/form-engine/certification-form-loading/schema-transcription

if [[ ! -f "$DIR/.tiller-screenshot-fixture" ]] || [[ -L "$DIR/.tiller-screenshot-fixture" ]] ||
  ! cmp -s <(printf '%s\n' "$SENTINEL_VALUE") "$DIR/.tiller-screenshot-fixture"; then
  echo "refusing to change $DIR: not a tiller-owned screenshot fixture (run acme.sh first)" >&2
  exit 2
fi
cd "$DIR"
source "$HERE/lib.sh"

# One task's whole loop, from claim to a merge that sync records as done.
merge_task() { # <task key> <contributor>
  local key=$1 who=$2
  $TILLER claim task "$key" --contributor "$who" >/dev/null
  $TILLER start task "$key" --contributor "$who" >/dev/null
  git checkout -q -b "tiller/$key"
  echo "$key" >> work.txt
  git add -A && git commit -qm "$key: the work"
  $TILLER check task "$key" --contributor "$who" >/dev/null
  $TILLER submit task "$key" --contributor "$who" >/dev/null
  git add -A && git commit -qm "$key: review"
  git checkout -q main
  git merge -q --no-ff -m "Merge $key" "tiller/$key"
  $TILLER sync --contributor robin >/dev/null
  commit "sync: $key done"
}

git checkout -q main

# ---------- foundation: a feature merged and done ----------
$TILLER add feature foundation/dev-environment --title "Developer environment" --contributor robin --depends-on foundation/app-scaffold >/dev/null
$TILLER add task foundation/dev-environment/seed-data --title "Seed data for every role" --contributor robin >/dev/null
spec_feature foundation/dev-environment "Developer environment" "One command brings up the app with realistic seed data for every role."
$TILLER approve feature foundation/dev-environment --contributor robin >/dev/null
commit "foundation/dev-environment: plan approved"
merge_task foundation/dev-environment/seed-data dana
# Its only task is merged; the feature's own gates take it across.
$TILLER check feature foundation/dev-environment --contributor robin >/dev/null
$TILLER sync --contributor robin >/dev/null
commit "foundation/dev-environment: done"

# ---------- foundation: a feature half done, with a task worth reading ----------
$TILLER add feature foundation/error-tracking --title "Error tracking" --contributor robin --depends-on foundation/app-scaffold >/dev/null
$TILLER add task foundation/error-tracking/exception-capture --title "Exception capture" --contributor robin >/dev/null
$TILLER add task foundation/error-tracking/alert-routing --title "Alert routing" --contributor robin --depends-on foundation/error-tracking/exception-capture >/dev/null
cat > .tiller/epics/foundation/features/error-tracking/tasks/alert-routing/task.md <<'EOF'
# Alert routing

## Approach

Send each captured exception to whoever is on call for the part of the app it
came from. Areas come from the controller namespace, the on-call schedule
lives in `config/on_call.yml`, and anything without an owner goes to the
platform channel so nothing is dropped.

## Steps

1. Add `AlertRouter`, which takes a captured exception and returns a destination.
2. Map controller namespaces to areas in `config/alert_areas.yml`.
3. Deliver through the mailer for email destinations and the chat webhook for channels.
4. Hold repeats of the same fingerprint to one alert every 10 minutes.

## Done when

- An exception raised under `Admin::` reaches the admin on-call address.
- An exception with no owning area reaches the platform channel.
- A burst of 50 identical exceptions sends one alert.
EOF
spec_feature foundation/error-tracking "Error tracking" "Capture every unhandled exception with its request context and route alerts to whoever is on call."
$TILLER approve feature foundation/error-tracking --contributor robin >/dev/null
commit "foundation/error-tracking: plan approved"
merge_task foundation/error-tracking/exception-capture dana
$TILLER claim task foundation/error-tracking/alert-routing --contributor dana >/dev/null
$TILLER start task foundation/error-tracking/alert-routing --contributor dana >/dev/null
commit "foundation/error-tracking: alert routing in progress"

# ---------- two plans specced and waiting on approval ----------
$TILLER add task form-engine/form-renderer/field-layout --title "Field layout grid" --contributor robin >/dev/null
$TILLER add task form-engine/form-renderer/validation-messages --title "Inline validation messages" --contributor robin >/dev/null
$TILLER add task form-engine/form-renderer/conditional-sections --title "Conditional sections" --contributor robin >/dev/null
spec_feature form-engine/form-renderer "Form renderer" "Render any loaded certification form version as an accessible, multi-page form with inline validation."
commit "form-engine/form-renderer: specced"
$TILLER add task payments/invoicing/invoice-model --title "Invoice model and numbering" --contributor robin >/dev/null
$TILLER add task payments/invoicing/invoice-pdf --title "Invoice PDF" --contributor robin >/dev/null
spec_feature payments/invoicing "Invoicing" "Issue a numbered invoice for every paid enrollment, downloadable as a PDF from the employer's account."
commit "payments/invoicing: specced"

# ---------- a task added after the plan gate waits as proposed ----------
$TILLER add task foundation/auth-and-roles/password-reset --title "Password reset" --contributor robin >/dev/null
commit "foundation/auth-and-roles: password reset proposed"

git checkout -q "$OWNER_BRANCH"
git merge -q --no-edit main
echo "Acme carried on for the homepage shots at $DIR"
