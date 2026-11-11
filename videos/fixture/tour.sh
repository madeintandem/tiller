#!/usr/bin/env bash
# Build the project the console tour in video 1 is shot from, and save it as
# the `tour` mark. Acme Orders is empty at the end of video 1, so the tour
# shows a busier project instead: the Acme Platform stand-in the documentation
# site's screenshots use (site/screenshots/acme.sh, carried on by homepage.sh),
# plus what the tour names and the stand-in lacks: an answered question, a
# second open one, and two amendments (one waiting on approval, one applied).
#
# usage: videos/fixture/tour.sh <marks-dir>
# env:   TILLER=<path to a tiller binary>       default: tiller on PATH
set -euo pipefail
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
SHOTS=$(cd "$HERE/../../site/screenshots" && pwd -P)
MARKS=${1:?usage: tour.sh <marks-dir>}
mkdir -p "$MARKS"
MARKS=$(cd "$MARKS" && pwd -P)
export TILLER=${TILLER:-tiller}

WORK=$(mktemp -d "${TMPDIR:-/tmp}/acme-platform-tour.XXXXXX")
trap 'rm -rf "$WORK" 2>/dev/null || true' EXIT
# No background git housekeeping: it would still be writing when the trap cleans up.
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0
DIR="$WORK/acme-platform"

"$SHOTS/acme.sh" "$DIR" >/dev/null
"$SHOTS/homepage.sh" "$DIR" >/dev/null
cd "$DIR"
source "$SHOTS/lib.sh"
git checkout -q main

# ---------- questions: one answered, a second one open ----------
Q=$($TILLER add question "Do auditors sign in with the program's single sign-on, or with their own email and password?" \
  --title "Auditor sign-in" --blocks foundation/auth-and-roles --contributor robin --json | jq -r .data.id)
$TILLER answer question "$Q" --contributor robin \
  --answer "Their own email and password in v1. Single sign-on waits for the program's identity provider, which is not ready." >/dev/null
DUE=$(date -v+9d +%Y-%m-%d 2>/dev/null || date -d "+9 days" +%Y-%m-%d)
$TILLER add question "Does the annual report cover the calendar year or the program's fiscal year?" \
  --title "Report year" --blocks reporting/annual-report --due "$DUE" --contributor dana >/dev/null
commit "decisions: auditor sign-in answered, report year asked"

# ---------- amendments: one applied, one waiting on a person ----------
$TILLER add amendment sms-reminders --title "Reminders by text message as well as email" --contributor robin >/dev/null
$TILLER amend sms-reminders --add-feature notifications/sms-delivery --title "SMS delivery" --contributor robin >/dev/null
cat > .tiller/amendments/A-*-sms-reminders.md <<'EOF2'
# Reminders by text message as well as email

## Why

Technicians in the field miss renewal emails. The program asked for the same
reminders by text message, on the schedule the email reminders already keep.

## What changes

Notifications gains one feature, SMS delivery, beside Email delivery. The
reminder scheduler is untouched; it gains a second channel to send through.

## What to watch

Text messages cost money per send. The feature's spec needs a monthly cap.
EOF2
$TILLER approve amendment sms-reminders --contributor robin >/dev/null
commit "amendment: SMS delivery joins notifications"

$TILLER add amendment offline-visits --title "Auditors need site visits to work offline" --contributor dana >/dev/null
$TILLER amend offline-visits --add-feature audits/offline-capture --title "Offline visit capture" --contributor dana >/dev/null
$TILLER amend offline-visits --revise audits/site-visit --contributor dana >/dev/null
$TILLER amend offline-visits --add-question "How long may a device hold unsynced visit data before it must be wiped?" \
  --title "Offline data retention" --blocks audits/offline-capture --contributor dana >/dev/null
cat > .tiller/amendments/A-*-offline-visits.md <<'EOF2'
# Auditors need site visits to work offline

## Why

Two pilot audits were held in buildings with no signal. The auditors took
notes on paper and typed them in that night, which is the work this platform
exists to remove.

## What changes

The Audit Engine gains one feature, Offline visit capture, and On-site visit
scheduling is revised to hand a visit to the device before the auditor leaves.
One new question holds the new feature until it is answered.

## What to watch

Visit data sits on a device until it syncs. How long it may stay there is the
open question, and the answer shapes the storage design.
EOF2
commit "amendment: offline site visits proposed"

# The tour stays on main, so the Report page reads the integration branch and
# carries no "not main" banner. Then the history is spread over six weeks,
# because a project built in ten seconds draws a Report with nothing in it.
python3 "$HERE/backdate.py" "$DIR" 40 >/dev/null
commit "tour: history spread over six weeks"
COPYFILE_DISABLE=1 tar -czf "$MARKS/tour.tar.gz" -C "$DIR" .
echo "wrote the tour mark to $MARKS/tour.tar.gz"
