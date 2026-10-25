#!/usr/bin/env bash
# Build the Acme Platform stand-in project the documentation screenshots are
# shot from: a greenfield project caught mid-flight, with one feature merged,
# two in motion under two owners, a plan approved and waiting to be claimed,
# an epic awaiting approval, an open question, and bugs, chores, and tweaks on the
# shelves. Everything is driven through the tiller CLI so the project is
# always a tree the current binary would write itself, and it ends checked
# out on the second owner's task branch, where her task is in review.
#
# usage: site/screenshots/acme.sh [dir]        default /tmp/tiller-shots/acme
# env:   TILLER=<path to a tiller binary>       default: tiller on PATH
#
# Acme is the neutral stand-in the specs and fixtures use for the platform
# build; nothing here names a real client.
set -euo pipefail

DIR=${1:-/tmp/tiller-shots/acme}
TILLER=${TILLER:-tiller}
SENTINEL=.tiller-screenshot-fixture
SENTINEL_VALUE="tiller-owned Acme screenshot fixture"

if [[ -e "$DIR" ]]; then
  if [[ ! -f "$DIR/$SENTINEL" ]] || [[ -L "$DIR/$SENTINEL" ]] ||
    ! cmp -s <(printf '%s\n' "$SENTINEL_VALUE") "$DIR/$SENTINEL"; then
    echo "refusing to replace $DIR: not a tiller-owned screenshot fixture" >&2
    exit 2
  fi
  rm -rf "$DIR"
fi
mkdir -p "$DIR"
printf '%s\n' "$SENTINEL_VALUE" >"$DIR/$SENTINEL"
cd "$DIR"
git init -q -b main
printf '/%s\n' "$SENTINEL" >>.git/info/exclude
git config user.email "docs@acme.example"
git config user.name "Acme Docs"

commit() { git add -A && git commit -qm "$1"; }

# A feature's specify-stage output: the specification, the acceptance
# contract, and one task.md per task already added under it.
spec_feature() { # <epic/feature> "<Title>" "<summary sentence>"
  local key=$1 title=$2 summary=$3
  local dir=".tiller/epics/${key%%/*}/features/${key##*/}"
  local id
  id=$($TILLER show feature "$key" --json | grep -om1 'F-[a-z0-9]\{6\}')
  cat > "$dir/feature.md" <<EOF
# $title

## Summary

$summary

## Specification

The feature lands behind the existing application shell and reuses the
project's own conventions for models, routes, and views. Every user-facing
state is reachable from the primary navigation, and every write is covered
by a request spec.
EOF
  cat > "$dir/feature.eval.md" <<EOF
---
node: $id
criteria:
  - id: C1
    statement: The feature's primary route renders and its request specs pass
    polarity: introduce
    binding: { type: command, run: "true" }
---

# Acceptance

The acceptance contract binds to the project's test command; the criterion
above is satisfied when the route renders and its specs are green.
EOF
  for task in "$dir"/tasks/*/; do
    [ -d "$task" ] || continue
    local name
    name=$(basename "$task")
    cat > "$task/task.md" <<EOF
# ${name//-/ }

## Approach

Implement the slice named in the title, prove it with a request spec, and
leave the build green.
EOF
  done
  $TILLER spec feature "$key" --contributor jc >/dev/null
}

# ---------- project, contributors, design inputs ----------
$TILLER init --name "Acme Platform" --key acme --kind greenfield >/dev/null
$TILLER add contributor jc --name "JC Grubbs" >/dev/null
$TILLER add contributor dana --name "Dana Whitfield" >/dev/null
$TILLER join --as jc >/dev/null
cat > .tiller/design/00-index.md <<'EOF'
# Acme Platform: design inputs

Acme runs a certification program: technicians train and test, employers
enroll and renew, projects are submitted for audit, and the program reports
on all of it annually. The platform replaces a set of spreadsheets and PDF
forms with one Rails application.

- 01-foundation.md: the application skeleton, authentication, and roles
- 02-forms.md: the NRCA form engine and its renderer
- 03-payments.md: Stripe checkout and invoicing
- 04-notifications.md: email delivery and reminders
- 05-audits.md: the audit engine and the auditor review flow
- 06-reporting.md: the annual report and CSV export
EOF
commit "init: Acme Platform on tiller"

# ---------- decompose: six epics, their features, the cross-epic edges ----------
$TILLER add epic foundation --title "Foundation" --contributor jc >/dev/null
$TILLER add feature foundation/app-scaffold --title "App scaffold" --contributor jc >/dev/null
$TILLER add feature foundation/auth-and-roles --title "Authentication and roles" --contributor jc --depends-on foundation/app-scaffold >/dev/null

$TILLER add epic form-engine --title "Form Engine" --contributor jc >/dev/null
$TILLER add feature form-engine/nrca-form-loading --title "NRCA form loading" --contributor jc --depends-on foundation/app-scaffold >/dev/null
$TILLER add feature form-engine/form-renderer --title "Form renderer" --contributor jc --depends-on form-engine/nrca-form-loading >/dev/null

$TILLER add epic payments --title "Payments" --contributor jc --depends-on foundation >/dev/null
$TILLER add feature payments/stripe-checkout --title "Stripe checkout" --contributor jc --depends-on foundation/auth-and-roles >/dev/null
$TILLER add feature payments/invoicing --title "Invoicing" --contributor jc --depends-on payments/stripe-checkout >/dev/null

$TILLER add epic notifications --title "Notifications" --contributor jc --depends-on foundation >/dev/null
$TILLER add feature notifications/email-delivery --title "Email delivery" --contributor jc --depends-on foundation/app-scaffold >/dev/null
$TILLER add feature notifications/reminder-scheduler --title "Reminder scheduler" --contributor jc --depends-on notifications/email-delivery >/dev/null

$TILLER add epic audits --title "Audit Engine" --contributor jc --depends-on form-engine >/dev/null
$TILLER add feature audits/audit-model --title "Audit model and lifecycle" --contributor jc --depends-on form-engine/form-renderer >/dev/null
$TILLER add feature audits/auditor-review --title "Auditor review interface" --contributor jc --depends-on audits/audit-model >/dev/null
$TILLER add feature audits/site-visit --title "On-site visit scheduling" --contributor jc --depends-on audits/audit-model >/dev/null

$TILLER add epic reporting --title "Reporting" --contributor jc --depends-on audits >/dev/null
$TILLER add feature reporting/annual-report --title "Annual report" --contributor jc --depends-on audits/auditor-review >/dev/null
$TILLER add feature reporting/csv-export --title "CSV export" --contributor jc --depends-on payments/invoicing >/dev/null

$TILLER order epic foundation form-engine payments notifications audits reporting >/dev/null
commit "decompose: Acme epics and features"

# ---------- the questions ----------
DUE=$(date -v+5d +%Y-%m-%d 2>/dev/null || date -d "+5 days" +%Y-%m-%d)
$TILLER add question "Do refunds need proration in v1, or is a full refund inside 30 days enough?" \
  --title "Refund proration" --blocks payments/invoicing --due "$DUE" --contributor jc >/dev/null
commit "decisions: an open question"

# ---------- foundation: approved, one feature merged, one in motion ----------
$TILLER approve epic foundation --contributor jc >/dev/null
$TILLER add task foundation/app-scaffold/rails-skeleton --title "Rails skeleton and health route" --contributor jc >/dev/null
$TILLER add task foundation/app-scaffold/ci-pipeline --title "CI pipeline" --contributor jc --depends-on foundation/app-scaffold/rails-skeleton >/dev/null
spec_feature foundation/app-scaffold "App scaffold" "Stand up the application skeleton: the build, the entrypoint, one health route, and a CI pipeline that runs the suite on every push."
$TILLER approve feature foundation/app-scaffold --contributor jc >/dev/null
commit "foundation/app-scaffold: plan approved"
for task in rails-skeleton ci-pipeline; do
  $TILLER claim task foundation/app-scaffold/$task --contributor jc >/dev/null
  $TILLER start task foundation/app-scaffold/$task --contributor jc >/dev/null
  git checkout -q -b "tiller/foundation/app-scaffold/$task"
  echo "$task" >> app.txt
  git add -A && git commit -qm "foundation/app-scaffold/$task: the work"
  $TILLER check task foundation/app-scaffold/$task --contributor jc >/dev/null
  $TILLER submit task foundation/app-scaffold/$task --contributor jc >/dev/null
  git add -A && git commit -qm "foundation/app-scaffold/$task: review"
  if [[ "$task" == ci-pipeline ]]; then
    $TILLER check feature foundation/app-scaffold --contributor jc >/dev/null
    git add -A && git commit -qm "foundation/app-scaffold: gates"
  fi
  git checkout -q main
  git merge -q --no-ff -m "Merge app-scaffold/$task" "tiller/foundation/app-scaffold/$task"
  $TILLER sync --contributor jc >/dev/null
  commit "sync: app-scaffold/$task done"
done

$TILLER add task foundation/auth-and-roles/session-model --title "Session model and login" --contributor jc >/dev/null
$TILLER add task foundation/auth-and-roles/role-guards --title "Role guards on every route" --contributor jc --depends-on foundation/auth-and-roles/session-model >/dev/null
spec_feature foundation/auth-and-roles "Authentication and roles" "Sessions, login, and the four roles the program knows: technician, employer, auditor, and administrator, each guarding the routes it may reach."
$TILLER approve feature foundation/auth-and-roles --contributor jc >/dev/null
$TILLER claim task foundation/auth-and-roles/session-model --contributor jc >/dev/null
$TILLER start task foundation/auth-and-roles/session-model --contributor jc >/dev/null
commit "foundation/auth-and-roles: in progress"

# ---------- form engine: approved, dana's feature with a task in review ----------
$TILLER approve epic form-engine --contributor jc >/dev/null
$TILLER add task form-engine/nrca-form-loading/schema-transcription --title "Transcribe the NRCA schema" --contributor jc >/dev/null
$TILLER add task form-engine/nrca-form-loading/field-renderer --title "Render every field type" --contributor jc --depends-on form-engine/nrca-form-loading/schema-transcription >/dev/null
spec_feature form-engine/nrca-form-loading "NRCA form loading" "Load the NRCA definition families from their published schema and render each form version the program still accepts."
$TILLER approve feature form-engine/nrca-form-loading --contributor jc >/dev/null
commit "form-engine/nrca-form-loading: plan approved"

# ---------- payments: approved, a plan ready and unclaimed ----------
# Audits and reporting are approved too, so the desk holds one epic
# decision (notifications), one bug, and one question: three kinds, one row,
# with the pulse in the same frame. A project with a longer queue is still
# one command away (leave an epic unapproved) when a shot needs the bands.
$TILLER approve epic audits --contributor jc >/dev/null
$TILLER approve epic reporting --contributor jc >/dev/null
$TILLER approve epic payments --contributor jc >/dev/null
$TILLER add task payments/stripe-checkout/checkout-session --title "Create the checkout session" --contributor jc >/dev/null
$TILLER add task payments/stripe-checkout/webhook-receipt --title "Receive and verify the webhook" --contributor jc --depends-on payments/stripe-checkout/checkout-session >/dev/null
spec_feature payments/stripe-checkout "Stripe checkout" "One checkout session per enrollment fee, confirmed by Stripe's webhook rather than by the browser's return."
$TILLER approve feature payments/stripe-checkout --contributor jc >/dev/null
commit "payments/stripe-checkout: plan approved"

# ---------- the shelves ----------
$TILLER add bug blank-on-reload --title "Form blanks on a hard reload" --attach form-engine/nrca-form-loading --contributor jc >/dev/null
$TILLER add bug session-expiry --title "Session expires in the middle of a long form" --attach foundation/auth-and-roles --contributor jc >/dev/null
$TILLER add chore archive-notes --title "Archive resolved meeting notes" --attach foundation/app-scaffold --contributor jc >/dev/null
$TILLER add chore rotate-keys --title "Rotate the staging API keys" --contributor jc >/dev/null
$TILLER add tweak button-text --title "Change the scaffold button text" --attach foundation/app-scaffold --contributor jc >/dev/null
$TILLER add tweak help-spacing --title "Adjust help text spacing" --contributor jc >/dev/null
$TILLER approve bug session-expiry --contributor jc >/dev/null
$TILLER claim bug session-expiry --contributor jc >/dev/null
$TILLER start bug session-expiry --contributor jc >/dev/null
$TILLER approve chore rotate-keys --contributor jc >/dev/null
$TILLER approve chore archive-notes --contributor jc >/dev/null
$TILLER approve tweak help-spacing --contributor jc >/dev/null
commit "shelves: two bugs, two chores, two tweaks"

# ---------- dana's branch, last, and the checkout stays on it: the console
# reads the working tree, and her task is in review only where her work is ----------
$TILLER claim task form-engine/nrca-form-loading/schema-transcription --contributor dana >/dev/null
$TILLER start task form-engine/nrca-form-loading/schema-transcription --contributor dana >/dev/null
git checkout -q -b tiller/form-engine/nrca-form-loading/schema-transcription
echo schema >> forms.txt
git add -A && git commit -qm "schema-transcription: the work"
$TILLER check task form-engine/nrca-form-loading/schema-transcription --contributor dana >/dev/null
$TILLER submit task form-engine/nrca-form-loading/schema-transcription --contributor dana >/dev/null
# The optional review gates, attested, so the desk asks for the merge and
# not four attestations; leave one unattested when a shot wants that card.
for gate in code_review security_review performance_review accessibility_review; do
  $TILLER attest task form-engine/nrca-form-loading/schema-transcription --gate $gate --status pass --contributor jc >/dev/null
done
git add -A && git commit -qm "schema-transcription: in review, reviews attested"

echo "Acme Platform stand-in ready at $DIR"
echo "serve it with: (cd $DIR && $TILLER serve --port 4477)"
